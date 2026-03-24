package main

import "path/filepath"
import "testing"

func TestLearnWithModeScoreOnlyUpdatesEpoch(t *testing.T) {
	l := NewLearner(filepath.Join(t.TempDir(), "learning_state.json"))

	before := l.Weights.Epoch
	startWeight := l.Weights.Weights["technology"]["ai_llms"]

	scored := []ScoredIdea{
		{
			Idea:    StartupIdea{Coordinates: map[string]string{"technology": "ai_llms", "category": "ai_productivity"}},
			Metrics: SimulationMetrics{CompositeScore: 0.8, ConversionRate: 0.4},
		},
		{
			Idea:    StartupIdea{Coordinates: map[string]string{"technology": "iot", "category": "travel"}},
			Metrics: SimulationMetrics{CompositeScore: 0.2, ConversionRate: 0.05},
		},
	}

	pass := l.LearnWithMode(scored, nil, "score_only", 0.55)
	if !pass {
		t.Fatalf("expected score_only learning to pass")
	}
	if l.Weights.Epoch != before+1 {
		t.Fatalf("expected epoch increment; got before=%d after=%d", before, l.Weights.Epoch)
	}
	if l.Weights.Weights["technology"]["ai_llms"] <= startWeight {
		t.Fatalf("expected ai_llms weight to increase")
	}
}
