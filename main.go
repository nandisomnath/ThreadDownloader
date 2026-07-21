package main

import (
	"fmt"

	"github.com/nandisomnath/thd/thd"
)

func main() {
	fmt.Println("Hello Somnath")
	url := "https://go.dev/dl/go1.26.5.linux-amd64.tar.gz"
	filePath := "/home/somnath/Downloads/go1.26.5.linux-amd64.tar.gz"
	thd.Download(url, filePath)
}