package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

const MaxClawModel = "minimax-m2.7"

// MaxClawProvider handles Anthropic-format API (MiniMax via MaxClaw proxy)
type MaxClawProvider struct {
	name   string
	client *http.Client
}

func NewMaxClawProvider() *MaxClawProvider {
	return &MaxClawProvider{
		name: "maxclaw",
		client: &http.Client{
			Timeout: 600 * time.Second, // 10 min for complex prompts
			Transport: &http.Transport{
				MaxIdleConns:        500,
				MaxIdleConnsPerHost: 500,
				MaxConnsPerHost:     0, // unlimited
				IdleConnTimeout:     90 * time.Second,
			},
		},
	}
}

func (p *MaxClawProvider) Name() string {
	return p.name
}

func (p *MaxClawProvider) Call(request LLMRequest) LLMResponse {
	start := time.Now()

	// Honor caller token budget while keeping sane defaults/caps.
	maxTokens := request.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 4000
	}
	if maxTokens > 16000 {
		maxTokens = 16000
	}

	log.Printf("[maxclaw] model=%s tokens=%d", MaxClawModel, maxTokens)

	// Anthropic format
	payload := map[string]interface{}{
		"model": MaxClawModel,
		"messages": []map[string]string{
			{"role": "user", "content": request.Prompt},
		},
		"max_tokens": maxTokens,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return LLMResponse{Error: fmt.Errorf("marshal: %w", err), Provider: p.name}
	}

	req, err := http.NewRequest("POST", "http://100.107.233.111:9999/v1/messages", bytes.NewBuffer(jsonData))
	if err != nil {
		return LLMResponse{Error: fmt.Errorf("request: %w", err), Provider: p.name}
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("anthropic-version", "2023-06-01")

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

	// Parse Anthropic response format
	var result struct {
		Content []struct {
			Type     string `json:"type"`
			Text     string `json:"text"`
			Thinking string `json:"thinking"` // MiniMax thinking block
		} `json:"content"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return LLMResponse{Error: fmt.Errorf("parse: %w", err), Provider: p.name}
	}

	var text, thinking string
	for _, block := range result.Content {
		switch block.Type {
		case "text":
			if text == "" {
				text = block.Text
			}
		case "thinking":
			if thinking == "" {
				thinking = block.Thinking
			}
		}
	}

	if text == "" {
		return LLMResponse{Error: fmt.Errorf("no text in response"), Provider: p.name}
	}

	return LLMResponse{
		Content:  text,
		Thinking: thinking,
		Provider: p.name,
		Latency:  time.Since(start).Seconds(),
	}
}
