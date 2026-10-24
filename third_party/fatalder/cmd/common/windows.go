//go:build windows

package common

import (
	"os"
	"syscall"
	"unsafe"
)

func enableANSIColors() {
	var outMode uint32
	outHandle := syscall.Handle(os.Stdout.Fd())
	if err := syscall.GetConsoleMode(outHandle, &outMode); err == nil {
		const enableVirtualTerminalProcessing = 0x0004
		dll, err := syscall.LoadDLL("kernel32.dll")
		if err != nil {
			return
		}
		proc, err := dll.FindProc("SetConsoleMode")
		if err != nil {
			return
		}
		_, _, _ = proc.Call(
			uintptr(unsafe.Pointer(outHandle)),
			uintptr(outMode|enableVirtualTerminalProcessing),
		)
	}
}
