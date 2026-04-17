// Package sysinfo detects the physical memory and core topology used
// to size JVM heap, GC threads and large-page flags.
package sysinfo

import (
	"fmt"
	"runtime"
	"syscall"
	"unsafe"

	"github.com/EXBO-Community/stalcraft-jvm-optimization/internal/winapi"
)

type Info struct {
	TotalRAM uint64
	FreeRAM  uint64
	// CPUCores is the number of physical cores reported by Windows.
	// Used for hardware-class decisions (e.g. X3D big-cache bonus).
	CPUCores int
	// CPUThreads is the total number of logical threads the OS exposes
	// (runtime.NumCPU). Differs from CPUCores×2 on CPUs without SMT/HT
	// — for example Intel i5-9600KF is 6C/6T, not 6C/12T — which is
	// why GC worker sizing uses this value instead of assuming 2-way SMT.
	CPUThreads int
	// L3CacheMB is the largest unified L3 cache reported by Windows. On
	// multi-CCD CPUs (e.g. 5950X) this is the per-CCD size, not the sum,
	// since a hot thread only benefits from its own CCD's cache.
	L3CacheMB int
	// MemSpeedMTs is the highest ConfiguredMemoryClockSpeed reported
	// across populated DIMMs, in MT/s. Zero means the SMBIOS probe
	// failed or no DIMM is populated; the caller should treat that as
	// "unknown" and fall back to the mid memory tier.
	MemSpeedMTs   int
	LargePages    bool
	LargePageSize uint64
}

func (i Info) TotalRAMGB() float64 { return float64(i.TotalRAM) / (1 << 30) }
func (i Info) FreeRAMGB() float64  { return float64(i.FreeRAM) / (1 << 30) }
func (i Info) TotalGB() uint64     { return i.TotalRAM >> 30 }
func (i Info) FreeGB() uint64      { return i.FreeRAM >> 30 }

// HasBigCache reports whether the CPU has an X3D-class L3 cache (>=64 MB
// per CCD). The threshold is chosen so non-3D dual-CCD parts do not
// trigger big-cache tuning (their effective per-CCD cache is 32 MB).
func (i Info) HasBigCache() bool { return i.L3CacheMB >= 64 }

// MemTier classifies ConfiguredMemoryClockSpeed into three bandwidth
// bands that the G1 tuning profile keys off. The thresholds roughly
// split DDR generations:
//
//   - slow covers stock DDR4 at SPD defaults on H-chipset boards
//     where XMP is unavailable (2133 / 2400 / 2666 MT/s).
//   - mid  covers XMP-enabled DDR4 and baseline DDR5 (3000 – 4800).
//   - fast covers tuned DDR5 (5200 / 5600 / 6000 / 6400 MT/s).
//
// When the SMBIOS probe fails and MemSpeedMTs is zero, MemMid is
// returned as a conservative fallback: neither over-tightening the
// pause target for a slow-memory system, nor leaving a fast-memory
// system under-tuned.
type MemTier int

const (
	MemSlow MemTier = iota
	MemMid
	MemFast
)

// MemTier returns the bandwidth tier for this system's memory.
func (i Info) MemTier() MemTier {
	switch {
	case i.MemSpeedMTs == 0:
		return MemMid
	case i.MemSpeedMTs <= 2933:
		return MemSlow
	case i.MemSpeedMTs < 4800:
		return MemMid
	default:
		return MemFast
	}
}

// String gives a short label suitable for debug and menu output.
func (t MemTier) String() string {
	switch t {
	case MemSlow:
		return "slow"
	case MemFast:
		return "fast"
	default:
		return "mid"
	}
}

var (
	procGlobalMemoryStatusEx             = winapi.Kernel32.NewProc("GlobalMemoryStatusEx")
	procGetLargePageMinimum              = winapi.Kernel32.NewProc("GetLargePageMinimum")
	procGetLogicalProcessorInformationEx = winapi.Kernel32.NewProc("GetLogicalProcessorInformationEx")
)

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

// Detect queries Windows for memory, core count and large-page eligibility.
// It never fails: any probe that errors falls back to a sensible default so
// the caller can still size the JVM.
func Detect() Info {
	info := Info{
		CPUCores:    physicalCores(),
		CPUThreads:  runtime.NumCPU(),
		L3CacheMB:   detectL3CacheMB(),
		MemSpeedMTs: detectMemSpeedMTs(),
	}

	var ms memoryStatusEx
	ms.dwLength = uint32(unsafe.Sizeof(ms))
	if ret, _, _ := procGlobalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&ms))); ret != 0 {
		info.TotalRAM = ms.ullTotalPhys
		info.FreeRAM = ms.ullAvailPhys
	}

	if ret, _, _ := procGetLargePageMinimum.Call(); ret != 0 {
		info.LargePageSize = uint64(ret)
		info.LargePages = hasLargePagePrivilege()
	}

	return info
}

// detectL3CacheMB walks SYSTEM_LOGICAL_PROCESSOR_INFORMATION_EX entries
// and returns the size of the largest L3 unified cache found. Returning
// max-per-entry (not sum) correctly reflects what a single thread can
// actually hit — multi-CCD parts don't share L3 across dies.
func detectL3CacheMB() int {
	const (
		relationCache = 2
		cacheUnified  = 0
	)

	var bufLen uint32
	procGetLogicalProcessorInformationEx.Call(relationCache, 0, uintptr(unsafe.Pointer(&bufLen)))
	if bufLen == 0 {
		return 0
	}
	buf := make([]byte, bufLen)
	if ret, _, _ := procGetLogicalProcessorInformationEx.Call(
		relationCache,
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(unsafe.Pointer(&bufLen)),
	); ret == 0 {
		return 0
	}

	var maxBytes uint64
	for off := uint32(0); off < bufLen; {
		size := *(*uint32)(unsafe.Pointer(&buf[off+4]))
		level := buf[off+8]
		cacheSize := *(*uint32)(unsafe.Pointer(&buf[off+12]))
		typ := *(*uint32)(unsafe.Pointer(&buf[off+16]))
		if level == 3 && typ == cacheUnified && uint64(cacheSize) > maxBytes {
			maxBytes = uint64(cacheSize)
		}
		off += size
	}
	return int(maxBytes >> 20)
}

func physicalCores() int {
	const relationProcessorCore = 0
	var bufLen uint32
	procGetLogicalProcessorInformationEx.Call(relationProcessorCore, 0, uintptr(unsafe.Pointer(&bufLen)))
	if bufLen == 0 {
		return runtime.NumCPU()
	}
	buf := make([]byte, bufLen)
	ret, _, _ := procGetLogicalProcessorInformationEx.Call(
		relationProcessorCore,
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(unsafe.Pointer(&bufLen)),
	)
	if ret == 0 {
		return runtime.NumCPU()
	}
	cores := 0
	for off := uint32(0); off < bufLen; {
		size := *(*uint32)(unsafe.Pointer(&buf[off+4]))
		cores++
		off += size
	}
	if cores == 0 {
		return runtime.NumCPU()
	}
	return cores
}

var (
	procOpenProcessToken      = winapi.Advapi32.NewProc("OpenProcessToken")
	procLookupPrivilegeValueW = winapi.Advapi32.NewProc("LookupPrivilegeValueW")
	procPrivilegeCheck        = winapi.Advapi32.NewProc("PrivilegeCheck")
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

// hasLargePagePrivilege probes SeLockMemoryPrivilege on the current token.
// Required to use -XX:+UseLargePages without it JVM silently degrades.
func hasLargePagePrivilege() bool {
	proc, err := syscall.GetCurrentProcess()
	if err != nil {
		return false
	}
	var token syscall.Handle
	if ret, _, _ := procOpenProcessToken.Call(uintptr(proc), 0x0008, uintptr(unsafe.Pointer(&token))); ret == 0 {
		return false
	}
	defer syscall.CloseHandle(token)

	name, err := syscall.UTF16PtrFromString("SeLockMemoryPrivilege")
	if err != nil {
		return false
	}
	var id luid
	if ret, _, _ := procLookupPrivilegeValueW.Call(0, uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(&id))); ret == 0 {
		return false
	}

	ps := privilegeSet{
		PrivilegeCount: 1,
		Privilege:      [1]luidAndAttributes{{Luid: id, Attributes: 0x00000002}},
	}
	var result int32
	ret, _, _ := procPrivilegeCheck.Call(uintptr(token), uintptr(unsafe.Pointer(&ps)), uintptr(unsafe.Pointer(&result)))
	return ret != 0 && result != 0
}

// Describe returns a single-line human summary, used in the interactive menu.
func (i Info) Describe() string {
	s := fmt.Sprintf("%d cores, %.1f GB RAM (%.1f GB free)", i.CPUCores, i.TotalRAMGB(), i.FreeRAMGB())
	if i.L3CacheMB > 0 {
		s += fmt.Sprintf(", L3 %d MB", i.L3CacheMB)
	}
	if i.MemSpeedMTs > 0 {
		s += fmt.Sprintf(", %d MT/s (%s tier)", i.MemSpeedMTs, i.MemTier())
	} else {
		s += fmt.Sprintf(", mem speed unknown (%s tier fallback)", i.MemTier())
	}
	if i.LargePages {
		s += ", large pages available"
	}
	return s
}
