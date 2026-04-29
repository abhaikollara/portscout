package main

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/shirou/gopsutil/v3/net"
	"github.com/shirou/gopsutil/v3/process"
)

// Connection represents a single network connection with its associated process.
type Connection struct {
	LocalPort  uint32
	PID        int32
	Process    string
	Status     string
	Protocol   string
	RemoteAddr string
}

var (
	nameCache     = make(map[int32]string)
	nameCacheMu   sync.Mutex
	scanCallCount int
)

// ScanConnections returns all inet connections with resolved process names.
func ScanConnections() ([]Connection, error) {
	conns, err := net.Connections("inet")
	if err != nil {
		return nil, err
	}

	nameCacheMu.Lock()
	scanCallCount++
	if scanCallCount >= 10 {
		nameCache = make(map[int32]string)
		scanCallCount = 0
	}
	nameCacheMu.Unlock()

	type dedupKey struct {
		port   uint32
		pid    int32
		status string
		remote string
	}
	seen := make(map[dedupKey]bool)

	var result []Connection
	for _, c := range conns {
		if c.Pid == 0 {
			continue
		}

		name := resolveProcessName(c.Pid)

		proto := "TCP"
		if c.Type == 2 { // syscall.SOCK_DGRAM
			proto = "UDP"
		}

		var remote string
		if c.Raddr.IP != "" {
			remote = fmt.Sprintf("%s:%d", c.Raddr.IP, c.Raddr.Port)
		}

		dk := dedupKey{c.Laddr.Port, c.Pid, c.Status, remote}
		if seen[dk] {
			continue
		}
		seen[dk] = true

		result = append(result, Connection{
			LocalPort:  c.Laddr.Port,
			PID:        c.Pid,
			Process:    name,
			Status:     c.Status,
			Protocol:   proto,
			RemoteAddr: remote,
		})
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].LocalPort < result[j].LocalPort
	})

	return result, nil
}

// ProcessDetail holds extended info about a process.
type ProcessDetail struct {
	PID        int32
	Name       string
	Cmdline    string
	User       string
	CPUPercent float64
	MemPercent float32
	Status     string
	CreateTime int64
	NumFDs     int32
	Cwd        string
}

// GetProcessDetail fetches extended information for a given PID.
func GetProcessDetail(pid int32) (ProcessDetail, error) {
	p, err := process.NewProcess(pid)
	if err != nil {
		return ProcessDetail{}, err
	}

	d := ProcessDetail{PID: pid}

	if name, err := p.Name(); err == nil {
		d.Name = name
	}
	if cmd, err := p.Cmdline(); err == nil {
		d.Cmdline = cmd
	}
	if user, err := p.Username(); err == nil {
		d.User = user
	}
	if cpu, err := p.CPUPercent(); err == nil {
		d.CPUPercent = cpu
	}
	if mem, err := p.MemoryPercent(); err == nil {
		d.MemPercent = mem
	}
	if statuses, err := p.Status(); err == nil {
		d.Status = strings.Join(statuses, ",")
	}
	if ct, err := p.CreateTime(); err == nil {
		d.CreateTime = ct
	}
	if fds, err := p.NumFDs(); err == nil {
		d.NumFDs = fds
	}
	if cwd, err := p.Cwd(); err == nil {
		d.Cwd = cwd
	}

	return d, nil
}

func resolveProcessName(pid int32) string {
	nameCacheMu.Lock()
	if name, ok := nameCache[pid]; ok {
		nameCacheMu.Unlock()
		return name
	}
	nameCacheMu.Unlock()

	name := "unknown"
	if p, err := process.NewProcess(pid); err == nil {
		if n, err := p.Name(); err == nil {
			name = n
		}
	}

	nameCacheMu.Lock()
	nameCache[pid] = name
	nameCacheMu.Unlock()

	return name
}
