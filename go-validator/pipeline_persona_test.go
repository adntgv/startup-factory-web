package main

import (
	"fmt"
	"sync"
	"testing"
)

type stubLLM struct {
	responses []LLMResponse
	requests  []LLMRequest
	mu        sync.Mutex
}

func (s *stubLLM) Name() string { return "stub" }

func (s *stubLLM) Call(req LLMRequest) LLMResponse {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.requests = append(s.requests, req)
	if len(s.responses) == 0 {
		return LLMResponse{Error: fmt.Errorf("no stub response")}
	}
	resp := s.responses[0]
	s.responses = s.responses[1:]
	return resp
}

func TestGeneratePersonasViaMaxClaw_RetriesAndParses(t *testing.T) {
	stub := &stubLLM{responses: []LLMResponse{
		{Content: `<think>truncated without close`},
		{Content: `[
			{"name":"Ava","age":31,"role":"PM","company_size":"11-50","experience_years":8,"pain_level":7,"skepticism":6,"current_workflow":"docs","current_tools":"Notion","personality":"pragmatic","archetype":"optimizer","budget":"moderate","decision_authority":"self"},
			{"name":"Ben","age":29,"role":"Engineer","company_size":"1-10","experience_years":5,"pain_level":8,"skepticism":7,"current_workflow":"scripts","current_tools":"VS Code","personality":"skeptical","archetype":"builder","budget":"tight","decision_authority":"needs_approval"}
		]`},
	}}

	p := &Pipeline{
		maxclaw: stub,
		config:  PipelineConfig{NumPersonas: 2, MaxConcurrent: 4},
	}

	personas := p.generatePersonasBatched("AI workflow assistant", 2)
	if len(personas) != 2 {
		t.Fatalf("expected 2 personas, got %d", len(personas))
	}

	if len(stub.requests) != 2 {
		t.Fatalf("expected 2 LLM calls (retry then success), got %d", len(stub.requests))
	}

}
