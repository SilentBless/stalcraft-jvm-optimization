// Package phantom creates an invisible top-level window that keeps the
// wrapper process alive in the explorer window list during launch.
// Without it, certain overlays fail to attach to the child process.
package phantom

import (
	"runtime"
	"syscall"
	"unsafe"

	"github.com/EXBO-Community/stalcraft-jvm-optimization/internal/winapi"
)

type wndClassExW struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   uintptr
	Icon       uintptr
	Cursor     uintptr
	Background uintptr
	MenuName   *uint16
	ClassName  *uint16
	IconSm     uintptr
}

type point struct{ X, Y int32 }

type msg struct {
	Hwnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      point
}

// Start runs the window loop on its own OS thread.
// It runs until the process exits; there is no stop hook.
func Start() {
	go pump()
}

func pump() {
	runtime.LockOSThread()

	className, err := syscall.UTF16PtrFromString("StalcraftWrapper")
	if err != nil {
		return
	}

	var (
		defWindowProc              = winapi.User32.NewProc("DefWindowProcW")
		registerClassExW           = winapi.User32.NewProc("RegisterClassExW")
		createWindowExW            = winapi.User32.NewProc("CreateWindowExW")
		setLayeredWindowAttributes = winapi.User32.NewProc("SetLayeredWindowAttributes")
		getMessageW                = winapi.User32.NewProc("GetMessageW")
		translateMessage           = winapi.User32.NewProc("TranslateMessage")
		dispatchMessageW           = winapi.User32.NewProc("DispatchMessageW")
	)

	wc := wndClassExW{
		Size:      uint32(unsafe.Sizeof(wndClassExW{})),
		WndProc:   defWindowProc.Addr(),
		ClassName: className,
	}
	registerClassExW.Call(uintptr(unsafe.Pointer(&wc)))

	hwnd, _, _ := createWindowExW.Call(
		0x00000080|0x00080000, // WS_EX_TOOLWINDOW | WS_EX_LAYERED
		uintptr(unsafe.Pointer(className)), 0,
		0x10000000|0x80000000, // WS_VISIBLE | WS_POPUP
		0, 0, 0, 0, 0, 0, 0, 0,
	)
	setLayeredWindowAttributes.Call(hwnd, 0, 0, 0x02)

	var m msg
	for {
		ret, _, _ := getMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if ret == 0 || ret == ^uintptr(0) {
			return
		}
		translateMessage.Call(uintptr(unsafe.Pointer(&m)))
		dispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
}
