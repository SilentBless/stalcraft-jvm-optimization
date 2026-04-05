package main

import (
	"runtime"
	"unsafe"
)

type SystemInfo struct {
	TotalRAM      uint64
	FreeRAM       uint64
	CPUCores      int
	LargePages    bool
	LargePageSize uint64
}

type memoryStatusEx struct {
	dwLength                uint32
	dwMemoryLoad            uint32
	ullTotalPhys            uint64
	ullAvailPhys            uint64
	ullTotalPageFile        uint64
	ullAvailPageFile        uint64
	ullTotalVirtual         uint64
	ullAvailVirtual         uint64
	ullAvailExtendedVirtual uint64
}

var (
	procGlobalMemoryStatusEx = kernel32.NewProc("GlobalMemoryStatusEx")
	procGetLargePageMinimum  = kernel32.NewProc("GetLargePageMinimum")
)

func detectSystem() SystemInfo {
	info := SystemInfo{CPUCores: runtime.NumCPU()}

	var ms memoryStatusEx
	ms.dwLength = uint32(unsafe.Sizeof(ms))
	if ret, _, _ := procGlobalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&ms))); ret != 0 {
		info.TotalRAM = ms.ullTotalPhys
		info.FreeRAM = ms.ullAvailPhys
	}

	ret, _, _ := procGetLargePageMinimum.Call()
	info.LargePageSize = uint64(ret)
	info.LargePages = ret > 0

	return info
}

func (s SystemInfo) TotalRAMGB() float64 { return float64(s.TotalRAM) / (1 << 30) }
func (s SystemInfo) FreeRAMGB() float64  { return float64(s.FreeRAM) / (1 << 30) }

func bytesToGB(b uint64) uint64 { return b >> 30 }
