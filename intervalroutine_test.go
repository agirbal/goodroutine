package goodroutine

import (
	"errors"
	"testing"
	"time"
)

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

func TestTrigger(t *testing.T) {
	called := make(chan bool)
	f := func() error {
		called <- true
		return nil
	}
	rt := NewIntervalRoutine(f, 0, 0)
	rt.Start()
	defer rt.Stop()
	// should be called at start
	select {
	case <-called:
	case <-time.Tick(10 * time.Millisecond):
		t.Error("function was not called")
	}

	for i := 0; i < 100; i++ {
		rt.TriggerRun()
		select {
		case <-called:
		case <-time.Tick(10 * time.Millisecond):
			t.Error("function was not called")
		}
	}
}

func TestTriggerBlock(t *testing.T) {
	called := make(chan bool)
	barrier := make(chan bool)
	f := func() error {
		called <- true
		<-barrier
		return nil
	}
	rt := NewIntervalRoutine(f, 0, 0)
	rt.Start()
	defer rt.Stop()
	// should be called at start
	select {
	case <-called:
	case <-time.Tick(10 * time.Millisecond):
		t.Error("function was not called")
	}
	// here we're still in function due to barrier

	// do many triggers
	// they should never block
	triggersDone := make(chan bool)
	go func() {
		for i := 0; i < 100; i++ {
			rt.TriggerRun()
		}
		triggersDone <- true
	}()
	select {
	case <-triggersDone:
	case <-time.Tick(10 * time.Millisecond):
		t.Error("triggers took too long")
	}

	// release barrier
	close(barrier)
	// only one more call should be made
	select {
	case <-called:
	case <-time.Tick(10 * time.Millisecond):
		t.Error("function was not called")
	}
	select {
	case <-called:
		t.Error("function was called too many times")
	case <-time.Tick(10 * time.Millisecond):
	}
}

func TestStop(t *testing.T) {
	called := make(chan bool)
	barrier := make(chan bool)
	f := func() error {
		called <- true
		<-barrier
		return nil
	}
	rt := NewIntervalRoutine(f, 0, 0)
	rt.Start()
	// should be called at start
	select {
	case <-called:
	case <-time.Tick(10 * time.Millisecond):
		t.Error("function was not called")
	}
	// here we're stuck in the function
	// trigger another run, then stop
	rt.TriggerRun()
	rt.Stop()

	// release barrier
	close(barrier)
	// no more calls should be made
	// this relies on select priority, and may only fail sometimes
	select {
	case <-called:
		t.Error("function called after stop()")
	case <-time.Tick(10 * time.Millisecond):
	}
}

func TestInterval(t *testing.T) {
	called := make(chan bool)
	f := func() error {
		called <- true
		return nil
	}
	interval := 100 * time.Millisecond
	rt := NewIntervalRoutine(f, interval, 0)
	rt.Start()
	// should be called at start
	select {
	case <-called:
	case <-time.Tick(10 * time.Millisecond):
		t.Error("function was not called")
	}

	// should be called at interval
	for i := 0; i < 5; i++ {
		time.Sleep(interval)
		select {
		case <-called:
		case <-time.Tick(10 * time.Millisecond):
			t.Error("function was not called")
		}
	}

	rt.Stop()
	// no more calls should be made
	select {
	case <-called:
		t.Error("function called after stop()")
	case <-time.Tick(interval):
	}
}

func TestRetryInterval(t *testing.T) {
	called := make(chan bool)
	f := func() error {
		called <- true
		return errors.New("error")
	}
	run := 1 * time.Second
	retry := 100 * time.Millisecond
	rt := NewIntervalRoutine(f, run, retry)
	rt.Start()
	// should be called at start
	select {
	case <-called:
	case <-time.Tick(10 * time.Millisecond):
		t.Error("function was not called")
	}

	// should be called at exponential interval
	sleep := retry
	for i := 0; i < 5; i++ {
		time.Sleep(sleep)
		select {
		case <-called:
		case <-time.Tick(10 * time.Millisecond):
			t.Error("function was not called")
		}

		sleep = 2 * sleep
		if retry > run {
			sleep = run
		}
	}

	// now again but no backoff
	rt.RetryBackoffDisabled = true
	time.Sleep(run)
	select {
	case <-called:
	case <-time.Tick(10 * time.Millisecond):
		t.Error("function was not called")
	}

	for i := 0; i < 5; i++ {
		time.Sleep(retry)
		select {
		case <-called:
		case <-time.Tick(10 * time.Millisecond):
			t.Error("function was not called")
		}
	}

	rt.Stop()
	// no more calls should be made
	select {
	case <-called:
		t.Error("function called after stop()")
	case <-time.Tick(run):
	}
}
