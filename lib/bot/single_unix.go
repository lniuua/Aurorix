//go:build !windows

package bot

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
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
	if err := syscall.Kill(pid, syscall.SIGTERM); err != nil && err != syscall.ESRCH {
		return fmt.Errorf("终止旧实例失败: %w", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if syscall.Kill(pid, 0) != nil {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	logger.Warnf("旧实例(pid=%d)未在 5 秒内退出, 强制终止", pid)
	if err := syscall.Kill(pid, syscall.SIGKILL); err != nil && err != syscall.ESRCH {
		return fmt.Errorf("强制终止旧实例失败: %w", err)
	}
	return nil
}

func portOwnerPID(port uint16) int {
	hexPort := strings.ToUpper(fmt.Sprintf("%04X", port))
	want := make(map[string]bool)
	for _, path := range []string{"/proc/net/tcp", "/proc/net/tcp6"} {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(data), "\n")[1:] {
			f := strings.Fields(line)
			if len(f) < 10 || f[3] != "0A" {
				continue
			}
			local := f[1]
			sep := strings.LastIndex(local, ":")
			if sep < 0 || strings.ToUpper(local[sep+1:]) != hexPort {
				continue
			}
			want[f[9]] = true
		}
	}
	if len(want) == 0 {
		return 0
	}
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return 0
	}
	for _, e := range entries {
		if !isDigits(e.Name()) {
			continue
		}
		fds, err := os.ReadDir("/proc/" + e.Name() + "/fd")
		if err != nil {
			continue
		}
		for _, fd := range fds {
			link, err := os.Readlink("/proc/" + e.Name() + "/fd/" + fd.Name())
			if err != nil || len(link) < 9 || link[:8] != "socket:[" {
				continue
			}
			if want[link[8:len(link)-1]] {
				pid, _ := strconv.Atoi(e.Name())
				return pid
			}
		}
	}
	return 0
}

func isSelfBinary(pid int) bool {
	self, err := os.Executable()
	if err != nil {
		return false
	}
	name := filepath.Base(self)
	if comm, err := os.ReadFile(fmt.Sprintf("/proc/%d/comm", pid)); err == nil {
		if strings.TrimSpace(string(comm)) == name {
			return true
		}
	}
	if exe, err := os.Readlink(fmt.Sprintf("/proc/%d/exe", pid)); err == nil {
		return filepath.Base(exe) == name
	}
	return false
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
