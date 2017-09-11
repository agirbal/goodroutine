// Package goodroutine provide some generic maintenance utilities generally useful for server implementations.
// The goal of this package is to provide predictability of initialization, intervals, retries, and panic recovery
// to maintenance functions that would otherwise just run bare in goroutines.
// It also includes often needed aspects of server's health.
//
// Features include:
// - interval-based goroutine that safely runs a function
// - threshold based up / down healthcheck
//
package goodroutine

import (
	"fmt"
	"runtime/debug"
	"sync"
	"time"
)

// IntervalRoutine implements a management goroutine.
// It provides a safe way to run a function, at interval, from a single goroutine.
type IntervalRoutine struct {
	f               func() error
	runInterval     time.Duration
	retryInterval   time.Duration
	currentInterval time.Duration
	ticker          *time.Ticker
	force           chan bool
	done            chan bool
	start           sync.Once
	stop            sync.Once

	// NoRecover if set to true, panics are not recovered
	NoRecover bool
	OnPanic   func(recovered interface{})
}

// NewIntervalRoutine creates a new IntervalRoutine, which takes care of running f().
// Runs may be triggered in 3 ways:
// - normally at each run interval
// - at the retry interval, if f() returned an error
// - if TriggerRun was called
// A typical usage is a runInterval of 5min, retryInterval of 5sec.
func NewIntervalRoutine(f func() error, runInterval time.Duration, retryInterval time.Duration) *IntervalRoutine {
	return &IntervalRoutine{
		f:             f,
		runInterval:   runInterval,
		retryInterval: retryInterval,
		force:         make(chan bool, 1),
		done:          make(chan bool, 1),
	}
}

// TriggerRun triggers a run as soon as possible.
// Does nothing if a forced run is already scheduled.
func (rrt *IntervalRoutine) TriggerRun() {
	select {
	case rrt.force <- true:
	default:
		// already has a force
	}
}

func (rrt *IntervalRoutine) updateTicker(interval time.Duration) {
	if interval == rrt.currentInterval {
		// nothing to change
		return
	}

	if rrt.ticker != nil {
		rrt.ticker.Stop()
		rrt.ticker = nil
	}

	rrt.currentInterval = interval
	if interval > 0 {
		rrt.ticker = time.NewTicker(interval)
	}
}

// Start the management routine.
func (rrt *IntervalRoutine) Start() {
	rrt.start.Do(func() {
		go func() {
			// add a force to run once at startup, ticker will get set after
			rrt.force <- true
			for {
				if !rrt.runSafe() {
					break
				}
			}
		}()
	})
}

// Stop the management routine.
func (rrt *IntervalRoutine) Stop() {
	rrt.stop.Do(func() {
		close(rrt.done)
	})
}

func (rrt *IntervalRoutine) runSafe() bool {
	if !rrt.NoRecover {
		// recover any panic
		defer func() {
			if r := recover(); r != nil {
				if rrt.OnPanic != nil {
					rrt.OnPanic(r)
				} else {
					fmt.Printf("recovered: %v, stack: %s\n", r, debug.Stack())
				}
			}
		}()
	}

	var err error
	var tickerC <-chan time.Time
	// if ticker not set, channel will appropriately block
	if rrt.ticker != nil {
		tickerC = rrt.ticker.C
	}

	select {
	case <-tickerC:
		err = rrt.f()
	case <-rrt.force:
		err = rrt.f()
	case <-rrt.done:
		if rrt.ticker != nil {
			rrt.ticker.Stop()
		}
		return false
	}

	interval := rrt.runInterval
	if err != nil && rrt.retryInterval > 0 {
		// retry
		interval = rrt.retryInterval
	}
	rrt.updateTicker(interval)
	return true
}
