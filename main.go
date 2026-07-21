package main

import (
	"fmt"

	"github.com/nandisomnath/thd/thd"
)

func main() {
	fmt.Println("Hello Somnath")
	url := "https://go.dev/dl/go1.26.5.linux-amd64.tar.gz"
	filePath := "/home/somnath/Downloads/go1.26.5.linux-amd64.tar.gz"
	
	thdl := thd.NewThreadDownloader()

	thdl.AddDownloader(thd.NewDownloader(url, filePath))
	thdl.AddDownloader(thd.NewDownloader(url, "/home/somnath/Downloads/go1.26.5.linux-amd64-1.tar.gz"))
	thdl.AddDownloader(thd.NewDownloader(url, "/home/somnath/Downloads/go1.26.5.linux-amd64-2.tar.gz"))
	thdl.AddDownloader(thd.NewDownloader(url, "/home/somnath/Downloads/go1.26.5.linux-amd64-3.tar.gz"))
	thdl.AddDownloader(thd.NewDownloader(url, "/home/somnath/Downloads/go1.26.5.linux-amd64-4.tar.gz"))

	thdl.Start()

}