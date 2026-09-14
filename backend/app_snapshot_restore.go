package backend

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"ant-chrome/backend/internal/snapshot"

	"github.com/google/uuid"
)

const (
	snapshotRestoreStagingSuffix = "snapshot-restore-staging"
	snapshotRestoreBackupSuffix  = "snapshot-restore-backup"
)

// 恢复成功后保留的备份目录数量上限，以及备份的最长保留时间。
const (
	snapshotRestoreBackupKeep   = 2
	snapshotRestoreBackupMaxAge = 7 * 24 * time.Hour
)

// snapshotRestoreUnzipLimits 限制快照解压规模。
// 取值足够覆盖真实的浏览器用户数据目录，同时挡住异常或损坏的压缩包，
// 避免恢复过程把磁盘写满。
var snapshotRestoreUnzipLimits = snapshot.UnzipLimits{
	MaxEntries:           300000,
	MaxUncompressedBytes: 64 << 30, // 64 GiB
	MaxSingleFileBytes:   16 << 30, // 16 GiB
	MaxCompressedBytes:   32 << 30, // 32 GiB
}

// restoreUserDataDirFromZip 把快照压缩包安全地恢复到 userDataDir。
//
// 流程：解压到同级暂存目录 → 校验解压结果 → 原子替换（原目录改名为备份）→ 失败回滚。
// 与旧实现（先删掉整个用户数据目录再解压）的关键区别是：只有在新数据完整落盘并通过校验
// 之后才会动原目录，任何一步失败都保留原有数据。
//
// 返回本次保留的备份目录路径；若恢复前目标目录不存在则返回空字符串。
func restoreUserDataDirFromZip(zipPath string, userDataDir string) (string, error) {
	userDataDir = strings.TrimSpace(userDataDir)
	if userDataDir == "" {
		return "", fmt.Errorf("用户数据目录不能为空")
	}
	zipPath = strings.TrimSpace(zipPath)
	if zipPath == "" {
		return "", fmt.Errorf("快照文件路径不能为空")
	}

	// 清掉上次异常中断残留的暂存目录，避免脏数据被当成有效结果。
	pruneSnapshotRestoreStagingDirs(userDataDir)

	stagingDir := userDataDir + "." + snapshotRestoreStagingSuffix + "-" + uuid.NewString()
	if err := os.RemoveAll(stagingDir); err != nil {
		return "", fmt.Errorf("清理暂存目录失败: %w", err)
	}
	if err := os.MkdirAll(stagingDir, 0o755); err != nil {
		return "", fmt.Errorf("创建暂存目录失败: %w", err)
	}

	committed := false
	defer func() {
		if !committed {
			_ = os.RemoveAll(stagingDir)
		}
	}()

	if err := snapshot.UnzipToWithLimits(zipPath, stagingDir, snapshotRestoreUnzipLimits); err != nil {
		return "", fmt.Errorf("解压快照失败: %w", err)
	}

	fileCount, _, err := directoryUsage(stagingDir)
	if err != nil {
		return "", fmt.Errorf("校验解压结果失败: %w", err)
	}
	if fileCount == 0 {
		return "", fmt.Errorf("快照内容为空，已取消恢复以保护现有数据")
	}

	swap, err := replaceDirectoryWithBackup(stagingDir, userDataDir, snapshotRestoreBackupSuffix)
	if err != nil {
		return "", err
	}
	committed = true

	pruneSnapshotRestoreBackups(userDataDir, swap.BackupDir)
	return swap.BackupDir, nil
}

// directoryUsage 统计目录下的文件数量与总字节数。
func directoryUsage(dir string) (int, int64, error) {
	fileCount := 0
	var totalBytes int64
	err := filepath.WalkDir(dir, func(_ string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		fileCount++
		totalBytes += info.Size()
		return nil
	})
	if err != nil {
		return 0, 0, err
	}
	return fileCount, totalBytes, nil
}

// pruneSnapshotRestoreStagingDirs 删除目标目录下遗留的暂存目录。
func pruneSnapshotRestoreStagingDirs(userDataDir string) {
	matches, err := filepath.Glob(userDataDir + "." + snapshotRestoreStagingSuffix + "-*")
	if err != nil {
		return
	}
	for _, match := range matches {
		_ = os.RemoveAll(match)
	}
}

// pruneSnapshotRestoreBackups 保留最近若干次恢复的备份目录，清理过旧或过多的备份。
// keepDir 是本次恢复刚生成的备份，一定会被保留。
func pruneSnapshotRestoreBackups(userDataDir string, keepDir string) {
	matches, err := filepath.Glob(userDataDir + "." + snapshotRestoreBackupSuffix + "-*")
	if err != nil {
		return
	}
	type backupEntry struct {
		path    string
		modTime time.Time
	}
	entries := make([]backupEntry, 0, len(matches))
	for _, match := range matches {
		info, err := os.Stat(match)
		if err != nil {
			continue
		}
		entries = append(entries, backupEntry{path: match, modTime: info.ModTime()})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].modTime.After(entries[j].modTime)
	})

	kept := 0
	for _, item := range entries {
		if item.path == keepDir {
			kept++
			continue
		}
		if kept < snapshotRestoreBackupKeep && time.Since(item.modTime) <= snapshotRestoreBackupMaxAge {
			kept++
			continue
		}
		_ = os.RemoveAll(item.path)
	}
}
