package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"unsafe"
)

var (
	kernel32                    = syscall.NewLazyDLL("kernel32.dll")
	ntdll                       = syscall.NewLazyDLL("ntdll.dll")
	procSetProcessPriorityBoost = kernel32.NewProc("SetProcessPriorityBoost")
	procNtSetInformationProcess = ntdll.NewProc("NtSetInformationProcess")
)

const (
	highPriorityClass    = 0x00000080
	processMemoryPriority = 0x27
	memoryPriorityNormal  = 5
	processIoPriority     = 0x21
	ioPriorityHigh        = 3
	processSetInfo        = 0x0200
	processQueryInfo      = 0x0400
)

var exactRemove = map[string]bool{
	"-XX:-PrintCommandLineFlags": true,
	"-XX:+UseG1GC":               true,
}

var prefixRemove = []string{
	"-XX:MaxGCPauseMillis=",
	"-XX:MetaspaceSize=",
	"-XX:MaxMetaspaceSize=",
	"-XX:G1HeapRegionSize=",
	"-XX:G1NewSizePercent=",
	"-XX:G1MaxNewSizePercent=",
	"-XX:G1ReservePercent=",
	"-XX:G1HeapWastePercent=",
	"-XX:G1MixedGCCountTarget=",
	"-XX:InitiatingHeapOccupancyPercent=",
	"-XX:G1MixedGCLiveThresholdPercent=",
	"-XX:G1RSetUpdatingPauseTimePercent=",
	"-XX:SurvivorRatio=",
	"-XX:MaxTenuringThreshold=",
	"-XX:ParallelGCThreads=",
	"-XX:ConcGCThreads=",
	"-XX:SoftRefLRUPolicyMSPerMB=",
	"-XX:ReservedCodeCacheSize=",
	"-XX:NonNMethodCodeHeapSize=",
	"-XX:ProfiledCodeHeapSize=",
	"-XX:NonProfiledCodeHeapSize=",
	"-XX:MaxInlineLevel=",
	"-XX:FreqInlineSize=",
	"-XX:LargePageSizeInBytes=",
	"-Xms",
	"-Xmx",
}

func splitArgs(args []string) (jvm []string, mainClass string, app []string) {
	for i := 0; i < len(args); {
		arg := args[i]

		if arg == "-classpath" || arg == "-cp" || arg == "-jar" {
			jvm = append(jvm, arg)
			i++
			if i < len(args) {
				jvm = append(jvm, args[i])
			}
			i++
			continue
		}

		if strings.HasPrefix(arg, "-") {
			jvm = append(jvm, arg)
			i++
			continue
		}

		mainClass = arg
		app = args[i+1:]
		return
	}
	return
}

func shouldRemove(arg string) bool {
	if exactRemove[arg] {
		return true
	}
	for _, p := range prefixRemove {
		if strings.HasPrefix(arg, p) {
			return true
		}
	}
	return false
}

func modifyArgs(orig, injected []string) []string {
	jvm, mainClass, app := splitArgs(orig)

	var filtered []string
	removed := 0
	for _, a := range jvm {
		if shouldRemove(a) {
			removed++
		} else {
			filtered = append(filtered, a)
		}
	}

	log("[wrapper] Flags: %d injected, %d removed", len(injected), removed)

	result := make([]string, 0, len(filtered)+len(injected)+1+len(app))
	result = append(result, filtered...)
	result = append(result, injected...)
	if mainClass != "" {
		result = append(result, mainClass)
	}
	return append(result, app...)
}

func boostProcess(handle syscall.Handle) {
	if ret, _, err := procSetProcessPriorityBoost.Call(uintptr(handle), 1); ret == 0 {
		log("[wrapper] SetProcessPriorityBoost failed: %v", err)
	}

	mem := uint32(memoryPriorityNormal)
	if ret, _, _ := procNtSetInformationProcess.Call(
		uintptr(handle), uintptr(processMemoryPriority),
		uintptr(unsafe.Pointer(&mem)), unsafe.Sizeof(mem),
	); ret != 0 {
		log("[wrapper] NtSetInformationProcess(MemoryPriority) NTSTATUS: 0x%X", ret)
	}

	io := uint32(ioPriorityHigh)
	if ret, _, _ := procNtSetInformationProcess.Call(
		uintptr(handle), uintptr(processIoPriority),
		uintptr(unsafe.Pointer(&io)), unsafe.Sizeof(io),
	); ret != 0 {
		log("[wrapper] NtSetInformationProcess(IoPriority) NTSTATUS: 0x%X", ret)
	}
}

func log(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
}

func main() {
	if len(os.Args) >= 2 {
		switch os.Args[1] {
		case "--install":
			install()
			return
		case "--uninstall":
			uninstall()
			return
		case "--status":
			status()
			return
		}
	}

	if len(os.Args) < 2 {
		log("[wrapper] Usage:")
		log("  wrapper.exe <target.exe> [args...]")
		log("  wrapper.exe --install    Register IFEO (requires admin)")
		log("  wrapper.exe --uninstall  Remove IFEO (requires admin)")
		log("  wrapper.exe --status     Check installation status")
		os.Exit(1)
	}

	target := os.Args[1]
	sys := detectSystem()
	log("[wrapper] System: %d cores, %.1fGB total, %.1fGB free, large pages: %v",
		sys.CPUCores, sys.TotalRAMGB(), sys.FreeRAMGB(), sys.LargePages)

	injected := generateFlags(sys)
	heap := calcHeap(sys)
	parallel, conc := calcGCThreads(sys)
	log("[wrapper] Heap: %dg | GC: parallel=%d concurrent=%d | Region: %dm",
		heap, parallel, conc, calcRegionSize(heap))

	args := modifyArgs(os.Args[2:], injected)

	cmd := exec.Command(target, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: highPriorityClass}

	if err := cmd.Start(); err != nil {
		log("[wrapper] Failed to start %s: %v", target, err)
		os.Exit(1)
	}
	log("[wrapper] Started PID %d", cmd.Process.Pid)

	handle, err := syscall.OpenProcess(processSetInfo|processQueryInfo, false, uint32(cmd.Process.Pid))
	if err == nil {
		boostProcess(handle)
		syscall.CloseHandle(handle)
		log("[wrapper] Process boosted")
	} else {
		log("[wrapper] OpenProcess failed: %v", err)
	}

	if err := cmd.Wait(); err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			os.Exit(exit.ExitCode())
		}
		log("[wrapper] %v", err)
		os.Exit(1)
	}
}
