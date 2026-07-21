package thd

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
)

var mutex sync.Mutex

func percentage(n, d float64) float64 {
	return (n / d) * 100.0
}

// simply panic with any error
func panicHandler(err error) {
	if err != nil {
		panic(err.Error())
	}
}

// Wrap the download function using downloader struct
type Downloader struct {
	url      string
	filePath string
	id       int
}

func NewDownloader(url, filePath string, id int) Downloader {
	return Downloader{
		url:      url,
		filePath: filePath,
		id:       id,
	}
}

func (d Downloader) print(per float64) {
	mutex.Lock()
	defer mutex.Unlock()

	row := (1 + (2 * d.id)) + 4
	fmt.Printf("\033[%d;1H", row) // if you know the absolute row
	fmt.Printf("(%d) => {url: %s,output:%s}\n", d.id, d.url, d.filePath)
	// TODO: print here
	fmt.Printf("Download finished: %.2f%%\n", per)
}

// This function download the file and save in a location
func (d Downloader) Download() {

	res, err := http.Get(d.url)
	panicHandler(err)
	defer res.Body.Close()

	out, err := os.Create(d.filePath)
	panicHandler(err)
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
			panicHandler(e)

			saved += float64(s)

			per := percentage(saved, total)
			d.print(per)
		}

		if err == io.EOF {
			break
		}
	}

}
