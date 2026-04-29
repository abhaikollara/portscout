# PortScout

A terminal UI tool for monitoring network connections and managing the processes behind them. Built with [Bubble Tea](https://github.com/charmbracelet/bubbletea).

## Install

### Homebrew (macOS, Apple Silicon)

```
brew tap abhaikollara/tap
brew install portscout
```

### From source

```
go install github.com/abhaikollara/portscout@latest
```

### Manual

```
git clone https://github.com/abhaikollara/portscout.git
cd portscout
make install
```

## Usage

```
portscout            # Launch the interactive TUI
portscout 8080       # Show process details for port 8080
portscout -k 8080    # Kill the process on port 8080
portscout --version  # Print version
```

> **Note:** Some connections require elevated permissions to resolve process names. Run with `sudo` if you see `unknown` processes.

## Keybindings

| Key | Action |
|-----|--------|
| `q` / `Ctrl+C` | Quit |
| `/` | Start filtering by port, process name, protocol, or remote address |
| `Enter` | Filter mode: confirm filter. Normal mode: view process details. Group mode: drill down into process |
| `Esc` | Clear filter / exit filter mode |
| `k` | Kill selected process (with confirmation) |
| `s` | Cycle sort column (Port, Process, PID, Proto, Status, Remote) |
| `S` | Reverse sort direction |
| `f` | Freeze / unfreeze auto-refresh |
| `g` | Toggle group-by-PID view |
| `Up` / `Down` | Navigate rows |

## Features

**Live connection table** -- Shows port, process name, PID, protocol (TCP/UDP), status, and remote address. Auto-refreshes every second.

**Search and filter** -- Press `/` and type to filter by any field. Partial matches work -- typing `node` shows all Node.js connections, typing `8080` shows anything on that port.

**Process detail view** -- Press `Enter` on any row to see full process info: command line, user, CPU/memory usage, file descriptors, working directory, and start time.

**Group by PID** -- Press `g` to collapse connections by process. Shows connection count per process, sorted by most connections. Press `Enter` on a group to drill down into that process's individual connections.

**Kill processes** -- Press `k` on any row, confirm with `y`. Works in both normal and grouped views.

**Sort** -- Press `s` to cycle the sort column, `S` to reverse direction. Active sort column shows a `▲`/`▼` indicator.

**Freeze** -- Press `f` to pause auto-refresh. Useful for inspecting a snapshot without the table updating under you.

## Tech Stack

- [bubbletea](https://github.com/charmbracelet/bubbletea) -- TUI framework
- [lipgloss](https://github.com/charmbracelet/lipgloss) -- Styling
- [bubbles](https://github.com/charmbracelet/bubbles) -- Table component
- [gopsutil](https://github.com/shirou/gopsutil) -- Cross-platform process and network data

## License

MIT
