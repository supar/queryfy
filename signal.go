package main

import (
	"os"
	"os/signal"
)

// signals represents simple struct to keep system signals and there handlers
type signals struct {
	sig chan os.Signal

	// shutdown function
	shtFn func()

	// wait for the shutdown
	wait chan struct{}
}

func newShutdownSignal(fn func()) <-chan struct{} {
	s := &signals{
		shtFn: fn,

		sig:  make(chan os.Signal, 1),
		wait: make(chan struct{}),
	}

	signal.Notify(s.sig, os.Interrupt)

	go s.run()

	return s.wait
}

func (s *signals) run() {
	<-s.sig

	s.shtFn()

	close(s.wait)
}
