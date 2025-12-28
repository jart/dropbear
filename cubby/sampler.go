package cubby

import (
	"dropbear/clocky"
	"sync/atomic"
	"time"
)

type samplerDaemon struct {
	interval clocky.Duration
	closer   chan struct{}
	report   *report
	ready    atomic.Bool
}

func newSamplerDaemon(interval clocky.Duration, report *report) *samplerDaemon {
	return &samplerDaemon{
		interval: interval,
		closer:   make(chan struct{}),
		report:   report,
	}
}

func (sd *samplerDaemon) Close() {
	close(sd.closer)
}

func (sd *samplerDaemon) Run() {
	oldOnReady := Exchanges.OnReady
	Exchanges.OnReady = func() {
		if oldOnReady != nil {
			oldOnReady()
		}
		sd.report.Init()
		sd.ready.Store(true)
	}
	go func() {
		t := time.NewTicker(time.Duration(int64(sd.interval) * int64(time.Microsecond) / 10))
		for {
			select {
			case <-t.C:
			case <-sd.closer:
				t.Stop()
				return
			}
			if sd.ready.Load() {
				sd.report.Sample(clocky.Now())
			}
		}
	}()
}
