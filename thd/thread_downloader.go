package thd

import (
	"sync"
)

// ThreadDownloader handles Downloaders
// it handles all downloader and manages the download simultanously
type ThreadDownloader struct {

	// List to store all downloaders
	dl []Downloader

	// for each download have this id with increment order
	id int

	// callback is shared among all downloaders
	callback ProgressCallback
}

func NewThreadDownloader() ThreadDownloader {
	return ThreadDownloader{
		dl: make([]Downloader, 0),
		id: 0,
	}
}

// SetProgressCallback sets the progress callback for all future downloaders.
func (thdl *ThreadDownloader) SetProgressCallback(cb ProgressCallback) {
	thdl.callback = cb
}

func (thdl *ThreadDownloader) AddDownloader(url, filePath string) {
	d := NewDownloader(url, filePath, thdl.id)
	d.ProgressCallback = thdl.callback
	thdl.dl = append(thdl.dl, d)
	thdl.id += 1
}

// CancelDownload cancels a download by its ID.
// Returns true if the download was found and cancelled, false otherwise.
func (thdl *ThreadDownloader) CancelDownload(id int) bool {
	for i := range thdl.dl {
		if thdl.dl[i].id == id {
			select {
			case <-thdl.dl[i].CancelChan:
				// already closed
			default:
				close(thdl.dl[i].CancelChan)
			}
			return true
		}
	}
	return false
}

// GetActiveDownloads returns IDs of all downloads that are not completed/cancelled/errored.
func (thdl *ThreadDownloader) GetDownloads() []Downloader {
	return thdl.dl
}

func (thdl *ThreadDownloader) Start() {
	var wg sync.WaitGroup

	for i := 0; i < len(thdl.dl); i++ {
		wg.Add(1)
		go func(j int, wg *sync.WaitGroup) {
			thdl.dl[j].Download()
			wg.Done()
		}(i, &wg)
	}

	wg.Wait()
}
