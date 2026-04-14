// Package winapi holds shared lazy-loaded Windows DLLs used by sibling packages.
// Each DLL handle is cached by the syscall runtime so importing this package is free.
package winapi

import "syscall"

var (
	Kernel32 = syscall.NewLazyDLL("kernel32.dll")
	Ntdll    = syscall.NewLazyDLL("ntdll.dll")
	User32   = syscall.NewLazyDLL("user32.dll")
	Shell32  = syscall.NewLazyDLL("shell32.dll")
	Advapi32 = syscall.NewLazyDLL("advapi32.dll")
)
