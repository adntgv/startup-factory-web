package main

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// ScoreSimulation calculates composite metrics from simulation results
// Ported from scoring.py score_simulation()
func ScoreSimulation(results []SimulationResult) SimulationMetrics {
	if len(results) == 0 {
		return SimulationMetrics{
			SimulatedCPL:     999,
			FrictionScore:    10,
			ConversionCILow:  0,
			ConversionCIHigh: 1,
		}
	}

	conversions := 0
	rejections := 0
	errors := 0
	totalCPL := 0.0
	totalIntent := 0.0
	frictionCount := 0
	totalImpression := 0.0
	totalRelevance := 0.0
	hopeGaps := 0 // Track unmet hopes

	for _, r := range results {
		switch r.Status {
		case "converted":
			conversions++
		case "rejected":
			rejections++
		case "error":
			errors++
		default:
			// Fallback for old data without Status field
			if r.Converted {
				conversions++
			} else {
				rejections++
			}
		}
		totalCPL += r.CPLEquivalent
		totalIntent += float64(r.IntentStrength)
		frictionCount += len(r.FrictionPoints)
		totalImpression += float64(r.ImpressionScore)
		totalRelevance += float64(r.RelevanceScore)
		
		// Track hope-delivery gap
		if !r.HopeMet {
			hopeGaps++
		}
	}

	n := float64(len(results))
	validPersonas := conversions + rejections
	convRate := 0.0
	if validPersonas > 0 {
		convRate = float64(conversions) / float64(validPersonas)
	}

	avgCPL := totalCPL / n
	avgIntent := totalIntent / n
	frictionScore := float64(frictionCount) / n
	avgImpression := totalImpression / n
	avgRelevance := totalRelevance / n

	ciLow, ciHigh := WilsonInterval(conversions, validPersonas, 1.96)
	uncertaintyPenalty := (ciHigh - ciLow) * 0.2
	
	// Hope-delivery gap penalty: -0.15 per unmet hope (15% weight)
	hopeGapRate := 0.0
	if validPersonas > 0 {
		hopeGapRate = float64(hopeGaps) / float64(validPersonas)
	}
	hopeGapPenalty := hopeGapRate * 0.15
	
	composite := CompositeScoreFromStats(convRate, avgRelevance, avgImpression, avgCPL) - uncertaintyPenalty - hopeGapPenalty

	// Score: scaled composite for compatibility with prior reporting
	score := composite * 10

	return SimulationMetrics{
		Score:              score,
		CompositeScore:     composite,
		ConversionRate:     convRate,
		ConversionCILow:    ciLow,
		ConversionCIHigh:   ciHigh,
		UncertaintyPenalty: uncertaintyPenalty,
		SimulatedCPL:       avgCPL,
		IntentStrength:     avgIntent,
		FrictionScore:      frictionScore,
		AvgImpression:      avgImpression,
		AvgRelevance:       avgRelevance,
		Conversions:        conversions,
		Rejections:         rejections,
		Errors:             errors,
		TotalPersonas:      len(results),
		ValidPersonas:      validPersonas,
	}
}

// CompositeScore calculates the weighted composite score used for ranking
// Ported from parallel_pipeline.py _aggregate()
func CompositeScore(results []SimulationResult) float64 {
	if len(results) == 0 {
		return 0
	}
	m := ScoreSimulation(results)
	return m.CompositeScore
}

func CompositeScoreFromStats(convRate, avgRelevance, avgImpression, avgCPL float64) float64 {
	if avgCPL > 100 {
		avgCPL = 100
	}
	if avgCPL < 0 {
		avgCPL = 0
	}
	cplComponent := 1 - (avgCPL / 100)
	return convRate*0.5 +
		(avgRelevance/10)*0.25 +
		(avgImpression/10)*0.15 +
		cplComponent*0.1
}

func WilsonInterval(successes, total int, z float64) (float64, float64) {
	if total <= 0 {
		return 0, 1
	}
	p := float64(successes) / float64(total)
	n := float64(total)
	z2 := z * z
	denom := 1 + z2/n
	center := (p + z2/(2*n)) / denom
	half := (z * math.Sqrt((p*(1-p)+z2/(4*n))/n)) / denom
	low := center - half
	high := center + half
	if low < 0 {
		low = 0
	}
	if high > 1 {
		high = 1
	}
	return low, high
}

// ExtractInsights generates actionable insights from simulation results
func ExtractInsights(results []SimulationResult) []string {
	if len(results) == 0 {
		return nil
	}

	var insights []string

	// Conversion by archetype
	archetypeStats := make(map[string][2]int) // [converted, total]
	for _, r := range results {
		arch := r.Archetype
		if arch == "" {
			arch = "general"
		}
		s := archetypeStats[arch]
		s[1]++
		if r.Converted {
			s[0]++
		}
		archetypeStats[arch] = s
	}

	type archStat struct {
		name  string
		rate  float64
		conv  int
		total int
	}
	var archStats []archStat
	for arch, s := range archetypeStats {
		rate := 0.0
		if s[1] > 0 {
			rate = float64(s[0]) / float64(s[1])
		}
		archStats = append(archStats, archStat{arch, rate, s[0], s[1]})
	}
	sort.Slice(archStats, func(i, j int) bool { return archStats[i].rate > archStats[j].rate })
	for _, as := range archStats[:min3(3, len(archStats))] {
		insights = append(insights, fmt.Sprintf("%s: %d/%d converted (%.0f%%)",
			as.name, as.conv, as.total, as.rate*100))
	}

	// Top friction points
	frictionCounts := make(map[string]int)
	for _, r := range results {
		for _, fp := range r.FrictionPoints {
			fp = strings.TrimSpace(fp)
			if fp != "" {
				frictionCounts[fp]++
			}
		}
	}
	if len(frictionCounts) > 0 {
		topFriction := topN(frictionCounts, 3)
		for _, f := range topFriction {
			if len(f) > 80 {
				f = f[:80] + "..."
			}
			insights = append(insights, fmt.Sprintf("Friction: %s", f))
		}
	}

	// Decision timeline distribution
	timelines := make(map[string]int)
	for _, r := range results {
		if r.DecisionTimeline != "" {
			timelines[r.DecisionTimeline]++
		}
	}
	if len(timelines) > 0 {
		var tl []string
		for k, v := range timelines {
			tl = append(tl, fmt.Sprintf("%s(%d)", k, v))
		}
		insights = append(insights, "Decision timelines: "+strings.Join(tl, ", "))
	}

	// High-relevance conversion rate
	hiRelConv := 0
	hiRelTotal := 0
	for _, r := range results {
		if r.RelevanceScore >= 7 {
			hiRelTotal++
			if r.Converted {
				hiRelConv++
			}
		}
	}
	if hiRelTotal > 0 {
		rate := float64(hiRelConv) / float64(hiRelTotal)
		insights = append(insights, fmt.Sprintf("High-relevance (7+) converts at %.0f%% (%d/%d)",
			rate*100, hiRelConv, hiRelTotal))
	}

	return insights
}

// RankIdeas sorts ScoredIdeas by composite score and assigns ranks
func RankIdeas(scored []ScoredIdea) []ScoredIdea {
	sort.Slice(scored, func(i, j int) bool {
		si := scored[i].Metrics.CompositeScore
		sj := scored[j].Metrics.CompositeScore
		if si == sj {
			return scored[i].Idea.FitScore > scored[j].Idea.FitScore
		}
		return si > sj
	})
	for i := range scored {
		scored[i].Rank = i + 1
	}
	stability := computeRankStability(scored)
	for i := range scored {
		scored[i].RankStability = stability[i]
	}
	return scored
}

func computeRankStability(scored []ScoredIdea) map[int]float64 {
	weights := [][4]float64{
		{0.5, 0.25, 0.15, 0.1},
		{0.6, 0.2, 0.1, 0.1},
		{0.4, 0.3, 0.2, 0.1},
		{0.45, 0.2, 0.25, 0.1},
	}
	baseRank := make(map[string]int, len(scored))
	for _, s := range scored {
		baseRank[s.Idea.Description] = s.Rank
	}
	out := make(map[int]float64, len(scored))
	for _, s := range scored {
		id := s.Idea.Description
		stableCount := 0
		for _, w := range weights {
			rank := rankUnderWeights(scored, w, id)
			if math.Abs(float64(rank-baseRank[id])) <= 2 {
				stableCount++
			}
		}
		out[s.Rank] = float64(stableCount) / float64(len(weights))
	}
	return out
}

func rankUnderWeights(scored []ScoredIdea, w [4]float64, ideaID string) int {
	type row struct {
		id    string
		score float64
	}
	rows := make([]row, 0, len(scored))
	for _, s := range scored {
		m := s.Metrics
		cpl := m.SimulatedCPL
		if cpl > 100 {
			cpl = 100
		}
		score := m.ConversionRate*w[0] + (m.AvgRelevance/10)*w[1] + (m.AvgImpression/10)*w[2] + (1-cpl/100)*w[3]
		rows = append(rows, row{id: s.Idea.Description, score: score})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].score > rows[j].score })
	for i, r := range rows {
		if r.id == ideaID {
			return i + 1
		}
	}
	return len(rows)
}

func topN(counts map[string]int, n int) []string {
	type kv struct {
		k string
		v int
	}
	var pairs []kv
	for k, v := range counts {
		pairs = append(pairs, kv{k, v})
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].v > pairs[j].v })
	var result []string
	for i := 0; i < n && i < len(pairs); i++ {
		result = append(result, pairs[i].k)
	}
	return result
}

func min3(a, b int) int {
	if a < b {
		return a
	}
	return b
}
