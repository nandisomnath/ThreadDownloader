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

	// Panel 1 — Input (left)
	inputForm *tview.Form

	// Panel 2 — Downloads table (right top)
	downloadsTable *tview.Table

	// Panel 3 — Cancel (right bottom) — List with progress items
	activeList *tview.List

	// Focus groups for Tab cycling within each panel
	group1     []tview.Primitive
	group2     []tview.Primitive
	group3     []tview.Primitive
	focusedIdx int
	currentGrp *[]tview.Primitive

	// Download state
	mu        sync.RWMutex
	items     map[int]*DownloadItem
	itemsIdx  []int
	cancelStg map[int]int // 0=green, 1=orange, 2=red (cancelled)

	// Track ESC press for Alt+key detection (some terminals send ESC then key)
	altEsc bool
}

type DownloadItem struct {
	ID       int
	URL      string
	FilePath string
	Progress float64
	Status   string
	Filename string
}

func NewTUI(thdl *thd.ThreadDownloader) *TUI {
	t := &TUI{
		app:            tview.NewApplication(),
		thdl:           thdl,
		updatesCh:      make(chan ProgressUpdate, 100),
		items:          make(map[int]*DownloadItem),
		itemsIdx:       make([]int, 0),
		cancelStg:      make(map[int]int),
		inputForm:      tview.NewForm(),
		downloadsTable: tview.NewTable().SetBorders(true),
		activeList:     tview.NewList().ShowSecondaryText(false),
		altEsc:         false,
	}
	t.setupUI()
	t.setupHandlers()
	return t
}

func extractFilename(filePath string) string {
	parts := strings.Split(filePath, "/")
	return parts[len(parts)-1]
}

func (t *TUI) setupUI() {
	// Panel 1 (Input)
	t.inputForm.SetBorder(true).SetTitle(`[Alt+1] Input`)
	t.inputForm.SetButtonBackgroundColor(tcell.ColorDarkBlue)
	t.inputForm.SetButtonTextColor(tcell.ColorWhite)
	t.inputForm.SetFieldBackgroundColor(tcell.ColorDarkBlue)
	t.inputForm.SetFieldTextColor(tcell.ColorWhite)
	t.inputForm.SetLabelColor(tcell.ColorWhite)
	t.inputForm.AddInputField("URL:", "", 0, nil, nil)
	t.inputForm.AddInputField("Path:", "", 0, nil, nil)
	t.inputForm.AddButton("Download", t.startDownload)
	urlField := t.inputForm.GetFormItem(0)
	pathField := t.inputForm.GetFormItem(1)
	downloadBtn := t.inputForm.GetButton(0)
	t.group1 = []tview.Primitive{urlField, pathField, downloadBtn}

	// Panel 2 (Downloads table)
	t.downloadsTable.SetTitle("[Alt+2] Downloads").SetBorder(true)
	t.downloadsTable.SetSelectable(false, false)
	t.downloadsTable.SetCell(0, 0, tview.NewTableCell("ID").SetTextColor(tcell.ColorYellow).SetSelectable(false))
	t.downloadsTable.SetCell(0, 1, tview.NewTableCell("URL").SetTextColor(tcell.ColorYellow).SetSelectable(false))
	t.downloadsTable.SetCell(0, 2, tview.NewTableCell("Path").SetTextColor(tcell.ColorYellow).SetSelectable(false))
	t.downloadsTable.SetCell(0, 3, tview.NewTableCell("Progress").SetTextColor(tcell.ColorYellow).SetSelectable(false))
	t.downloadsTable.SetCell(0, 4, tview.NewTableCell("Status").SetTextColor(tcell.ColorYellow).SetSelectable(false))
	t.group2 = []tview.Primitive{t.downloadsTable}

	// Panel 3 (Cancel) — using List for proper focus handling
	activeInner := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(t.activeList, 0, 1, true)
	activeInner.SetBorder(true).SetTitle("[Alt+3] Cancel")
	t.activeList.SetBackgroundColor(tcell.ColorBlack)
	t.activeList.SetMainTextColor(tcell.ColorWhite)
	t.activeList.SetSelectedBackgroundColor(tcell.ColorDarkBlue)
	t.group3 = []tview.Primitive{t.activeList}

	// Layout
	rightSide := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(t.downloadsTable, 0, 7, false).
		AddItem(activeInner, 0, 3, false)
	root := tview.NewFlex().
		AddItem(t.inputForm, 0, 3, true).
		AddItem(rightSide, 0, 7, false)
	t.app.SetRoot(root, true)
	t.currentGrp = &t.group1
	t.focusedIdx = 0
}

func (t *TUI) setupHandlers() {
	t.inputForm.GetButton(0).SetSelectedFunc(func() { t.startDownload() })

	// Double-Enter for cancellation in panel 3:
	// 1st Enter → mark pending (orange)
	// 2nd Enter → cancel (red)
	t.activeList.SetSelectedFunc(func(idx int, mainText, secondaryText string, shortcut rune) {
		t.mu.Lock()
		if idx < 0 || idx >= len(t.itemsIdx) {
			t.mu.Unlock()
			return
		}
		id := t.itemsIdx[idx]

		switch t.cancelStg[id] {
		case 0:
			// First Enter: mark pending (orange)
			t.cancelStg[id] = 1
			t.mu.Unlock()
			t.updateActiveList()
		case 1:
			// Second Enter: cancel immediately (red)
			t.cancelStg[id] = 2
			t.mu.Unlock()
			t.thdl.CancelDownload(id)
			t.updateActiveList()
		default:
			t.mu.Unlock()
		}
	})

	t.app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		// Handle ESC key (used as Alt prefix in some terminals)
		if event.Key() == tcell.KeyEsc {
			t.altEsc = true
			return nil
		}

		// If previous key was ESC, treat this rune as Alt+key
		if t.altEsc {
			t.altEsc = false
			switch event.Rune() {
			case '1':
				t.switchGroup(&t.group1)
				return nil
			case '2':
				t.switchGroup(&t.group2)
				return nil
			case '3':
				t.switchGroup(&t.group3)
				return nil
			}
		}

		switch event.Key() {
		case tcell.KeyCtrlC:
			t.app.Stop()
			return nil
		case tcell.KeyTab:
			t.focusWithinGroup(1)
			return nil
		case tcell.KeyBacktab:
			t.focusWithinGroup(-1)
			return nil
		case tcell.KeyRune:
			if event.Modifiers()&tcell.ModAlt != 0 {
				switch event.Rune() {
				case '1':
					t.switchGroup(&t.group1)
					return nil
				case '2':
					t.switchGroup(&t.group2)
					return nil
				case '3':
					t.switchGroup(&t.group3)
					return nil
				}
			}
		}
		return event
	})
}

func (t *TUI) switchGroup(grp *[]tview.Primitive) {
	t.currentGrp = grp
	t.focusedIdx = 0
	if len(*grp) > 0 {
		t.app.SetFocus((*grp)[0])
	}
}

func (t *TUI) focusWithinGroup(dir int) {
	if t.currentGrp == nil || len(*t.currentGrp) == 0 {
		return
	}
	grp := *t.currentGrp
	t.focusedIdx = (t.focusedIdx + dir) % len(grp)
	if t.focusedIdx < 0 {
		t.focusedIdx += len(grp)
	}
	t.app.SetFocus(grp[t.focusedIdx])
}

func (t *TUI) startDownload() {
	urlField := t.inputForm.GetFormItem(0).(*tview.InputField)
	pathField := t.inputForm.GetFormItem(1).(*tview.InputField)
	url := strings.TrimSpace(urlField.GetText())
	filePath := strings.TrimSpace(pathField.GetText())
	if url == "" || filePath == "" {
		return
	}

	t.mu.Lock()
	id := len(t.itemsIdx)
	t.items[id] = &DownloadItem{
		ID: id, URL: url, FilePath: filePath,
		Filename: extractFilename(filePath), Progress: 0, Status: "queued",
	}
	t.itemsIdx = append(t.itemsIdx, id)
	t.cancelStg[id] = 0
	t.mu.Unlock()

	t.thdl.AddDownloader(url, filePath)
	t.updateActiveList()
	urlField.SetText("")
	pathField.SetText("")
}

func (t *TUI) updateActiveList() {
	t.activeList.Clear()

	t.mu.RLock()
	defer t.mu.RUnlock()

	for _, id := range t.itemsIdx {
		item := t.items[id]
		progressStr := fmt.Sprintf("%.0f%%", item.Progress)

		var colorTag string
		switch t.cancelStg[id] {
		case 0:
			colorTag = "[green::]" // initial
		case 1:
			colorTag = "[orange::]" // pending cancel
		case 2:
			colorTag = "[red::]" // cancelled
		default:
			colorTag = "[white::]"
		}

		mainText := fmt.Sprintf("%s%s  [%s]", colorTag, item.Filename, progressStr)
		t.activeList.AddItem(mainText, "", 0, nil)
	}
}

func (t *TUI) updateTable() {
	t.mu.RLock()
	defer t.mu.RUnlock()

	row := 1
	for _, id := range t.itemsIdx {
		item := t.items[id]
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

func (t *TUI) updateFromChannel() {
	for update := range t.updatesCh {
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

func (t *TUI) callback(id int, url, filePath string, progress float64, status string) {
	t.updatesCh <- ProgressUpdate{ID: id, URL: url, FilePath: filePath, Progress: progress, Status: status}
}

func (t *TUI) Run() error {
	go t.updateFromChannel()
	return t.app.Run()
}
