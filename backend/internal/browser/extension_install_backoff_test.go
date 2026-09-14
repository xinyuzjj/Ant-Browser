package browser

import (
	"ant-chrome/backend/internal/config"
	"testing"
)

const backoffTestExtensionID = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func newBackoffTestManager(t *testing.T) (*Manager, Extension) {
	t.Helper()
	appRoot := t.TempDir()
	manager := NewManager(config.DefaultConfig(), appRoot)
	manager.ExtensionDAO = newTestExtensionDAO(t, appRoot)
	return manager, Extension{ExtensionID: backoffTestExtensionID, Name: "Test", Version: "1.0.0"}
}

func recordBackoffTestFailure(t *testing.T, manager *Manager, extension Extension, status string) {
	t.Helper()
	if err := manager.ExtensionDAO.UpsertProfileExtensionRuntime(ProfileExtensionRuntime{
		ProfileID:        "profile-1",
		ExtensionID:      extension.ExtensionID,
		InstalledVersion: extension.Version,
		Status:           status,
		LastError:        "插件持久安装失败（Test）：等待浏览器完成插件安装超时",
	}); err != nil {
		t.Fatalf("UpsertProfileExtensionRuntime returned error: %v", err)
	}
}

// 失败后短时间内再次启动实例时，应复用上次的错误而不是重新付出
// "备份 + 冷启动等待 + 回滚"的完整代价。
func TestExtensionInstallFailureReasonReusedInsideBackoff(t *testing.T) {
	manager, extension := newBackoffTestManager(t)
	recordBackoffTestFailure(t, manager, extension, ExtensionRuntimeStatusError)

	if reason := manager.extensionInstallFailureReason("profile-1", extension); reason == "" {
		t.Fatal("expected the recorded failure to be reused inside the backoff window")
	}
}

func TestExtensionInstallFailureReasonExpiresAfterBackoff(t *testing.T) {
	originalBackoff := extensionInstallFailureBackoff
	extensionInstallFailureBackoff = 0
	defer func() { extensionInstallFailureBackoff = originalBackoff }()

	manager, extension := newBackoffTestManager(t)
	recordBackoffTestFailure(t, manager, extension, ExtensionRuntimeStatusError)

	if reason := manager.extensionInstallFailureReason("profile-1", extension); reason != "" {
		t.Fatalf("reason = %q, want empty after the backoff window expired", reason)
	}
}

// 插件版本变化时必须立即重试，否则升级会被冷却期误挡。
func TestExtensionInstallFailureReasonIgnoresVersionChange(t *testing.T) {
	manager, extension := newBackoffTestManager(t)
	recordBackoffTestFailure(t, manager, extension, ExtensionRuntimeStatusError)

	upgraded := extension
	upgraded.Version = "2.0.0"
	if reason := manager.extensionInstallFailureReason("profile-1", upgraded); reason != "" {
		t.Fatalf("reason = %q, want empty when the extension version changed", reason)
	}
}

func TestExtensionInstallFailureReasonIgnoresSuccessfulState(t *testing.T) {
	manager, extension := newBackoffTestManager(t)
	recordBackoffTestFailure(t, manager, extension, ExtensionRuntimeStatusInstalled)

	if reason := manager.extensionInstallFailureReason("profile-1", extension); reason != "" {
		t.Fatalf("reason = %q, want empty for an installed extension", reason)
	}
}

func TestExtensionInstallFailureReasonWithoutRuntimeState(t *testing.T) {
	manager, extension := newBackoffTestManager(t)
	if reason := manager.extensionInstallFailureReason("profile-1", extension); reason != "" {
		t.Fatalf("reason = %q, want empty when no runtime state exists", reason)
	}
}

func TestExtensionInstallFailureReasonWithoutDAO(t *testing.T) {
	manager := NewManager(config.DefaultConfig(), t.TempDir())
	extension := Extension{ExtensionID: backoffTestExtensionID, Name: "Test", Version: "1.0.0"}
	if reason := manager.extensionInstallFailureReason("profile-1", extension); reason != "" {
		t.Fatalf("reason = %q, want empty without an injected DAO", reason)
	}
}
