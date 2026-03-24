package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Provider interface for LLM providers
type Provider interface {
	Call(request LLMRequest) LLMResponse
	Name() string
}

// RateLimitedProvider wraps a provider with rate limiting
type RateLimitedProvider struct {
	inner    *BaseProvider
	minDelay time.Duration
	lastCall time.Time
	mu       sync.Mutex
}

// BaseProvider implements common HTTP logic
type BaseProvider struct {
	name   string
	url    string
	apiKey string
	model  string
	client *http.Client
}

func NewBaseProvider(name, url, apiKey, model string, timeout time.Duration) *BaseProvider {
	return &BaseProvider{
		name:   name,
		url:    url,
		apiKey: apiKey,
		model:  model,
		client: &http.Client{Timeout: timeout},
	}
}

func NewRateLimitedProvider(inner *BaseProvider, minDelay time.Duration) *RateLimitedProvider {
	return &RateLimitedProvider{
		inner:    inner,
		minDelay: minDelay,
	}
}

func (p *RateLimitedProvider) Name() string { return p.inner.name }

func (p *RateLimitedProvider) Call(request LLMRequest) LLMResponse {
	// Rate limit: wait if needed
	p.mu.Lock()
	elapsed := time.Since(p.lastCall)
	if elapsed < p.minDelay {
		time.Sleep(p.minDelay - elapsed)
	}
	p.lastCall = time.Now()
	p.mu.Unlock()

	// Try up to 5 times with exponential backoff
	for attempt := 0; attempt < 5; attempt++ {
		resp := p.inner.Call(request)
		if resp.Error == nil {
			if attempt > 0 {
				fmt.Printf(" ✅ %s succeeded (attempt %d)\n", p.inner.name, attempt+1)
			}
			return resp
		}
		
		// Retry on rate limit (429) or timeout errors
		if attempt < 4 {
			errStr := resp.Error.Error()
			shouldRetry := len(errStr) > 8 && errStr[:8] == "HTTP 429" ||
				strings.Contains(errStr, "timeout") ||
				strings.Contains(errStr, "context deadline exceeded")
			
			if shouldRetry {
				// Exponential backoff: 2s, 4s, 8s, 16s
				delay := time.Duration(1<<uint(attempt)) * 2 * time.Second
				fmt.Printf("\n⏳ %s error (attempt %d/%d), backoff %v", p.inner.name, attempt+1, 5, delay)
				time.Sleep(delay)
				continue
			}
		}
		fmt.Printf("\n❌ %s failed: %v\n", p.inner.name, resp.Error)
		return resp
	}
	return LLMResponse{Error: fmt.Errorf("max retries exceeded"), Provider: p.inner.name}
}

func (p *BaseProvider) Name() string {
	return p.name
}

func (p *BaseProvider) Call(request LLMRequest) LLMResponse {
	start := time.Now()

	payload := map[string]interface{}{
		"model": p.model,
		"messages": []map[string]string{
			{"role": "user", "content": request.Prompt},
		},
		"max_tokens":  request.MaxTokens,
		"temperature": request.Temperature,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return LLMResponse{Error: fmt.Errorf("marshal: %w", err), Provider: p.name}
	}

	req, err := http.NewRequest("POST", p.url, bytes.NewBuffer(jsonData))
	if err != nil {
		return LLMResponse{Error: fmt.Errorf("request: %w", err), Provider: p.name}
	}

	req.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return LLMResponse{Error: fmt.Errorf("HTTP: %w", err), Provider: p.name}
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return LLMResponse{Error: fmt.Errorf("read: %w", err), Provider: p.name}
	}

	if resp.StatusCode != 200 {
		return LLMResponse{
			Error:    fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncate(string(body), 200)),
			Provider: p.name,
		}
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return LLMResponse{Error: fmt.Errorf("parse: %w", err), Provider: p.name}
	}

	// Extract content (OpenAI format)
	choices, ok := result["choices"].([]interface{})
	if !ok || len(choices) == 0 {
		return LLMResponse{Error: fmt.Errorf("no choices in response"), Provider: p.name}
	}

	choice, ok := choices[0].(map[string]interface{})
	if !ok {
		return LLMResponse{Error: fmt.Errorf("invalid choice format"), Provider: p.name}
	}

	message, ok := choice["message"].(map[string]interface{})
	if !ok {
		return LLMResponse{Error: fmt.Errorf("invalid message format"), Provider: p.name}
	}

	content, ok := message["content"].(string)
	if !ok {
		return LLMResponse{Error: fmt.Errorf("content not string"), Provider: p.name}
	}

	return LLMResponse{
		Content:  content,
		Provider: p.name,
		Latency:  time.Since(start).Seconds(),
	}
}

func truncate(s string, maxLen int) string {
	if len(s) > maxLen {
		return s[:maxLen] + "..."
	}
	return s
}

// HealthCheck tests if a provider is accessible
func (p *BaseProvider) HealthCheck() bool {
	req := LLMRequest{
		Prompt:      "test",
		MaxTokens:   5,
		Temperature: 0.7,
	}
	
	resp := p.Call(req)
	return resp.Error == nil
}

// InitializeProviders creates all provider instances with rate limiting
func InitializeProviders(names []string) []Provider {
	configs := map[string]struct {
		url, key, model string
		timeout         time.Duration
		rateDelay       time.Duration
	}{
		"openrouter": {
			url:       "https://openrouter.ai/api/v1/chat/completions",
			key:       "",
			model:     "arcee-ai/trinity-large-preview:free",
			timeout:   180 * time.Second,
			rateDelay: 500 * time.Millisecond,
		},
		"groq": {
			url:       "https://api.groq.com/openai/v1/chat/completions",
			key:       "",
			model:     "llama-3.3-70b-versatile",
			timeout:   90 * time.Second,
			rateDelay: 3 * time.Second,
		},
		"cerebras": {
			url:       "https://api.cerebras.ai/v1/chat/completions",
			key:       "",
			model:     "llama3.1-8b",
			timeout:   90 * time.Second,
			rateDelay: 3 * time.Second,
		},
		"lmstudio": {
			url:       "http://100.107.233.111:1234/v1/chat/completions", // MacBook via Tailscale
			key:       "",
			model:     "local-model", // LMStudio uses whatever model is loaded
			timeout:   120 * time.Second,
			rateDelay: 0,
		},
		"github": {
			url:       "https://models.inference.ai.azure.com/chat/completions",
			key:       "",
			model:     "gpt-4o-mini",
			timeout:   90 * time.Second,
			rateDelay: 1 * time.Second,
		},
	}

	var providers []Provider
	fmt.Println("\n🔍 Testing provider health...")
	
	for _, name := range names {
		// Special case: MaxClaw uses Anthropic API format
		if name == "maxclaw" {
			fmt.Printf("  • maxclaw... ")
			maxclaw := NewMaxClawProvider()
			testReq := LLMRequest{Prompt: "Say hello", MaxTokens: 4000}  // Use 4k tokens (provider will enforce min)
			testResp := maxclaw.Call(testReq)
			if testResp.Error == nil {
				fmt.Println("✅")
				providers = append(providers, maxclaw)
			} else {
				fmt.Printf("❌ (%v)\n", testResp.Error)
			}
			continue
		}
		
		cfg, ok := configs[name]
		if !ok {
			continue
		}
		base := NewBaseProvider(name, cfg.url, cfg.key, cfg.model, cfg.timeout)
		
		// Health check with 10s timeout
		fmt.Printf("  • %s... ", name)
		testBase := NewBaseProvider(name, cfg.url, cfg.key, cfg.model, 10*time.Second)
		if testBase.HealthCheck() {
			fmt.Println("✅")
			if cfg.rateDelay > 0 {
				providers = append(providers, NewRateLimitedProvider(base, cfg.rateDelay))
			} else {
				providers = append(providers, base)
			}
		} else {
			fmt.Println("❌ (skipping)")
		}
	}
	
	fmt.Printf("\n✅ %d/%d providers ready\n", len(providers), len(names))
	return providers
}
