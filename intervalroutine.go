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
	var timerC <-chan time.Time
	if rrt.currentInterval > 0 {
		timerC = time.NewTimer(rrt.currentInterval).C
	}

	select {
	case <-timerC:
		select {
		case <-rrt.done:
			return false
		default:
		}
		err = rrt.f()
	case <-rrt.force:
		select {
		case <-rrt.done:
			return false
		default:
		}
		err = rrt.f()
	case <-rrt.done:
		return false
	}

	if err != nil && rrt.retryInterval > 0 {
		rrt.currentInterval = rrt.retryInterval
	} else {
		rrt.currentInterval = rrt.runInterval
	}
	return true
}
