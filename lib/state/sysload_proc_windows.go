//go:build windows

package state

import (
	"encoding/binary"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

var (
	procGetProcessTimes      = kernel32.NewProc("GetProcessTimes")
	procGetDiskFreeSpaceExW  = kernel32.NewProc("GetDiskFreeSpaceExW")
	procGetTickCount64       = kernel32.NewProc("GetTickCount64")
	psapi                    = syscall.NewLazyDLL("psapi.dll")
	procGetProcessMemoryInfo = psapi.NewProc("GetProcessMemoryInfo")
	iphlpapi                 = syscall.NewLazyDLL("iphlpapi.dll")
	procGetIfTable           = iphlpapi.NewProc("GetIfTable")
)

// currentProcess 是 GetCurrentProcess() 的伪句柄(-1)。
var currentProcess = ^uintptr(0)

// readProcCPU 用 GetProcessTimes 取本进程累计 CPU 时间(内核态+用户态), 单位纳秒。
// FILETIME 的 1 个计数是 100ns。
func readProcCPU() (uint64, bool) {
	var creation, exit, kernel, user filetime
	r, _, _ := procGetProcessTimes.Call(
		currentProcess,
		uintptr(unsafe.Pointer(&creation)),
		uintptr(unsafe.Pointer(&exit)),
		uintptr(unsafe.Pointer(&kernel)),
		uintptr(unsafe.Pointer(&user)),
	)
	if r == 0 {
		return 0, false
	}
	return (kernel.value() + user.value()) * 100, true
}

type processMemoryCounters struct {
	Cb                         uint32
	PageFaultCount             uint32
	PeakWorkingSetSize         uintptr
	WorkingSetSize             uintptr
	QuotaPeakPagedPoolUsage    uintptr
	QuotaPagedPoolUsage        uintptr
	QuotaPeakNonPagedPoolUsage uintptr
	QuotaNonPagedPoolUsage     uintptr
	PagefileUsage              uintptr
	PeakPagefileUsage          uintptr
}

// readProcRSS 用 GetProcessMemoryInfo 取本进程工作集(物理内存实际占用)。
func readProcRSS() (uint64, bool) {
	var pmc processMemoryCounters
	pmc.Cb = uint32(unsafe.Sizeof(pmc))
	r, _, _ := procGetProcessMemoryInfo.Call(
		currentProcess,
		uintptr(unsafe.Pointer(&pmc)),
		uintptr(pmc.Cb),
	)
	if r == 0 {
		return 0, false
	}
	return uint64(pmc.WorkingSetSize), true
}

// readDisk 用 GetDiskFreeSpaceExW 取 dir 所在卷的已用/总量。
func readDisk(dir string) (used, total uint64, ok bool) {
	path, err := syscall.UTF16PtrFromString(volumeRoot(dir))
	if err != nil {
		return 0, 0, false
	}
	var freeToCaller, totalBytes, totalFree uint64
	r, _, _ := procGetDiskFreeSpaceExW.Call(
		uintptr(unsafe.Pointer(path)),
		uintptr(unsafe.Pointer(&freeToCaller)),
		uintptr(unsafe.Pointer(&totalBytes)),
		uintptr(unsafe.Pointer(&totalFree)),
	)
	if r == 0 || totalBytes == 0 || totalFree > totalBytes {
		return 0, 0, false
	}
	return totalBytes - totalFree, totalBytes, true
}

// volumeRoot 把任意路径折算成 GetDiskFreeSpaceExW 需要的卷根, 如 "D:\"。
func volumeRoot(dir string) string {
	dir = filepath.Clean(dir)
	if len(dir) >= 2 && dir[1] == ':' {
		return dir[:2] + `\`
	}
	if rest, found := strings.CutPrefix(dir, `\\`); found {
		parts := strings.SplitN(rest, `\`, 3)
		if len(parts) >= 2 {
			return `\\` + parts[0] + `\` + parts[1] + `\`
		}
	}
	return dir
}

// ---- GetIfTable: MIB_IFTABLE / MIB_IFROW ----
//
// 结构固定且自 Win2000 未变, 这里按下标从裸缓冲区读取, 避免维护一长串 Go 结构体。
const (
	ifRowSize      = 860 // sizeof(MIB_IFROW)
	ifOffType      = 516 // dwType
	ifOffInOctets  = 552 // dwInOctets
	ifOffOutOctets = 576 // dwOutOctets
	ifTypeLoopback = 24  // IF_TYPE_SOFTWARE_LOOPBACK
)

// readNet 汇总所有非回环网卡的收发字节数。
// 注意 dwInOctets/dwOutOctets 是 32 位, 超过 4GiB 会回绕, 增量由上层判定。
func readNet() (rx, tx uint64, ok bool) {
	var size uint32
	procGetIfTable.Call(0, uintptr(unsafe.Pointer(&size)), 0)
	if size < ifRowSize {
		return 0, 0, false
	}
	buf := make([]byte, size)
	r, _, _ := procGetIfTable.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)), 0)
	if r != 0 {
		return 0, 0, false
	}
	if len(buf) < 4 {
		return 0, 0, false
	}
	count := int(binary.LittleEndian.Uint32(buf[:4]))
	for i := 0; i < count; i++ {
		base := 4 + i*ifRowSize
		if base+ifRowSize > len(buf) {
			break
		}
		if binary.LittleEndian.Uint32(buf[base+ifOffType:]) == ifTypeLoopback {
			continue
		}
		rx += uint64(binary.LittleEndian.Uint32(buf[base+ifOffInOctets:]))
		tx += uint64(binary.LittleEndian.Uint32(buf[base+ifOffOutOctets:]))
	}
	return rx, tx, true
}

// readLoad1 Windows 没有 load average 概念, 交由前端显示 "—"。
func readLoad1() (float64, bool) { return 0, false }

// readSysUptime 用 GetTickCount64 取开机时长(毫秒)。
func readSysUptime() (int64, bool) {
	r, _, _ := procGetTickCount64.Call()
	if r == 0 {
		return 0, false
	}
	return int64(r / 1000), true
}
