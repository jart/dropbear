package loggy

import (
	"dropbear/clocky"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
)

var (
	flagLog = flag.String("log", "log", "path to log file (used in live trading mode)")
)

// Init initializes logging.
// This should be called from main() after flag.Parse().
func Init() {
	log.SetFlags(0)
	log.SetOutput(gLogWriter)
}

// Fatalf logs an error, raises SIGINT to trigger cleanup, and blocks.
// This is important to use for fatal errors so active orders can be cancelled.
func Fatalf(format string, args ...any) {
	log.Printf("FATAL: "+format, args...)
	panic(fmt.Sprintf(format, args...))
}

// AlsoLogToFile enables logging to -log FILE in addition to stderr.
// The teddy framework calls this when initialized in live trading mode.
func AlsoLogToFile() {
	if *flagLog != "" {
		var err error
		gLogWriter.file, err = os.OpenFile(*flagLog, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			log.Fatalf("opening log file: %v", err)
		}
	}
}

// CommandLine returns the full command line with all flag values (including defaults).
func CommandLine() string {
	prog := os.Args[0]
	if idx := strings.LastIndex(prog, "/"); idx != -1 {
		prog = prog[idx+1:]
	}
	var parts []string
	parts = append(parts, prog)
	flag.VisitAll(func(f *flag.Flag) {
		parts = append(parts, fmt.Sprintf("-%s=%s", f.Name, f.Value.String()))
	})
	parts = append(parts, flag.Args()...)
	return strings.Join(parts, " ")
}

type logWriter struct {
	file *os.File
}

var gLogWriter = logWriter{}

func (w logWriter) Write(p []byte) (n int, err error) {
	line := clocky.Now().String() + " " + string(p)
	os.Stderr.WriteString(line)
	if w.file != nil {
		w.file.WriteString(line)
	}
	return len(p), nil
}
