//go:build ignore

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// Reuse transport across all requests instead of creating a client per goroutine
var httpClient = &http.Client{
	Timeout: 180 * time.Second,
	Transport: &http.Transport{
		MaxIdleConns:        500,
		MaxIdleConnsPerHost: 500,
		MaxConnsPerHost:     0, // unlimited
		IdleConnTimeout:     90 * time.Second,
		DisableKeepAlives:   false,
	},
}

type result struct {
	latency time.Duration
	tokens  int
	err     error
}

func main() {
	fmt.Println("🔥 MaxClaw Stress Test")
	fmt.Println("======================")

	concurrencyLevels := []int{10, 100}
	prompt := `Generate 3 realistic user personas for a developer tool. Each has: name, age, role, income, budget. Output as JSON array.`

	// Pre-marshal the payload once — it's identical for every request
	payload, err := json.Marshal(map[string]interface{}{
		"model": "minimax-m2.5",
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
		"max_tokens": 2000,
	})
	if err != nil {
		panic(err)
	}

	for _, c := range concurrencyLevels {
		fmt.Printf("\n--- %d concurrent requests ---\n", c)
		results := runBatch(c, payload)
		printStats(results, c)
		time.Sleep(3 * time.Second)
	}
}

func runBatch(concurrency int, payload []byte) []result {
	results := make([]result, concurrency)
	var wg sync.WaitGroup
	var completed atomic.Int64

	wg.Add(concurrency)
	start := time.Now()

	for i := range concurrency {
		go func(idx int) {
			defer wg.Done()
			results[idx] = doRequest(payload)
			n := completed.Add(1)
			if n%50 == 0 || n == int64(concurrency) {
				fmt.Printf("  progress: %d/%d (%.1fs)\n", n, concurrency, time.Since(start).Seconds())
			}
		}(i)
	}

	wg.Wait()
	return results
}

func doRequest(payload []byte) result {
	start := time.Now()

	req, err := http.NewRequest("POST", "http://100.107.233.111:9999/v1/messages", bytes.NewReader(payload))
	if err != nil {
		return result{err: fmt.Errorf("new request: %w", err)}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := httpClient.Do(req)
	if err != nil {
		return result{err: fmt.Errorf("do: %w", err)}
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return result{err: fmt.Errorf("read body: %w", err)}
	}

	if resp.StatusCode != http.StatusOK {
		return result{err: fmt.Errorf("HTTP %d: %.200s", resp.StatusCode, body)}
	}

	var apiResp struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		Usage struct {
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return result{err: fmt.Errorf("unmarshal: %w", err)}
	}

	// Use real token count from API if available; fall back to estimate
	tokens := apiResp.Usage.OutputTokens
	if tokens == 0 {
		for _, block := range apiResp.Content {
			if block.Type == "text" {
				tokens = len(block.Text) / 4
				break
			}
		}
	}
	if tokens == 0 {
		return result{err: fmt.Errorf("empty response (%d blocks, %d bytes)", len(apiResp.Content), len(body))}
	}

	return result{latency: time.Since(start), tokens: tokens}
}

func printStats(results []result, total int) {
	var successes int
	var totalTokens int
	var totalLatency time.Duration
	latencies := make([]float64, 0, total)

	for _, r := range results {
		if r.err != nil {
			continue
		}
		successes++
		totalTokens += r.tokens
		totalLatency += r.latency
		latencies = append(latencies, r.latency.Seconds())
	}

	failures := total - successes
	if failures > 0 {
		fmt.Printf("  ❌ %d failed\n", failures)
	}
	if successes == 0 {
		fmt.Printf("  ❌ All %d failed\n", total)
		return
	}

	sort.Float64s(latencies)
	avgLat := totalLatency.Seconds() / float64(successes)
	wallClock := latencies[len(latencies)-1] // max = wall clock for parallel batch

	fmt.Printf("  ✅ %d/%d succeeded\n", successes, total)
	fmt.Printf("  🚀 Combined throughput: %.0f tok/s\n", float64(totalTokens)/wallClock)
	fmt.Printf("  📊 Per-request avg:     %.0f tok/s\n", float64(totalTokens)/totalLatency.Seconds()*float64(1))
	fmt.Printf("  ⏱  Latency  avg=%.1fs  wall=%.1fs\n", avgLat, wallClock)
	fmt.Printf("  ⏱  Latency  p50=%.1fs  p95=%.1fs  p99=%.1fs  max=%.1fs\n",
		percentile(latencies, 0.50),
		percentile(latencies, 0.95),
		percentile(latencies, 0.99),
		latencies[len(latencies)-1],
	)
	fmt.Printf("  📦 Total tokens: %d\n", totalTokens)
}

func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := p * float64(len(sorted)-1)
	lower := int(math.Floor(idx))
	upper := int(math.Ceil(idx))
	if lower == upper {
		return sorted[lower]
	}
	frac := idx - float64(lower)
	return sorted[lower]*(1-frac) + sorted[upper]*frac
}