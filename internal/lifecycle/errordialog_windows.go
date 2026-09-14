//go:build windows
// +build windows

package lifecycle

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32ErrorDialog    = windows.NewLazySystemDLL("user32.dll")
	procMessageBoxErrorW = user32ErrorDialog.NewProc("MessageBoxW")
)

const (
	messageBoxOK            = 0x00000000
	messageBoxIconError     = 0x00000010
	messageBoxSetForeground = 0x00010000
	messageBoxTopmost       = 0x00040000
)

// ShowFatalErrorDialog 在 GUI 宿主创建失败时弹出原生对话框。
// 缺少 WebView2 运行时时 wails.Run 会直接返回错误，进程随后退出；
// 如果没有这个对话框，用户只会看到进程一闪而过、完全不知道失败原因。
func ShowFatalErrorDialog(title string, message string) {
	if message == "" {
		return
	}
	titlePtr, err := windows.UTF16PtrFromString(title)
	if err != nil {
		return
	}
	messagePtr, err := windows.UTF16PtrFromString(message)
	if err != nil {
		return
	}
	_, _, _ = procMessageBoxErrorW.Call(
		0,
		uintptr(unsafe.Pointer(messagePtr)),
		uintptr(unsafe.Pointer(titlePtr)),
		uintptr(messageBoxOK|messageBoxIconError|messageBoxSetForeground|messageBoxTopmost),
	)
}
