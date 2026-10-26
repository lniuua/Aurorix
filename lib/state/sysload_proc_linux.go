//go:build linux

package state

import (
	"os"
	"strconv"
	"strings"
	"syscall"
)

// userHZ 是 /proc/self/stat 里 utime/stime 的时钟频率。
// Linux 内核的 USER_HZ 恒为 100, 与架构无关, 纯 Go 取不到 sysconf 也无妨。
const userHZ = 100

// readProcCPU 读本进程累计 CPU 时间(用户态+内核态), 单位纳秒。
func readProcCPU() (uint64, bool) {
	data, err := os.ReadFile("/proc/self/stat")
	if err != nil {
		return 0, false
	}
	s := string(data)
	// comm 字段带括号且可能含空格, 从前缀里跳过, 之后按空格切分。
	idx := strings.LastIndexByte(s, ')')
	if idx < 0 || idx+1 >= len(s) {
		return 0, false
	}
	fields := strings.Fields(s[idx+1:])
	// 切分后 fields[0] 是第 3 个字段(state), utime/stime 是第 14/15 个 → 下标 11/12。
	if len(fields) < 13 {
		return 0, false
	}
	utime, err1 := strconv.ParseUint(fields[11], 10, 64)
	stime, err2 := strconv.ParseUint(fields[12], 10, 64)
	if err1 != nil || err2 != nil {
		return 0, false
	}
	return (utime + stime) * (1e9 / userHZ), true
}

// readProcRSS 读本进程常驻内存(物理内存实际占用), 单位字节。
func readProcRSS() (uint64, bool) {
	data, err := os.ReadFile("/proc/self/statm")
	if err != nil {
		return 0, false
	}
	fields := strings.Fields(string(data))
	if len(fields) < 2 {
		return 0, false
	}
	pages, err := strconv.ParseUint(fields[1], 10, 64)
	if err != nil {
		return 0, false
	}
	return pages * uint64(os.Getpagesize()), true
}

// readDisk 读 dir 所在文件系统的已用/总量。
// 已用按"非特权用户可用"口径算, 与 df 的 Use% 一致(扣掉 root 预留)。
func readDisk(dir string) (used, total uint64, ok bool) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(dir, &st); err != nil || st.Blocks == 0 {
		return 0, 0, false
	}
	blockSize := uint64(st.Bsize)
	total = st.Blocks * blockSize
	avail := st.Bavail * blockSize
	if avail > total {
		avail = total
	}
	return total - avail, total, true
}

// readNet 汇总 /proc/net/dev 里除 lo 之外所有网卡的收发字节数。
func readNet() (rx, tx uint64, ok bool) {
	data, err := os.ReadFile("/proc/net/dev")
	if err != nil {
		return 0, 0, false
	}
	found := false
	for _, line := range strings.Split(string(data), "\n") {
		name, stats, hasColon := strings.Cut(line, ":")
		if !hasColon {
			continue
		}
		name = strings.TrimSpace(name)
		if name == "" || name == "lo" {
			continue
		}
		fields := strings.Fields(stats)
		if len(fields) < 9 {
			continue
		}
		in, err1 := strconv.ParseUint(fields[0], 10, 64)
		out, err2 := strconv.ParseUint(fields[8], 10, 64)
		if err1 != nil || err2 != nil {
			continue
		}
		rx += in
		tx += out
		found = true
	}
	return rx, tx, found
}

// readLoad1 读 /proc/loadavg 的 1 分钟平均负载。
func readLoad1() (float64, bool) {
	data, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return 0, false
	}
	fields := strings.Fields(string(data))
	if len(fields) == 0 {
		return 0, false
	}
	v, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

// readSysUptime 读 /proc/uptime 的整机开机时长(秒)。
func readSysUptime() (int64, bool) {
	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0, false
	}
	fields := strings.Fields(string(data))
	if len(fields) == 0 {
		return 0, false
	}
	v, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0, false
	}
	return int64(v), true
}
