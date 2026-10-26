//go:build linux

package state

import (
	"os"
	"strconv"
	"strings"
)

// readCPUTimes 读 /proc/stat 的第一行 "cpu ..."。
// 返回的 idle 含 iowait, total 是全部时间片之和。
func readCPUTimes() (idle, total uint64, ok bool) {
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return 0, 0, false
	}
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(line, "cpu ") {
			continue
		}
		fields := strings.Fields(line)[1:]
		if len(fields) < 4 {
			return 0, 0, false
		}
		vals := make([]uint64, 0, len(fields))
		for _, f := range fields {
			v, _ := strconv.ParseUint(f, 10, 64)
			vals = append(vals, v)
		}
		idle = vals[3] // idle
		if len(vals) > 4 {
			idle += vals[4] // iowait 本质也是闲着
		}
		for _, v := range vals {
			total += v
		}
		return idle, total, true
	}
	return 0, 0, false
}

// readMem 读 /proc/meminfo; available 缺失时退回 free+buffers+cached。
func readMem() (used, total uint64, ok bool) {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0, 0, false
	}
	var memTotal, memAvail, memFree, buffers, cached uint64
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		v, _ := strconv.ParseUint(fields[1], 10, 64)
		v *= 1024 // kB -> B
		switch fields[0] {
		case "MemTotal:":
			memTotal = v
		case "MemAvailable:":
			memAvail = v
		case "MemFree:":
			memFree = v
		case "Buffers:":
			buffers = v
		case "Cached:":
			cached = v
		}
	}
	if memTotal == 0 {
		return 0, 0, false
	}
	if memAvail == 0 {
		memAvail = memFree + buffers + cached
	}
	return memTotal - min(memAvail, memTotal), memTotal, true
}
