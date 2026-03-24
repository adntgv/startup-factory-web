package main

import (
	"fmt"
	"sync"
)

// WorkerPool manages a pool of workers across multiple providers
type WorkerPool struct {
	taskQueue   chan Task
	providers   []Provider
	stats       map[string]*ProviderStat
	statsMutex  sync.RWMutex
	wg          sync.WaitGroup
}

// NewWorkerPool creates a new worker pool
func NewWorkerPool(providers []Provider, workersPerProvider int) *WorkerPool {
	pool := &WorkerPool{
		taskQueue: make(chan Task, 100), // Buffered channel
		providers: providers,
		stats:     make(map[string]*ProviderStat),
	}
	
	// Initialize stats
	for _, p := range providers {
		pool.stats[p.Name()] = &ProviderStat{}
	}
	
	// Start workers
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

// worker processes tasks from the queue
func (p *WorkerPool) worker(provider Provider) {
	defer p.wg.Done()
	
	for task := range p.taskQueue {
		// Update stats - call
		p.statsMutex.Lock()
		p.stats[provider.Name()].Calls++
		p.statsMutex.Unlock()
		
		// Log which provider is handling this task
		fmt.Printf("  → %s handling %s\n", provider.Name(), task.Type)
		
		// Execute task
		response := provider.Call(task.Request)
		
		// Update stats - result
		p.statsMutex.Lock()
		if response.Error != nil {
			p.stats[provider.Name()].Failures++
		} else {
			p.stats[provider.Name()].Successes++
			p.stats[provider.Name()].TotalTime += response.Latency
		}
		p.statsMutex.Unlock()
		
		// Send result
		task.ResultCh <- TaskResult{
			Content:  response.Content,
			Provider: response.Provider,
			Latency:  response.Latency,
			Error:    response.Error,
		}
	}
}

// Submit submits a task to the pool and waits for result
func (p *WorkerPool) Submit(taskType string, request LLMRequest) TaskResult {
	resultCh := make(chan TaskResult, 1)
	
	task := Task{
		Type:     taskType,
		Request:  request,
		ResultCh: resultCh,
	}
	
	p.taskQueue <- task
	result := <-resultCh
	
	return result
}

// Close shuts down the worker pool
func (p *WorkerPool) Close() {
	close(p.taskQueue)
	p.wg.Wait()
}

// GetStats returns current provider statistics
func (p *WorkerPool) GetStats() map[string]ProviderStat {
	p.statsMutex.RLock()
	defer p.statsMutex.RUnlock()
	
	stats := make(map[string]ProviderStat)
	for name, stat := range p.stats {
		stats[name] = *stat
	}
	return stats
}
