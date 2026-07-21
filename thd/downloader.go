package thd

import (
	"fmt"
	"io"
	"net/http"
	"os"
)

func percentage(n, d float64) float64 {
	return (n/d) * 100.0
}

// Wrap the download function using downloader struct
type Downloader struct {
	url string
	filePath string
	id int
	logHandler LogHandler
}

func NewDownloader(url, filePath string, id int, logHandler LogHandler) Downloader {
	return Downloader{
		url: url,
		filePath: filePath,
		id: id,
		logHandler: logHandler,
	}
}


// This function download the file and save in a location
func (d Downloader) Download()  {
	
	res, err := http.Get(d.url)

	if err != nil {
		panic(err.Error())
	}

	defer res.Body.Close()
	
	out, err := os.Create(d.filePath)
	
	if err != nil {
		panic(err.Error())
	}

	defer out.Close()

	buf := make([]byte, 1024)

	var saved float64 = 0
	var total float64 = float64(res.ContentLength)
	
	for true {
		n, err := res.Body.Read(buf)

		if err != nil && err != io.EOF {
			panic(err.Error())
		}

		if n > 0 {
			s, e := out.Write(buf[0:n])

			if e != nil {
				panic(e.Error())
			}

			saved += float64(s)

			// TODO: print here 
			d.logHandler.AddLog(d.id, fmt.Sprintf("(%d)Download finished: %.2f%%\r", d.id, percentage(saved, total)))
			d.logHandler.PrintLogs()
		}
		
		if err == io.EOF {
			break;
		}
	}

}