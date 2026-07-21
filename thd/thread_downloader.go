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
}


func NewThreadDownloader() ThreadDownloader {
	return ThreadDownloader{
		dl: make([]Downloader, 0),
		id: 0,
	}
}



func (thdl *ThreadDownloader) AddDownloader(url , filePath string)  {
	d := NewDownloader(url, filePath, thdl.id)
	thdl.dl = append(thdl.dl, d)
	thdl.id += 1
}


func (thdl *ThreadDownloader) Start()  {
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

