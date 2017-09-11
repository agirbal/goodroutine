package goodroutine

import "testing"
import "errors"

func TestHealthChecker(t *testing.T) {
	type testRun struct {
		zerr     error
		numRuns  int
		current  bool
		expected bool
	}

	tests := []struct {
		name          string
		initial       bool
		thresholdUp   int
		thresholdDown int
		runs          []testRun
	}{
		{"updownup", true, 3, 5, []testRun{
			{nil, 10, true, true},
			{errors.New("error"), 10, true, false},
			{nil, 10, false, true},
		}},
		{"staydown", true, 3, 3, []testRun{
			{errors.New("error"), 10, true, false},
			{nil, 2, false, false},
			{errors.New("error"), 10, false, false},
			{nil, 2, false, false},
			{errors.New("error"), 10, false, false},
			{nil, 3, false, true},
		}},
		{"stayup", false, 3, 3, []testRun{
			{nil, 10, false, true},
			{errors.New("error"), 2, true, true},
			{nil, 10, true, true},
			{errors.New("error"), 2, true, true},
			{nil, 10, true, true},
			{errors.New("error"), 3, true, false},
		}},
		{"nothreshold", false, 0, -42, []testRun{
			{nil, 1, false, true},
			{errors.New("error"), 1, true, false},
			{nil, 1, false, true},
			{errors.New("error"), 1, true, false},
			{nil, 10, false, true},
			{errors.New("error"), 3, true, false},
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			f := func() error {
				return nil
			}
			hc := NewHealthChecker(f, tt.initial, tt.thresholdUp, tt.thresholdDown)

			// test default
			hc.OnUp = func(numUps int, numDowns int) {
				t.Errorf("Should not be called, numUps=%d, numDowns=%d", numUps, numDowns)
			}
			hc.OnDown = func(numUps int, numDowns int) {
				t.Errorf("Should not be called, numUps=%d, numDowns=%d", numUps, numDowns)
			}
			if g, w := hc.IsUp(), tt.initial; g != w {
				t.Errorf("Initial state invalid, got=%v, want=%v", g, w)
			}

			for _, run := range tt.runs {
				testStateChange(t, hc, run.zerr, run.numRuns, run.current, run.expected)
			}
		})
	}
}

func testStateChange(t *testing.T, hc *HealthChecker, checkErr error, numRuns int, current bool, expected bool) {
	hc.f = func() error {
		return checkErr
	}
	callback := false
	hc.OnUp = func(numUps int, numDowns int) {
		if expected && !current {
			callback = true
			if g, w := numUps, hc.thresholdUp; g != w {
				t.Errorf("Incorrect param, got=%v, want=%v", g, w)
			}
		} else {
			t.Errorf("Should not be called, numUps=%d, numDowns=%d", numUps, numDowns)
		}
	}
	hc.OnDown = func(numUps int, numDowns int) {
		if !expected && current {
			callback = true
			if g, w := numDowns, hc.thresholdDown; g != w {
				t.Errorf("Incorrect param, got=%v, want=%v", g, w)
			}
		} else {
			t.Errorf("Should not be called, numUps=%d, numDowns=%d", numUps, numDowns)
		}
	}

	for i := 0; i < numRuns; i++ {
		err := hc.Run()
		if g, w := err, checkErr; g != w {
			t.Errorf("Error does not match, got=%v, want=%v", g, w)
		}
		if expected != current {
			// it will take until threshold
			threshold := hc.thresholdUp
			if !expected {
				threshold = hc.thresholdDown
			}
			if i < threshold-1 {
				if g, w := hc.IsUp(), !expected; g != w {
					t.Errorf("State changed too quickly at i=%d, got=%v, want=%v", i, g, w)
				}
			} else {
				if g, w := hc.IsUp(), expected; g != w {
					t.Errorf("State should have changed at i=%d, got=%v, want=%v", i, g, w)
				}
			}
		} else {
			if g, w := hc.IsUp(), expected; g != w {
				t.Errorf("State should not have changed at i=%d, got=%v, want=%v", i, g, w)
			}
		}
	}
	if expected != current && !callback {
		t.Errorf("Callback not called")
	}
}
