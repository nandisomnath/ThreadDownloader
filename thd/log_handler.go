package thd

import (
	"fmt"
)


type LogHandler struct {
	logs []string
	id int
}

func NewLogHandler() LogHandler {
	return LogHandler {
		logs: make([]string, 30),
		id: 0,
	}	
}


func (lh *LogHandler) AddLog(id int, log string) {
	lh.logs[id] = log
	if id >= lh.id {
		lh.id = id
	}
}

func (lh *LogHandler) PrintLogs()  {
	
	// fmt.Print("\033[1A\r")
	var logString string
	// var blankString string

	count := 0

	for i := 0; i < lh.id; i++ {
		logString = fmt.Sprintf("%s\n%s", logString, lh.logs[i])
		count += 1

		// l := len(lh.logs[i])
		// blankString = fmt.Sprintf("%s\n%s", blankString, strings.Repeat(" ", l))
	
	}

	fmt.Printf("%s", logString)
	if count > 0 {
		fmt.Printf("\033[%dA\r", lh.id)
	} else {
		fmt.Printf("\r")
	}
	// fmt.Printf("%s", blankString)
	// os.Exit(0) // TODO: REMOVE THIS
}
