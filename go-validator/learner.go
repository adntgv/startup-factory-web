package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"
)

// decisionItem pairs an idea with one persona's simulation result
type decisionItem struct {
	idea StartupIdea
	sim  SimulationResult
}

// Learner manages dimension weights and topology expansion across epochs
type Learner struct {
	Weights     DimensionWeights
	Topology    *DynamicTopology
	History     []LearningEpoch
	historyPath string
	mu          sync.Mutex
}

// NewLearner creates or loads a learner from persistent state
func NewLearner(historyPath string) *Learner {
	l := &Learner{
		historyPath: historyPath,
		Weights: DimensionWeights{
			Weights: make(map[string]map[string]float64),
			Epoch:   0,
		},
		Topology: &DynamicTopology{
			Base:    Topology,
			Learned: make(map[string]Dimension),
			Order:   append([]string{}, TopologyOrder...),
		},
	}

	// Initialize uniform weights for all base dimensions
	for name, dim := range Topology {
		l.Weights.Weights[name] = uniformWeights(dim.Options)
	}

	// Try to load saved state
	if err := l.Load(); err != nil {
		// Fresh start is fine
		fmt.Printf("   Learner: starting fresh (no saved state at %s)\n", historyPath)
	} else {
		fmt.Printf("   Learner: loaded epoch %d with %d learned dimensions\n",
			l.Weights.Epoch, len(l.Topology.Learned))
	}

	return l
}

// GetWeights returns a pointer to the current dimension weights
func (l *Learner) GetWeights() *DimensionWeights {
	l.mu.Lock()
	defer l.mu.Unlock()
	return &l.Weights
}

// AnalyzeDecisions uses LLM to extract which dimensions drove YES/NO decisions
// Batches 10,000 decisions into groups of ~50 → ~200 MaxClaw calls in parallel
func (l *Learner) AnalyzeDecisions(scored []ScoredIdea, caller LLMCaller) []DecisionAnalysis {
	var items []decisionItem
	for _, si := range scored {
		for _, sim := range si.Sims {
			items = append(items, decisionItem{si.Idea, sim})
		}
	}

	if len(items) == 0 {
		return nil
	}

	// Batch into groups of 50
	batchSize := 50
	var batches [][]decisionItem
	for i := 0; i < len(items); i += batchSize {
		end := i + batchSize
		if end > len(items) {
			end = len(items)
		}
		batches = append(batches, items[i:end])
	}

	fmt.Printf("   Analyzing %d decisions in %d batches via LLM...\n", len(items), len(batches))

	// Run all batches in parallel
	results := make([][]DecisionAnalysis, len(batches))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 200) // up to 200 concurrent analysis calls

	for i, batch := range batches {
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int, b []decisionItem) {
			defer wg.Done()
			defer func() { <-sem }()

			analyses := l.analyzeBatch(b, caller)
			results[idx] = analyses
		}(i, batch)
	}
	wg.Wait()

	// Flatten
	var all []DecisionAnalysis
	for _, r := range results {
		all = append(all, r...)
	}
	return all
}

// analyzeBatch analyzes one batch of decisions via LLM
func (l *Learner) analyzeBatch(items []decisionItem, caller LLMCaller) []DecisionAnalysis {
	if len(items) == 0 {
		return nil
	}

	// Build prompt for the first item's idea (batches should be same idea for best results)
	// In practice batches mix ideas, but we describe the general task
	var decisionsText strings.Builder
	for i, item := range items {
		decision := "NO"
		if item.sim.Converted {
			decision = "YES"
		}
		fmt.Fprintf(&decisionsText, "%d. %s (%s): \"%s\"\n",
			i+1, item.sim.PersonaName, decision,
			truncate(item.sim.Reasoning, 200))
	}

	// Use first item's coordinates for context
	coordsJSON, _ := json.Marshal(items[0].idea.Coordinates)

	prompt := fmt.Sprintf(`Analyze startup idea validation results. For each persona decision below, identify which idea dimensions influenced it.

IDEA DIMENSIONS: %s

PERSONA DECISIONS:
%s

Task: For each numbered decision, output one JSON object.

Output a JSON array. Each object:
{"persona":"NAME","decision":"yes"or"no","positive_dimensions":["dim:val"],"negative_dimensions":["dim:val"],"missing_factors":["factor:description"]}

Rules:
- positive_dimensions: dimensions that helped the YES decision or didn't block a NO
- negative_dimensions: dimensions that caused a NO or reduced likelihood of YES
- missing_factors: key factors NOT captured by the listed dimensions (e.g. "existing_tool_overlap", "team_size", "urgency")
- Use exact format "dimension:value" e.g. "technology:ai_llms", "monetization:subscription"

Output the JSON array only, starting with [ and ending with ].`,
		string(coordsJSON), decisionsText.String())

	resp := caller.Call(LLMRequest{
		Prompt:    prompt,
		MaxTokens: 16000,
	})
	if resp.Error != nil {
		log.Printf("analyzeBatch LLM error: %v", resp.Error)
		return nil
	}

	jsonStr := extractJSON(resp.Content)
	var rawAnalyses []struct {
		Persona            string   `json:"persona"`
		Decision           string   `json:"decision"`
		PositiveDimensions []string `json:"positive_dimensions"`
		NegativeDimensions []string `json:"negative_dimensions"`
		MissingFactors     []string `json:"missing_factors"`
	}

	if err := json.Unmarshal([]byte(jsonStr), &rawAnalyses); err != nil {
		// Try to salvage partial JSON with fixJSON
		if err2 := json.Unmarshal([]byte(fixJSON(jsonStr)), &rawAnalyses); err2 != nil {
			log.Printf("analyzeBatch parse error: %v (response preview: %s)", err, truncate(resp.Content, 300))
			return nil
		}
	}

	var analyses []DecisionAnalysis
	for _, ra := range rawAnalyses {
		analyses = append(analyses, DecisionAnalysis{
			Persona:            ra.Persona,
			Decision:           ra.Decision,
			PositiveDimensions: ra.PositiveDimensions,
			NegativeDimensions: ra.NegativeDimensions,
			MissingFactors:     ra.MissingFactors,
		})
	}
	return analyses
}

// UpdateWeights adjusts dimension weights based on YES/NO analysis
func (l *Learner) UpdateWeights(scored []ScoredIdea, analyses []DecisionAnalysis) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if len(analyses) == 0 {
		return
	}

	lr := 0.1 // learning rate

	// Count positive/negative mentions per dimension:value
	posCount := make(map[string]map[string]int) // dim → val → count
	negCount := make(map[string]map[string]int)

	for _, a := range analyses {
		for _, dimVal := range a.PositiveDimensions {
			dim, val := splitDimVal(dimVal)
			if dim == "" {
				continue
			}
			if posCount[dim] == nil {
				posCount[dim] = make(map[string]int)
			}
			posCount[dim][val]++
		}
		for _, dimVal := range a.NegativeDimensions {
			dim, val := splitDimVal(dimVal)
			if dim == "" {
				continue
			}
			if negCount[dim] == nil {
				negCount[dim] = make(map[string]int)
			}
			negCount[dim][val]++
		}
	}

	// Apply weight updates
	for dim, dimWeights := range l.Weights.Weights {
		pos := posCount[dim]
		neg := negCount[dim]

		totalMentions := 0
		for _, v := range pos {
			totalMentions += v
		}
		for _, v := range neg {
			totalMentions += v
		}
		if totalMentions == 0 {
			continue
		}

		for val := range dimWeights {
			pCount := float64(pos[val])
			nCount := float64(neg[val])
			delta := lr * (pCount - nCount) / float64(totalMentions+1)
			dimWeights[val] += delta
			if dimWeights[val] < 0.01 {
				dimWeights[val] = 0.01 // floor
			}
		}

		// Normalize
		normalizeWeights(dimWeights)

		// Anti-collapse: mix 85% learned + 15% uniform
		uniform := uniformWeights(keysOf(dimWeights))
		for val := range dimWeights {
			dimWeights[val] = dimWeights[val]*0.85 + uniform[val]*0.15
		}
	}

	l.Weights.Epoch++

	// Record epoch
	avgConv := 0.0
	for _, si := range scored {
		avgConv += si.Metrics.ConversionRate
	}
	if len(scored) > 0 {
		avgConv /= float64(len(scored))
	}

	l.History = append(l.History, LearningEpoch{
		Epoch:         l.Weights.Epoch,
		IdeasTested:   len(scored),
		AvgConversion: avgConv,
		Timestamp:     time.Now().Format(time.RFC3339),
	})
}

// UpdateWeightsFromScores is a fallback learner that adjusts weights based on
// composite scores alone — no LLM analysis needed. High-scoring idea coordinates
// get a boost; low-scoring ones get penalized.
func (l *Learner) UpdateWeightsFromScores(scored []ScoredIdea) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if len(scored) == 0 {
		return
	}

	// Compute score statistics
	var totalScore float64
	for _, si := range scored {
		totalScore += CompositeScore(si.Sims)
	}
	avgScore := totalScore / float64(len(scored))

	lr := 0.15 // slightly higher learning rate for score-only mode

	for _, si := range scored {
		score := CompositeScore(si.Sims)
		delta := lr * (score - avgScore) // positive for above-avg, negative for below

		for dim, val := range si.Idea.Coordinates {
			if l.Weights.Weights[dim] == nil {
				continue
			}
			l.Weights.Weights[dim][val] += delta
			if l.Weights.Weights[dim][val] < 0.01 {
				l.Weights.Weights[dim][val] = 0.01
			}
		}
	}

	// Normalize + anti-collapse
	for dim, dimWeights := range l.Weights.Weights {
		normalizeWeights(dimWeights)
		if dim == "" {
			continue
		}
		uniform := uniformWeights(keysOf(dimWeights))
		for val := range dimWeights {
			dimWeights[val] = dimWeights[val]*0.85 + uniform[val]*0.15
		}
	}

	l.Weights.Epoch++

	// Record epoch
	avgConv := 0.0
	for _, si := range scored {
		avgConv += si.Metrics.ConversionRate
	}
	if len(scored) > 0 {
		avgConv /= float64(len(scored))
	}
	l.History = append(l.History, LearningEpoch{
		Epoch:         l.Weights.Epoch,
		IdeasTested:   len(scored),
		AvgConversion: avgConv,
		Timestamp:     time.Now().Format(time.RFC3339),
	})

	fmt.Printf("   Score-based update: epoch %d, avg conversion %.1f%%\n",
		l.Weights.Epoch, avgConv*100)
}

// LearnWithMode applies learning updates from simulation outcomes and optional LLM analysis.
// Returns true when learning gate passed and updates were applied.
func (l *Learner) LearnWithMode(scored []ScoredIdea, analyses []DecisionAnalysis, mode string, threshold float64) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	if len(scored) == 0 {
		return false
	}
	if threshold <= 0 || threshold > 1 {
		threshold = 0.55
	}

	train, holdout := splitTrainHoldout(scored)
	if len(train) == 0 {
		train = scored
	}

	scoreSignal := buildScoreSignal(train)
	analysisSignal := buildAnalysisSignal(analyses)

	alpha, beta := 0.75, 0.25
	switch mode {
	case "score_only":
		alpha, beta = 1.0, 0.0
	case "llm_explain":
		alpha, beta = 0.0, 1.0
	case "holdout":
		alpha, beta = 0.8, 0.2
	}

	combined := combineSignals(scoreSignal, analysisSignal, alpha, beta)
	agreement := holdoutAgreement(combined, holdout)
	gatePass := mode != "holdout" || agreement >= threshold

	if !gatePass {
		fmt.Printf("   Learning gate blocked update: holdout agreement %.2f < %.2f\n", agreement, threshold)
		return false
	}

	// Decay old beliefs to allow negative learning and adaptation.
	for _, dimWeights := range l.Weights.Weights {
		for val := range dimWeights {
			dimWeights[val] *= 0.97
			if dimWeights[val] < 0.01 {
				dimWeights[val] = 0.01
			}
		}
	}

	for dim, valMap := range combined {
		dimWeights := l.Weights.Weights[dim]
		if dimWeights == nil {
			continue
		}
		for val, delta := range valMap {
			if _, ok := dimWeights[val]; !ok {
				continue
			}
			dimWeights[val] += delta
			if dimWeights[val] < 0.01 {
				dimWeights[val] = 0.01
			}
		}
	}

	for _, dimWeights := range l.Weights.Weights {
		normalizeWeights(dimWeights)
		uniform := uniformWeights(keysOf(dimWeights))
		for val := range dimWeights {
			dimWeights[val] = dimWeights[val]*0.90 + uniform[val]*0.10
		}
	}

	l.Weights.Epoch++
	avgConv := 0.0
	for _, si := range scored {
		avgConv += si.Metrics.ConversionRate
	}
	avgConv /= float64(len(scored))
	l.History = append(l.History, LearningEpoch{
		Epoch:         l.Weights.Epoch,
		IdeasTested:   len(scored),
		AvgConversion: avgConv,
		Timestamp:     time.Now().Format(time.RFC3339),
	})

	fmt.Printf("   Applied %s learning update: holdout agreement %.2f\n", mode, agreement)
	return true
}

// ExpandTopology adds newly discovered dimensions from LLM suggestions
func (l *Learner) ExpandTopology(suggestions []SuggestedDimension, totalPersonas int) {
	l.mu.Lock()
	defer l.mu.Unlock()

	threshold := totalPersonas / 10
	if threshold < 2 {
		threshold = 2
	}

	for _, s := range suggestions {
		if s.MentionCount < threshold {
			continue
		}
		if len(s.Options) == 0 {
			continue
		}

		// Fuzzy match: check if similar dimension exists
		existingDim := l.findSimilarDimension(s.Name)
		if existingDim != "" {
			// Add as new options to existing dimension
			existing := l.Topology.Learned[existingDim]
			existing.Options = appendUnique(existing.Options, s.Options)
			l.Topology.Learned[existingDim] = existing
			// Add weights for new options
			for _, opt := range s.Options {
				if l.Weights.Weights[existingDim] == nil {
					l.Weights.Weights[existingDim] = make(map[string]float64)
				}
				if l.Weights.Weights[existingDim][opt] == 0 {
					l.Weights.Weights[existingDim][opt] = 1.0
				}
			}
			fmt.Printf("   Topology: added %d options to existing dim '%s'\n",
				len(s.Options), existingDim)
			continue
		}

		// Add new dimension
		l.Topology.Learned[s.Name] = Dimension{
			Name:    s.Name,
			Options: s.Options,
		}
		l.Topology.Order = append(l.Topology.Order, s.Name)
		l.Weights.Weights[s.Name] = uniformWeights(s.Options)
		fmt.Printf("   Topology: discovered new dimension '%s' with %d options\n",
			s.Name, len(s.Options))
	}
}

// Save persists learning state to disk
func (l *Learner) Save() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	type savedState struct {
		Weights        DimensionWeights     `json:"weights"`
		LearnedDims    map[string]Dimension `json:"learned_dimensions"`
		DimensionOrder []string             `json:"dimension_order"`
		History        []LearningEpoch      `json:"epochs"`
	}

	state := savedState{
		Weights:        l.Weights,
		LearnedDims:    l.Topology.Learned,
		DimensionOrder: l.Topology.Order,
		History:        l.History,
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(l.historyPath, data, 0644)
}

// Load restores learning state from disk
func (l *Learner) Load() error {
	data, err := os.ReadFile(l.historyPath)
	if err != nil {
		return err
	}

	var state struct {
		Weights        DimensionWeights     `json:"weights"`
		LearnedDims    map[string]Dimension `json:"learned_dimensions"`
		DimensionOrder []string             `json:"dimension_order"`
		History        []LearningEpoch      `json:"epochs"`
	}

	if err := json.Unmarshal(data, &state); err != nil {
		return err
	}

	if state.Weights.Weights != nil {
		l.Weights = state.Weights
	}
	if state.LearnedDims != nil {
		l.Topology.Learned = state.LearnedDims
	}
	if len(state.DimensionOrder) > 0 {
		l.Topology.Order = state.DimensionOrder
	}
	l.History = state.History

	return nil
}

// PrintInsights prints learning history and current weights
func (l *Learner) PrintInsights() {
	l.mu.Lock()
	defer l.mu.Unlock()

	fmt.Printf("\n%s\n", strings.Repeat("=", 70))
	fmt.Println("LEARNING INSIGHTS")
	fmt.Printf("%s\n\n", strings.Repeat("=", 70))
	fmt.Printf("Epoch: %d\n", l.Weights.Epoch)
	fmt.Printf("Learned dimensions: %d\n", len(l.Topology.Learned))
	fmt.Printf("History epochs: %d\n\n", len(l.History))

	// Top weights per dimension
	fmt.Println("TOP WEIGHTED OPTIONS PER DIMENSION:")
	for _, name := range TopologyOrder {
		dimWeights, ok := l.Weights.Weights[name]
		if !ok {
			continue
		}
		best := bestOption(dimWeights)
		fmt.Printf("  %-25s → %-30s (%.3f)\n", name, best.key, best.val)
	}

	if len(l.Topology.Learned) > 0 {
		fmt.Println("\nLEARNED DIMENSIONS:")
		for name, dim := range l.Topology.Learned {
			fmt.Printf("  %s: %v\n", name, dim.Options)
		}
	}

	if len(l.History) > 0 {
		fmt.Println("\nEPOCH HISTORY:")
		for _, ep := range l.History {
			fmt.Printf("  Epoch %d: %d ideas, %.1f%% avg conversion, %s\n",
				ep.Epoch, ep.IdeasTested, ep.AvgConversion*100, ep.Timestamp)
		}
	}
}

// findSimilarDimension finds a learned dimension with a similar name
func (l *Learner) findSimilarDimension(name string) string {
	nameLower := strings.ToLower(name)
	for existingName := range l.Topology.Learned {
		existingLower := strings.ToLower(existingName)
		if existingLower == nameLower ||
			strings.Contains(nameLower, existingLower) ||
			strings.Contains(existingLower, nameLower) {
			return existingName
		}
	}
	return ""
}

func splitTrainHoldout(scored []ScoredIdea) ([]ScoredIdea, []ScoredIdea) {
	train := make([]ScoredIdea, 0, len(scored))
	holdout := make([]ScoredIdea, 0, len(scored)/2)
	for i, s := range scored {
		if i%4 == 0 {
			holdout = append(holdout, s)
		} else {
			train = append(train, s)
		}
	}
	return train, holdout
}

func buildScoreSignal(scored []ScoredIdea) map[string]map[string]float64 {
	signal := make(map[string]map[string]float64)
	if len(scored) == 0 {
		return signal
	}
	avg := 0.0
	for _, s := range scored {
		avg += s.Metrics.CompositeScore
	}
	avg /= float64(len(scored))
	for _, s := range scored {
		delta := (s.Metrics.CompositeScore - avg) * 0.12
		for dim, val := range s.Idea.Coordinates {
			if signal[dim] == nil {
				signal[dim] = make(map[string]float64)
			}
			signal[dim][val] += delta
		}
	}
	return signal
}

func buildAnalysisSignal(analyses []DecisionAnalysis) map[string]map[string]float64 {
	signal := make(map[string]map[string]float64)
	if len(analyses) == 0 {
		return signal
	}
	for _, a := range analyses {
		for _, dimVal := range a.PositiveDimensions {
			dim, val := splitDimVal(dimVal)
			if dim == "" {
				continue
			}
			if signal[dim] == nil {
				signal[dim] = make(map[string]float64)
			}
			signal[dim][val] += 0.02
		}
		for _, dimVal := range a.NegativeDimensions {
			dim, val := splitDimVal(dimVal)
			if dim == "" {
				continue
			}
			if signal[dim] == nil {
				signal[dim] = make(map[string]float64)
			}
			signal[dim][val] -= 0.02
		}
	}
	return signal
}

func combineSignals(a, b map[string]map[string]float64, alpha, beta float64) map[string]map[string]float64 {
	out := make(map[string]map[string]float64)
	for dim, m := range a {
		if out[dim] == nil {
			out[dim] = make(map[string]float64)
		}
		for val, delta := range m {
			out[dim][val] += alpha * delta
		}
	}
	for dim, m := range b {
		if out[dim] == nil {
			out[dim] = make(map[string]float64)
		}
		for val, delta := range m {
			out[dim][val] += beta * delta
		}
	}
	return out
}

func holdoutAgreement(signal map[string]map[string]float64, holdout []ScoredIdea) float64 {
	if len(holdout) == 0 || len(signal) == 0 {
		return 1.0
	}
	totalW := 0.0
	agreeW := 0.0
	for dim, vals := range signal {
		for val, delta := range vals {
			if delta == 0 {
				continue
			}
			with := 0.0
			without := 0.0
			withN := 0
			withoutN := 0
			for _, s := range holdout {
				if s.Idea.Coordinates[dim] == val {
					with += s.Metrics.CompositeScore
					withN++
				} else {
					without += s.Metrics.CompositeScore
					withoutN++
				}
			}
			if withN == 0 || withoutN == 0 {
				continue
			}
			effect := (with / float64(withN)) - (without / float64(withoutN))
			w := abs(delta)
			totalW += w
			if effect*delta > 0 {
				agreeW += w
			}
		}
	}
	if totalW == 0 {
		return 1.0
	}
	return agreeW / totalW
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

// ── Helper functions ──

func uniformWeights(options []string) map[string]float64 {
	w := make(map[string]float64, len(options))
	for _, opt := range options {
		w[opt] = 1.0 / float64(len(options))
	}
	return w
}

func normalizeWeights(w map[string]float64) {
	total := 0.0
	for _, v := range w {
		total += v
	}
	if total <= 0 {
		return
	}
	for k := range w {
		w[k] /= total
	}
}

func keysOf(m map[string]float64) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func appendUnique(existing, newItems []string) []string {
	set := setOf(existing)
	result := append([]string{}, existing...)
	for _, item := range newItems {
		if _, exists := set[item]; !exists {
			result = append(result, item)
		}
	}
	return result
}

func splitDimVal(dimVal string) (string, string) {
	parts := strings.SplitN(dimVal, ":", 2)
	if len(parts) != 2 {
		return "", ""
	}
	return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
}

type kv struct {
	key string
	val float64
}

func bestOption(weights map[string]float64) kv {
	best := kv{}
	for k, v := range weights {
		if v > best.val {
			best = kv{k, v}
		}
	}
	return best
}

// parseSuggestedDimensions extracts dimension suggestions from LLM analysis responses
func parseSuggestedDimensions(analyses []DecisionAnalysis) []SuggestedDimension {
	// Count missing_factors across all analyses
	factorCounts := make(map[string]int)
	for _, a := range analyses {
		for _, mf := range a.MissingFactors {
			factor, _ := splitDimVal(mf)
			if factor != "" {
				factorCounts[factor]++
			}
		}
	}

	var suggestions []SuggestedDimension
	for factor, count := range factorCounts {
		if count >= 2 { // at least 2 mentions
			suggestions = append(suggestions, SuggestedDimension{
				Name:         factor,
				Reason:       fmt.Sprintf("Mentioned in %d persona decisions", count),
				Options:      []string{"low", "medium", "high"}, // default options
				MentionCount: count,
			})
		}
	}

	if len(suggestions) > 0 {
		log.Printf("   Discovered %d potential new dimensions from decision analysis", len(suggestions))
	}
	return suggestions
}
