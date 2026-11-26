package main

import (
	"sync"
	"testing"
	"time"
)

func TestRunLockedUnlocksOnPanic(t *testing.T) {
	var mu sync.Mutex
	func() {
		defer func() { recover() }()
		runLocked(&mu, func() { panic("boom") })
	}()
	if !mu.TryLock() {
		t.Fatal("mutex still locked after a panic")
	}
}

func TestSafeGoReportsPanic(t *testing.T) {
	got := make(chan any, 1)
	safeGo(func(r any) { got <- r }, func() { panic("boom") })
	select {
	case r := <-got:
		if r != "boom" {
			t.Fatalf("got %v", r)
		}
	case <-time.After(time.Second):
		t.Fatal("panic in goroutine was not reported")
	}
}
