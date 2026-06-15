//go:build windows

package main

import (
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// isElevated reports whether the current process is running with administrator
// privileges (i.e. the process token is elevated).
func isElevated() bool {
	token, err := windows.OpenCurrentProcessToken()
	if err != nil {
		return false
	}
	defer token.Close()
	return token.IsElevated()
}

// showServiceInstallMessage displays a Windows MessageBox explaining why
// administrator access is needed and how to grant it.
func showServiceInstallMessage() {
	title, _ := syscall.UTF16PtrFromString("Azan Agent — Administrator Access Required")
	text, _ := syscall.UTF16PtrFromString(
		"Azan Agent would like to install itself as a Windows Service so it " +
			"starts automatically at login and runs silently in the background " +
			"(no console window).\r\n\r\n" +
			"Registering a Windows Service requires administrator privileges, " +
			"but the application is not currently running as an administrator.\r\n\r\n" +
			"To install the service:\r\n" +
			"  • Right-click the Azan Agent executable\r\n" +
			"  • Select \"Run as administrator\"\r\n\r\n" +
			"For now, Azan Agent will run normally in this session only " +
			"and will not start automatically at next login.")

	user32 := syscall.NewLazyDLL("user32.dll")
	msgBox := user32.NewProc("MessageBoxW")
	// MB_OK | MB_ICONINFORMATION = 0x40
	msgBox.Call(0, uintptr(unsafe.Pointer(text)), uintptr(unsafe.Pointer(title)), 0x40)
}
