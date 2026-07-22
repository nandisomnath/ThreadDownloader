# TUI Implementation Plan

## Steps
- [x] Analyze existing codebase
- [x] Create implementation plan

### Phase 1: Modify downloader.go
- [x] Remove ANSI `print()` method
- [x] Add `ProgressCallback` field
- [x] Add `CancelChan` field
- [x] Modify `Download()` to use callback and support cancellation

### Phase 2: Modify thread_downloader.go
- [x] Add `callback` field
- [x] Add `SetProgressCallback()` method
- [x] Modify `AddDownloader()` to wire up callback
- [x] Add `CancelDownload(id)` method
- [x] Add `GetDownloads()` method

### Phase 3: Create tui.go
- [x] Define `ProgressUpdate` struct
- [x] Create TUI struct with widgets
- [x] Build layout (left input panel, right download table + active list)
- [x] Implement goroutine-safe UI update via channel + QueueUpdateDraw
- [x] Wire download button to add download
- [x] Wire cancel button to cancel download
- [x] Callback func to send updates to channel

### Phase 4: Modify main.go
- [x] Initialize TUI and ThreadDownloader
- [x] Wire progress callback
- [x] Run the application

### Phase 5: Finalize
- [x] Run `go mod tidy`
- [x] Build and test (successful)

