package browser

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// 插件安装前的备份目录结构为：
//
//	data/extension-backups/<profileID>/<extensionID>/<timestamp>/
//
// 早期版本没有任何保留策略，每次安装/修复失败都会留下一个完整快照，
// 长期使用后会出现上千个快照、占用上 GB 磁盘（实测 761 个快照 / 1.04GB）。
// 本文件为其补上保留策略。

const (
	// extensionBackupKeepPerTarget 每个（实例, 插件）组合保留的最近备份份数。
	extensionBackupKeepPerTarget = 3
	// extensionBackupMaxAge 超过该时长的备份一律清理。
	extensionBackupMaxAge = 7 * 24 * time.Hour
)

// extensionBackupMaxTotalBytes 备份目录总容量上限，是硬上限：
// 即使命中"最近 N 份"或"时效内"规则，超出后仍会从最旧的开始继续删除。
var extensionBackupMaxTotalBytes int64 = 512 << 20

type extensionBackupSnapshot struct {
	path      string
	profileID string
	extension string
	modTime   time.Time
	size      int64
}

// PruneExtensionBackups 按保留策略清理 data/extension-backups 下的历史快照，
// 返回删除的快照数量。应用启动时调用一次，即可回收此前无上限增长占用的空间。
func (m *Manager) PruneExtensionBackups() (int, error) {
	return m.pruneExtensionBackups("")
}

// pruneExtensionBackups 执行清理。keepPath 指向本次刚创建、必须保留的快照目录（可为空）。
//
// 保留规则（满足任一即保留）：
//  1. keepPath 指定的快照；
//  2. 每个（实例, 插件）组合最近的 extensionBackupKeepPerTarget 份；
//  3. 创建时间在 extensionBackupMaxAge 之内的快照。
//
// 最后再按 extensionBackupMaxTotalBytes 做硬上限裁剪：容量仍然超限时，
// 从最旧的已保留快照开始继续删除（keepPath 除外），确保占用不会无界增长。
func (m *Manager) pruneExtensionBackups(keepPath string) (int, error) {
	if m == nil {
		return 0, nil
	}
	root := filepath.Clean(m.ResolveRelativePath(filepath.Join("data", extensionBackupRoot)))
	if _, err := os.Stat(root); err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("读取插件备份根目录失败: %w", err)
	}

	snapshots, err := collectExtensionBackupSnapshots(root)
	if err != nil {
		return 0, err
	}
	if len(snapshots) == 0 {
		// 没有完整快照时仍清理可能残留的空目录，避免空壳目录长期堆积。
		removeEmptyExtensionBackupDirs(root)
		return 0, nil
	}
	// 新 → 旧排序，便于"每个目标保留最近 N 份"与"从最旧的开始删"。
	sort.SliceStable(snapshots, func(i int, j int) bool {
		return snapshots[i].modTime.After(snapshots[j].modTime)
	})

	kept := make(map[string]struct{}, len(snapshots))
	perTarget := make(map[string]int, len(snapshots))
	for _, snapshot := range snapshots {
		targetKey := snapshot.profileID + "\x00" + snapshot.extension
		if perTarget[targetKey] >= extensionBackupKeepPerTarget {
			continue
		}
		perTarget[targetKey]++
		kept[snapshot.path] = struct{}{}
	}
	now := time.Now()
	for _, snapshot := range snapshots {
		if now.Sub(snapshot.modTime) <= extensionBackupMaxAge {
			kept[snapshot.path] = struct{}{}
		}
	}
	protectedPath := ""
	if trimmed := strings.TrimSpace(keepPath); trimmed != "" {
		protectedPath = filepath.Clean(trimmed)
		kept[protectedPath] = struct{}{}
	}

	var totalBytes int64
	for _, snapshot := range snapshots {
		if _, ok := kept[snapshot.path]; ok {
			totalBytes += snapshot.size
		}
	}
	// 容量硬上限：从最旧的已保留快照开始让位。
	for i := len(snapshots) - 1; i >= 0 && totalBytes > extensionBackupMaxTotalBytes; i-- {
		snapshot := snapshots[i]
		if _, ok := kept[snapshot.path]; !ok {
			continue
		}
		if snapshot.path == protectedPath {
			continue
		}
		delete(kept, snapshot.path)
		totalBytes -= snapshot.size
	}

	removed := 0
	for _, snapshot := range snapshots {
		if _, ok := kept[snapshot.path]; ok {
			continue
		}
		if err := os.RemoveAll(snapshot.path); err != nil {
			return removed, fmt.Errorf("清理插件备份失败（%s）: %w", snapshot.path, err)
		}
		removed++
	}

	removeEmptyExtensionBackupDirs(root)
	return removed, nil
}

// collectExtensionBackupSnapshots 收集 <root>/<profileID>/<extensionID>/<timestamp> 三级快照。
func collectExtensionBackupSnapshots(root string) ([]extensionBackupSnapshot, error) {
	profileEntries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("读取插件备份目录失败: %w", err)
	}
	snapshots := make([]extensionBackupSnapshot, 0, 32)
	for _, profileEntry := range profileEntries {
		if !profileEntry.IsDir() {
			continue
		}
		profileDir := filepath.Join(root, profileEntry.Name())
		extensionEntries, err := os.ReadDir(profileDir)
		if err != nil {
			continue
		}
		for _, extensionEntry := range extensionEntries {
			if !extensionEntry.IsDir() {
				continue
			}
			extensionDir := filepath.Join(profileDir, extensionEntry.Name())
			snapshotEntries, err := os.ReadDir(extensionDir)
			if err != nil {
				continue
			}
			for _, snapshotEntry := range snapshotEntries {
				if !snapshotEntry.IsDir() {
					continue
				}
				snapshotPath := filepath.Join(extensionDir, snapshotEntry.Name())
				modTime := snapshotEntryModTime(snapshotPath, snapshotEntry)
				snapshots = append(snapshots, extensionBackupSnapshot{
					path:      snapshotPath,
					profileID: profileEntry.Name(),
					extension: extensionEntry.Name(),
					modTime:   modTime,
					size:      directorySize(snapshotPath),
				})
			}
		}
	}
	return snapshots, nil
}

func snapshotEntryModTime(path string, entry os.DirEntry) time.Time {
	if info, err := entry.Info(); err == nil {
		return info.ModTime()
	}
	if info, err := os.Stat(path); err == nil {
		return info.ModTime()
	}
	return time.Time{}
}

// directorySize 递归统计目录占用字节数；失败时返回已统计到的部分。
func directorySize(root string) int64 {
	var total int64
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		if info, infoErr := entry.Info(); infoErr == nil {
			total += info.Size()
		}
		return nil
	})
	return total
}

// removeEmptyExtensionBackupDirs 自底向上清理被清空后残留的空目录。
func removeEmptyExtensionBackupDirs(root string) {
	profileEntries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	for _, profileEntry := range profileEntries {
		if !profileEntry.IsDir() {
			continue
		}
		profileDir := filepath.Join(root, profileEntry.Name())
		extensionEntries, err := os.ReadDir(profileDir)
		if err != nil {
			continue
		}
		for _, extensionEntry := range extensionEntries {
			if !extensionEntry.IsDir() {
				continue
			}
			extensionDir := filepath.Join(profileDir, extensionEntry.Name())
			if isDirEmpty(extensionDir) {
				_ = os.Remove(extensionDir)
			}
		}
		if isDirEmpty(profileDir) {
			_ = os.Remove(profileDir)
		}
	}
}

func isDirEmpty(path string) bool {
	entries, err := os.ReadDir(path)
	if err != nil {
		return false
	}
	return len(entries) == 0
}
