//go:build windows

package bot

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

// ensureSingleInstance 检测占用同一端口的旧实例并终止, 保证单实例运行。
func ensureSingleInstance(port uint16) error {
	pid := portOwnerPID(port)
	if pid == 0 {
		return nil
	}
	if !isSelfBinary(pid) {
		logger.Warnf("端口 %d 被非本程序进程(pid=%d)占用, 不干预", port, pid)
		return nil
	}
	logger.Infof("检测到旧实例(pid=%d)占用端口 %d, 终止中", pid, port)
	handle, err := openProcess(pid)
	if err != nil {
		return fmt.Errorf("打开旧实例进程失败: %w", err)
	}
	defer closeHandle(handle)
	if err := terminateProcess(handle); err != nil {
		return fmt.Errorf("终止旧实例失败: %w", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if !processAlive(pid) {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	logger.Warnf("旧实例(pid=%d)未在 5 秒内退出, 强制终止", pid)
	if err := terminateProcess(handle); err != nil {
		return fmt.Errorf("强制终止旧实例失败: %w", err)
	}
	return nil
}

// portOwnerPID 通过 netstat 查找监听指定端口的进程 PID。
func portOwnerPID(port uint16) int {
	cmd := exec.Command("netstat", "-ano")
	out, err := cmd.Output()
	if err != nil {
		return 0
	}
	// 查找形如 "TCP  0.0.0.0:8080  0.0.0.0:0  LISTENING  1234" 的行
	targetSuffix := fmt.Sprintf(":%d ", port)
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}
		if fields[0] != "TCP" && fields[0] != "UDP" {
			continue
		}
		local := fields[1]
		if !strings.HasSuffix(local, targetSuffix) {
			continue
		}
		state := ""
		pidIdx := 4
		if fields[0] == "TCP" {
			state = fields[3]
			pidIdx = 4
		}
		// TCP 仅匹配 LISTENING
		if fields[0] == "TCP" && state != "LISTENING" {
			continue
		}
		if pidIdx >= len(fields) {
			continue
		}
		if pid, err := strconv.Atoi(fields[pidIdx]); err == nil && pid > 0 {
			return pid
		}
	}
	return 0
}

// isSelfBinary 判断 pid 对应进程是否为当前程序（通过可执行文件路径比对）。
func isSelfBinary(pid int) bool {
	self, err := os.Executable()
	if err != nil {
		return false
	}
	self = strings.ToLower(self)
	handle, err := openProcessQuery(pid)
	if err != nil {
		return false
	}
	defer closeHandle(handle)
	exePath, err := queryFullProcessImageName(handle)
	if err != nil {
		return false
	}
	return strings.EqualFold(strings.ToLower(exePath), self)
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// --- Windows API 封装 ---

var (
	modkernel32                        = syscall.NewLazyDLL("kernel32.dll")
	procOpenProcess                    = modkernel32.NewProc("OpenProcess")
	procCloseHandle                    = modkernel32.NewProc("CloseHandle")
	procTerminateProcess               = modkernel32.NewProc("TerminateProcess")
	procQueryFullProcessImageNameW     = modkernel32.NewProc("QueryFullProcessImageNameW")
	procWaitForSingleObject            = modkernel32.NewProc("WaitForSingleObject")
)

const (
	processTerminate       = 0x0001
	processQueryLimitedInfo = 0x1000
	processQueryInformation = 0x0400
)

func openProcess(pid int) (syscall.Handle, error) {
	return openProcessWithAccess(pid, processTerminate|processQueryLimitedInfo)
}

func openProcessQuery(pid int) (syscall.Handle, error) {
	return openProcessWithAccess(pid, processQueryLimitedInfo|processQueryInformation)
}

func openProcessWithAccess(pid int, access uint32) (syscall.Handle, error) {
	h, _, err := procOpenProcess.Call(uintptr(access), 0, uintptr(pid))
	if h == 0 {
		return 0, err
	}
	return syscall.Handle(h), nil
}

func closeHandle(h syscall.Handle) {
	if h != 0 {
		procCloseHandle.Call(uintptr(h))
	}
}

func terminateProcess(h syscall.Handle) error {
	r, _, err := procTerminateProcess.Call(uintptr(h), 1)
	if r == 0 {
		return err
	}
	return nil
}

func queryFullProcessImageName(h syscall.Handle) (string, error) {
	var buf [syscall.MAX_PATH * 2]uint16
	size := uint32(len(buf))
	r, _, err := procQueryFullProcessImageNameW.Call(
		uintptr(h), 0,
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(unsafe.Pointer(&size)),
	)
	if r == 0 {
		return "", err
	}
	return syscall.UTF16ToString(buf[:size]), nil
}

func processAlive(pid int) bool {
	h, err := openProcessQuery(pid)
	if err != nil {
		return false
	}
	defer closeHandle(h)
	return true
}
