package main

import (
	"dropbear/clocky"
	"os"
)

var gLogWriter logWriter

type logWriter struct {
	file    *os.File
	capture func(string)
}

func (w *logWriter) Write(p []byte) (n int, err error) {
	line := clocky.Now().String() + " " + string(p)
	if w.capture != nil {
		w.capture(line)
	} else {
		os.Stderr.WriteString(line)
	}
	if w.file != nil {
		w.file.WriteString(line)
	}
	if gLogMsg != nil {
		gLogMsg <- line
	}
	return len(p), nil
}
