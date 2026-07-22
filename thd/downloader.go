package thd

import (
	"io"
	"net/http"
	"os"
)

// ProgressCallback is invoked by the downloader to report progress/status.
type ProgressCallback func(id int, url, filePath string, progress float64, status string)

func percentage(n, d float64) float64 {
	return (n / d) * 100.0
}

// Wrap the download function using downloader struct
type Downloader struct {
	url      string
	filePath string
	id       int

	// ProgressCallback is called from the Download goroutine. Must be goroutine-safe.
	ProgressCallback ProgressCallback

	// CancelChan, when closed, signals the download to abort.
	CancelChan chan struct{}
}

func NewDownloader(url, filePath string, id int) Downloader {
	return Downloader{
		url:        url,
		filePath:   filePath,
		id:         id,
		CancelChan: make(chan struct{}),
	}
}

// This function download the file and save in a location
func (d Downloader) Download() {
	// Report starting status
	if d.ProgressCallback != nil {
		d.ProgressCallback(d.id, d.url, d.filePath, 0, "downloading")
	}

	res, err := http.Get(d.url)
	if err != nil {
		if d.ProgressCallback != nil {
			d.ProgressCallback(d.id, d.url, d.filePath, 0, "error")
		}
		return
	}
	defer res.Body.Close()

	out, err := os.Create(d.filePath)
	if err != nil {
		if d.ProgressCallback != nil {
			d.ProgressCallback(d.id, d.url, d.filePath, 0, "error")
		}
		return
	}
	defer out.Close()

	buf := make([]byte, 1024)

	var saved float64 = 0
	var total float64 = float64(res.ContentLength)

	for {
		// Check for cancellation before reading
		select {
		case <-d.CancelChan:
			out.Close()
			os.Remove(d.filePath)
			if d.ProgressCallback != nil {
				d.ProgressCallback(d.id, d.url, d.filePath, 0, "cancelled")
			}
			return
		default:
		}

		n, err := res.Body.Read(buf)

		if err != nil && err != io.EOF {
			if d.ProgressCallback != nil {
				d.ProgressCallback(d.id, d.url, d.filePath, 0, "error")
			}
			return
		}

		if n > 0 {
			s, e := out.Write(buf[0:n])
			if e != nil {
				if d.ProgressCallback != nil {
					d.ProgressCallback(d.id, d.url, d.filePath, 0, "error")
				}
				return
			}

			saved += float64(s)
			per := percentage(saved, total)

			if d.ProgressCallback != nil {
				d.ProgressCallback(d.id, d.url, d.filePath, per, "downloading")
			}
		}

		if err == io.EOF {
			break
		}
	}

	if d.ProgressCallback != nil {
		d.ProgressCallback(d.id, d.url, d.filePath, 100, "completed")
	}
}
