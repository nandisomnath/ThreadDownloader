# ThreadDownloader

A concurrent file download manager with a terminal user interface (TUI) built in Go.

ThreadDownloader allows you to download multiple files simultaneously, track real-time progress, and manage downloads — all from your terminal.

## Features

- **Concurrent Downloads** — Download multiple files in parallel using goroutines
- **Real-Time Progress** — Visual progress bars with percentage tracking
- **Interactive TUI** — Two-panel layout with keyboard navigation
- **Download Management** — View, track, and cancel downloads on the fly
- **Error Handling** — Graceful error reporting per download
- **Cancellation Support** — Cancel any active download with a single keystroke

## Keyboard Shortcuts

| Key | Action |
|-----|--------|
| `Alt+1` | Focus input panel (URL & Path fields) |
| `Alt+2` | Focus downloads table panel |
| `Tab` / `Shift+Tab` | Cycle through focused items |
| `Enter` | On downloads panel: cancel the selected download |
| `Ctrl+C` | Quit the application |

## Installation

### Prerequisites

- Go 1.26.5 or later

### From Source

```bash
git clone https://github.com/nandisomnath/thd.git
cd thd
go build -o thd .
./thd
```

### Using `go install`

```bash
go install github.com/nandisomnath/thd@latest
```

## Usage

1. Launch the application:
   
```bash
   ./thd
   
```

2. In the **Input Panel** (left), enter:
   - **URL** — The direct download link
   - **Path** — The local file path to save the download (e.g., `./downloads/file.zip`)

3. Click the **Download** button or press `Tab` to navigate to it and press `Enter`.

4. Switch to the **Downloads Panel** (right) using `Alt+2` to monitor progress.

5. Select a download using `Tab` / `Shift+Tab` and press `Enter` to cancel it.

### Layout

```
┌───────────────────┬──────────────────────────────────────────┐
│  [Alt+1] Input    │  [Alt+2] Downloads                       │
│                   │                                          │
│  URL: __________  │  ID │ URL                │ Path │ Prog │
│  Path: ________   │  ─────────────────────────────────────── │
│  [Download]       │  0  │ example.com/file  │ ./d/  │ ██░ 80%│
│                   │  1  │ example.com/img   │ ./d/  │ █░░ 30%│
│                   │                                          │
├───────────────────┴──────────────────────────────────────────┤
│ #0 filename.zip [80%] [downloading]     Hit Enter to cancel  │
└──────────────────────────────────────────────────────────────┘
```

## Dependencies

- [tview](https://github.com/rivo/tview) — Rich terminal UI toolkit
- [tcell](https://github.com/gdamore/tcell/v2) — Terminal cell rendering

## License

This project is licensed under the MIT License — see the [LICENSE](LICENSE) file for details.

Copyright 2026 Somnath Nandi (somnathnandi368@gmail.com)
