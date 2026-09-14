package browser

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// 本文件负责清理 profile 配置里"有记录无目录"的外部插件注册残留。
//
// 背景：插件以注册表外部扩展（location=3）或命令行（location=8）方式写入
// Secure Preferences 后，记录里的 path 指向插件代码目录。一旦该目录被删除
// （失败回滚、手工清理、旧版本迁移），配置里就留下一条指向不存在目录的记录。
// Chrome 读到这条记录会认为插件"已经装过"，于是跳过重新下载安装，导致持久安装
// 一直等不到目录出现而超时；更糟的是安装失败后的回滚又会把这条陈旧记录恢复回去，
// 形成"永远装不上"的自锁。
//
// 因此安装前与回滚后都必须调用 repairStaleExternalExtensionSettings，
// 把这类记录连同它的 MAC 保护项一起删掉，让 Chrome 重新走一遍安装流程。

// externalExtensionExternalLocationValues 是 Chrome 中表示"由外部渠道注册"的
// location 取值：3 = 外部注册（注册表 / preference），8 = 命令行加载。
// location=1 表示正常安装，不属于本函数的处理范围。
var externalExtensionExternalLocationValues = map[int]struct{}{
	3: {},
	8: {},
}

// repairStaleExternalExtensionSettings 清理 Secure Preferences / Preferences 中
// 指定 runtimeID 的陈旧外部插件记录，返回被清理的记录条数。
//
// 只有同时满足以下条件的记录才会被删除：
//   - location 为 3 或 8（外部注册来源）；
//   - path 字段非空且指向的目录在磁盘上确实不存在。
//
// 删除记录的同时会移除对应的 MAC 保护项，否则 Chrome 校验失败后会丢弃这次修改。
func repairStaleExternalExtensionSettings(userDataDir string, runtimeIDs []string) (int, error) {
	userDataDir = strings.TrimSpace(userDataDir)
	if userDataDir == "" {
		return 0, nil
	}
	targets := make(map[string]struct{}, len(runtimeIDs))
	for _, runtimeID := range runtimeIDs {
		normalized := NormalizeExtensionID(runtimeID)
		if normalized == "" {
			continue
		}
		targets[normalized] = struct{}{}
	}
	if len(targets) == 0 {
		return 0, nil
	}

	repaired := 0
	var firstErr error
	for _, fileName := range []string{"Secure Preferences", "Preferences"} {
		path := filepath.Join(userDataDir, "Default", fileName)
		removed, err := repairStaleExtensionSettingsFile(userDataDir, path, targets)
		repaired += removed
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return repaired, firstErr
}

func repairStaleExtensionSettingsFile(userDataDir string, path string, targets map[string]struct{}) (int, error) {
	root, err := readProfileJSON(path, false)
	if err != nil || root == nil {
		return 0, err
	}
	extensions, err := ensureProfileJSONMapIfPresent(root, "extensions")
	if err != nil || extensions == nil {
		return 0, err
	}
	settings, err := ensureProfileJSONMapIfPresent(extensions, "settings")
	if err != nil || settings == nil {
		return 0, err
	}

	removedIDs := make([]string, 0, len(targets))
	for runtimeID := range targets {
		setting, ok := settings[runtimeID].(map[string]any)
		if !ok {
			continue
		}
		if !staleExternalExtensionSetting(userDataDir, setting) {
			continue
		}
		delete(settings, runtimeID)
		removedIDs = append(removedIDs, runtimeID)
	}
	if len(removedIDs) == 0 {
		return 0, nil
	}
	sort.Strings(removedIDs)

	for _, runtimeID := range removedIDs {
		if _, err := removeProfileExtensionProtectionMACs(root, runtimeID); err != nil {
			return len(removedIDs), err
		}
	}
	if err := writeProfileJSON(path, root); err != nil {
		return len(removedIDs), err
	}
	return len(removedIDs), nil
}

// staleExternalExtensionSetting 判断一条插件设置记录是否属于"有记录无目录"的残留。
func staleExternalExtensionSetting(userDataDir string, setting map[string]any) bool {
	location, ok := setting["location"].(float64)
	if !ok {
		return false
	}
	if _, external := externalExtensionExternalLocationValues[int(location)]; !external {
		return false
	}
	storedPath, ok := setting["path"].(string)
	if !ok {
		return false
	}
	storedPath = strings.TrimSpace(storedPath)
	if storedPath == "" {
		return false
	}
	return !externalExtensionSettingPathExists(userDataDir, storedPath)
}

// externalExtensionSettingPathExists 按 Chrome 的两种 path 语义解析目录位置：
// 绝对路径直接判断；相对路径同时尝试 <userDataDir>/Default 与 <userDataDir> 两个基准，
// 避免因为基准目录判断错误而误删仍然有效的记录。
func externalExtensionSettingPathExists(userDataDir string, storedPath string) bool {
	candidates := make([]string, 0, 3)
	if filepath.IsAbs(storedPath) {
		candidates = append(candidates, storedPath)
	} else {
		candidates = append(candidates,
			filepath.Join(userDataDir, "Default", storedPath),
			filepath.Join(userDataDir, storedPath),
		)
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return true
		}
	}
	return false
}

// repairProfileExtensionSettingsAfterRestore 在回滚恢复备份后再次执行残留清理，
// 防止备份里携带的陈旧记录被重新写回而再次造成自锁。
func repairProfileExtensionSettingsAfterRestore(userDataDir string, runtimeIDs []string) {
	_, _ = repairStaleExternalExtensionSettings(userDataDir, runtimeIDs)
}
