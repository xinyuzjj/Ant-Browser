package backend

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

// directorySwap 记录一次「暂存目录 → 目标目录」的原子替换，用于失败时回滚。
//
// 替换分三步：先把现有 finalDir 改名为 backupDir（保留原数据），
// 再把 stagingDir 改名为 finalDir，最后按需清理 backupDir。
// 任一步失败都会把原目录还原，保证 finalDir 不会停留在半成品状态。
type directorySwap struct {
	FinalDir    string
	BackupDir   string
	HadOriginal bool
}

// replaceDirectoryWithBackup 用 stagingDir 原子替换 finalDir，原目录保留为备份。
//
// backupSuffix 用于命名备份目录：<finalDir>.<backupSuffix>-<uuid>。
// 返回的 directorySwap 交给 finalizeDirectorySwaps 提交或 rollbackDirectorySwaps 回滚。
func replaceDirectoryWithBackup(stagingDir string, finalDir string, backupSuffix string) (directorySwap, error) {
	if strings.TrimSpace(stagingDir) == "" || strings.TrimSpace(finalDir) == "" {
		return directorySwap{}, fmt.Errorf("目录路径不能为空")
	}
	if _, err := os.Stat(stagingDir); err != nil {
		return directorySwap{}, fmt.Errorf("暂存目录不可用: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(finalDir), 0o755); err != nil {
		return directorySwap{}, fmt.Errorf("创建目标目录父目录失败: %w", err)
	}

	suffix := strings.TrimSpace(backupSuffix)
	if suffix == "" {
		suffix = "backup"
	}
	backupDir := finalDir + "." + suffix + "-" + uuid.NewString()
	swap := directorySwap{
		FinalDir:  finalDir,
		BackupDir: backupDir,
	}

	if _, err := os.Stat(finalDir); err == nil {
		swap.HadOriginal = true
		if err := os.Rename(finalDir, backupDir); err != nil {
			return directorySwap{}, fmt.Errorf("备份现有目录失败: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return directorySwap{}, fmt.Errorf("检查目标目录失败: %w", err)
	} else {
		// 目标目录原本不存在，没有产生备份，BackupDir 必须保持为空。
		swap.BackupDir = ""
	}

	if err := os.Rename(stagingDir, finalDir); err != nil {
		if swap.HadOriginal {
			_ = os.Rename(backupDir, finalDir)
		}
		return directorySwap{}, fmt.Errorf("提交新目录失败: %w", err)
	}
	return swap, nil
}

// finalizeDirectorySwaps 提交替换结果，清理临时备份目录。
func finalizeDirectorySwaps(swaps []directorySwap) {
	for _, swap := range swaps {
		if !swap.HadOriginal || strings.TrimSpace(swap.BackupDir) == "" {
			continue
		}
		_ = os.RemoveAll(swap.BackupDir)
	}
}

// rollbackDirectorySwaps 回滚替换：删除新目录，把备份目录改回原名。
func rollbackDirectorySwaps(swaps []directorySwap) {
	for index := len(swaps) - 1; index >= 0; index-- {
		swap := swaps[index]
		if strings.TrimSpace(swap.FinalDir) == "" {
			continue
		}
		_ = os.RemoveAll(swap.FinalDir)
		if swap.HadOriginal && strings.TrimSpace(swap.BackupDir) != "" {
			_ = os.Rename(swap.BackupDir, swap.FinalDir)
		}
	}
}
