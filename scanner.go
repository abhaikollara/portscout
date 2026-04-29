package main

import (
	"sort"
	"sync"

	"github.com/shirou/gopsutil/v3/net"
	"github.com/shirou/gopsutil/v3/process"
)

// Connection represents a single network connection with its associated process.
type Connection struct {
	LocalPort uint32
	PID       int32
	Process   string
	Status    string
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

	var result []Connection
	for _, c := range conns {
		if c.Pid == 0 {
			continue
		}

		name := resolveProcessName(c.Pid)
		result = append(result, Connection{
			LocalPort: c.Laddr.Port,
			PID:       c.Pid,
			Process:   name,
			Status:    c.Status,
		})
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].LocalPort < result[j].LocalPort
	})

	return result, nil
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
