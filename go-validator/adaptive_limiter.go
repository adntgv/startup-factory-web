package main

import (
	"fmt"
	"math"
	"sync"
	"time"
)

// AdaptiveLimiter dynamically adjusts concurrency based on success rate and latency.
type AdaptiveLimiter struct {
	name    string
	current int
	min     int
	max     int

	mu       sync.Mutex
	sem      chan struct{}
	results  []callResult
	window   int // sliding window size
	adjusted int // number of adjustments made
}

type callResult struct {
	success bool
	latency time.Duration
}

// NewAdaptiveLimiter creates a limiter with initial/min/max concurrency.
func NewAdaptiveLimiter(name string, initial, min, max, window int) *AdaptiveLimiter {
	al := &AdaptiveLimiter{
		name:    name,
		current: initial,
		min:     min,
		max:     max,
		sem:     make(chan struct{}, max), // sized to max; we control via current
		window:  window,
	}
	// Fill semaphore to current level
	for i := 0; i < initial; i++ {
		al.sem <- struct{}{}
	}
	return al
}

// Acquire blocks until a slot is available.
func (al *AdaptiveLimiter) Acquire() {
	<-al.sem
}

// Release returns a slot.
func (al *AdaptiveLimiter) Release() {
	al.sem <- struct{}{}
}

// RecordResult records success/failure + latency and triggers adjustment if window is full.
func (al *AdaptiveLimiter) RecordResult(success bool, latency time.Duration) {
	al.mu.Lock()
	al.results = append(al.results, callResult{success: success, latency: latency})
	shouldAdjust := len(al.results) >= al.window
	al.mu.Unlock()

	if shouldAdjust {
		al.adjust()
	}
}

func (al *AdaptiveLimiter) adjust() {
	al.mu.Lock()
	defer al.mu.Unlock()

	if len(al.results) == 0 {
		return
	}

	// Calculate stats from recent results
	successes := 0
	var totalLatency time.Duration
	for _, r := range al.results {
		if r.success {
			successes++
		}
		totalLatency += r.latency
	}

	successRate := float64(successes) / float64(len(al.results))
	avgLatency := totalLatency / time.Duration(len(al.results))

	oldCurrent := al.current
	newCurrent := al.current

	if successRate < 0.70 || avgLatency > 120*time.Second {
		// Reduce by 20%
		newCurrent = int(math.Round(float64(al.current) * 0.8))
	} else if successRate > 0.95 && avgLatency < 60*time.Second {
		// Increase by 10%
		newCurrent = int(math.Round(float64(al.current) * 1.1))
	}

	// Clamp
	if newCurrent < al.min {
		newCurrent = al.min
	}
	if newCurrent > al.max {
		newCurrent = al.max
	}

	if newCurrent != oldCurrent {
		// Adjust semaphore capacity
		diff := newCurrent - oldCurrent
		if diff > 0 {
			// Add slots
			for i := 0; i < diff; i++ {
				select {
				case al.sem <- struct{}{}:
				default:
				}
			}
		} else {
			// Remove slots (drain)
			for i := 0; i < -diff; i++ {
				select {
				case <-al.sem:
				default:
				}
			}
		}
		al.current = newCurrent
		al.adjusted++
		fmt.Printf("  [Adaptive %s] %d → %d (success %.0f%%, latency %.1fs, window %d)\n",
			al.name, oldCurrent, newCurrent, successRate*100, avgLatency.Seconds(), len(al.results))
	}

	// Clear window
	al.results = al.results[:0]
}

// Current returns the current concurrency level.
func (al *AdaptiveLimiter) Current() int {
	al.mu.Lock()
	defer al.mu.Unlock()
	return al.current
}

// Stats returns summary string.
func (al *AdaptiveLimiter) Stats() string {
	al.mu.Lock()
	defer al.mu.Unlock()
	return fmt.Sprintf("%s: current=%d (adjusted %d times)", al.name, al.current, al.adjusted)
}
