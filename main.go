package main

import (
	// "fmt"

	"github.com/nandisomnath/thd/thd"
)

func main() {
	// fmt.Print("Hello \nSomnath")
	// fmt.Print("\r")
	// fmt.Println("Nice\t\t")
	url := "https://go.dev/dl/go1.26.5.linux-amd64.tar.gz"
	filePath := "/home/somnath/Downloads/go1.26.5.linux-amd64.tar.gz"
	
	thdl := thd.NewThreadDownloader()

	thdl.AddDownloader(url, filePath)
	thdl.AddDownloader(url, "/home/somnath/Downloads/go1.26.5.linux-amd64-1.tar.gz")
	thdl.AddDownloader(url, "/home/somnath/Downloads/go1.26.5.linux-amd64-2.tar.gz")
	thdl.AddDownloader(url, "/home/somnath/Downloads/go1.26.5.linux-amd64-3.tar.gz")
	thdl.AddDownloader(url, "/home/somnath/Downloads/go1.26.5.linux-amd64-4.tar.gz")

	thdl.Start()

}