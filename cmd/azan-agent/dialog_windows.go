//go:build windows

package main

import (
	"syscall"
	"unsafe"
)

var (
	user32     = syscall.NewLazyDLL("user32.dll")
	procMsgBox = user32.NewProc("MessageBoxW")
)

func messageBox(title, text string, flags uintptr) int32 {
	t, _ := syscall.UTF16PtrFromString(title)
	m, _ := syscall.UTF16PtrFromString(text)
	ret, _, _ := procMsgBox.Call(0,
		uintptr(unsafe.Pointer(m)),
		uintptr(unsafe.Pointer(t)),
		flags)
	return int32(ret)
}

func confirmDialog(title, message string) bool {
	// MB_YESNO | MB_ICONQUESTION = 0x24; IDYES = 6
	return messageBox(title, message, 0x24) == 6
}

func showInfoDialog(title, message string) {
	// MB_OK | MB_ICONINFORMATION = 0x40
	messageBox(title, message, 0x40)
}
