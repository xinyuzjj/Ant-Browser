package backend

import (
	"path/filepath"
	"strings"
)

// 恢复 / 导入流程产生的临时目录与备份目录，它们不是用户数据本身。
//
// 这些目录是目标目录的同级兄弟目录（放在同级是为了保证 os.Rename 不跨卷），
// 名字形如 `<用户数据目录>.snapshot-restore-backup-<uuid>`。如果导出备份时不排除，
// 一份备份的体积会成倍增长，恢复时还会把这些临时目录一并还原到目标机器上。
var backupTransientDirMarkers = []string{
	".snapshot-restore-backup-",
	".snapshot-restore-staging-",
	".profile-package-backup-",
}

// 实例包导入的暂存根目录，直接位于用户数据根目录下。
const backupTransientImportDir = ".imports"

// backupShouldSkipTransientPath 判断导出备份时是否应跳过该相对路径。
//
// rel 为相对某个组件根目录的路径，使用正斜杠分隔。
// 这些名字只会出现在用户数据根目录下，但对所有组件统一应用更简单，
// 且前缀足够具体（都带 uuid 后缀），不会误伤正常数据。
func backupShouldSkipTransientPath(rel string) bool {
	cleaned := strings.Trim(filepath.ToSlash(strings.TrimSpace(rel)), "/")
	if cleaned == "" {
		return false
	}

	segments := strings.Split(cleaned, "/")
	if segments[0] == backupTransientImportDir {
		return true
	}
	for _, segment := range segments {
		for _, marker := range backupTransientDirMarkers {
			if strings.Contains(segment, marker) {
				return true
			}
		}
	}
	return false
}
