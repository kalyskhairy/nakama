package main

import "sync"

// runLocked runs f with mu held and always releases it, even if f panics.
func runLocked(mu *sync.Mutex, f func()) {
	mu.Lock()
	defer mu.Unlock()
	f()
}

// safeGo runs f in a goroutine and hands any panic to onPanic instead of crashing.
func safeGo(onPanic func(any), f func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				onPanic(r)
			}
		}()
		f()
	}()
}
