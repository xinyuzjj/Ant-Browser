package backend

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ant-chrome/backend/internal/snapshot"
)

// writeTestFile 写入一个测试文件，自动创建父目录。
func writeTestFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("创建目录失败: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("写入文件失败: %v", err)
	}
}

// newUserDataDir 构造一个带标记文件的用户数据目录，用于验证「原数据是否被破坏」。
func newUserDataDir(t *testing.T, marker string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "user-data")
	writeTestFile(t, filepath.Join(dir, "Default", "Preferences"), marker)
	return dir
}

func readMarker(t *testing.T, userDataDir string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(userDataDir, "Default", "Preferences"))
	if err != nil {
		t.Fatalf("读取标记文件失败: %v", err)
	}
	return string(data)
}

func stagingDirs(t *testing.T, userDataDir string) []string {
	t.Helper()
	matches, err := filepath.Glob(userDataDir + "." + snapshotRestoreStagingSuffix + "-*")
	if err != nil {
		t.Fatalf("匹配暂存目录失败: %v", err)
	}
	return matches
}

func backupDirs(t *testing.T, userDataDir string) []string {
	t.Helper()
	matches, err := filepath.Glob(userDataDir + "." + snapshotRestoreBackupSuffix + "-*")
	if err != nil {
		t.Fatalf("匹配备份目录失败: %v", err)
	}
	return matches
}

// 损坏的压缩包不能让原用户数据目录被清空。
func TestRestoreUserDataDirFromZipKeepsOriginalOnCorruptArchive(t *testing.T) {
	userDataDir := newUserDataDir(t, "original")

	zipPath := filepath.Join(t.TempDir(), "broken.zip")
	if err := os.WriteFile(zipPath, []byte("this is not a zip archive"), 0o644); err != nil {
		t.Fatalf("写入损坏压缩包失败: %v", err)
	}

	if _, err := restoreUserDataDirFromZip(zipPath, userDataDir); err == nil {
		t.Fatalf("损坏压缩包应当返回错误")
	}

	if got := readMarker(t, userDataDir); got != "original" {
		t.Fatalf("原数据被破坏，期望 original，实际 %q", got)
	}
	if dirs := stagingDirs(t, userDataDir); len(dirs) != 0 {
		t.Fatalf("失败后残留暂存目录: %v", dirs)
	}
	if dirs := backupDirs(t, userDataDir); len(dirs) != 0 {
		t.Fatalf("失败后不应产生备份目录: %v", dirs)
	}
}

// 空压缩包不能覆盖现有数据。
func TestRestoreUserDataDirFromZipKeepsOriginalOnEmptyArchive(t *testing.T) {
	userDataDir := newUserDataDir(t, "original")

	emptyDir := t.TempDir()
	zipPath := filepath.Join(t.TempDir(), "empty.zip")
	if err := snapshot.ZipDir(emptyDir, zipPath); err != nil {
		t.Fatalf("构造空压缩包失败: %v", err)
	}

	if _, err := restoreUserDataDirFromZip(zipPath, userDataDir); err == nil {
		t.Fatalf("空压缩包应当返回错误")
	}

	if got := readMarker(t, userDataDir); got != "original" {
		t.Fatalf("原数据被破坏，期望 original，实际 %q", got)
	}
	if dirs := stagingDirs(t, userDataDir); len(dirs) != 0 {
		t.Fatalf("失败后残留暂存目录: %v", dirs)
	}
}

// 含路径穿越条目的压缩包必须被拒绝，且不能在用户数据目录外落文件。
func TestRestoreUserDataDirFromZipRejectsPathTraversal(t *testing.T) {
	userDataDir := newUserDataDir(t, "original")

	zipPath := filepath.Join(t.TempDir(), "traversal.zip")
	file, err := os.Create(zipPath)
	if err != nil {
		t.Fatalf("创建压缩包失败: %v", err)
	}
	writer := zip.NewWriter(file)
	entry, err := writer.Create("../escaped.txt")
	if err != nil {
		t.Fatalf("创建压缩包条目失败: %v", err)
	}
	if _, err := entry.Write([]byte("escaped")); err != nil {
		t.Fatalf("写入压缩包条目失败: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("关闭压缩包失败: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("关闭压缩包文件失败: %v", err)
	}

	if _, err := restoreUserDataDirFromZip(zipPath, userDataDir); err == nil {
		t.Fatalf("路径穿越压缩包应当返回错误")
	}

	if got := readMarker(t, userDataDir); got != "original" {
		t.Fatalf("原数据被破坏，期望 original，实际 %q", got)
	}
	escaped := filepath.Join(filepath.Dir(userDataDir), "escaped.txt")
	if _, err := os.Stat(escaped); err == nil {
		t.Fatalf("路径穿越条目被解压到目录外: %s", escaped)
	}
}

// 正常恢复：新数据就位，原数据保留在备份目录里。
func TestRestoreUserDataDirFromZipReplacesContentAndKeepsBackup(t *testing.T) {
	userDataDir := newUserDataDir(t, "original")

	sourceDir := filepath.Join(t.TempDir(), "snapshot-content")
	writeTestFile(t, filepath.Join(sourceDir, "Default", "Preferences"), "restored")
	writeTestFile(t, filepath.Join(sourceDir, "Default", "Cookies"), "cookie-data")
	zipPath := filepath.Join(t.TempDir(), "snapshot.zip")
	if err := snapshot.ZipDir(sourceDir, zipPath); err != nil {
		t.Fatalf("构造快照压缩包失败: %v", err)
	}

	backupDir, err := restoreUserDataDirFromZip(zipPath, userDataDir)
	if err != nil {
		t.Fatalf("恢复失败: %v", err)
	}

	if got := readMarker(t, userDataDir); got != "restored" {
		t.Fatalf("恢复后内容不正确，期望 restored，实际 %q", got)
	}
	if _, err := os.Stat(filepath.Join(userDataDir, "Default", "Cookies")); err != nil {
		t.Fatalf("恢复后缺少 Cookies 文件: %v", err)
	}

	if strings.TrimSpace(backupDir) == "" {
		t.Fatalf("恢复前存在原目录，应当返回备份目录路径")
	}
	backupMarker, err := os.ReadFile(filepath.Join(backupDir, "Default", "Preferences"))
	if err != nil {
		t.Fatalf("备份目录缺少原数据: %v", err)
	}
	if string(backupMarker) != "original" {
		t.Fatalf("备份内容不正确，期望 original，实际 %q", string(backupMarker))
	}
	if dirs := stagingDirs(t, userDataDir); len(dirs) != 0 {
		t.Fatalf("成功后残留暂存目录: %v", dirs)
	}
}

// 目标目录不存在时也能恢复，且不产生备份目录。
func TestRestoreUserDataDirFromZipCreatesMissingTarget(t *testing.T) {
	userDataDir := filepath.Join(t.TempDir(), "user-data")

	sourceDir := filepath.Join(t.TempDir(), "snapshot-content")
	writeTestFile(t, filepath.Join(sourceDir, "Default", "Preferences"), "restored")
	zipPath := filepath.Join(t.TempDir(), "snapshot.zip")
	if err := snapshot.ZipDir(sourceDir, zipPath); err != nil {
		t.Fatalf("构造快照压缩包失败: %v", err)
	}

	backupDir, err := restoreUserDataDirFromZip(zipPath, userDataDir)
	if err != nil {
		t.Fatalf("恢复失败: %v", err)
	}
	if strings.TrimSpace(backupDir) != "" {
		t.Fatalf("目标目录原本不存在，不应产生备份目录，实际 %q", backupDir)
	}
	if got := readMarker(t, userDataDir); got != "restored" {
		t.Fatalf("恢复后内容不正确，期望 restored，实际 %q", got)
	}
}

// 恢复成功后清理残留的暂存目录，避免磁盘泄漏。
func TestRestoreUserDataDirFromZipCleansStaleStagingDirs(t *testing.T) {
	userDataDir := newUserDataDir(t, "original")

	staleDir := userDataDir + "." + snapshotRestoreStagingSuffix + "-stale"
	writeTestFile(t, filepath.Join(staleDir, "leftover.txt"), "junk")

	sourceDir := filepath.Join(t.TempDir(), "snapshot-content")
	writeTestFile(t, filepath.Join(sourceDir, "Default", "Preferences"), "restored")
	zipPath := filepath.Join(t.TempDir(), "snapshot.zip")
	if err := snapshot.ZipDir(sourceDir, zipPath); err != nil {
		t.Fatalf("构造快照压缩包失败: %v", err)
	}

	if _, err := restoreUserDataDirFromZip(zipPath, userDataDir); err != nil {
		t.Fatalf("恢复失败: %v", err)
	}
	if _, err := os.Stat(staleDir); !os.IsNotExist(err) {
		t.Fatalf("残留暂存目录未被清理: %s", staleDir)
	}
}

// 备份目录保留策略：只留最近的若干个，过旧的清理掉。
func TestPruneSnapshotRestoreBackupsKeepsNewest(t *testing.T) {
	userDataDir := newUserDataDir(t, "original")

	now := time.Now()
	paths := make([]string, 0, 4)
	for index := 0; index < 4; index++ {
		dir := userDataDir + "." + snapshotRestoreBackupSuffix + "-old" + string(rune('a'+index))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("创建备份目录失败: %v", err)
		}
		stamp := now.Add(-time.Duration(index) * time.Minute)
		if err := os.Chtimes(dir, stamp, stamp); err != nil {
			t.Fatalf("设置备份目录时间失败: %v", err)
		}
		paths = append(paths, dir)
	}

	pruneSnapshotRestoreBackups(userDataDir, paths[0])

	remaining := backupDirs(t, userDataDir)
	if len(remaining) != snapshotRestoreBackupKeep {
		t.Fatalf("期望保留 %d 个备份，实际 %d: %v", snapshotRestoreBackupKeep, len(remaining), remaining)
	}
	if _, err := os.Stat(paths[0]); err != nil {
		t.Fatalf("本次备份必须保留: %v", err)
	}
}
