//go:build windows

package platform

import (
	"strings"
	"syscall"
	"unsafe"
)

const (
	mbIconError     = 0x10
	mbSetForeground = 0x10000
	mbTopmost       = 0x40000
)

var messageBox = syscall.NewLazyDLL("user32.dll").NewProc("MessageBoxW")

func ShowFatal(title, msg string) bool {
	t, err := syscall.UTF16PtrFromString(strings.ReplaceAll(title, "\x00", ""))
	if err != nil {
		return false
	}
	m, err := syscall.UTF16PtrFromString(strings.ReplaceAll(msg, "\x00", ""))
	if err != nil {
		return false
	}
	r, _, _ := messageBox.Call(0, uintptr(unsafe.Pointer(m)), uintptr(unsafe.Pointer(t)), mbIconError|mbSetForeground|mbTopmost)
	return r != 0
}
