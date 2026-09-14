package browser

import (
	"ant-chrome/backend/internal/config"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

const (
	backupRetentionProfileID = "profile-1"
	backupRetentionExtension = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
)

// createBackupSnapshot 造一个 <root>/<profileID>/<extensionID>/<timestamp> 快照，
// 并把目录时间回拨到 age 之前，用于模拟历史积累的备份。
func createBackupSnapshot(t *testing.T, root string, profileID string, extensionID string, name string, age time.Duration) string {
	t.Helper()
	snapshot := filepath.Join(root, profileID, extensionID, name)
	if err := os.MkdirAll(snapshot, 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(snapshot, "Preferences"), []byte("payload"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	modTime := time.Now().Add(-age)
	if err := os.Chtimes(snapshot, modTime, modTime); err != nil {
		t.Fatalf("Chtimes returned error: %v", err)
	}
	return snapshot
}

func backupRetentionRoot(appRoot string) string {
	return filepath.Join(appRoot, "data", extensionBackupRoot)
}

func TestPruneExtensionBackupsKeepsNewestSnapshotsPerTarget(t *testing.T) {
	appRoot := t.TempDir()
	manager := NewManager(config.DefaultConfig(), appRoot)
	root := backupRetentionRoot(appRoot)

	// 快照按时间从新到旧：1 天、5 天、10 天、20 天、30 天前。
	ages := []time.Duration{
		24 * time.Hour,
		5 * 24 * time.Hour,
		10 * 24 * time.Hour,
		20 * 24 * time.Hour,
		30 * 24 * time.Hour,
	}
	snapshots := make([]string, 0, len(ages))
	for index, age := range ages {
		snapshots = append(snapshots, createBackupSnapshot(
			t, root, backupRetentionProfileID, backupRetentionExtension,
			fmt.Sprintf("2026010%d-000000.000000000", index+1), age,
		))
	}

	removed, err := manager.PruneExtensionBackups()
	if err != nil {
		t.Fatalf("PruneExtensionBackups returned error: %v", err)
	}
	// 保留"最近 3 份"（1/5/10 天）+ "7 天以内"（1/5 天）→ 10/20/30 天前的两份被删。
	if removed != 2 {
		t.Fatalf("removed = %d, want 2", removed)
	}
	for index, snapshot := range snapshots {
		_, statErr := os.Stat(snapshot)
		switch index {
		case 0, 1, 2:
			if statErr != nil {
				t.Fatalf("snapshot %d should be kept, stat error = %v", index, statErr)
			}
		default:
			if !os.IsNotExist(statErr) {
				t.Fatalf("snapshot %d should be removed, stat error = %v", index, statErr)
			}
		}
	}
}

func TestPruneExtensionBackupsRemovesEmptyParentDirectories(t *testing.T) {
	appRoot := t.TempDir()
	manager := NewManager(config.DefaultConfig(), appRoot)
	root := backupRetentionRoot(appRoot)

	// 造 keepPerTarget+1 份全部过期的快照：只有最近 N 份保留，其余被删。
	extensionDir := filepath.Join(root, backupRetentionProfileID, backupRetentionExtension)
	for index := 0; index < extensionBackupKeepPerTarget+1; index++ {
		createBackupSnapshot(
			t, root, backupRetentionProfileID, backupRetentionExtension,
			fmt.Sprintf("2026010%d-000000.000000000", index+1),
			time.Duration(index+1)*10*24*time.Hour,
		)
	}

	if _, err := manager.PruneExtensionBackups(); err != nil {
		t.Fatalf("PruneExtensionBackups returned error: %v", err)
	}
	entries, err := os.ReadDir(extensionDir)
	if err != nil {
		t.Fatalf("ReadDir returned error: %v", err)
	}
	if len(entries) != extensionBackupKeepPerTarget {
		t.Fatalf("kept snapshots = %d, want %d", len(entries), extensionBackupKeepPerTarget)
	}
}

func TestPruneExtensionBackupsRemovesOrphanEmptyDirectories(t *testing.T) {
	appRoot := t.TempDir()
	manager := NewManager(config.DefaultConfig(), appRoot)
	root := backupRetentionRoot(appRoot)

	// 只有 <profile>/<extension> 两层、没有第三级快照目录的空壳，
	// 属于历史清理后残留的空目录，应当一并移除。
	emptyExtensionDir := filepath.Join(root, backupRetentionProfileID, "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	if err := os.MkdirAll(emptyExtensionDir, 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}

	if _, err := manager.PruneExtensionBackups(); err != nil {
		t.Fatalf("PruneExtensionBackups returned error: %v", err)
	}
	if _, err := os.Stat(emptyExtensionDir); !os.IsNotExist(err) {
		t.Fatalf("empty extension backup directory should be removed, stat error = %v", err)
	}
}

func TestPruneExtensionBackupsEnforcesTotalSizeCap(t *testing.T) {
	originalCap := extensionBackupMaxTotalBytes
	extensionBackupMaxTotalBytes = 14
	defer func() { extensionBackupMaxTotalBytes = originalCap }()

	appRoot := t.TempDir()
	manager := NewManager(config.DefaultConfig(), appRoot)
	root := backupRetentionRoot(appRoot)

	// 5 份全部过期（10/20/30/40/50 天前），每份 7 字节。
	// 先按"最近 3 份"保留 21 字节 > 14 字节上限，需再让出最旧的一份。
	snapshots := make([]string, 0, 5)
	for index := 0; index < 5; index++ {
		snapshots = append(snapshots, createBackupSnapshot(
			t, root, backupRetentionProfileID, backupRetentionExtension,
			fmt.Sprintf("2026010%d-000000.000000000", index+1),
			time.Duration(index+1)*10*24*time.Hour,
		))
	}

	removed, err := manager.PruneExtensionBackups()
	if err != nil {
		t.Fatalf("PruneExtensionBackups returned error: %v", err)
	}
	if removed != 3 {
		t.Fatalf("removed = %d, want 3 (5 snapshots trimmed down to the 2 newest)", removed)
	}
	for index, snapshot := range snapshots {
		_, statErr := os.Stat(snapshot)
		if index < 2 {
			if statErr != nil {
				t.Fatalf("snapshot %d should be kept, stat error = %v", index, statErr)
			}
			continue
		}
		if !os.IsNotExist(statErr) {
			t.Fatalf("snapshot %d should be removed, stat error = %v", index, statErr)
		}
	}
}

func TestPruneExtensionBackupsKeepsCurrentSnapshotEvenWhenOverCap(t *testing.T) {
	originalCap := extensionBackupMaxTotalBytes
	extensionBackupMaxTotalBytes = 1
	defer func() { extensionBackupMaxTotalBytes = originalCap }()

	appRoot := t.TempDir()
	manager := NewManager(config.DefaultConfig(), appRoot)
	root := backupRetentionRoot(appRoot)

	keepPath := createBackupSnapshot(
		t, root, backupRetentionProfileID, backupRetentionExtension,
		"20260101-000000.000000000", 30*24*time.Hour,
	)
	createBackupSnapshot(
		t, root, backupRetentionProfileID, backupRetentionExtension,
		"20260102-000000.000000000", 20*24*time.Hour,
	)

	if _, err := manager.pruneExtensionBackups(keepPath); err != nil {
		t.Fatalf("pruneExtensionBackups returned error: %v", err)
	}
	// 容量上限必须给本次新备份让路：它不能被删掉，否则回滚将无备份可用。
	if _, err := os.Stat(keepPath); err != nil {
		t.Fatalf("protected snapshot should be kept, stat error = %v", err)
	}
}

func TestPruneExtensionBackupsWithoutDirectory(t *testing.T) {
	appRoot := t.TempDir()
	manager := NewManager(config.DefaultConfig(), appRoot)
	removed, err := manager.PruneExtensionBackups()
	if err != nil {
		t.Fatalf("PruneExtensionBackups returned error: %v", err)
	}
	if removed != 0 {
		t.Fatalf("removed = %d, want 0 when no backup directory exists", removed)
	}
}
