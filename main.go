package main

import (
	"fmt"
	"os"
	"strconv"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/shirou/gopsutil/v3/process"
)

var version = "dev"

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "--version", "-v":
			fmt.Printf("portscout %s\n", version)
			return
		case "-k":
			if len(os.Args) < 3 {
				fmt.Fprintf(os.Stderr, "Usage: portscout -k <port>\n")
				os.Exit(1)
			}
			port, err := strconv.ParseUint(os.Args[2], 10, 32)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Usage: portscout -k <port>\n")
				os.Exit(1)
			}
			killPort(uint32(port))
			return
		default:
			port, err := strconv.ParseUint(os.Args[1], 10, 32)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Usage: portscout [port] [-k port]\n")
				os.Exit(1)
			}
			lookupPort(uint32(port))
			return
		}
	}

	p := tea.NewProgram(newModel(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func killPort(port uint32) {
	conns, err := ScanConnections()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error scanning connections: %v\n", err)
		os.Exit(1)
	}

	killed := make(map[int32]bool)
	for _, c := range conns {
		if c.LocalPort != port || killed[c.PID] {
			continue
		}

		p, err := process.NewProcess(c.PID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to find process %d: %v\n", c.PID, err)
			continue
		}
		if err := p.Kill(); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to kill %s (PID %d): %v\n", c.Process, c.PID, err)
		} else {
			fmt.Printf("Killed %s (PID %d) on port %d\n", c.Process, c.PID, port)
		}
		killed[c.PID] = true
	}

	if len(killed) == 0 {
		fmt.Printf("No process found on port %d\n", port)
	}
}

func lookupPort(port uint32) {
	conns, err := ScanConnections()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error scanning connections: %v\n", err)
		os.Exit(1)
	}

	var found []Connection
	for _, c := range conns {
		if c.LocalPort == port {
			found = append(found, c)
		}
	}

	if len(found) == 0 {
		fmt.Printf("No process found on port %d\n", port)
		return
	}

	seen := make(map[int32]bool)
	for _, c := range found {
		fmt.Printf("Port %d — %s (PID %d) [%s] %s\n", c.LocalPort, c.Process, c.PID, c.Status, c.RemoteAddr)

		if seen[c.PID] {
			continue
		}
		seen[c.PID] = true

		d, err := GetProcessDetail(c.PID)
		if err != nil {
			continue
		}
		fmt.Printf("  %-14s %s\n", "User:", d.User)
		fmt.Printf("  %-14s %s\n", "Status:", d.Status)
		fmt.Printf("  %-14s %.1f%%\n", "CPU:", d.CPUPercent)
		fmt.Printf("  %-14s %.1f%%\n", "Memory:", d.MemPercent)
		fmt.Printf("  %-14s %d\n", "File Desc.:", d.NumFDs)
		fmt.Printf("  %-14s %s\n", "Working Dir:", d.Cwd)
		fmt.Printf("  %-14s %s\n", "Command:", d.Cmdline)
		fmt.Println()
	}
}
