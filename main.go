// Command tui is a terminal UI for downloading files, built with tview.
//
// Layout:
//
//	+-------------------+-----------------------------------+
//	|                    |                                   |
//	|  New Download      |  Downloads (url, path, progress)  |
//	|  (URL, path, btn)  |                                   |
//	|                    |                                   |
//	|                    +-----------------------------------+
//	|                    |  Active Downloads (cancel)        |
//	|                    |                                   |
//	+-------------------+-----------------------------------+
package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// Status values for a download.
const (
	StatusQueued      = "queued"
	StatusDownloading = "downloading"
	StatusDone        = "done"
	StatusCancelled   = "cancelled"
	StatusError       = "error"
)

// Download holds everything the UI needs to know about a single transfer.
// Progress/Status are only ever mutated while Manager.mu is held; the
// download goroutine reports changes through Manager.UpdateProgress /
// Manager.SetStatus rather than touching the struct directly from outside
// the lock.
type Download struct {
	ID       string
	URL      string
	FilePath string
	Progress float64 // 0-100; -1 means "unknown total size"
	Status   string
	Err      error

	cancel context.CancelFunc
}

// Manager owns the download list and is the single point through which
// background goroutines push updates into the UI. Any goroutine, no matter
// where it lives, can safely call UpdateProgress/SetStatus/AddDownload at
// any time.
type Manager struct {
	app *tview.Application

	mu        sync.Mutex
	downloads map[string]*Download
	order     []string // preserves insertion order for stable rendering

	logView    *tview.TextView
	cancelList *tview.List

	nextID int

	client *http.Client
}

func NewManager(app *tview.Application, logView *tview.TextView, cancelList *tview.List) *Manager {
	return &Manager{
		app:        app,
		downloads:  make(map[string]*Download),
		logView:    logView,
		cancelList: cancelList,
		client:     &http.Client{},
	}
}

// AddDownload registers a new download and starts it in a background
// goroutine. Safe to call from the UI goroutine (e.g. a button handler).
func (m *Manager) AddDownload(url, path string) {
	m.mu.Lock()
	m.nextID++
	id := fmt.Sprintf("dl-%d", m.nextID)
	d := &Download{
		ID:       id,
		URL:      url,
		FilePath: path,
		Progress: 0,
		Status:   StatusQueued,
	}
	m.downloads[id] = d
	m.order = append(m.order, id)
	m.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	m.mu.Lock()
	d.cancel = cancel
	m.mu.Unlock()

	m.refreshUI()

	go m.runDownload(ctx, id)
}

// CancelDownload cancels an in-progress download by ID. Safe to call from
// the UI goroutine.
func (m *Manager) CancelDownload(id string) {
	m.mu.Lock()
	d, ok := m.downloads[id]
	m.mu.Unlock()
	if !ok || d.cancel == nil {
		return
	}
	d.cancel()
}

// UpdateProgress is the function background goroutines call to report
// progress. It is safe to call from any goroutine at any time; it takes
// the lock, updates state, and schedules a UI redraw via
// QueueUpdateDraw so the render itself always happens on tview's own
// goroutine.
func (m *Manager) UpdateProgress(id string, progress float64) {
	m.mu.Lock()
	if d, ok := m.downloads[id]; ok {
		d.Progress = progress
	}
	m.mu.Unlock()
	m.refreshUI()
}

// SetStatus updates a download's status (and optionally an error) and
// redraws. Safe to call from any goroutine.
func (m *Manager) SetStatus(id, status string, err error) {
	m.mu.Lock()
	if d, ok := m.downloads[id]; ok {
		d.Status = status
		d.Err = err
	}
	m.mu.Unlock()
	m.refreshUI()
}

// refreshUI rebuilds the log view and cancel list from current state.
// QueueUpdateDraw is the tview-blessed way to touch widgets from a
// non-UI goroutine: it queues the closure to run on the application's
// event loop and triggers a redraw afterwards, so this is safe to call
// from anywhere.
func (m *Manager) refreshUI() {
	m.app.QueueUpdateDraw(func() {
		m.mu.Lock()
		defer m.mu.Unlock()

		var b strings.Builder
		for _, id := range m.order {
			d := m.downloads[id]
			b.WriteString(fmt.Sprintf("[yellow]%s[-]\n", d.ID))
			b.WriteString(fmt.Sprintf("  URL:    %s\n", tview.Escape(d.URL)))
			b.WriteString(fmt.Sprintf("  Path:   %s\n", tview.Escape(d.FilePath)))
			b.WriteString(fmt.Sprintf("  Status: %s\n", statusColor(d.Status)))
			b.WriteString("  " + progressBar(d.Progress, 30) + "\n\n")
		}
		if len(m.order) == 0 {
			b.WriteString("[gray]No downloads yet.[-]\n")
		}
		m.logView.SetText(b.String())
		m.logView.ScrollToEnd()

		// Rebuild the cancel list. We only list downloads that are still
		// active; finished/cancelled/errored ones stay in the log above
		// but drop out of the cancel list.
		m.cancelList.Clear()
		for _, id := range m.order {
			d := m.downloads[id]
			if d.Status != StatusQueued && d.Status != StatusDownloading {
				continue
			}
			label := filepath.Base(d.FilePath)
			if label == "" || label == "." {
				label = d.URL
			}
			m.cancelList.AddItem(label, d.ID, 0, nil)
		}
	})
}

// progressBar renders a simple ASCII progress bar. progress < 0 means the
// total size is unknown, so we render an indeterminate marker instead.
func progressBar(progress float64, width int) string {
	if progress < 0 {
		return "[?????????????????????????????] unknown size"
	}
	if progress > 100 {
		progress = 100
	}
	filled := int(progress / 100 * float64(width))
	if filled > width {
		filled = width
	}
	bar := strings.Repeat("=", filled) + strings.Repeat(" ", width-filled)
	return fmt.Sprintf("[%s] %5.1f%%", bar, progress)
}

func statusColor(status string) string {
	switch status {
	case StatusDone:
		return "[green]done[-]"
	case StatusError:
		return "[red]error[-]"
	case StatusCancelled:
		return "[orange]cancelled[-]"
	case StatusDownloading:
		return "[aqua]downloading[-]"
	default:
		return "[white]queued[-]"
	}
}

// progressReader wraps an io.Reader and reports bytes read so callers can
// compute download progress without buffering the whole body in memory.
type progressReader struct {
	r          io.Reader
	total      int64
	read       int64
	onProgress func(read, total int64)
	lastEmit   time.Time
}

func (pr *progressReader) Read(p []byte) (int, error) {
	n, err := pr.r.Read(p)
	if n > 0 {
		pr.read += int64(n)
		// Throttle UI updates: at most ~10/sec per download so a fast
		// local link or huge file doesn't flood QueueUpdateDraw.
		if time.Since(pr.lastEmit) > 100*time.Millisecond {
			pr.lastEmit = time.Now()
			pr.onProgress(pr.read, pr.total)
		}
	}
	if err == io.EOF {
		pr.onProgress(pr.read, pr.total)
	}
	return n, err
}

// runDownload performs the actual HTTP download for id, reporting progress
// and final status back through the Manager. It runs entirely on its own
// goroutine; all communication with the UI goes through Manager's
// exported update functions.
func (m *Manager) runDownload(ctx context.Context, id string) {
	m.mu.Lock()
	d := m.downloads[id]
	m.mu.Unlock()
	if d == nil {
		return
	}

	m.SetStatus(id, StatusDownloading, nil)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, d.URL, nil)
	if err != nil {
		m.SetStatus(id, StatusError, err)
		return
	}

	resp, err := m.client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			m.SetStatus(id, StatusCancelled, nil)
			return
		}
		m.SetStatus(id, StatusError, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		m.SetStatus(id, StatusError, fmt.Errorf("unexpected status: %s", resp.Status))
		return
	}

	if dir := filepath.Dir(d.FilePath); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			m.SetStatus(id, StatusError, err)
			return
		}
	}

	out, err := os.Create(d.FilePath)
	if err != nil {
		m.SetStatus(id, StatusError, err)
		return
	}
	defer out.Close()

	total := resp.ContentLength // -1 if unknown
	pr := &progressReader{
		r:     resp.Body,
		total: total,
		onProgress: func(read, total int64) {
			if total <= 0 {
				m.UpdateProgress(id, -1)
				return
			}
			pct := float64(read) / float64(total) * 100
			m.UpdateProgress(id, pct)
		},
	}

	_, err = io.Copy(out, pr)
	if err != nil {
		// A cancelled context surfaces here as a read error; treat that
		// as a cancellation rather than a generic error.
		if ctx.Err() != nil {
			os.Remove(d.FilePath)
			m.SetStatus(id, StatusCancelled, nil)
			return
		}
		m.SetStatus(id, StatusError, err)
		return
	}

	m.UpdateProgress(id, 100)
	m.SetStatus(id, StatusDone, nil)
}

func buildUI(app *tview.Application, manager **Manager) tview.Primitive {
	// Right side, top: scrolling log of every download and its progress.
	logView := tview.NewTextView().
		SetDynamicColors(true).
		SetScrollable(true)
	logView.SetBorder(true).SetTitle(" Downloads ")

	// Right side, bottom: active downloads with a cancel button.
	cancelList := tview.NewList().
		ShowSecondaryText(false)

	m := NewManager(app, logView, cancelList)
	*manager = m

	cancelButton := tview.NewButton("Cancel Selected").SetSelectedFunc(func() {
		idx := cancelList.GetCurrentItem()
		if idx < 0 {
			return
		}
		_, id := cancelList.GetItemText(idx)
		m.CancelDownload(id)
	})

	cancelBox := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(cancelList, 0, 1, true).
		AddItem(cancelButton, 3, 0, false)
	cancelBox.SetBorder(true).SetTitle(" Active Downloads (Enter/click button to cancel) ")

	rightSide := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(logView, 0, 2, false).
		AddItem(cancelBox, 0, 1, false)

	// Left side: input form.
	var urlField, pathField *tview.InputField
	form := tview.NewForm()
	form.SetBorder(true).SetTitle(" New Download ")

	form.AddInputField("URL", "", 0, nil, nil)
	form.AddInputField("Filepath", "", 0, nil, nil)
	urlField = form.GetFormItem(0).(*tview.InputField)
	pathField = form.GetFormItem(1).(*tview.InputField)

	statusMsg := tview.NewTextView().SetDynamicColors(true)

	doDownload := func() {
		url := strings.TrimSpace(urlField.GetText())
		path := strings.TrimSpace(pathField.GetText())
		if url == "" || path == "" {
			statusMsg.SetText("[red]Both URL and filepath are required.[-]")
			return
		}
		m.AddDownload(url, path)
		statusMsg.SetText("[green]Download started.[-]")
		urlField.SetText("")
		pathField.SetText("")
	}

	form.AddButton("Download", doDownload)
	form.AddButton("Quit", func() {
		app.Stop()
	})

	leftSide := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(form, 0, 1, true).
		AddItem(statusMsg, 1, 0, false)

	root := tview.NewFlex().
		AddItem(leftSide, 0, 1, true).
		AddItem(rightSide, 0, 2, false)

	return root
}

func main() {
	logFile, err := os.OpenFile("tui-debug.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err == nil {
		log.SetOutput(logFile)
		defer logFile.Close()
	}

	app := tview.NewApplication()

	var manager *Manager
	root := buildUI(app, &manager)

	// Global key handling: Ctrl+C quits from anywhere.
	app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyCtrlC {
			app.Stop()
			return nil
		}
		return event
	})

	if err := app.SetRoot(root, true).SetFocus(root).Run(); err != nil {
		log.Fatalf("tui: %v", err)
	}
}