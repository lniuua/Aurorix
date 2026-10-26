//go:build windows

package state

import (
	"syscall"
	"unsafe"
)

var (
	kernel32                 = syscall.NewLazyDLL("kernel32.dll")
	procGetSystemTimes       = kernel32.NewProc("GetSystemTimes")
	procGlobalMemoryStatusEx = kernel32.NewProc("GlobalMemoryStatusEx")
)

type filetime struct {
	Low  uint32
	High uint32
}

func (f filetime) value() uint64 { return uint64(f.High)<<32 | uint64(f.Low) }

// readCPUTimes 用 GetSystemTimes 取累计时间。
// 注意 kernel 时间已经包含 idle, 所以 total = kernel + user。
func readCPUTimes() (idle, total uint64, ok bool) {
	var idleT, kernelT, userT filetime
	r, _, _ := procGetSystemTimes.Call(
		uintptr(unsafe.Pointer(&idleT)),
		uintptr(unsafe.Pointer(&kernelT)),
		uintptr(unsafe.Pointer(&userT)),
	)
	if r == 0 {
		return 0, 0, false
	}
	idle = idleT.value()
	total = kernelT.value() + userT.value()
	return idle, total, true
}

type memoryStatusEx struct {
	Length               uint32
	MemoryLoad           uint32
	TotalPhys            uint64
	AvailPhys            uint64
	TotalPageFile        uint64
	AvailPageFile        uint64
	TotalVirtual         uint64
	AvailVirtual         uint64
	AvailExtendedVirtual uint64
}

// readMem 用 GlobalMemoryStatusEx 取物理内存总量与可用量。
func readMem() (used, total uint64, ok bool) {
	var m memoryStatusEx
	m.Length = uint32(unsafe.Sizeof(m))
	r, _, _ := procGlobalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&m)))
	if r == 0 || m.TotalPhys == 0 {
		return 0, 0, false
	}
	return m.TotalPhys - m.AvailPhys, m.TotalPhys, true
}
