package main

import (
	"os"
	"os/exec"
	"path/filepath"
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
	highPriorityClass     = 0x00000080
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

var procGetConsoleWindow = kernel32.NewProc("GetConsoleWindow")

func hideConsole() {
	user32 := syscall.NewLazyDLL("user32.dll")
	showWindow := user32.NewProc("ShowWindow")
	hwnd, _, _ := procGetConsoleWindow.Call()
	if hwnd != 0 {
		showWindow.Call(hwnd, 0)
	}
}

func resolveTarget(target string) string {
	base := strings.ToLower(filepath.Base(target))
	dir := filepath.Dir(target)

	// stalcraftw.exe -> javaw.exe, stalcraft.exe -> java.exe
	javaExe := "java.exe"
	if base == "stalcraftw.exe" {
		javaExe = "javaw.exe"
	}

	p := filepath.Join(dir, javaExe)
	if _, err := os.Stat(p); err == nil {
		return p
	}
	return target
}

func splitArgs(args []string) (jvm []string, mainClass string, app []string) {
	for i := 0; i < len(args); {
		a := args[i]
		if a == "-classpath" || a == "-cp" || a == "-jar" {
			jvm = append(jvm, a)
			i++
			if i < len(args) {
				jvm = append(jvm, args[i])
			}
			i++
			continue
		}
		if strings.HasPrefix(a, "-") {
			jvm = append(jvm, a)
			i++
			continue
		}
		mainClass = a
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

func filterArgs(orig, injected []string) []string {
	jvm, mainClass, app := splitArgs(orig)

	var filtered []string
	for _, a := range jvm {
		if !shouldRemove(a) {
			filtered = append(filtered, a)
		}
	}
	result := make([]string, 0, len(filtered)+len(injected)+1+len(app))
	result = append(result, filtered...)
	result = append(result, injected...)
	if mainClass != "" {
		result = append(result, mainClass)
	}
	return append(result, app...)
}

func boostProcess(pid uint32) {
	handle, err := syscall.OpenProcess(processSetInfo|processQueryInfo, false, pid)
	if err != nil {
		return
	}
	defer syscall.CloseHandle(handle)

	procSetProcessPriorityBoost.Call(uintptr(handle), 1)

	mem := uint32(memoryPriorityNormal)
	procNtSetInformationProcess.Call(
		uintptr(handle), uintptr(processMemoryPriority),
		uintptr(unsafe.Pointer(&mem)), unsafe.Sizeof(mem),
	)

	iop := uint32(ioPriorityHigh)
	procNtSetInformationProcess.Call(
		uintptr(handle), uintptr(processIoPriority),
		uintptr(unsafe.Pointer(&iop)), unsafe.Sizeof(iop),
	)
}

func run() int {
	target := resolveTarget(os.Args[1])
	sys := detectSystem()

	var args []string
	if calcHeap(sys) == 0 {
		args = os.Args[2:]
	} else {
		args = filterArgs(os.Args[2:], generateFlags(sys))
	}

	cmd := exec.Command(target, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: highPriorityClass}

	if err := cmd.Start(); err != nil {
		return 1
	}

	boostProcess(uint32(cmd.Process.Pid))

	if err := cmd.Wait(); err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			return exit.ExitCode()
		}
		return 1
	}
	return 0
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
		interactiveMenu()
		return
	}

	hideConsole()
	os.Exit(run())
}
