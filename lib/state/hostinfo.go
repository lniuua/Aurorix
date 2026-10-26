package state

import (
	"os"
	"runtime"
	"sync"
)

// HostInfo 是进程所在主机的静态信息, 进程启动后算一次即可。
type HostInfo struct {
	Hostname   string `json:"hostname"`
	OS         string `json:"os"`
	Arch       string `json:"arch"`
	PID        int    `json:"pid"`
	CPUCores   int    `json:"cpu_cores"`
	Cwd        string `json:"cwd"`
	Executable string `json:"executable"`
}

var (
	hostOnce sync.Once
	hostInfo HostInfo
)

// HostSnapshot 返回主机静态信息; 读不到的字段留空, 前端显示 "—"。
func HostSnapshot() HostInfo {
	hostOnce.Do(func() {
		name, _ := os.Hostname()
		cwd, _ := os.Getwd()
		exe, _ := os.Executable()
		hostInfo = HostInfo{
			Hostname:   name,
			OS:         runtime.GOOS,
			Arch:       runtime.GOARCH,
			PID:        os.Getpid(),
			CPUCores:   runtime.NumCPU(),
			Cwd:        cwd,
			Executable: exe,
		}
	})
	return hostInfo
}
