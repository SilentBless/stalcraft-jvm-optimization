package main

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows/registry"
)

const (
	ifeoPath  = `SOFTWARE\Microsoft\Windows NT\CurrentVersion\Image File Execution Options`
	targetExe = "stalcraft.exe"
)

func install() {
	self, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "[install] Failed to get executable path: %v\n", err)
		os.Exit(1)
	}

	key, _, err := registry.CreateKey(
		registry.LOCAL_MACHINE,
		ifeoPath+`\`+targetExe,
		registry.SET_VALUE,
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[install] Failed to create IFEO key (run as admin): %v\n", err)
		os.Exit(1)
	}
	defer key.Close()

	if err := key.SetStringValue("Debugger", self); err != nil {
		fmt.Fprintf(os.Stderr, "[install] Failed to set Debugger value: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("[install] IFEO registered for %s\n", targetExe)
	fmt.Printf("[install] Debugger = %s\n", self)
}

func uninstall() {
	key, err := registry.OpenKey(
		registry.LOCAL_MACHINE,
		ifeoPath+`\`+targetExe,
		registry.SET_VALUE,
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[uninstall] IFEO key not found: %v\n", err)
		os.Exit(1)
	}
	defer key.Close()

	if err := key.DeleteValue("Debugger"); err != nil {
		fmt.Fprintf(os.Stderr, "[uninstall] Failed to delete Debugger value: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("[uninstall] IFEO removed for %s\n", targetExe)
}

func status() {
	key, err := registry.OpenKey(
		registry.LOCAL_MACHINE,
		ifeoPath+`\`+targetExe,
		registry.QUERY_VALUE,
	)
	if err != nil {
		fmt.Println("[status] Not installed")
		return
	}
	defer key.Close()

	val, _, err := key.GetStringValue("Debugger")
	if err != nil {
		fmt.Println("[status] Not installed")
		return
	}

	fmt.Printf("[status] Installed: %s -> %s\n", targetExe, val)
}
