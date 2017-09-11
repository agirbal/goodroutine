package goodroutine

import "testing"
import "time"

func TestRecover(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Panic %v was not recovered in routine", r)
		}
	}()

	called := make(chan bool)
	f := func() error {
		called <- true
		panic("blah")
	}
	rt := NewIntervalRoutine(f, 0, 0)
	rt.Start()
	defer rt.Stop()
	select {
	case <-called:
	case <-time.Tick(10 * time.Millisecond):
		t.Error("function was not called")
	}
}
