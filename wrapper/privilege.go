package main

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

var (
	procOpenProcessToken      = advapi32.NewProc("OpenProcessToken")
	procLookupPrivilegeValueW = advapi32.NewProc("LookupPrivilegeValueW")
	procPrivilegeCheck        = advapi32.NewProc("PrivilegeCheck")
	procShellExecuteExW       = shell32.NewProc("ShellExecuteExW")
)

type luid struct {
	LowPart  uint32
	HighPart int32
}

type luidAndAttributes struct {
	Luid       luid
	Attributes uint32
}

type privilegeSet struct {
	PrivilegeCount uint32
	Control        uint32
	Privilege      [1]luidAndAttributes
}

func hasLargePagePrivilege() bool {
	var token syscall.Handle
	proc, _ := syscall.GetCurrentProcess()
	ret, _, _ := procOpenProcessToken.Call(uintptr(proc), 0x0008, uintptr(unsafe.Pointer(&token)))
	if ret == 0 {
		return false
	}
	defer syscall.CloseHandle(token)

	name, _ := syscall.UTF16PtrFromString("SeLockMemoryPrivilege")
	var id luid
	ret, _, _ = procLookupPrivilegeValueW.Call(0, uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(&id)))
	if ret == 0 {
		return false
	}

	ps := privilegeSet{
		PrivilegeCount: 1,
		Privilege:      [1]luidAndAttributes{{Luid: id, Attributes: 0x00000002}},
	}
	var result int32
	ret, _, _ = procPrivilegeCheck.Call(uintptr(token), uintptr(unsafe.Pointer(&ps)), uintptr(unsafe.Pointer(&result)))
	return ret != 0 && result != 0
}

type shellExecuteInfo struct {
	cbSize       uint32
	fMask        uint32
	hwnd         uintptr
	lpVerb       *uint16
	lpFile       *uint16
	lpParameters *uint16
	lpDirectory  *uint16
	nShow        int32
	hInstApp     uintptr
	lpIDList     uintptr
	lpClass      *uint16
	hkeyClass    uintptr
	dwHotKey     uint32
	hIcon        uintptr
	hProcess     syscall.Handle
}

const (
	seeMaskNocloseProcess = 0x00000040
	seeMaskNoAsync        = 0x00000100
)

// runElevated re-launches the wrapper with the given CLI args under an elevated token (UAC prompt).
func runElevated(args string) (int, error) {
	self, err := os.Executable()
	if err != nil {
		return 1, err
	}

	verb, _ := syscall.UTF16PtrFromString("runas")
	file, _ := syscall.UTF16PtrFromString(self)
	params, _ := syscall.UTF16PtrFromString(args)

	sei := shellExecuteInfo{
		fMask:        seeMaskNocloseProcess | seeMaskNoAsync,
		lpVerb:       verb,
		lpFile:       file,
		lpParameters: params,
		nShow:        0,
	}
	sei.cbSize = uint32(unsafe.Sizeof(sei))

	ret, _, _ := procShellExecuteExW.Call(uintptr(unsafe.Pointer(&sei)))
	if ret == 0 {
		return 1, fmt.Errorf("UAC denied or ShellExecuteEx failed")
	}
	defer syscall.CloseHandle(sei.hProcess)

	syscall.WaitForSingleObject(sei.hProcess, syscall.INFINITE)

	var exitCode uint32
	procGetExitCodeProcess.Call(uintptr(sei.hProcess), uintptr(unsafe.Pointer(&exitCode)))
	return int(exitCode), nil
}
