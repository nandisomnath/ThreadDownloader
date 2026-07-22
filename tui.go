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

	// Panel 2 — Downloads table (right)
	downloadsTable *tview.Table

	// Status bar at the bottom
	statusBar *tview.Flex
	statusL   *tview.TextView // left: selected download info
	statusR   *tview.TextView // right: "Hit Enter to cancel this"

	// Focus groups for Tab cycling
	group1     []tview.Primitive
	group2     []tview.Primitive
	focusedIdx int
	currentGrp *[]tview.Primitive

	// Download state
	mu         sync.RWMutex
	items      map[int]*DownloadItem
	itemsIdx   []int
	selectedID int // ID of the currently selected download in panel 2, -1 = none

	// Track ESC press for Alt+key detection
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
		selectedID:     -1,
		inputForm:      tview.NewForm(),
		downloadsTable: tview.NewTable().SetBorders(true),
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
	// Panel 1 (Input) — left
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

	// Panel 2 (Downloads table) — right, NOT selectable (no visual row change)
	t.downloadsTable.SetTitle("[Alt+2] Downloads").SetBorder(true)
	t.downloadsTable.SetSelectable(false, false)
	t.downloadsTable.SetCell(0, 0, tview.NewTableCell("ID").SetTextColor(tcell.ColorYellow).SetSelectable(false))
	t.downloadsTable.SetCell(0, 1, tview.NewTableCell("URL").SetTextColor(tcell.ColorYellow).SetSelectable(false))
	t.downloadsTable.SetCell(0, 2, tview.NewTableCell("Path").SetTextColor(tcell.ColorYellow).SetSelectable(false))
	t.downloadsTable.SetCell(0, 3, tview.NewTableCell("Progress").SetTextColor(tcell.ColorYellow).SetSelectable(false))
	t.downloadsTable.SetCell(0, 4, tview.NewTableCell("Status").SetTextColor(tcell.ColorYellow).SetSelectable(false))
	t.group2 = []tview.Primitive{t.downloadsTable}

	// Status bar at the bottom
	t.statusL = tview.NewTextView()
	t.statusL.SetDynamicColors(true)
	t.statusL.SetTextAlign(tview.AlignLeft)
	t.statusL.SetBackgroundColor(tcell.ColorDarkBlue)
	t.statusL.SetTextColor(tcell.ColorWhite)
	t.statusL.SetText("No download selected")

	t.statusR = tview.NewTextView()
	t.statusR.SetDynamicColors(true)
	t.statusR.SetTextAlign(tview.AlignRight)
	t.statusR.SetBackgroundColor(tcell.ColorDarkBlue)
	t.statusR.SetTextColor(tcell.ColorYellow)
	t.statusR.SetText("")

	t.statusBar = tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(t.statusL, 0, 3, false).
		AddItem(t.statusR, 0, 2, false)
	t.statusBar.SetBackgroundColor(tcell.ColorDarkBlue)

	// Layout: input left, (table + status) right stacked vertically
	rightSide := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(t.downloadsTable, 0, 7, false).
		AddItem(t.statusBar, 1, 0, false)
	root := tview.NewFlex().
		AddItem(t.inputForm, 0, 3, true).
		AddItem(rightSide, 0, 7, false)
	t.app.SetRoot(root, true)
	t.currentGrp = &t.group1
	t.focusedIdx = 0
}

func (t *TUI) setupHandlers() {
	t.inputForm.GetButton(0).SetSelectedFunc(func() { t.startDownload() })

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
			}
		}

		switch event.Key() {
		case tcell.KeyCtrlC:
			t.app.Stop()
			return nil
		case tcell.KeyTab:
			// In panel 2, Tab cycles through downloads internally (no visual table change)
			if t.currentGrp == &t.group2 && len(t.itemsIdx) > 0 {
				t.mu.Lock()
				t.focusedIdx = (t.focusedIdx + 1) % len(t.itemsIdx)
				t.selectedID = t.itemsIdx[t.focusedIdx]
				t.mu.Unlock()
				t.updateStatusBar()
				return nil
			}
			t.focusWithinGroup(1)
			return nil
		case tcell.KeyBacktab:
			// In panel 2, Backtab cycles backwards through downloads internally
			if t.currentGrp == &t.group2 && len(t.itemsIdx) > 0 {
				t.mu.Lock()
				t.focusedIdx = (t.focusedIdx - 1 + len(t.itemsIdx)) % len(t.itemsIdx)
				t.selectedID = t.itemsIdx[t.focusedIdx]
				t.mu.Unlock()
				t.updateStatusBar()
				return nil
			}
			t.focusWithinGroup(-1)
			return nil
		case tcell.KeyEnter:
			// In panel 2, Enter cancels the selected download
			if t.currentGrp == &t.group2 && t.selectedID >= 0 {
				t.thdl.CancelDownload(t.selectedID)
				t.mu.Lock()
				if item, ok := t.items[t.selectedID]; ok {
					item.Status = "cancelled"
				}
				t.mu.Unlock()
				t.updateTable()
				t.updateStatusBar()
				return nil
			}
		case tcell.KeyRune:
			if event.Modifiers()&tcell.ModAlt != 0 {
				switch event.Rune() {
				case '1':
					t.switchGroup(&t.group1)
					return nil
				case '2':
					t.switchGroup(&t.group2)
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
	t.mu.Unlock()

	t.thdl.AddDownloader(url, filePath)
	t.updateTable()
	t.updateStatusBar()
	urlField.SetText("")
	pathField.SetText("")
}

func (t *TUI) updateStatusBar() {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if t.selectedID >= 0 {
		if item, ok := t.items[t.selectedID]; ok {
			leftText := fmt.Sprintf(" [yellow]#%d[white] [green]%s[white]  [%.0f%%]  [%s]", item.ID, item.Filename, item.Progress, item.Status)
			t.statusL.SetText(leftText)
			t.statusR.SetText("[yellow]Hit Enter to cancel this  ")
			return
		}
	}
	t.statusL.SetText("No download selected")
	t.statusR.SetText("")
}

func (t *TUI) updateTable() {
	t.mu.RLock()
	defer t.mu.RUnlock()

	// Clear existing data rows (keep header)
	for row := t.downloadsTable.GetRowCount() - 1; row >= 1; row-- {
		t.downloadsTable.RemoveRow(row)
	}

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
			t.updateStatusBar()
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
