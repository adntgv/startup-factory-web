package main

import (
	"fmt"
	"sync"
)

// WorkerPoolV2 with provider preference for specific task types
type WorkerPoolV2 struct {
	taskQueue   chan Task
	providers   map[string]Provider  // name -> provider
	stats       map[string]*ProviderStat
	statsMutex  sync.RWMutex
	wg          sync.WaitGroup
	
	// Provider preferences for task types
	preferences map[string][]string  // taskType -> [preferredProvider1, preferredProvider2, ...]
}

func NewWorkerPoolV2(providers []Provider, workersPerProvider int, prefs map[string][]string) *WorkerPoolV2 {
	pool := &WorkerPoolV2{
		taskQueue:   make(chan Task, 100),
		providers:   make(map[string]Provider),
		stats:       make(map[string]*ProviderStat),
		preferences: prefs,
	}
	
	// Index providers by name
	for _, p := range providers {
		pool.providers[p.Name()] = p
		pool.stats[p.Name()] = &ProviderStat{}
	}
	
	// Start workers for each provider
	for _, provider := range providers {
		for i := 0; i < workersPerProvider; i++ {
			pool.wg.Add(1)
			go pool.worker(provider)
		}
	}
	
	fmt.Printf("✅ Worker pool started: %d providers × %d workers = %d total workers\n",
		len(providers), workersPerProvider, len(providers)*workersPerProvider)
	
	return pool
}

func (p *WorkerPoolV2) worker(provider Provider) {
	defer p.wg.Done()
	
	for task := range p.taskQueue {
		// Check if this provider is preferred for this task type
		if preferred, ok := p.preferences[task.Type]; ok {
			// Skip if this provider is not in the preference list
			isPreferred := false
			for _, pref := range preferred {
				if pref == provider.Name() {
					isPreferred = true
					break
				}
			}
			if !isPreferred {
				// Put task back in queue for preferred worker
				go func(t Task) { p.taskQueue <- t }(task)
				continue
			}
		}
		
		// Log which provider handles this
		fmt.Printf("  → %s handling %s\n", provider.Name(), task.Type)
		
		// Execute
		p.statsMutex.Lock()
		p.stats[provider.Name()].Calls++
		p.statsMutex.Unlock()
		
		response := provider.Call(task.Request)
		
		p.statsMutex.Lock()
		if response.Error != nil {
			p.stats[provider.Name()].Failures++
		} else {
			p.stats[provider.Name()].Successes++
			p.stats[provider.Name()].TotalTime += response.Latency
		}
		p.statsMutex.Unlock()
		
		task.ResultCh <- TaskResult{
			Content:  response.Content,
			Provider: response.Provider,
			Latency:  response.Latency,
			Error:    response.Error,
		}
	}
}

func (p *WorkerPoolV2) Submit(taskType string, request LLMRequest) TaskResult {
	resultCh := make(chan TaskResult, 1)
	
	p.taskQueue <- Task{
		Type:     taskType,
		Request:  request,
		ResultCh: resultCh,
	}
	
	return <-resultCh
}

func (p *WorkerPoolV2) Close() {
	close(p.taskQueue)
	p.wg.Wait()
}

func (p *WorkerPoolV2) GetStats() map[string]*ProviderStat {
	p.statsMutex.RLock()
	defer p.statsMutex.RUnlock()
	
	statsCopy := make(map[string]*ProviderStat)
	for name, stat := range p.stats {
		statsCopy[name] = stat
	}
	return statsCopy
}
