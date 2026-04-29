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
var groupedColumnNames = []string{"PID", "Process", "Conns", "Ports", "Statuses"}

type tickMsg time.Time
type scanResultMsg []Connection
type killResultMsg struct {
	pid int32
	err error
}
type detailResultMsg struct {
	detail ProcessDetail
	err    error
}

type model struct {
	table       table.Model
	connections []Connection
	filtered    []Connection

	filter       string
	filterActive bool
	filterPID    int32 // when set, only show this PID (from group drill-down)

	statusMsg   string
	statusError bool
	statusTicks int

	confirmKill bool
	killTarget  *Connection

	sortCol int
	sortAsc bool
	frozen  bool
	grouped bool

	groups []processGroup // populated when grouped=true

	showDetail bool
	detail     *ProcessDetail

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

func fetchDetailCmd(pid int32) tea.Cmd {
	return func() tea.Msg {
		d, err := GetProcessDetail(pid)
		return detailResultMsg{detail: d, err: err}
	}
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
		if m.frozen {
			return m, tickCmd()
		}
		return m, tea.Batch(scanCmd(), tickCmd())

	case scanResultMsg:
		m.connections = []Connection(msg)
		m.refreshView()
		return m, nil

	case detailResultMsg:
		if msg.err == nil {
			m.detail = &msg.detail
			m.showDetail = true
		}
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
	// Detail view mode — any key closes it
	if m.showDetail {
		m.showDetail = false
		m.detail = nil
		return m, nil
	}

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
			m.refreshView()
			return m, nil
		case "enter":
			m.filterActive = false
			return m, nil
		case "backspace":
			if len(m.filter) > 0 {
				m.filter = m.filter[:len(m.filter)-1]
				m.refreshView()
			}
			return m, nil
		default:
			if len(msg.String()) == 1 {
				m.filter += msg.String()
				m.refreshView()
			}
			return m, nil
		}
	}

	// Normal mode
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "esc":
		if m.filter != "" || m.filterPID > 0 {
			m.filter = ""
			m.filterPID = 0
			m.refreshView()
		}
		return m, nil
	case "/":
		m.filterActive = true
		return m, nil
	case "enter":
		cursor := m.table.Cursor()
		if m.grouped {
			if cursor >= 0 && cursor < len(m.groups) {
				g := m.groups[cursor]
				m.grouped = false
				m.filterPID = g.PID
				m.refreshView()
			}
			return m, nil
		}
		var pid int32
		if cursor >= 0 && cursor < len(m.filtered) {
			pid = m.filtered[cursor].PID
		}
		if pid > 0 {
			return m, fetchDetailCmd(pid)
		}
		return m, nil
	case "k":
		cursor := m.table.Cursor()
		if m.grouped {
			if cursor >= 0 && cursor < len(m.groups) {
				g := m.groups[cursor]
				m.killTarget = &Connection{PID: g.PID, Process: g.Process}
				m.confirmKill = true
			}
		} else if cursor >= 0 && cursor < len(m.filtered) {
			target := m.filtered[cursor]
			m.killTarget = &target
			m.confirmKill = true
		}
		return m, nil
	case "s":
		if !m.grouped {
			m.sortCol = (m.sortCol + 1) % len(columnNames)
			m.sortConnections()
			m.rebuildTable()
			m.resizeColumns()
		}
		return m, nil
	case "S":
		if !m.grouped {
			m.sortAsc = !m.sortAsc
			m.sortConnections()
			m.rebuildTable()
			m.resizeColumns()
		}
		return m, nil
	case "f":
		m.frozen = !m.frozen
		return m, nil
	case "g":
		m.grouped = !m.grouped
		m.resizeColumns()
		if m.grouped {
			m.buildGroups()
			m.rebuildGroupedTable()
		} else {
			m.rebuildTable()
		}
		return m, nil
	}

	// Pass navigation keys to table
	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

func (m *model) refreshView() {
	m.applyFilter()
	m.sortConnections()
	m.resizeColumns()
	if m.grouped {
		m.buildGroups()
		m.rebuildGroupedTable()
	} else {
		m.rebuildTable()
	}
}

func (m *model) applyFilter() {
	if m.filter == "" && m.filterPID == 0 {
		m.filtered = m.connections
		return
	}
	f := strings.ToLower(m.filter)
	var result []Connection
	for _, c := range m.connections {
		if m.filterPID > 0 && c.PID != m.filterPID {
			continue
		}
		if f != "" {
			portStr := fmt.Sprintf("%d", c.LocalPort)
			if !strings.Contains(portStr, f) &&
				!strings.Contains(strings.ToLower(c.Process), f) &&
				!strings.Contains(strings.ToLower(c.Protocol), f) &&
				!strings.Contains(strings.ToLower(c.RemoteAddr), f) {
				continue
			}
		}
		result = append(result, c)
	}
	m.filtered = result
}

func (m *model) resizeColumns() {
	if m.width <= 0 {
		return
	}

	// Clear rows when column count changes to prevent index-out-of-range
	// panics in the table's internal renderRow.
	currentColCount := len(m.table.Columns())
	targetColCount := len(columnNames)
	if m.grouped {
		targetColCount = len(groupedColumnNames)
	}
	if currentColCount != targetColCount {
		m.table.SetRows([]table.Row{})
	}

	if m.grouped {
		names := groupedColumnNames
		titles := make([]string, len(names))
		copy(titles, names)

		// Grouped: PID(8), Process(flex), Conns(6), Ports(flex), Statuses(18)
		// padding = 2 * 5 cols = 10
		fixed := 8 + 6 + 18 + 10
		remaining := m.width - fixed
		if remaining < 20 {
			remaining = 20
		}
		processWidth := remaining * 40 / 100
		portsWidth := remaining - processWidth

		m.table.SetColumns([]table.Column{
			{Title: titles[0], Width: 8},
			{Title: titles[1], Width: processWidth},
			{Title: titles[2], Width: 6},
			{Title: titles[3], Width: portsWidth},
			{Title: titles[4], Width: 18},
		})
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

type processGroup struct {
	PID      int32
	Process  string
	Count    int
	Ports    string
	Statuses string
}

func (m *model) buildGroups() {
	type pidData struct {
		process  string
		ports    []string
		statuses map[string]bool
		count    int
	}

	order := []int32{}
	byPID := make(map[int32]*pidData)

	for _, c := range m.filtered {
		d, ok := byPID[c.PID]
		if !ok {
			d = &pidData{
				process:  c.Process,
				statuses: make(map[string]bool),
			}
			byPID[c.PID] = d
			order = append(order, c.PID)
		}
		d.count++
		d.ports = append(d.ports, fmt.Sprintf("%d", c.LocalPort))
		d.statuses[c.Status] = true
	}

	m.groups = make([]processGroup, 0, len(order))
	for _, pid := range order {
		d := byPID[pid]

		// Deduplicate and truncate ports
		portStr := strings.Join(d.ports, ",")
		if len(portStr) > 30 {
			portStr = portStr[:27] + "..."
		}

		// Collect unique statuses
		ss := make([]string, 0, len(d.statuses))
		for s := range d.statuses {
			ss = append(ss, s)
		}
		sort.Strings(ss)

		m.groups = append(m.groups, processGroup{
			PID:      pid,
			Process:  d.process,
			Count:    d.count,
			Ports:    portStr,
			Statuses: strings.Join(ss, ","),
		})
	}

	// Sort grouped view by connection count descending by default
	sort.SliceStable(m.groups, func(i, j int) bool {
		return m.groups[i].Count > m.groups[j].Count
	})
}

func (m *model) rebuildGroupedTable() {
	rows := make([]table.Row, len(m.groups))
	for i, g := range m.groups {
		rows[i] = table.Row{
			fmt.Sprintf("%d", g.PID),
			g.Process,
			fmt.Sprintf("%d", g.Count),
			g.Ports,
			g.Statuses,
		}
	}
	m.table.SetRows(rows)
	if len(rows) == 0 {
		m.table.SetCursor(0)
	} else if m.table.Cursor() >= len(rows) {
		m.table.SetCursor(len(rows) - 1)
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
			c.Status,
			c.RemoteAddr,
		}
	}
	m.table.SetRows(rows)
	if len(rows) == 0 {
		m.table.SetCursor(0)
	} else if m.table.Cursor() >= len(rows) {
		m.table.SetCursor(len(rows) - 1)
	}
}

func (m model) View() string {
	// Header
	title := "PortScout"
	if m.filterPID > 0 {
		title += fmt.Sprintf("  PID: %d", m.filterPID)
	}
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
		freezeLabel := "[f] Freeze"
		if m.frozen {
			freezeLabel = "[f] Unfreeze"
		}
		groupLabel := "[g] Group"
		if m.grouped {
			groupLabel = "[g] Ungroup"
		}
		footer = footerStyle.Width(m.width).Render("[q] Quit  [k] Kill  [/] Search  [s] Sort  [S] Reverse  " + freezeLabel + "  " + groupLabel + "  [esc] Clear")
	}

	if m.showDetail && m.detail != nil {
		return m.renderDetail()
	}

	return lipgloss.JoinVertical(lipgloss.Left, header, tableView, footer)
}

func (m model) renderDetail() string {
	d := m.detail

	row := func(label, value string) string {
		return detailLabelStyle.Render(fmt.Sprintf("%-14s", label)) + " " + detailValueStyle.Render(value)
	}

	cmdline := d.Cmdline
	maxCmd := m.width - 24
	if maxCmd < 20 {
		maxCmd = 20
	}
	runes := []rune(cmdline)
	if len(runes) > maxCmd {
		cmdline = string(runes[:maxCmd-3]) + "..."
	}

	createTime := "N/A"
	if d.CreateTime > 0 {
		t := time.UnixMilli(d.CreateTime)
		createTime = t.Format("2006-01-02 15:04:05")
	}

	lines := []string{
		detailLabelStyle.Render("Process Detail"),
		"",
		row("PID:", fmt.Sprintf("%d", d.PID)),
		row("Name:", d.Name),
		row("User:", d.User),
		row("Status:", d.Status),
		row("CPU %:", fmt.Sprintf("%.1f%%", d.CPUPercent)),
		row("Memory %:", fmt.Sprintf("%.1f%%", d.MemPercent)),
		row("File Desc.:", fmt.Sprintf("%d", d.NumFDs)),
		row("Started:", createTime),
		row("Working Dir:", d.Cwd),
		row("Command:", cmdline),
		"",
		footerStyle.Render("Press any key to close"),
	}

	content := strings.Join(lines, "\n")
	box := detailBoxStyle.Width(m.width - 6).Render(content)

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}
