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

	// Panel 2 — Downloads table (right top) — read-only display
	downloadsTable *tview.Table

	// Panel 3 — Cancel (right bottom)
	activeList *tview.List
	cancelBtn  *tview.Button

	// Pre-built focus groups for Tab cycling within each panel
	group1     []tview.Primitive  // input panel focusables
	group2     []tview.Primitive  // table (single item)
	group3     []tview.Primitive  // cancel panel focusables
	focusedIdx int                // index within current group
	currentGrp *[]tview.Primitive // pointer to current focus group

	// Store download info
	mu       sync.RWMutex
	items    map[int]*DownloadItem
	itemsIdx []int

	startOnce sync.Once
}

// DownloadItem holds the runtime state of a download for UI rendering.
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
		inputForm:      tview.NewForm(),
		downloadsTable: tview.NewTable().SetBorders(true),
		activeList:     tview.NewList().ShowSecondaryText(false),
		cancelBtn:      tview.NewButton("Cancel Selected"),
	}

	t.setupUI()
	t.setupHandlers()

	return t
}

func extractFilename(filePath string) string {
	parts := strings.Split(filePath, "/")
	return parts[len(parts)-1]
}

// setupUI builds the layout with three numbered panels.
func (t *TUI) setupUI() {
	// ---------- Panel 1 (Input) — left side ----------
	t.inputForm.SetBorder(true).SetTitle(`[Alt+1] Input`)
	t.inputForm.SetButtonBackgroundColor(tcell.ColorDarkBlue)
	t.inputForm.SetButtonTextColor(tcell.ColorWhite)
	t.inputForm.SetFieldBackgroundColor(tcell.ColorDarkBlue)
	t.inputForm.SetFieldTextColor(tcell.ColorWhite)
	t.inputForm.SetLabelColor(tcell.ColorWhite)
	t.inputForm.AddInputField("URL:", "", 0, nil, nil)
	t.inputForm.AddInputField("Path:", "", 0, nil, nil)
	t.inputForm.AddButton("Download", t.startDownload)

	// Focus group for panel 1: URL field (idx 0), Path field (idx 1), Download button (idx 2)
	urlField := t.inputForm.GetFormItem(0)
	pathField := t.inputForm.GetFormItem(1)
	// The button is always the last item; we'll find it by iterating form items
	// Actually Form stores buttons separately. Let's just use the form itself —
	// but for our manual cycling we need individual primitives.
	// Since Form's GetFormItem returns InputFields and GetButton returns buttons:
	downloadBtn := t.inputForm.GetButton(0)
	t.group1 = []tview.Primitive{urlField, pathField, downloadBtn}

	// ---------- Panel 2 (Downloads table) — right top ----------
	t.downloadsTable.SetTitle("[Alt+2] Downloads").SetBorder(true)
	t.downloadsTable.SetSelectable(false, false)
	t.downloadsTable.SetCell(0, 0, tview.NewTableCell("ID").SetTextColor(tcell.ColorYellow).SetSelectable(false))
	t.downloadsTable.SetCell(0, 1, tview.NewTableCell("URL").SetTextColor(tcell.ColorYellow).SetSelectable(false))
	t.downloadsTable.SetCell(0, 2, tview.NewTableCell("Path").SetTextColor(tcell.ColorYellow).SetSelectable(false))
	t.downloadsTable.SetCell(0, 3, tview.NewTableCell("Progress").SetTextColor(tcell.ColorYellow).SetSelectable(false))
	t.downloadsTable.SetCell(0, 4, tview.NewTableCell("Status").SetTextColor(tcell.ColorYellow).SetSelectable(false))

	// Focus group for panel 2: just the table (read-only, focus for visual indication)
	t.group2 = []tview.Primitive{t.downloadsTable}

	// ---------- Panel 3 (Cancel) — right bottom ----------
	activeInner := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(t.activeList, 0, 1, true).
		AddItem(t.cancelBtn, 1, 0, false)
	activeInner.SetBorder(true).SetTitle("[Alt+3] Cancel")
	t.activeList.SetBackgroundColor(tcell.ColorDarkBlue)
	t.activeList.SetMainTextColor(tcell.ColorWhite)
	t.cancelBtn.SetBackgroundColor(tcell.ColorDarkBlue)
	t.cancelBtn.SetLabelColor(tcell.ColorWhite)

	t.group3 = []tview.Primitive{t.activeList, t.cancelBtn}

	// ---------- Right side: stack panel 2 on top of panel 3 ----------
	rightSide := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(t.downloadsTable, 0, 7, false).
		AddItem(activeInner, 0, 3, false)

	// ---------- Root: panel 1 left, rightSide on right ----------
	root := tview.NewFlex().
		AddItem(t.inputForm, 0, 3, true).
		AddItem(rightSide, 0, 7, false)

	// Set overall background
	root.SetBackgroundColor(tcell.ColorDarkBlue)
	t.downloadsTable.SetBackgroundColor(tcell.ColorDarkBlue)
	t.downloadsTable.SetSelectedStyle(tcell.StyleDefault.Background(tcell.ColorNavy))
	t.app.SetRoot(root, true)

	// Default focus: panel 1, first item
	t.currentGrp = &t.group1
	t.focusedIdx = 0
}

// setupHandlers wires buttons, list selection, and number-key navigation.
func (t *TUI) setupHandlers() {
	t.cancelBtn.SetSelectedFunc(func() {
		t.cancelSelected()
	})

	downloadBtn := t.inputForm.GetButton(0)
	downloadBtn.SetSelectedFunc(func() {
		t.startDownload()
	})

	// Enter on a list item also cancels that download
	t.activeList.SetSelectedFunc(func(idx int, mainText, secondaryText string, shortcut rune) {
		t.cancelSelected()
	})

	// Global input capture
	t.app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
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
			// Only intercept Alt+number combinations for panel switching.
			// Bare number keys are NOT intercepted so typing into input fields works.
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

// switchGroup changes focus to a different panel's focus group.
func (t *TUI) switchGroup(grp *[]tview.Primitive) {
	t.currentGrp = grp
	t.focusedIdx = 0
	if len(*grp) > 0 {
		t.app.SetFocus((*grp)[0])
	}
}

// focusWithinGroup moves focus forward (+1) or backward (-1) within the current group.
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

// startDownload reads the inputs and starts a new download.
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
		ID:       id,
		URL:      url,
		FilePath: filePath,
		Filename: extractFilename(filePath),
		Progress: 0,
		Status:   "queued",
	}
	t.itemsIdx = append(t.itemsIdx, id)
	t.mu.Unlock()

	t.thdl.AddDownloader(url, filePath)
	t.updateActiveList()

	urlField.SetText("")
	pathField.SetText("")

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
			t.activeList.AddItem(mainText, "", 0, nil)
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

// updateFromChannel receives progress updates and refreshes the UI on the main goroutine.
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

// callback is the ProgressCallback passed to the ThreadDownloader.
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
