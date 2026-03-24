package main

import (
	"fmt"
	"sync"
	"sync/atomic"
)

// MultiProvider distributes calls across multiple providers using round-robin
type MultiProvider struct {
	providers []Provider
	index     uint64
	mu        sync.Mutex
	stats     map[string]*ProviderStats
}

type ProviderStats struct {
	Calls     int
	Successes int
	Failures  int
}

func NewMultiProvider(providers []Provider) *MultiProvider {
	stats := make(map[string]*ProviderStats)
	for _, p := range providers {
		stats[p.Name()] = &ProviderStats{}
	}
	return &MultiProvider{
		providers: providers,
		stats:     stats,
	}
}

func (mp *MultiProvider) Call(request LLMRequest) LLMResponse {
	if len(mp.providers) == 0 {
		return LLMResponse{Error: fmt.Errorf("no providers available")}
	}

	// Round-robin: increment and wrap
	idx := atomic.AddUint64(&mp.index, 1) % uint64(len(mp.providers))
	provider := mp.providers[idx]

	// Call provider
	resp := provider.Call(request)

	// Track stats
	mp.mu.Lock()
	if stat, ok := mp.stats[resp.Provider]; ok {
		stat.Calls++
		if resp.Error == nil {
			stat.Successes++
		} else {
			stat.Failures++
		}
	}
	mp.mu.Unlock()

	return resp
}

func (mp *MultiProvider) Name() string {
	return fmt.Sprintf("MultiProvider(%d providers)", len(mp.providers))
}

func (mp *MultiProvider) Stats() map[string]ProviderStat {
	mp.mu.Lock()
	defer mp.mu.Unlock()

	result := make(map[string]ProviderStat)
	for name, stat := range mp.stats {
		result[name] = ProviderStat{
			Calls:     stat.Calls,
			Successes: stat.Successes,
			Failures:  stat.Failures,
		}
	}
	return result
}
