package schwab

import (
	"dropbear/clocky"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
)

var (
	logFlag = flag.String("schwab-log", os.ExpandEnv("$HOME/.schwab.log"), "log file for schwab interactions (empty string to disable)")
	logOnce sync.Once
	logSave *os.File
)

func getLog() *os.File {
	logOnce.Do(func() {
		var err error
		if *logFlag != "" && *logFlag != "/dev/null" {
			logSave, err = os.OpenFile(*logFlag, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
			if err != nil {
				log.Fatalf("%s: could not open schwab log file: %v", *logFlag, err)
			}
		}
	})
	return logSave
}

func logf(format string, v ...any) {
	line := fmt.Sprintf(format, v...)
	log.Print(line)
	flog := getLog()
	if flog != nil {
		var sb strings.Builder
		sb.WriteString(clocky.Now().String())
		sb.WriteString(" ")
		sb.WriteString(line)
		flog.Write([]byte(sb.String()))
	}
}
