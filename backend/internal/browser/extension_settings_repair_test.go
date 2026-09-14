package browser

import (
	"os"
	"path/filepath"
	"testing"
)

const repairTestRuntimeID = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func writeRepairTestPreferences(t *testing.T, path string, root profileExtensionJSON) {
	t.Helper()
	if err := writeProfileJSON(path, root); err != nil {
		t.Fatalf("writeProfileJSON returned error: %v", err)
	}
}

func readRepairTestPreferences(t *testing.T, path string) profileExtensionJSON {
	t.Helper()
	root, err := readProfileJSON(path, false)
	if err != nil {
		t.Fatalf("readProfileJSON returned error: %v", err)
	}
	if root == nil {
		t.Fatalf("preferences file %q missing", path)
	}
	return root
}

func repairTestSettings(t *testing.T, root profileExtensionJSON) map[string]any {
	t.Helper()
	extensions, err := ensureProfileJSONMapIfPresent(root, "extensions")
	if err != nil || extensions == nil {
		return map[string]any{}
	}
	settings, err := ensureProfileJSONMapIfPresent(extensions, "settings")
	if err != nil || settings == nil {
		return map[string]any{}
	}
	return settings
}

func repairTestMACs(t *testing.T, root profileExtensionJSON) map[string]any {
	t.Helper()
	protection, err := ensureProfileJSONMapIfPresent(root, "protection")
	if err != nil || protection == nil {
		return map[string]any{}
	}
	macs, err := ensureProfileJSONMapIfPresent(protection, "macs")
	if err != nil || macs == nil {
		return map[string]any{}
	}
	extensions, err := ensureProfileJSONMapIfPresent(macs, "extensions")
	if err != nil || extensions == nil {
		return map[string]any{}
	}
	return extensions
}

func staleExternalSettingPreferences(root profileExtensionJSON) profileExtensionJSON {
	return profileExtensionJSON{
		"extensions": profileExtensionJSON{
			"settings": profileExtensionJSON{
				repairTestRuntimeID: profileExtensionJSON{
					"location": float64(3),
					"path":     filepath.Join("data", "extensions", repairTestRuntimeID),
				},
			},
		},
		"protection": profileExtensionJSON{
			"macs": profileExtensionJSON{
				"extensions": profileExtensionJSON{
					"settings":                profileExtensionJSON{repairTestRuntimeID: "stale-mac"},
					"settings_encrypted_hash": profileExtensionJSON{repairTestRuntimeID: "stale-hash"},
				},
			},
		},
	}
}

// 复现用户遇到的"插件持久安装失败：等待浏览器完成插件安装超时"根因：
// Secure Preferences 里残留一条 location=3、path 指向不存在目录的记录，
// Chrome 因此认为插件已安装而跳过重新下载，安装只能一路等到超时。
func TestRepairStaleExternalExtensionSettingsRemovesEntryWithMissingDirectory(t *testing.T) {
	userDataDir := t.TempDir()
	preferencesPath := filepath.Join(userDataDir, "Default", "Secure Preferences")
	writeRepairTestPreferences(t, preferencesPath, staleExternalSettingPreferences(profileExtensionJSON{}))

	repaired, err := repairStaleExternalExtensionSettings(userDataDir, []string{repairTestRuntimeID})
	if err != nil {
		t.Fatalf("repairStaleExternalExtensionSettings returned error: %v", err)
	}
	if repaired != 1 {
		t.Fatalf("repaired = %d, want 1", repaired)
	}

	root := readRepairTestPreferences(t, preferencesPath)
	if _, exists := repairTestSettings(t, root)[repairTestRuntimeID]; exists {
		t.Fatal("stale extension setting still present after repair")
	}
	// MAC 必须一并删除：只删记录不删 MAC，Chrome 会判定配置被篡改并丢弃这次修改。
	extensionMACs := repairTestMACs(t, root)
	for _, key := range []string{"settings", "settings_encrypted_hash"} {
		values, err := ensureProfileJSONMapIfPresent(extensionMACs, key)
		if err != nil {
			t.Fatalf("read macs.%s returned error: %v", key, err)
		}
		if values == nil {
			continue
		}
		if _, exists := values[repairTestRuntimeID]; exists {
			t.Fatalf("stale mac entry macs.extensions.%s.%s still present after repair", key, repairTestRuntimeID)
		}
	}
}

func TestRepairStaleExternalExtensionSettingsKeepsEntryWithExistingDirectory(t *testing.T) {
	userDataDir := t.TempDir()
	preferencesPath := filepath.Join(userDataDir, "Default", "Secure Preferences")
	writeRepairTestPreferences(t, preferencesPath, staleExternalSettingPreferences(profileExtensionJSON{}))
	// 目录真实存在：记录有效，必须原样保留。
	if err := os.MkdirAll(filepath.Join(userDataDir, "data", "extensions", repairTestRuntimeID), 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}

	repaired, err := repairStaleExternalExtensionSettings(userDataDir, []string{repairTestRuntimeID})
	if err != nil {
		t.Fatalf("repairStaleExternalExtensionSettings returned error: %v", err)
	}
	if repaired != 0 {
		t.Fatalf("repaired = %d, want 0 for an entry whose directory exists", repaired)
	}
	root := readRepairTestPreferences(t, preferencesPath)
	if _, exists := repairTestSettings(t, root)[repairTestRuntimeID]; !exists {
		t.Fatal("valid extension setting was removed")
	}
}

func TestRepairStaleExternalExtensionSettingsIgnoresNormalInstalledLocation(t *testing.T) {
	userDataDir := t.TempDir()
	preferencesPath := filepath.Join(userDataDir, "Default", "Secure Preferences")
	writeRepairTestPreferences(t, preferencesPath, profileExtensionJSON{
		"extensions": profileExtensionJSON{
			"settings": profileExtensionJSON{
				repairTestRuntimeID: profileExtensionJSON{
					"location": float64(1),
					"path":     filepath.Join("Extensions", repairTestRuntimeID, "1.0.0_0"),
				},
			},
		},
	})

	repaired, err := repairStaleExternalExtensionSettings(userDataDir, []string{repairTestRuntimeID})
	if err != nil {
		t.Fatalf("repairStaleExternalExtensionSettings returned error: %v", err)
	}
	if repaired != 0 {
		t.Fatalf("repaired = %d, want 0 for location=1 entries", repaired)
	}
	root := readRepairTestPreferences(t, preferencesPath)
	if _, exists := repairTestSettings(t, root)[repairTestRuntimeID]; !exists {
		t.Fatal("location=1 extension setting should not be touched")
	}
}

func TestRepairStaleExternalExtensionSettingsResolvesAbsolutePath(t *testing.T) {
	userDataDir := t.TempDir()
	externalDir := filepath.Join(t.TempDir(), "external-extension")
	preferencesPath := filepath.Join(userDataDir, "Default", "Secure Preferences")
	writeRepairTestPreferences(t, preferencesPath, profileExtensionJSON{
		"extensions": profileExtensionJSON{
			"settings": profileExtensionJSON{
				repairTestRuntimeID: profileExtensionJSON{
					"location": float64(3),
					"path":     externalDir,
				},
			},
		},
	})

	// 绝对路径目录不存在 → 判定为残留并清理。
	repaired, err := repairStaleExternalExtensionSettings(userDataDir, []string{repairTestRuntimeID})
	if err != nil {
		t.Fatalf("repairStaleExternalExtensionSettings returned error: %v", err)
	}
	if repaired != 1 {
		t.Fatalf("repaired = %d, want 1", repaired)
	}

	// 目录补上后同一条记录不再算残留。
	writeRepairTestPreferences(t, preferencesPath, profileExtensionJSON{
		"extensions": profileExtensionJSON{
			"settings": profileExtensionJSON{
				repairTestRuntimeID: profileExtensionJSON{
					"location": float64(3),
					"path":     externalDir,
				},
			},
		},
	})
	if err := os.MkdirAll(externalDir, 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	repaired, err = repairStaleExternalExtensionSettings(userDataDir, []string{repairTestRuntimeID})
	if err != nil {
		t.Fatalf("repairStaleExternalExtensionSettings returned error: %v", err)
	}
	if repaired != 0 {
		t.Fatalf("repaired = %d, want 0 after the directory exists", repaired)
	}
}

func TestRepairStaleExternalExtensionSettingsWithoutPreferencesFile(t *testing.T) {
	userDataDir := t.TempDir()
	repaired, err := repairStaleExternalExtensionSettings(userDataDir, []string{repairTestRuntimeID})
	if err != nil {
		t.Fatalf("repairStaleExternalExtensionSettings returned error: %v", err)
	}
	if repaired != 0 {
		t.Fatalf("repaired = %d, want 0 when no preferences file exists", repaired)
	}
}

func TestRepairProfileExtensionSettingsAfterRestoreCleansRestoredFile(t *testing.T) {
	userDataDir := t.TempDir()
	preferencesPath := filepath.Join(userDataDir, "Default", "Secure Preferences")
	writeRepairTestPreferences(t, preferencesPath, staleExternalSettingPreferences(profileExtensionJSON{}))

	repairProfileExtensionSettingsAfterRestore(userDataDir, []string{repairTestRuntimeID})

	root := readRepairTestPreferences(t, preferencesPath)
	if _, exists := repairTestSettings(t, root)[repairTestRuntimeID]; exists {
		t.Fatal("restored stale extension setting should be cleaned up")
	}
}
