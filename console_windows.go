package main

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

var procGetConsoleProcessList = syscall.NewLazyDLL("kernel32.dll").NewProc("GetConsoleProcessList")

// pauseBeforeExit waits for Enter when the program was started by double-clicking it, so the
// console window doesn't close before the user can read the error. When started from an existing
// terminal, the console is shared with the shell and there is nothing to wait for.
func pauseBeforeExit() {
	var pids [2]uint32
	if n, _, _ := procGetConsoleProcessList.Call(uintptr(unsafe.Pointer(&pids[0])), uintptr(len(pids))); n != 1 {
		return
	}
	fmt.Fprint(os.Stderr, "\nPress Enter to close this window...")
	fmt.Scanln()
}
