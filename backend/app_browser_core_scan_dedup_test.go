package backend

import (
	"ant-chrome/backend/internal/browser"
	"ant-chrome/backend/internal/config"
	"os"
	"path/filepath"
	"testing"
)

func writeTestBrowserCore(t *testing.T, root string, name string) {
	t.Helper()
	coreDir := filepath.Join(root, "chrome", name)
	exePath := filepath.Join(coreDir, filepath.FromSlash(browser.CoreExecutableCandidates()[0]))
	if err := os.MkdirAll(filepath.Dir(exePath), 0o755); err != nil {
		t.Fatalf("创建测试内核目录失败: %v", err)
	}
	if err := os.WriteFile(exePath, []byte("stub"), 0o755); err != nil {
		t.Fatalf("写入测试内核可执行文件失败: %v", err)
	}
}

func newCoreScanTestApp(t *testing.T) (*App, *browserCoreDAOStub) {
	t.Helper()
	root := t.TempDir()
	cfg := config.DefaultConfig()
	cfg.Browser.Cores = nil

	app := NewApp(root)
	app.config = cfg
	app.browserMgr = browser.NewManager(cfg, root)
	dao := &browserCoreDAOStub{}
	app.browserMgr.CoreDAO = dao
	return app, dao
}

// 启动序列里 ensureDefaultCores 与 autoDetectCores 会先后扫描同一个 chrome/ 目录，
// 第二次必须复用第一次的结果，避免同一目录被扫描两遍。
func TestBrowserCoreScanReusesStartupScanResult(t *testing.T) {
	app, _ := newCoreScanTestApp(t)
	writeTestBrowserCore(t, app.appRoot, "chromium-148")

	// 启动序列第一步：扫描并缓存。
	app.ensureDefaultCores()

	// 两次扫描之间磁盘上新增了一个内核目录。
	writeTestBrowserCore(t, app.appRoot, "chromium-150")

	// 启动序列第二步：复用缓存，不应看到刚新增的目录。
	if cores := app.scanAndRegisterCores(); len(cores) != 1 {
		t.Fatalf("启动序列内第二次扫描应复用缓存结果，got=%d", len(cores))
	}

	// 缓存已被取走：后续手动重扫必须拿到最新目录状态。
	if cores := app.scanAndRegisterCores(); len(cores) != 2 {
		t.Fatalf("缓存失效后重扫应看到 2 个内核，got=%d", len(cores))
	}
}

func TestBrowserCoreScanHandoffIsClearedAfterUse(t *testing.T) {
	app, _ := newCoreScanTestApp(t)
	writeTestBrowserCore(t, app.appRoot, "chromium-148")

	app.ensureDefaultCores()
	if _, ok := app.takeCoreScanHandoff(app.browserCoreRoot()); !ok {
		t.Fatal("首次取出应命中缓存")
	}
	if _, ok := app.takeCoreScanHandoff(app.browserCoreRoot()); ok {
		t.Fatal("缓存取出后应失效，不能被重复复用")
	}
}

func TestBrowserCoreScanHandoffIgnoresDifferentRoot(t *testing.T) {
	app, _ := newCoreScanTestApp(t)
	app.storeCoreScanHandoff("chrome", nil)
	if _, ok := app.takeCoreScanHandoff("other-root"); ok {
		t.Fatal("不同内核根目录不应命中缓存")
	}
}
