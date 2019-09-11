package goodroutine

import (
	"sync"
	"sync/atomic"
)

// HealthChecker implements a health check, using a threshold for up / down logic.
// It can be combined with an IntervalRoutine to implement a health check goroutine.
type HealthChecker struct {
	mu            sync.RWMutex
	runner        Runner
	state         int32
	ups           int
	downs         int
	thresholdUp   int
	thresholdDown int
	lastErr       error
	firstRun      bool

	// OnUp is called when state changes to up, numDowns is number of prior downs
	OnUp func(numUps int, numDowns int)
	// OnDown is called when state changes to down, numUps is number of prior ups, lastErr is last error recorded
	OnDown func(numUps int, numDowns int, lastErr error)
	// NoRecover if set to true, panics are not recovered
	NoRecover bool
	// FastStart if set to true, threshold fully apply from start
	FastStart bool
}

// NewHealthChecker creates a new HealthChecker.
// f is the function to run to obtain the health.
// defaultState is the default up / down state before any run occurs.
// thresholdUp defines the number of non-error runs before going from down to up.
// thresholdDown defines the number of error runs before going from up to down.
// By default it uses fast start, which means the 1st run will reset the state.
// A typical usage is a defaultState of false, thresholdUp of 3 and thresholdDown of 5.
func NewHealthChecker(runner Runner, defaultState bool, thresholdUp int, thresholdDown int) *HealthChecker {
	if thresholdDown < 1 {
		thresholdDown = 1
	}
	if thresholdUp < 1 {
		thresholdUp = 1
	}

	hrt := &HealthChecker{
		runner:        runner,
		thresholdUp:   thresholdUp,
		thresholdDown: thresholdDown,
		FastStart:     true,
	}
	hrt.Reset(defaultState)
	return hrt
}

// Reset sets the healthcheck to the given state, resetting all other aspects.
func (hrt *HealthChecker) Reset(newState bool) {
	hrt.mu.Lock()
	defer hrt.mu.Unlock()
	var state int32
	if newState {
		state = 1
		if hrt.OnUp != nil {
			defer hrt.OnUp(hrt.ups, hrt.downs)
		}
	} else {
		if hrt.OnDown != nil {
			defer hrt.OnDown(hrt.ups, hrt.downs, hrt.lastErr)
		}
	}
	atomic.StoreInt32(&hrt.state, state)
	hrt.ups = 0
	hrt.downs = 0
	hrt.firstRun = true
}

// IntervalRun implements the Runner interface
func (hrt *HealthChecker) IntervalRun() error {
	err := hrt.runner.IntervalRun()

	hrt.mu.Lock()
	faststart := hrt.FastStart && hrt.firstRun
	wasUp := hrt.IsUp()
	if err != nil {
		hrt.downs++
		if !wasUp {
			// clear any progress
			hrt.ups = 0
		} else if faststart || hrt.downs >= hrt.thresholdDown {
			// going down
			atomic.StoreInt32(&hrt.state, 0)
			if hrt.OnDown != nil {
				defer hrt.OnDown(hrt.ups, hrt.downs, err)
			}
			hrt.ups = 0
		}
		hrt.lastErr = err
	} else {
		hrt.ups++
		if wasUp {
			// clear any progress
			hrt.downs = 0
		} else if faststart || hrt.ups >= hrt.thresholdUp {
			// going up
			atomic.StoreInt32(&hrt.state, 1)
			if hrt.OnUp != nil {
				defer hrt.OnUp(hrt.ups, hrt.downs)
			}
			hrt.downs = 0
		}
	}
	hrt.firstRun = false
	// unlock manually so that defers are lock-less
	hrt.mu.Unlock()
	return err
}

// IsUp returns the current state, up (true) or down (false)
func (hrt *HealthChecker) IsUp() bool {
	return atomic.LoadInt32(&hrt.state) == 1
}

// LastErr returns the last error
func (hrt *HealthChecker) LastErr() error {
	hrt.mu.RLock()
	defer hrt.mu.RUnlock()
	return hrt.lastErr
}
