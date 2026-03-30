// dbnconntest opens N simultaneous live gateway connections to OPRA.PILLAR,
// each subscribing to a different symbol's options, to determine whether
// databento's 10-connection limit is per-account or per-subscription.
package main

import (
	"dropbear/broker/databento"
	"io"
	"log"
	"sync"
	"sync/atomic"

	"fmt"
)

type sub struct {
	dataset string
	symbol  string
}

var subs = []sub{
	// OPRA connections
	{"OPRA.PILLAR", "AAPL.OPT"},
	{"OPRA.PILLAR", "MSFT.OPT"},
	{"OPRA.PILLAR", "NVDA.OPT"},
	{"OPRA.PILLAR", "TSLA.OPT"},
	{"OPRA.PILLAR", "AMZN.OPT"},
	{"OPRA.PILLAR", "META.OPT"},
	{"OPRA.PILLAR", "GOOG.OPT"},
	{"OPRA.PILLAR", "SPY.OPT"},
	{"OPRA.PILLAR", "QQQ.OPT"},
	{"OPRA.PILLAR", "AMD.OPT"},
	// CME connections
	{"GLBX.MDP3", "ES.FUT"},
	{"GLBX.MDP3", "NQ.FUT"},
	{"GLBX.MDP3", "CL.FUT"},
	{"GLBX.MDP3", "GC.FUT"},
	{"GLBX.MDP3", "SI.FUT"},
	{"GLBX.MDP3", "ZN.FUT"},
	{"GLBX.MDP3", "ZB.FUT"},
	{"GLBX.MDP3", "HG.FUT"},
	{"GLBX.MDP3", "NG.FUT"},
	{"GLBX.MDP3", "YM.FUT"},
}

func main() {
	key := databento.MustLoadDefaultKey()
	var connected atomic.Int32
	allConnected := make(chan struct{})

	// phase 1: connect all, then wait for everyone
	var wg sync.WaitGroup
	for _, s := range subs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			label := fmt.Sprintf("%s/%s", s.dataset, s.symbol)
			client, err := databento.Dial(s.dataset, key)
			if err != nil {
				log.Printf("%-25s DIAL FAILED: %v", label, err)
				return
			}
			defer client.Close()
			client.MustSubscribe(databento.Subscription{
				Schema:  databento.SchemaCMBP1,
				SType:   databento.STypeParent,
				Symbols: []string{s.symbol},
			})
			_, err = client.Start()
			if err != nil {
				log.Printf("%-25s START FAILED: %v", label, err)
				return
			}
			n := connected.Add(1)
			log.Printf("%-25s connected (%d total)", label, n)

			// wait for all connections to be established
			if int(n) == len(subs) {
				close(allConnected)
			}
			<-allConnected

			// now read with all connections held open
			for i := 0; i < 1000; i++ {
				_, err := client.Read()
				if err != nil {
					if err == io.EOF {
						log.Printf("%-25s EOF after %d records", label, i)
					} else {
						log.Printf("%-25s READ ERROR after %d records: %v", label, i, err)
					}
					return
				}
			}
			log.Printf("%-25s got 1000 records while all connections held open", label)
		}()
	}

	wg.Wait()
	log.Printf("done: %d out of %d connections succeeded concurrently", connected.Load(), len(subs))
}
