package main

import (
	"bytes"
	"context"
	"database/sql"
	"dropbear/decimal"
	"log"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

type Run struct {
	ID      int64
	Symbol  string
	Date    string
	Flags   string
	DbnPath string
	GitRev  string
	Status  string
	Winning decimal.Decimal
	Balance decimal.Decimal
	Fees    decimal.Decimal
}

func executeRun(ctx context.Context, db *sql.DB, run *Run) {
	now := time.Now().UnixNano()
	db.Exec(`UPDATE varulab_runs SET status = 'running', started_at = ? WHERE id = ?`, now, run.ID)
	broadcastRunStatus(run.ID, "running")

	// build command
	args := []string{"run", "./cmd/varu",
		"-dbn", run.DbnPath,
		"-symbol", run.Symbol,
		"-date", run.Date,
	}
	if flags := parseFlagString(run.Flags); len(flags) > 0 {
		args = append(args, flags...)
	}

	log.Printf("exec: go %s", strings.Join(args, " "))
	cmd := exec.CommandContext(ctx, "go", args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		// kill the entire process group so child processes die too
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err := cmd.Run()

	finished := time.Now().UnixNano()
	logOutput := stderr.String()

	if err != nil {
		// check if we still got a result despite non-zero exit
		if winning, balance, fees, ok := parseWinning(logOutput); ok {
			db.Exec(`UPDATE varulab_runs SET status = 'done', winning = ?, balance = ?, fees = ?, log = ?, finished_at = ? WHERE id = ?`,
				int64(winning), int64(balance), int64(fees), truncateLog(logOutput), finished, run.ID)
			run.Status = "done"
			run.Winning = winning
		} else {
			db.Exec(`UPDATE varulab_runs SET status = 'failed', log = ?, finished_at = ? WHERE id = ?`,
				truncateLog(logOutput), finished, run.ID)
			run.Status = "failed"
			log.Printf("run %d (%s %s %s) failed: %v", run.ID, run.Symbol, run.Date, run.Flags, err)
		}
	} else {
		winning, balance, fees, ok := parseWinning(logOutput)
		if !ok {
			db.Exec(`UPDATE varulab_runs SET status = 'failed', log = ?, finished_at = ? WHERE id = ?`,
				truncateLog(logOutput), finished, run.ID)
			run.Status = "failed"
			log.Printf("run %d (%s %s %s): could not parse winning from output", run.ID, run.Symbol, run.Date, run.Flags)
		} else {
			db.Exec(`UPDATE varulab_runs SET status = 'done', winning = ?, balance = ?, fees = ?, log = ?, finished_at = ? WHERE id = ?`,
				int64(winning), int64(balance), int64(fees), truncateLog(logOutput), finished, run.ID)
			run.Status = "done"
			run.Winning = winning
		}
	}

	broadcastRunStatus(run.ID, run.Status)
}

// truncateLog keeps only the first and last 1000 lines to bound storage.
func truncateLog(s string) string {
	lines := strings.Split(s, "\n")
	if len(lines) <= 2000 {
		return s
	}
	head := lines[:1000]
	tail := lines[len(lines)-1000:]
	return strings.Join(head, "\n") + "\n\n... truncated ...\n\n" + strings.Join(tail, "\n")
}
