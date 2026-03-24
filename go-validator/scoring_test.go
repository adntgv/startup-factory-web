package main

import "testing"

func TestWilsonIntervalBounds(t *testing.T) {
	low, high := WilsonInterval(3, 10, 1.96)
	if low < 0 || high > 1 || low >= high {
		t.Fatalf("invalid Wilson interval: low=%.4f high=%.4f", low, high)
	}
}

func TestCompositeScoreFromStatsPenalizesHigherCPL(t *testing.T) {
	base := CompositeScoreFromStats(0.2, 7.0, 6.0, 10)
	worse := CompositeScoreFromStats(0.2, 7.0, 6.0, 70)
	if worse >= base {
		t.Fatalf("expected higher CPL to reduce score; base=%.4f worse=%.4f", base, worse)
	}
}
