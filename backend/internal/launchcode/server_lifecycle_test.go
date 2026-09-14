package launchcode

import (
	"errors"
	"net"
	"strconv"
	"testing"
)

// TestLaunchServerStopReleasesPortImmediately 覆盖一个真实存在过的竞态：
// Start 把 listener 交给 goroutine 里的 Serve 去登记，而 Stop 只调用 http.Server.Shutdown。
// 若 Stop 抢在 Serve 登记之前执行，Shutdown 看不到任何 listener，端口不会被释放，
// 调用方（重启失败后回滚旧服务）紧接着重新绑定同一端口就会失败。
// 这里刻意在 Start 之后立刻 Stop，把竞态窗口放到最大，并断言 Stop 返回时 listener 已关闭——
// 关闭 listener 正是端口被释放的充要条件，且该断言不受其他进程抢端口影响。
func TestLaunchServerStopReleasesPortImmediately(t *testing.T) {
	const iterations = 50

	for i := 0; i < iterations; i++ {
		srv := NewLaunchServer(nil, nil, nil, 0)
		if err := srv.Start(); err != nil {
			t.Fatalf("iteration %d: Start returned error: %v", i, err)
		}

		srv.mu.Lock()
		ln := srv.listener
		port := srv.port
		srv.mu.Unlock()
		if ln == nil || port <= 0 {
			t.Fatalf("iteration %d: Start did not record a bound listener", i)
		}

		if err := srv.Stop(); err != nil {
			t.Fatalf("iteration %d: Stop returned error: %v", i, err)
		}

		// 已关闭的 listener 再次 Close 返回 net.ErrClosed；若仍处于打开状态则返回 nil。
		if err := ln.Close(); !errors.Is(err, net.ErrClosed) {
			t.Fatalf("iteration %d: listener on port %d is still open after Stop returned (Close returned %v)", i, port, err)
		}
	}
}

// TestLaunchServerStopIsIdempotent 保证重复 Stop 不会 panic，也不会影响之后的重新启动。
func TestLaunchServerStopIsIdempotent(t *testing.T) {
	srv := NewLaunchServer(nil, nil, nil, 0)
	if err := srv.Start(); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}

	if err := srv.Stop(); err != nil {
		t.Fatalf("first Stop returned error: %v", err)
	}
	if err := srv.Stop(); err != nil {
		t.Fatalf("second Stop returned error: %v", err)
	}

	if err := srv.Start(); err != nil {
		t.Fatalf("Start after Stop returned error: %v", err)
	}
	port := srv.Port()
	if port <= 0 {
		t.Fatal("restarted LaunchServer does not have a bound port")
	}

	conn, err := net.Dial("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		t.Fatalf("restarted LaunchServer is not accepting connections: %v", err)
	}
	_ = conn.Close()

	if err := srv.Stop(); err != nil {
		t.Fatalf("final Stop returned error: %v", err)
	}
}
