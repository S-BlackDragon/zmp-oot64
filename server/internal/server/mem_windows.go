//go:build windows

package server

import (
	"syscall"
	"unsafe"
)

var procGetProcessMemoryInfo = syscall.NewLazyDLL("psapi.dll").NewProc("GetProcessMemoryInfo")

// processMemoryKB returns what the operating system counts for this process: the memory it has in RAM right now
// (what tasklist and docker stats show) and the memory committed to it alone. 0, 0 when it cannot be read.
func processMemoryKB() (resident, private uint64) {
	// PROCESS_MEMORY_COUNTERS_EX
	var c struct {
		cb                         uint32
		pageFaultCount             uint32
		peakWorkingSetSize         uintptr
		workingSetSize             uintptr
		quotaPeakPagedPoolUsage    uintptr
		quotaPagedPoolUsage        uintptr
		quotaPeakNonPagedPoolUsage uintptr
		quotaNonPagedPoolUsage     uintptr
		pagefileUsage              uintptr
		peakPagefileUsage          uintptr
		privateUsage               uintptr
	}
	c.cb = uint32(unsafe.Sizeof(c))
	h, err := syscall.GetCurrentProcess()
	if err != nil {
		return 0, 0
	}
	r, _, _ := procGetProcessMemoryInfo.Call(uintptr(h), uintptr(unsafe.Pointer(&c)), uintptr(c.cb))
	if r == 0 {
		return 0, 0
	}
	return uint64(c.workingSetSize) / 1024, uint64(c.privateUsage) / 1024
}
