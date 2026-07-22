package main

import (
	"fmt"
	"strings"
	"sync"

	"github.com/gdamore/tcell/v2"
	"github.com/nandisomnath/thd/thd"
	"github.com/rivo/tview"
)

// ProgressUpdate carries download progress info from the download goroutine to the TUI.
type ProgressUpdate struct {
	ID       int
	URL      string
	FilePath string
	Progress float64 // 0.0 - 100.0
	Status   string  // "downloading", "completed", "cancelled", "error"
}

// TUI encapsulates the terminal UI application.
type TUI struct {
	app       *tview.Application
	thdl      *thd.ThreadDownloader
	updatesCh chan ProgressUpdate

	// Left panel — form
	inputForm *tview.Form

	// Right top panel — table of all downloads
	downloadsTable *tview.Table

	// Right bottom panel — active download filenames list
	activeList *tview.List
	cancelBtn  *tview.Button

	// Store download info for display updates
	mu       sync.RWMutex
	items    map[int]*DownloadItem
	itemsIdx []int // ordered list of IDs for table rendering

	// Flag to prevent starting downloads multiple times
	startOnce sync.Once
}

// DownloadItem holds the runtime state of a download for UI rendering.
type DownloadItem struct {
	ID       int
	URL      string
	FilePath string
	Progress float64
	Status   string
	Filename string // extracted from filePath
}

func NewTUI(thdl *thd.ThreadDownloader) *TUI {
	t := &TUI{
		app:            tview.NewApplication(),
		thdl:           thdl,
		updatesCh:      make(chan ProgressUpdate, 100),
		items:          make(map[int]*DownloadItem),
		itemsIdx:       make([]int, 0),
		inputForm:      tview.NewForm(),
		downloadsTable: tview.NewTable().SetBorders(true),
		activeList:     tview.NewList(),
		cancelBtn:      tview.NewButton("Cancel Selected"),
	}

	t.setupUI()
	t.setupHandlers()

	return t
}

// extractFilename returns the last component of a file path.
func extractFilename(filePath string) string {
	parts := strings.Split(filePath, "/")
	return parts[len(parts)-1]
}

// setupUI builds the layout and configures widget options.
func (t *TUI) setupUI() {
	// ---------- Left panel: input form ----------
	t.inputForm.SetBorder(true).SetTitle(" Input ")
	t.inputForm.AddInputField("URL:", "", 0, nil, nil)
	t.inputForm.AddInputField("Path:", "", 0, nil, nil)
	t.inputForm.AddButton("Download", t.startDownload)

	// ---------- Right top panel: downloads table ----------
	t.downloadsTable.SetTitle(" Downloads ").SetBorder(true)
	t.downloadsTable.SetSelectable(false, false)
	// Set column headers
	t.downloadsTable.SetCell(0, 0, tview.NewTableCell("ID").SetTextColor(tcell.ColorYellow).SetSelectable(false))
	t.downloadsTable.SetCell(0, 1, tview.NewTableCell("URL").SetTextColor(tcell.ColorYellow).SetSelectable(false))
	t.downloadsTable.SetCell(0, 2, tview.NewTableCell("Path").SetTextColor(tcell.ColorYellow).SetSelectable(false))
	t.downloadsTable.SetCell(0, 3, tview.NewTableCell("Progress").SetTextColor(tcell.ColorYellow).SetSelectable(false))
	t.downloadsTable.SetCell(0, 4, tview.NewTableCell("Status").SetTextColor(tcell.ColorYellow).SetSelectable(false))

	// ---------- Right bottom panel: active list ----------
	activeFlex := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(tview.NewTextView().SetText("[::b]Active Downloads[::-]").SetTextAlign(tview.AlignCenter), 1, 0, false).
		AddItem(t.activeList, 0, 1, true).
		AddItem(t.cancelBtn, 1, 0, false)
	activeFlex.SetBorder(true).SetTitle(" Cancel ")

	// ---------- Right panel ----------
	rightPanel := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(t.downloadsTable, 0, 7, false).
		AddItem(activeFlex, 0, 3, false)

	// ---------- Root layout ----------
	root := tview.NewFlex().
		AddItem(t.inputForm, 0, 3, true).
		AddItem(rightPanel, 0, 7, false)

	t.app.SetRoot(root, true)
}

// setupHandlers wires buttons and keyboard shortcuts.
func (t *TUI) setupHandlers() {
	t.cancelBtn.SetSelectedFunc(func() {
		t.cancelSelected()
	})

	// Global quit shortcut
	t.app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyCtrlC {
			t.app.Stop()
			return nil
		}
		return event
	})
}

// startDownload reads the inputs and starts a new download.
func (t *TUI) startDownload() {
	// Get form fields by index (0 = URL, 1 = Path)
	urlField := t.inputForm.GetFormItem(0).(*tview.InputField)
	pathField := t.inputForm.GetFormItem(1).(*tview.InputField)

	url := strings.TrimSpace(urlField.GetText())
	filePath := strings.TrimSpace(pathField.GetText())

	if url == "" || filePath == "" {
		return
	}

	// Create a local copy of IDs to track before adding
	t.mu.Lock()
	id := len(t.itemsIdx) // next ID
	t.items[id] = &DownloadItem{
		ID:       id,
		URL:      url,
		FilePath: filePath,
		Filename: extractFilename(filePath),
		Progress: 0,
		Status:   "queued",
	}
	t.itemsIdx = append(t.itemsIdx, id)
	t.mu.Unlock()

	// Add to download manager
	t.thdl.AddDownloader(url, filePath)

	// Update the active list
	t.updateActiveList()

	// Clear inputs
	urlField.SetText("")
	pathField.SetText("")

	// Start downloads only once (subsequent AddDownloader calls add to the queue)
	t.startOnce.Do(func() {
		go t.thdl.Start()
	})
}

// cancelSelected cancels the download selected in the active list.
func (t *TUI) cancelSelected() {
	if t.activeList.GetItemCount() == 0 {
		return
	}
	idx := t.activeList.GetCurrentItem()
	t.mu.RLock()
	if idx < 0 || idx >= len(t.itemsIdx) {
		t.mu.RUnlock()
		return
	}
	id := t.itemsIdx[idx]
	t.mu.RUnlock()

	t.thdl.CancelDownload(id)
}

// updateActiveList refreshes the active-list with filenames of non-terminal downloads.
func (t *TUI) updateActiveList() {
	t.activeList.Clear()

	t.mu.RLock()
	defer t.mu.RUnlock()

	for _, id := range t.itemsIdx {
		item := t.items[id]
		if item.Status == "downloading" || item.Status == "queued" {
			progressStr := fmt.Sprintf("%.0f%%", item.Progress)
			mainText := fmt.Sprintf("%s  [%s]", item.Filename, progressStr)
			t.activeList.AddItem(mainText, item.URL, 0, nil)
		}
	}
}

// updateTable refreshes the downloads table with all known items.
func (t *TUI) updateTable() {
	t.mu.RLock()
	defer t.mu.RUnlock()

	row := 1
	for _, id := range t.itemsIdx {
		item := t.items[id]

		// Progress bar
		bar := renderProgressBar(item.Progress, 20)

		statusColor := tcell.ColorWhite
		switch item.Status {
		case "completed":
			statusColor = tcell.ColorGreen
		case "error", "cancelled":
			statusColor = tcell.ColorRed
		case "downloading":
			statusColor = tcell.ColorAqua
		}

		t.downloadsTable.SetCell(row, 0, tview.NewTableCell(fmt.Sprintf("%d", item.ID)).SetAlign(tview.AlignRight))
		t.downloadsTable.SetCell(row, 1, tview.NewTableCell(truncate(item.URL, 35)).SetAlign(tview.AlignLeft))
		t.downloadsTable.SetCell(row, 2, tview.NewTableCell(truncate(item.FilePath, 25)).SetAlign(tview.AlignLeft))
		t.downloadsTable.SetCell(row, 3, tview.NewTableCell(bar).SetAlign(tview.AlignCenter))
		t.downloadsTable.SetCell(row, 4, tview.NewTableCell(item.Status).SetTextColor(statusColor).SetAlign(tview.AlignCenter))
		row++
	}
}

// renderProgressBar returns a visual bar string like "[████████░░░░] 65%".
func renderProgressBar(progress float64, width int) string {
	filled := int(progress * float64(width) / 100.0)
	if filled > width {
		filled = width
	}
	if filled < 0 {
		filled = 0
	}
	bar := strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
	return fmt.Sprintf("[%s] %.0f%%", bar, progress)
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// updateFromChannel receives progress updates and refreshes the UI on the main goroutine.
func (t *TUI) updateFromChannel() {
	for update := range t.updatesCh {
		// Make a local copy to avoid race in the closure
		up := update

		t.app.QueueUpdateDraw(func() {
			t.mu.Lock()
			if item, ok := t.items[up.ID]; ok {
				item.Progress = up.Progress
				item.Status = up.Status
			}
			t.mu.Unlock()

			t.updateTable()
			t.updateActiveList()
		})
	}
}

// callback is the ProgressCallback passed to the ThreadDownloader.
// It sends updates to the channel (goroutine-safe).
func (t *TUI) callback(id int, url, filePath string, progress float64, status string) {
	t.updatesCh <- ProgressUpdate{
		ID:       id,
		URL:      url,
		FilePath: filePath,
		Progress: progress,
		Status:   status,
	}
}

// Run starts the TUI application.
func (t *TUI) Run() error {
	go t.updateFromChannel()
	return t.app.Run()
}
