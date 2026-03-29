package main

import (
	"context"
	"database/sql"
	"log"
	"sync"
)

type Scheduler struct {
	db     *sql.DB
	sem    chan struct{}
	mu     sync.Mutex
	active map[string]int // dbn_path -> count of running
	wg     sync.WaitGroup
	cancel context.CancelFunc
	ctx    context.Context
}

func NewScheduler(db *sql.DB, maxJobs int) *Scheduler {
	ctx, cancel := context.WithCancel(context.Background())
	return &Scheduler{
		db:     db,
		sem:    make(chan struct{}, maxJobs),
		active: map[string]int{},
		ctx:    ctx,
		cancel: cancel,
	}
}

// GenerateRuns creates pending run rows for all (symbol, date, flags)
// combinations that haven't been run at the current git revision.
func (s *Scheduler) GenerateRuns(gitRev string) int {
	combos, err := generateFlagCombinations()
	if err != nil {
		log.Printf("generate flag combinations: %v", err)
		return 0
	}

	// find all available dbn files
	dbnFiles := discoverDbnFiles()
	created := 0
	for _, df := range dbnFiles {
		for _, flags := range combos {
			_, err := s.db.Exec(`INSERT OR IGNORE INTO varulab_runs (symbol, date, flags, dbn_path, git_rev, status, created_at) VALUES (?, ?, ?, ?, ?, 'pending', ?)`,
				df.Symbol, df.Date, flags, df.Path, gitRev, now())
			if err != nil {
				log.Printf("insert run: %v", err)
				continue
			}
			created++
		}
	}
	return created
}

// Run starts the scheduling loop. It dispatches pending runs preferring
// dbn paths that already have active runs (mmap page cache is hot).
func (s *Scheduler) Run() {
	for {
		select {
		case <-s.ctx.Done():
			return
		default:
		}

		run := s.pickNext()
		if run == nil {
			select {
			case <-s.ctx.Done():
				return
			case <-gSchedulerNotify:
				continue
			}
		}

		// acquire semaphore slot, respecting cancellation
		select {
		case s.sem <- struct{}{}:
		case <-s.ctx.Done():
			return
		}

		s.mu.Lock()
		s.active[run.DbnPath]++
		s.mu.Unlock()

		s.wg.Add(1)
		go func(r *Run) {
			defer s.wg.Done()
			defer func() {
				<-s.sem
				s.mu.Lock()
				s.active[r.DbnPath]--
				if s.active[r.DbnPath] == 0 {
					delete(s.active, r.DbnPath)
				}
				s.mu.Unlock()
				select {
				case gSchedulerNotify <- struct{}{}:
				default:
				}
			}()
			executeRun(s.ctx, s.db, r)
		}(run)
	}
}

// pickNext selects the best pending run. Prefers dbn paths with active
// runs (hot mmap), then paths with the most pending work.
func (s *Scheduler) pickNext() *Run {
	s.mu.Lock()
	defer s.mu.Unlock()

	// first try to find a pending run on a hot path
	for path, count := range s.active {
		if count > 0 {
			run := fetchPendingRun(s.db, path)
			if run != nil {
				return run
			}
		}
	}
	// fall back to any pending run
	return fetchPendingRun(s.db, "")
}

func (s *Scheduler) Stop() {
	s.cancel() // cancels context, kills all running subprocesses
	s.wg.Wait()
	// reset any runs that were running/claimed back to pending
	s.db.Exec(`UPDATE varulab_runs SET status = 'pending' WHERE status IN ('running', 'claimed')`)
}

func fetchPendingRun(db *sql.DB, preferPath string) *Run {
	var row *sql.Row
	if preferPath != "" {
		row = db.QueryRow(`SELECT id, symbol, date, flags, dbn_path, git_rev FROM varulab_runs WHERE status = 'pending' AND dbn_path = ? LIMIT 1`, preferPath)
	} else {
		row = db.QueryRow(`SELECT id, symbol, date, flags, dbn_path, git_rev FROM varulab_runs WHERE status = 'pending' LIMIT 1`)
	}
	var r Run
	if err := row.Scan(&r.ID, &r.Symbol, &r.Date, &r.Flags, &r.DbnPath, &r.GitRev); err != nil {
		return nil
	}
	// mark as claimed to prevent double-dispatch
	res, _ := db.Exec(`UPDATE varulab_runs SET status = 'claimed' WHERE id = ? AND status = 'pending'`, r.ID)
	if n, _ := res.RowsAffected(); n == 0 {
		return nil // someone else grabbed it
	}
	r.Status = "claimed"
	return &r
}

// gSchedulerNotify is used to wake the scheduler when new work is available.
var gSchedulerNotify = make(chan struct{}, 1)

func notifyScheduler() {
	select {
	case gSchedulerNotify <- struct{}{}:
	default:
	}
}
