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

// Runner implements a function that is run at interval
type Runner interface {
	IntervalRun() error
}

// The RunnerFunc type is an adapter to allow the use of
// ordinary functions as Runner. If f is a function
// with the appropriate signature, RunnerFunc(f) is a
// Runner that calls f.
type RunnerFunc func() error

// IntervalRun implements the Runner interface
func (rf RunnerFunc) IntervalRun() error {
	return rf()
}

// IntervalRoutine implements a management goroutine.
// It provides a safe way to run a function, at interval, from a single goroutine.
type IntervalRoutine struct {
	runner          Runner
	runInterval     time.Duration
	retryInterval   time.Duration
	currentInterval time.Duration
	force           chan bool
	done            chan bool
	start           sync.Once
	stop            sync.Once

	// PanicRecoverDisabled if set to true, panics are not recovered
	PanicRecoverDisabled bool
	// RetryBackoffDisabled if set to true, retry interval does not increase exponentially
	RetryBackoffDisabled bool
	OnPanic              func(recovered interface{})
}

// NewIntervalRoutine creates a new IntervalRoutine.
// Runs may be triggered in 3 ways:
// - normally at each run interval
// - at the retry interval, if the last run returned an error
// - if TriggerRun was called
// A typical usage is a runInterval of 5min, retryInterval of 30sec.
// By default the retry interval increases exponentially from retryInterval up to runInterval.
// retryInterval cannot be set higher than runInterval.
func NewIntervalRoutine(runner Runner, runInterval time.Duration, retryInterval time.Duration) *IntervalRoutine {
	if retryInterval > runInterval {
		// wrong interval, disable custom retry
		retryInterval = 0
	}
	return &IntervalRoutine{
		runner:        runner,
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
	if !rrt.PanicRecoverDisabled {
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
		timer := time.NewTimer(rrt.currentInterval)
		timerC = timer.C
		defer timer.Stop()
	}

	select {
	case <-timerC:
		select {
		case <-rrt.done:
			return false
		default:
		}
		err = rrt.runner.IntervalRun()
	case <-rrt.force:
		select {
		case <-rrt.done:
			return false
		default:
		}
		err = rrt.runner.IntervalRun()
	case <-rrt.done:
		return false
	}

	if err != nil && rrt.retryInterval > 0 {
		retryInterval := rrt.retryInterval
		// rrt.currentInterval == rrt.runInterval on the first retry only
		if !rrt.RetryBackoffDisabled && rrt.currentInterval > 0 && rrt.currentInterval < rrt.runInterval {
			// backoff, starting from rrt.retryInterval, up to rrt.runInterval
			retryInterval = rrt.currentInterval * 2
			if retryInterval >= rrt.runInterval {
				// set the interval just under run interval to differentiate
				retryInterval = rrt.runInterval - 1
			}
		}
		rrt.currentInterval = retryInterval
	} else {
		rrt.currentInterval = rrt.runInterval
	}
	return true
}
