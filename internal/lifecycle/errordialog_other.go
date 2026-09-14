//go:build !windows
// +build !windows

package lifecycle

// ShowFatalErrorDialog 在非 Windows 平台是空实现：
// 这些平台的启动失败原因通常已经能从终端输出直接看到。
func ShowFatalErrorDialog(title string, message string) {}
