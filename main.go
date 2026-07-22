package main

import (
	"github.com/nandisomnath/thd/thd"
)

func main() {
	thdl := thd.NewThreadDownloader()
	tui := NewTUI(&thdl)

	// Wire the TUI callback so download progress flows to the UI
	thdl.SetProgressCallback(tui.callback)

	if err := tui.Run(); err != nil {
		panic(err)
	}
}
