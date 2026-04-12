package main

import (
	"fmt"
	"unsafe"
)

const (
	stdInputHandle  = ^uintptr(10 - 1)
	stdOutputHandle = ^uintptr(11 - 1)

	enableEchoInput                = 0x0004
	enableLineInput                = 0x0002
	enableVirtualTerminalProcessing = 0x0004

	keyEvent = 0x0001
)

var procReadConsoleInput = kernel32.NewProc("ReadConsoleInputW")

type inputRecord struct {
	EventType uint16
	_         uint16
	KeyDown   int32
	RepCount  uint16
	VKeyCode  uint16
	VScanCode uint16
	Char      uint16
	CtrlState uint32
}

type menuItem struct {
	label string
	// action returns true if the menu should wait for Enter before redrawing.
	action func() bool
}

func enableVT() func() {
	hOut, _, _ := procGetStdHandle.Call(stdOutputHandle)
	var mode uint32
	procGetConsoleMode.Call(hOut, uintptr(unsafe.Pointer(&mode)))
	procSetConsoleMode.Call(hOut, uintptr(mode|enableVirtualTerminalProcessing))
	return func() { procSetConsoleMode.Call(hOut, uintptr(mode)) }
}

func interactiveMenu() {
	restoreVT := enableVT()
	defer restoreVT()

	ensureConfigExists()

	for {
		active := getActiveName()

		fmt.Println("STALCRAFT JVM Optimization Wrapper")
		fmt.Println("-----------------------------------")
		fmt.Printf("Active config: %s\n", active)
		fmt.Println()
		fmt.Println("RU: Стрелки для выбора, Enter для подтверждения.")
		fmt.Println("EN: Arrow keys to select, Enter to confirm.")
		fmt.Println()

		exit := false
		items := []menuItem{
			{"Install", elevatedAction("--install", "install")},
			{"Uninstall", elevatedAction("--uninstall", "uninstall")},
			{"Status", func() bool { status(); return true }},
			{"Select Config", func() bool { selectConfigMenu(); return false }},
			{"Regenerate Config", func() bool { regenerateConfig(); return true }},
			{"Exit", func() bool { exit = true; return false }},
		}

		wait := runMenu(items)
		if exit {
			return
		}

		if wait {
			fmt.Println()
			fmt.Println("Press Enter to continue...")
			fmt.Scanln()
		}
		fmt.Print("\033[2J\033[H")
	}
}

func elevatedAction(flag, label string) func() bool {
	return func() bool {
		fmt.Printf("[%s] Requesting administrator privileges...\n", label)
		code, err := runElevated(flag)
		if err != nil {
			fmt.Printf("[error] %v\n", err)
		} else if code != 0 {
			fmt.Printf("[error] %s failed (exit code %d)\n", label, code)
		} else {
			fmt.Printf("[%s] Done.\n", label)
		}
		return true
	}
}

func selectConfigMenu() {
	configs := listConfigs()
	if len(configs) == 0 {
		fmt.Println("[config] No configs found in configs/")
		return
	}

	active := getActiveName()
	items := make([]menuItem, 0, len(configs)+1)
	for _, name := range configs {
		n := name
		label := "  " + n
		if n == active {
			label = "* " + n
		}
		items = append(items, menuItem{label, func() bool {
			setActiveConfig(n)
			fmt.Printf("[config] Active config set to: %s\n", n)
			return false
		}})
	}
	items = append(items, menuItem{"< Back", func() bool { return false }})

	fmt.Println()
	fmt.Println("Select config (* = active):")
	runMenu(items)
}

func regenerateConfig() {
	sys := detectSystem()
	cfg := generateConfig(sys)

	fmt.Printf("[config] Detected: %d cores, %.1f GB RAM (%.1f GB free)",
		sys.CPUCores, sys.TotalRAMGB(), sys.FreeRAMGB())
	if sys.LargePages {
		fmt.Print(", large pages available")
	}
	fmt.Println()

	if sys.CPUCores >= 8 {
		fmt.Println("[config] Profile: strong (8+ cores)")
	} else {
		fmt.Println("[config] Profile: standard")
	}

	if cfg.HeapSizeGB == 0 {
		fmt.Println("[warning] Not enough free RAM for JVM optimization.")
		fmt.Println("[warning] Make sure the page file is enabled and close unnecessary programs.")
	} else {
		totalGB := bytesToGB(sys.TotalRAM)
		if totalGB <= 16 {
			fmt.Println("[warning] 16 GB RAM: enable the page file to avoid memory issues.")
		}
	}

	if err := saveConfigAs(cfg, "default"); err != nil {
		fmt.Printf("[error] Failed to save: %v\n", err)
		return
	}
	setActiveConfig("default")
	fmt.Println("[config] Regenerated default config.")
}

func runMenu(items []menuItem) bool {
	hIn, _, _ := procGetStdHandle.Call(stdInputHandle)
	hOut, _, _ := procGetStdHandle.Call(stdOutputHandle)

	cursorInfo := [2]uint32{100, 0}
	kernel32.NewProc("SetConsoleCursorInfo").Call(hOut, uintptr(unsafe.Pointer(&cursorInfo)))
	defer func() {
		cursorInfo[1] = 1
		kernel32.NewProc("SetConsoleCursorInfo").Call(hOut, uintptr(unsafe.Pointer(&cursorInfo)))
	}()

	var oldMode uint32
	procGetConsoleMode.Call(hIn, uintptr(unsafe.Pointer(&oldMode)))
	procSetConsoleMode.Call(hIn, uintptr(oldMode&^(enableLineInput|enableEchoInput)))
	defer procSetConsoleMode.Call(hIn, uintptr(oldMode))

	selected := 0
	drawMenu(items, selected)

	for {
		vk := readKey(hIn)
		switch vk {
		case 0x26: // VK_UP
			if selected > 0 {
				selected--
			}
		case 0x28: // VK_DOWN
			if selected < len(items)-1 {
				selected++
			}
		case 0x0D: // VK_RETURN
			clearMenu(len(items))
			procSetConsoleMode.Call(hIn, uintptr(oldMode))
			return items[selected].action()
		case 0x1B: // VK_ESCAPE
			clearMenu(len(items))
			return false
		default:
			continue
		}
		drawMenu(items, selected)
	}
}

func drawMenu(items []menuItem, selected int) {
	for i := range items {
		fmt.Print("\033[2K\r")
		if i == selected {
			fmt.Printf("  > %s", items[i].label)
		} else {
			fmt.Printf("    %s", items[i].label)
		}
		if i < len(items)-1 {
			fmt.Print("\n")
		}
	}
	fmt.Printf("\033[%dA\r", len(items)-1)
}

func clearMenu(n int) {
	for i := 0; i < n; i++ {
		fmt.Print("\033[2K\r")
		if i < n-1 {
			fmt.Print("\n")
		}
	}
	fmt.Printf("\033[%dA\r", n-1)
}

func readKey(hIn uintptr) uint16 {
	for {
		var rec inputRecord
		var read uint32
		procReadConsoleInput.Call(
			hIn,
			uintptr(unsafe.Pointer(&rec)),
			1,
			uintptr(unsafe.Pointer(&read)),
		)
		if read > 0 && rec.EventType == keyEvent && rec.KeyDown != 0 {
			return rec.VKeyCode
		}
	}
}
