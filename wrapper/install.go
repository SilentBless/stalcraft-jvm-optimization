package main

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows/registry"
)

const (
	ifeoPath = `SOFTWARE\Microsoft\Windows NT\CurrentVersion\Image File Execution Options`
)

var targetExes = []string{"stalcraft.exe", "stalcraftw.exe"}

func install() error {
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	for _, target := range targetExes {
		key, _, err := registry.CreateKey(
			registry.LOCAL_MACHINE,
			ifeoPath+`\`+target,
			registry.ALL_ACCESS,
		)
		if err != nil {
			return fmt.Errorf("failed to create IFEO key for %s: %w", target, err)
		}

		if err := key.SetStringValue("Debugger", `"`+self+`"`); err != nil {
			key.Close()
			return fmt.Errorf("failed to set Debugger value for %s: %w", target, err)
		}
		key.Close()
	}
	return nil
}

func uninstall() error {
	var lastErr error
	for _, target := range targetExes {
		key, err := registry.OpenKey(
			registry.LOCAL_MACHINE,
			ifeoPath+`\`+target,
			registry.SET_VALUE,
		)
		if err != nil {
			lastErr = fmt.Errorf("failed to open IFEO key for %s: %w", target, err)
			continue
		}

		if err := key.DeleteValue("Debugger"); err != nil {
			key.Close()
			lastErr = fmt.Errorf("failed to delete Debugger for %s: %w", target, err)
			continue
		}
		key.Close()
	}
	return lastErr
}

func status() {
	found := false
	for _, target := range targetExes {
		key, err := registry.OpenKey(
			registry.LOCAL_MACHINE,
			ifeoPath+`\`+target,
			registry.QUERY_VALUE,
		)
		if err != nil {
			fmt.Printf("[status] %s: not installed\n", target)
			continue
		}

		val, _, err := key.GetStringValue("Debugger")
		key.Close()
		if err != nil {
			fmt.Printf("[status] %s: not installed\n", target)
			continue
		}

		fmt.Printf("[status] %s -> %s\n", target, val)
		found = true
	}
	if !found {
		fmt.Println("[status] Not installed")
	}
}
