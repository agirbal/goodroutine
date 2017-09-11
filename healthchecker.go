package goodroutine

import "sync/atomic"

// HealthChecker implements a health check, using a threshold for up / down logic.
// It can be combined with an IntervalRoutine to implement a health check goroutine.
type HealthChecker struct {
	f             func() error
	state         int32
	ups           int
	downs         int
	thresholdUp   int
	thresholdDown int

	// OnUp is called when state changes to up, numDowns is number of prior downs
	OnUp func(numUps int, numDowns int)
	// OnDown is called when state changes to down, numUps is number of prior ups
	OnDown func(numUps int, numDowns int)
	// NoRecover if set to true, panics are not recovered
	NoRecover bool
}

// NewHealthChecker creates a new HealthChecker.
// f is the function to run to obtain the health.
// defaultState is the default up / down state before any run occurs.
// thresholdUp defines the number of non-error runs before going from down to up.
// thresholdDown defines the number of error runs before going from up to down.
// A typical usage is a defaultState of 1, thresholdUp of 1 (instant up) and thresholdDown of 3.
func NewHealthChecker(f func() error, defaultState bool, thresholdUp int, thresholdDown int) *HealthChecker {
	if thresholdDown < 1 {
		thresholdDown = 1
	}
	if thresholdUp < 1 {
		thresholdUp = 1
	}

	state := 1
	if !defaultState {
		state = 0
	}

	return &HealthChecker{
		f:             f,
		state:         int32(state),
		thresholdUp:   thresholdUp,
		thresholdDown: thresholdDown,
	}
}

// Run the health check
func (hrt *HealthChecker) Run() error {
	err := hrt.f()

	wasUp := hrt.IsUp()
	if err != nil {
		hrt.downs++
		if !wasUp {
			// clear any progress
			hrt.ups = 0
		} else if hrt.downs >= hrt.thresholdDown {
			// going down
			atomic.StoreInt32(&hrt.state, 0)
			if hrt.OnDown != nil {
				hrt.OnDown(hrt.ups, hrt.downs)
			}
			hrt.ups = 0
		}
	} else {
		hrt.ups++
		if wasUp {
			// clear any progress
			hrt.downs = 0
		} else if hrt.ups >= hrt.thresholdUp {
			// going up
			atomic.StoreInt32(&hrt.state, 1)
			if hrt.OnUp != nil {
				hrt.OnUp(hrt.ups, hrt.downs)
			}
			hrt.downs = 0
		}
	}
	return err
}

// IsUp returns the current state, up (true) or down (false)
// Note that it defaults to down if no run has happened yet.
func (hrt *HealthChecker) IsUp() bool {
	return atomic.LoadInt32(&hrt.state) == 1
}
