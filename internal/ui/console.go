package ui

import (
	"syscall"
	"unsafe"

	"github.com/EXBO-Community/stalcraft-jvm-optimization/internal/winapi"
)

const (
	stdInputHandle  = ^uintptr(10 - 1)
	stdOutputHandle = ^uintptr(11 - 1)

	enableEchoInput                 = 0x0004
	enableLineInput                 = 0x0002
	enableVirtualTerminalProcessing = 0x0004

	keyEvent = 0x0001
)

var (
	procGetStdHandle         = winapi.Kernel32.NewProc("GetStdHandle")
	procGetConsoleMode       = winapi.Kernel32.NewProc("GetConsoleMode")
	procSetConsoleMode       = winapi.Kernel32.NewProc("SetConsoleMode")
	procReadConsoleInput     = winapi.Kernel32.NewProc("ReadConsoleInputW")
	procSetConsoleCursorInfo = winapi.Kernel32.NewProc("SetConsoleCursorInfo")
)

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

// enableVT turns on ANSI escape processing for stdout and returns a
// restore function that reverts to the previous console mode.
func enableVT() func() {
	hOut, _, _ := procGetStdHandle.Call(stdOutputHandle)
	var mode uint32
	procGetConsoleMode.Call(hOut, uintptr(unsafe.Pointer(&mode)))
	procSetConsoleMode.Call(hOut, uintptr(mode|enableVirtualTerminalProcessing))
	return func() { procSetConsoleMode.Call(hOut, uintptr(mode)) }
}

func hideCursor() func() {
	hOut, _, _ := procGetStdHandle.Call(stdOutputHandle)
	cursorInfo := [2]uint32{100, 0}
	procSetConsoleCursorInfo.Call(hOut, uintptr(unsafe.Pointer(&cursorInfo)))
	return func() {
		cursorInfo[1] = 1
		procSetConsoleCursorInfo.Call(hOut, uintptr(unsafe.Pointer(&cursorInfo)))
	}
}

// rawMode switches stdin out of cooked line/echo mode for arrow-key input.
func rawMode() (restore func(), hIn uintptr) {
	hIn, _, _ = procGetStdHandle.Call(stdInputHandle)
	var oldMode uint32
	procGetConsoleMode.Call(hIn, uintptr(unsafe.Pointer(&oldMode)))
	procSetConsoleMode.Call(hIn, uintptr(oldMode&^(enableLineInput|enableEchoInput)))
	return func() { procSetConsoleMode.Call(hIn, uintptr(oldMode)) }, hIn
}

// readKey blocks until the user presses a key, returning its Windows VK code.
func readKey(hIn uintptr) uint16 {
	for {
		var rec inputRecord
		var read uint32
		if _, _, err := procReadConsoleInput.Call(
			hIn,
			uintptr(unsafe.Pointer(&rec)),
			1,
			uintptr(unsafe.Pointer(&read)),
		); err != syscall.Errno(0) && read == 0 {
			return 0
		}
		if read > 0 && rec.EventType == keyEvent && rec.KeyDown != 0 {
			return rec.VKeyCode
		}
	}
}
