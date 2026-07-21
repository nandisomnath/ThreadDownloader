package thd

import (
	"fmt"
	"io"
	"net/http"
	"os"
)

// This is a downloder struct
// Only download one file at a time
type Downloader struct {
	
}



func percentage(n, d float64) float64 {
	return (n/d) * 100.0
}

func Download(url string, filePath string)  {
	
	res, err := http.Get(url)

	if err != nil {
		panic(err.Error())
	}

	defer res.Body.Close()
	
	out, err := os.Create(filePath)
	
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
			fmt.Printf("Completed: %f %%", percentage(saved, total))

		}
		
		if err == io.EOF {
			break;
		}
	}

}