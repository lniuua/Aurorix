//go:build !linux && !windows

package state

// 其它平台没有轻量的标准库接口, 采集返回不可用, 概览页显示 "—"。

func readCPUTimes() (idle, total uint64, ok bool) { return 0, 0, false }

func readMem() (used, total uint64, ok bool) { return 0, 0, false }

func readProcCPU() (uint64, bool) { return 0, false }

func readProcRSS() (uint64, bool) { return 0, false }

func readDisk(string) (used, total uint64, ok bool) { return 0, 0, false }

func readNet() (rx, tx uint64, ok bool) { return 0, 0, false }

func readLoad1() (float64, bool) { return 0, false }

func readSysUptime() (int64, bool) { return 0, false }
