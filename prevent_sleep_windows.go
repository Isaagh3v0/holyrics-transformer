//go:build windows
// +build windows

package main

import (
	"syscall"
)

func preventSleep() {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	setThreadExecutionState := kernel32.NewProc("SetThreadExecutionState")

	const (
		ES_CONTINUOUS      = 0x80000000
		ES_SYSTEM_REQUIRED = 0x00000001
	)

	setThreadExecutionState.Call(uintptr(ES_CONTINUOUS | ES_SYSTEM_REQUIRED))
}
