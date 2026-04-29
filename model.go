package main

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/shirou/gopsutil/v3/process"
)

var columnNames = []string{"Port", "Process", "PID", "Proto", "Status", "Remote"}

type tickMsg time.Time
type scanResultMsg []Connection
type killResultMsg struct {
	pid int32
	err error
}

type model struct {
	table       table.Model
	connections []Connection
	filtered    []Connection

	filter       string
	filterActive bool

	statusMsg   string
	statusError bool
	statusTicks int

	confirmKill bool
	killTarget  *Connection

	sortCol int
	sortAsc bool

	width  int
	height int
}

func newModel() model {
	columns := []table.Column{
		{Title: "Port", Width: 8},
		{Title: "Process", Width: 20},
		{Title: "PID", Width: 8},
		{Title: "Proto", Width: 5},
		{Title: "Status", Width: 16},
		{Title: "Remote", Width: 20},
	}

	km := table.DefaultKeyMap()
	// Remove 'k' from LineUp so we can use it for kill
	km.LineUp = key.NewBinding(
		key.WithKeys("up"),
		key.WithHelp("↑", "up"),
	)

	t := table.New(
		table.WithColumns(columns),
		table.WithRows([]table.Row{}),
		table.WithFocused(true),
		table.WithHeight(20),
		table.WithStyles(newTableStyles()),
		table.WithKeyMap(km),
	)

	return model{
		table:   t,
		sortCol: 0,
		sortAsc: true,
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(scanCmd(), tickCmd())
}

func scanCmd() tea.Cmd {
	return func() tea.Msg {
		conns, err := ScanConnections()
		if err != nil {
			return scanResultMsg(nil)
		}
		return scanResultMsg(conns)
	}
}

func tickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func killCmd(pid int32) tea.Cmd {
	return func() tea.Msg {
		p, err := process.NewProcess(pid)
		if err != nil {
			return killResultMsg{pid: pid, err: err}
		}
		err = p.Kill()
		return killResultMsg{pid: pid, err: err}
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.table.SetWidth(msg.Width)
		m.table.SetHeight(msg.Height - 2) // header + footer
		m.resizeColumns()
		return m, nil

	case tickMsg:
		if m.statusTicks > 0 {
			m.statusTicks--
			if m.statusTicks == 0 {
				m.statusMsg = ""
			}
		}
		return m, tea.Batch(scanCmd(), tickCmd())

	case scanResultMsg:
		m.connections = []Connection(msg)
		m.applyFilter()
		m.sortConnections()
		m.rebuildTable()
		return m, nil

	case killResultMsg:
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("Failed to kill PID %d: %v", msg.pid, msg.err)
			m.statusError = true
		} else {
			m.statusMsg = fmt.Sprintf("Killed PID %d", msg.pid)
			m.statusError = false
		}
		m.statusTicks = 3
		m.confirmKill = false
		m.killTarget = nil
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	return m, nil
}

func (m model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Kill confirmation mode
	if m.confirmKill {
		switch msg.String() {
		case "y", "Y", "enter":
			if m.killTarget != nil {
				return m, killCmd(m.killTarget.PID)
			}
			m.confirmKill = false
			m.killTarget = nil
			return m, nil
		case "n", "N", "esc":
			m.confirmKill = false
			m.killTarget = nil
			return m, nil
		}
		return m, nil
	}

	// Filter input mode
	if m.filterActive {
		switch msg.String() {
		case "esc":
			m.filter = ""
			m.filterActive = false
			m.applyFilter()
			m.sortConnections()
			m.rebuildTable()
			return m, nil
		case "enter":
			m.filterActive = false
			return m, nil
		case "backspace":
			if len(m.filter) > 0 {
				m.filter = m.filter[:len(m.filter)-1]
				m.applyFilter()
				m.sortConnections()
				m.rebuildTable()
			}
			return m, nil
		default:
			if len(msg.String()) == 1 {
				m.filter += msg.String()
				m.applyFilter()
				m.sortConnections()
				m.rebuildTable()
			}
			return m, nil
		}
	}

	// Normal mode
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "esc":
		if m.filter != "" {
			m.filter = ""
			m.applyFilter()
			m.sortConnections()
			m.rebuildTable()
		}
		return m, nil
	case "/":
		m.filterActive = true
		return m, nil
	case "k":
		if len(m.filtered) > 0 {
			cursor := m.table.Cursor()
			if cursor >= 0 && cursor < len(m.filtered) {
				target := m.filtered[cursor]
				m.killTarget = &target
				m.confirmKill = true
			}
		}
		return m, nil
	case "s":
		m.sortCol = (m.sortCol + 1) % len(columnNames)
		m.sortConnections()
		m.rebuildTable()
		m.resizeColumns()
		return m, nil
	case "S":
		m.sortAsc = !m.sortAsc
		m.sortConnections()
		m.rebuildTable()
		m.resizeColumns()
		return m, nil
	}

	// Pass navigation keys to table
	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

func (m *model) applyFilter() {
	if m.filter == "" {
		m.filtered = m.connections
		return
	}
	f := strings.ToLower(m.filter)
	var result []Connection
	for _, c := range m.connections {
		portStr := fmt.Sprintf("%d", c.LocalPort)
		if strings.Contains(portStr, f) ||
			strings.Contains(strings.ToLower(c.Process), f) ||
			strings.Contains(strings.ToLower(c.Protocol), f) ||
			strings.Contains(strings.ToLower(c.RemoteAddr), f) {
			result = append(result, c)
		}
	}
	m.filtered = result
}

func (m *model) resizeColumns() {
	if m.width <= 0 {
		return
	}

	// Build titles with sort indicator
	titles := make([]string, len(columnNames))
	for i, name := range columnNames {
		if i == m.sortCol {
			arrow := "▲"
			if !m.sortAsc {
				arrow = "▼"
			}
			titles[i] = name + " " + arrow
		} else {
			titles[i] = name
		}
	}

	// Fixed columns: Port(8), PID(8), Proto(5), Status(16), padding(12 = 2*6 cols)
	// Process and Remote split the remaining width (60/40)
	fixed := 8 + 8 + 5 + 16 + 12
	remaining := m.width - fixed
	if remaining < 20 {
		remaining = 20
	}
	processWidth := remaining * 60 / 100
	remoteWidth := remaining - processWidth

	m.table.SetColumns([]table.Column{
		{Title: titles[0], Width: 8},
		{Title: titles[1], Width: processWidth},
		{Title: titles[2], Width: 8},
		{Title: titles[3], Width: 5},
		{Title: titles[4], Width: 16},
		{Title: titles[5], Width: remoteWidth},
	})
}

func (m *model) sortConnections() {
	sort.SliceStable(m.filtered, func(i, j int) bool {
		var less bool
		a, b := m.filtered[i], m.filtered[j]
		switch m.sortCol {
		case 0: // Port
			less = a.LocalPort < b.LocalPort
		case 1: // Process
			less = strings.ToLower(a.Process) < strings.ToLower(b.Process)
		case 2: // PID
			less = a.PID < b.PID
		case 3: // Proto
			less = a.Protocol < b.Protocol
		case 4: // Status
			less = a.Status < b.Status
		case 5: // Remote
			less = a.RemoteAddr < b.RemoteAddr
		}
		if !m.sortAsc {
			return !less
		}
		return less
	})
}

func statusSymbol(status string) string {
	switch status {
	case "LISTEN":
		return "◉ LISTEN"
	case "ESTABLISHED":
		return "⬤ ESTABLISHED"
	case "CLOSE_WAIT":
		return "◌ CLOSE_WAIT"
	case "TIME_WAIT":
		return "○ TIME_WAIT"
	default:
		return "· " + status
	}
}

func (m *model) rebuildTable() {
	rows := make([]table.Row, len(m.filtered))
	for i, c := range m.filtered {
		rows[i] = table.Row{
			fmt.Sprintf("%d", c.LocalPort),
			c.Process,
			fmt.Sprintf("%d", c.PID),
			c.Protocol,
			statusSymbol(c.Status),
			c.RemoteAddr,
		}
	}
	m.table.SetRows(rows)
	if m.table.Cursor() >= len(rows) && len(rows) > 0 {
		m.table.SetCursor(len(rows) - 1)
	}
}

func (m model) View() string {
	// Header
	title := "PortScout"
	if m.filter != "" || m.filterActive {
		cursor := ""
		if m.filterActive {
			cursor = "▏"
		}
		title += fmt.Sprintf("  Filter: %s%s", m.filter, cursor)
	}
	header := headerStyle.Width(m.width).Render(title)

	// Table
	tableView := m.table.View()

	// Footer
	var footer string
	if m.confirmKill && m.killTarget != nil {
		footer = confirmStyle.Width(m.width).Render(
			fmt.Sprintf("Kill %s (PID %d)? [y/n]", m.killTarget.Process, m.killTarget.PID),
		)
	} else if m.statusMsg != "" {
		if m.statusError {
			footer = statusErrorStyle.Width(m.width).Render(m.statusMsg)
		} else {
			footer = statusSuccessStyle.Width(m.width).Render(m.statusMsg)
		}
	} else {
		footer = footerStyle.Width(m.width).Render("[q] Quit  [k] Kill  [/] Search  [s] Sort  [S] Reverse  [esc] Clear Filter")
	}

	return lipgloss.JoinVertical(lipgloss.Left, header, tableView, footer)
}
