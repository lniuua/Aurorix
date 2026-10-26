package state

import (
	"os"
	"runtime"
	"sync"
	"time"
)

// SysLoad 是整机与进程资源占用的最近一次采样。
// 平台读不到的字段保持 0(或 Load1 = -1), 前端据此显示 "—"。
type SysLoad struct {
	CPUPercent float64 `json:"cpu_percent"`
	MemUsed    uint64  `json:"mem_used"`
	MemTotal   uint64  `json:"mem_total"`

	// 本进程占用: CPU 已按核心数折算成"整机占比", 与 CPUPercent 同量纲。
	ProcCPUPercent float64 `json:"proc_cpu_percent"`
	ProcRSS        uint64  `json:"proc_rss"`

	// 进程工作目录所在分区的占用。
	DiskUsed  uint64 `json:"disk_used"`
	DiskTotal uint64 `json:"disk_total"`
	DiskPath  string `json:"disk_path"`

	// 网络: Rate 是字节/秒, Total 是采样时刻的累计值(32 位平台可能回绕)。
	NetRxRate  uint64 `json:"net_rx_rate"`
	NetTxRate  uint64 `json:"net_tx_rate"`
	NetRxTotal uint64 `json:"net_rx_total"`
	NetTxTotal uint64 `json:"net_tx_total"`

	// Load1 是 1 分钟平均负载, -1 表示平台不提供(Windows)。
	Load1 float64 `json:"load1"`

	// SysUptimeSec 是整机开机时长(秒)。
	SysUptimeSec int64 `json:"sys_uptime_sec"`
}

var (
	sysMu    sync.RWMutex
	sysLoad  = SysLoad{Load1: -1}
	sysOnce  sync.Once
	diskRoot string
)

// StartSysSampler 启动整机资源采样的后台循环(幂等)。interval 建议 1~3 秒。
//
// CPU 占用率必须靠"两次时间差"算, 单次读数只能得到累计值, 所以这里常驻一个 ticker,
// 概览接口只读最近一次结果 —— 请求多快都不会多采样。
func StartSysSampler(interval time.Duration) {
	if interval <= 0 {
		interval = 2 * time.Second
	}
	sysOnce.Do(func() {
		diskRoot = workingDir()
		go sampleLoop(interval)
	})
}

func sampleLoop(interval time.Duration) {
	cores := runtime.NumCPU()
	prevIdle, prevTotal, prevCPUOK := readCPUTimes()
	prevProc, prevProcOK := readProcCPU()
	prevRx, prevTx, prevNetOK := readNet()
	prevAt := time.Now()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for range ticker.C {
		now := time.Now()
		elapsed := now.Sub(prevAt).Seconds()
		if elapsed <= 0 {
			elapsed = interval.Seconds()
		}
		prevAt = now

		sysMu.RLock()
		snap := sysLoad
		sysMu.RUnlock()
		snap.DiskPath = diskRoot

		// ---- 整机 CPU ----
		idle, total, cpuOK := readCPUTimes()
		if cpuOK && prevCPUOK && total > prevTotal {
			deltaTotal := total - prevTotal
			snap.CPUPercent = clampPercent(float64(deltaTotal-(idle-prevIdle)) * 100 / float64(deltaTotal))
		}
		if cpuOK {
			prevIdle, prevTotal = idle, total
		}
		prevCPUOK = cpuOK

		// ---- 整机内存 ----
		if used, memTotal, memOK := readMem(); memOK {
			snap.MemUsed, snap.MemTotal = used, memTotal
		}

		// ---- 本进程 CPU / RSS ----
		procNow, procOK := readProcCPU()
		if procOK && prevProcOK && procNow > prevProc {
			cpuSec := float64(procNow-prevProc) / 1e9
			snap.ProcCPUPercent = clampPercent(cpuSec / elapsed * 100 / float64(cores))
		}
		if procOK {
			prevProc = procNow
		}
		prevProcOK = procOK
		if rss, rssOK := readProcRSS(); rssOK {
			snap.ProcRSS = rss
		}

		// ---- 磁盘 ----
		if diskUsed, diskTotal, diskOK := readDisk(diskRoot); diskOK {
			snap.DiskUsed, snap.DiskTotal = diskUsed, diskTotal
		}

		// ---- 网络 ----
		rx, tx, netOK := readNet()
		if netOK {
			if prevNetOK {
				snap.NetRxRate = bytesPerSec(prevRx, rx, elapsed)
				snap.NetTxRate = bytesPerSec(prevTx, tx, elapsed)
			}
			snap.NetRxTotal, snap.NetTxTotal = rx, tx
			prevRx, prevTx = rx, tx
		}
		prevNetOK = netOK

		// ---- 负载 / 开机时长 ----
		if load, loadOK := readLoad1(); loadOK {
			snap.Load1 = load
		} else {
			snap.Load1 = -1
		}
		if up, upOK := readSysUptime(); upOK {
			snap.SysUptimeSec = up
		}

		sysMu.Lock()
		sysLoad = snap
		sysMu.Unlock()
	}
}

// 计数器回绕(32 位网卡字节数约 4GiB 一圈)时无法分辨真实增量, 直接记 0。
func bytesPerSec(prev, cur uint64, elapsed float64) uint64 {
	if cur <= prev || elapsed <= 0 {
		return 0
	}
	return uint64(float64(cur-prev) / elapsed)
}

func workingDir() string {
	if dir, err := os.Getwd(); err == nil {
		return dir
	}
	if exe, err := os.Executable(); err == nil {
		return exe
	}
	return "."
}

// SysLoadSnapshot 返回最近一次整机采样; 采样器未启动时为零值。
func SysLoadSnapshot() SysLoad {
	sysMu.RLock()
	defer sysMu.RUnlock()
	return sysLoad
}

func clampPercent(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}
