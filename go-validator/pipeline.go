package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// LLMCaller is the minimal interface for making LLM calls
type LLMCaller interface {
	Call(request LLMRequest) LLMResponse
	Name() string
}

// Pipeline orchestrates the 5-phase startup validation pipeline
type Pipeline struct {
	maxclaw LLMCaller
	config  PipelineConfig
	profile *FounderProfile
	learner *Learner
	stats   PipelineStats
	events  *EventEmitter
	mu      sync.Mutex
}

// NewPipeline creates a new Pipeline from config
func NewPipeline(config PipelineConfig) (*Pipeline, error) {
	// Load profile
	profile, err := LoadProfile(config.ProfilePath)
	if err != nil {
		return nil, fmt.Errorf("loading profile: %w", err)
	}

	// Initialize MaxClaw provider
	maxclaw := NewMaxClawProvider()

	// Create output directory
	if err := os.MkdirAll(config.OutputDir, 0755); err != nil {
		return nil, fmt.Errorf("creating output dir: %w", err)
	}

	// Initialize learner
	learnerPath := config.OutputDir + "/learning_state.json"
	learner := NewLearner(learnerPath)
	rand.Seed(config.Seed)

	// Initialize event emitter
	events := NewEventEmitter(config.OutputDir)

	p := &Pipeline{
		maxclaw: maxclaw,
		config:  config,
		profile: profile,
		learner: learner,
		events:  events,
		stats: PipelineStats{
			PhaseTimes: make(map[string]float64),
		},
	}

	return p, nil
}

// Run executes the full 5-phase pipeline
func (p *Pipeline) Run() (*PipelineResult, error) {
	start := time.Now()

	// Emit initial progress
	if p.events != nil {
		p.events.EmitProgress(0, "Starting validation...")
		p.events.EmitPhase("idea", "active", "Analyzing idea...")
	}

	fmt.Printf("\n%s\n", strings.Repeat("=", 70))
	fmt.Printf("STARTUP FACTORY PIPELINE: %d ideas × %d personas\n",
		p.config.NumIdeas, p.config.NumPersonas)
	fmt.Printf("Max concurrent: %d | Min score: %.2f\n",
		p.config.MaxConcurrent, p.config.MinScore)
	fmt.Printf("%s\n\n", strings.Repeat("=", 70))

	// ── Phase 1: Generate ideas (local, instant) ──
	phase1Start := time.Now()

	var ideas []StartupIdea
	if len(p.config.PinnedIdeas) > 0 {
		fmt.Printf("Phase 1: Using %d pinned idea(s) (skipping generation)...\n", len(p.config.PinnedIdeas))
		ideas = p.config.PinnedIdeas
	} else {
		fmt.Println("Phase 1: Generating ideas (local)...")
		ideas = GenerateWeightedIdeas(
			p.profile,
			p.learner.GetWeights(),
			p.config.NumIdeas,
			p.config.MinScore,
		)
	}

	p.stats.PhaseTimes["idea_generation"] = time.Since(phase1Start).Seconds()
	fmt.Printf("  %d ideas ready in %.1fs\n\n",
		len(ideas), p.stats.PhaseTimes["idea_generation"])

	if p.events != nil {
		p.events.EmitProgress(10, "Idea analyzed")
		p.events.EmitPhase("idea", "complete", "Complete")
	}

	if len(ideas) == 0 {
		return nil, fmt.Errorf("no viable ideas generated")
	}

	// ── Phase 2: Generate landing pages ──
	phase2Start := time.Now()
	if p.events != nil {
		p.events.EmitProgress(15, "Generating landing page...")
		p.events.EmitPhase("landing", "active", "Generating...")
	}
	
	var landings []*LandingPage

	if p.config.ReusePersonasFrom != "" {
		fmt.Println("Phase 2: Regenerating landing page with updated prompt...")
		landings = p.parallelGenerateLandings(ideas)
	} else {
		fmt.Printf("Phase 2: Generating %d landing pages in parallel...\n", len(ideas))
		landings = p.parallelGenerateLandings(ideas)
	}

	p.stats.PhaseTimes["landing_generation"] = time.Since(phase2Start).Seconds()
	fmt.Printf("  Generated %d landing pages in %.1fs\n\n",
		len(landings), p.stats.PhaseTimes["landing_generation"])
	
	if p.events != nil && len(ideas) > 0 {
		p.events.SaveLandingPage(ideas[0])
		p.events.EmitProgress(20, "Landing page ready")
		p.events.EmitPhase("landing", "complete", "Complete")
	}

	// ── Phase 3: Generate or reuse personas ──
	// Skipped if pure committee mode (no personas needed)
	phase3Start := time.Now()
	var allPersonas [][]Persona
	pureCommitteeMode := p.config.B2BMode == "committee"

	if pureCommitteeMode {
		fmt.Println("Phase 3: Skipped (committee mode — personas not needed)")
		allPersonas = make([][]Persona, len(ideas))
	} else if p.config.ReusePersonasFrom != "" {
		fmt.Printf("Phase 3: Loading personas from %s...\n", p.config.ReusePersonasFrom)
		reused, err := loadPersonasFromResult(p.config.ReusePersonasFrom, len(ideas))
		if err != nil {
			log.Printf("Warning: could not load personas from %s: %v — regenerating", p.config.ReusePersonasFrom, err)
			allPersonas = p.parallelGeneratePersonas(ideas, p.config.NumPersonas)
		} else {
			allPersonas = reused
			total := 0
			for _, ps := range allPersonas {
				total += len(ps)
			}
			fmt.Printf("  Loaded %d personas in %.1fs\n\n", total, time.Since(phase3Start).Seconds())
		}
	} else {
		fmt.Printf("Phase 3: Generating personas for %d ideas in parallel...\n", len(ideas))
		allPersonas = p.parallelGeneratePersonas(ideas, p.config.NumPersonas)
	}

	p.stats.PhaseTimes["persona_generation"] = time.Since(phase3Start).Seconds()
	totalPersonas := 0
	for _, ps := range allPersonas {
		totalPersonas += len(ps)
	}
	if !pureCommitteeMode && p.config.ReusePersonasFrom == "" {
		fmt.Printf("  Generated %d total personas in %.1fs\n\n",
			totalPersonas, p.stats.PhaseTimes["persona_generation"])
	}

	// ── Phase 3b: Generate buying committees (committee/auto mode) ──
	phase3bStart := time.Now()
	var allCommittees [][]B2BCommittee

	needsCommittee := p.config.B2BMode == "committee"
	if p.config.B2BMode == "auto" || p.config.B2BMode == "" {
		for _, idea := range ideas {
			if isB2BIdea(idea.Description) {
				needsCommittee = true
				break
			}
		}
	}

	if needsCommittee && p.config.B2BMode != "individual" {
		fmt.Printf("Phase 3b: Generating %d buying committees (%d×3 parallel LLM calls)...\n",
			p.config.NumPersonas, p.config.NumPersonas)
		committees := p.generateB2BCommittees(p.config.NumPersonas)
		allCommittees = make([][]B2BCommittee, len(ideas))
		for i, idea := range ideas {
			useCommittee := p.config.B2BMode == "committee" || isB2BIdea(idea.Description)
			if useCommittee {
				allCommittees[i] = committees
			}
		}
		fmt.Printf("  Generated %d committees in %.1fs\n\n",
			len(committees), time.Since(phase3bStart).Seconds())
	}
	p.stats.PhaseTimes["committee_generation"] = time.Since(phase3bStart).Seconds()

	// ── Phase 4: Simulate decisions (N×M parallel MaxClaw calls) ──
	phase4Start := time.Now()
	totalSims := 0
	pricePoints := p.effectivePricePoints()
	for i, idea := range ideas {
		if i >= len(landings) || landings[i] == nil {
			continue
		}
		if allCommittees != nil && i < len(allCommittees) && len(allCommittees[i]) > 0 {
			totalSims += len(allCommittees[i]) * len(pricePoints)
		} else if i < len(allPersonas) {
			totalSims += len(allPersonas[i]) * len(pricePoints)
		}
		_ = idea
	}
	fmt.Printf("Phase 4: Running %d simulations in parallel...\n", totalSims)
	if p.events != nil {
		p.events.EmitProgress(55, "Running validations...")
		p.events.EmitPhase("validation", "active", "Validating...")
	}

	allResults := p.parallelSimulate(ideas, landings, allPersonas, allCommittees, pricePoints)

	p.stats.PhaseTimes["simulation"] = time.Since(phase4Start).Seconds()
	fmt.Printf("  Completed %d simulations in %.1fs\n\n",
		p.stats.Successful+p.stats.Failed, p.stats.PhaseTimes["simulation"])
	
	if p.events != nil {
		p.events.EmitProgress(85, "Validation complete")
		p.events.EmitPhase("validation", "complete", "Complete")
	}

	// ── Phase 5a: Score results (local, instant) ──
	phase5aStart := time.Now()
	fmt.Println("Phase 5a: Scoring results...")

	scored := p.scoreAndRank(ideas, landings, allResults)

	if p.config.SampleMode == "two_stage" {
		phaseDeepStart := time.Now()
		fmt.Println("Phase 5a.2: Deep-evaluating top ideas...")
		scored = p.deepEvaluateTopIdeas(scored)
		p.stats.PhaseTimes["deep_evaluation"] = time.Since(phaseDeepStart).Seconds()
	}
	observedConv := aggregateConversion(scored)
	expLow, expHigh := expectedConversionRange(p.config.FunnelStage)
	p.stats.ObservedConversion = observedConv
	p.stats.ExpectedConvLow = expLow
	p.stats.ExpectedConvHigh = expHigh
	p.stats.CalibrationWarnings = p.computeCalibrationWarnings(scored)

	p.stats.PhaseTimes["scoring"] = time.Since(phase5aStart).Seconds()

	// ── Phase 5b: Analyze decisions via LLM (optional, ~200 parallel calls) ──
	var analyses []DecisionAnalysis
	requiresLLMAnalysis := p.config.LearningMode == "llm_explain" || p.config.LearningMode == "hybrid" || p.config.LearningMode == "holdout"
	if p.config.Epochs > 0 && requiresLLMAnalysis {
		phase5bStart := time.Now()
		fmt.Println("Phase 5b: Analyzing decisions via LLM...")

		analyses = p.learner.AnalyzeDecisions(scored, p.maxclaw)

		p.stats.PhaseTimes["analysis"] = time.Since(phase5bStart).Seconds()
		fmt.Printf("  Analyzed %d decisions in %.1fs\n\n",
			len(analyses), p.stats.PhaseTimes["analysis"])
	}

	// ── Phase 5c: Learn + expand topology (local, instant) ──
	if p.config.Epochs > 0 {
		fmt.Println("Phase 5c: Updating weights + expanding topology...")
		doLearn := true
		if p.config.EnforceCalibration {
			floor := p.effectiveConversionFloor()
			if observedConv < floor {
				doLearn = false
				p.stats.LearningGatePassed = false
				fmt.Printf("  Learning skipped: observed conversion %.2f%% below floor %.2f%% for %s stage\n\n", observedConv*100, floor*100, p.config.FunnelStage)
			}
		}

		if doLearn {
			pass := p.learner.LearnWithMode(scored, analyses, p.config.LearningMode, p.config.CIThreshold)
			p.stats.LearningGatePassed = pass
			if len(analyses) > 0 {
				suggestions := parseSuggestedDimensions(analyses)
				p.learner.ExpandTopology(suggestions, totalSims)
			}

			if err := p.learner.Save(); err != nil {
				fmt.Printf("  Warning: failed to save learning state: %v\n", err)
			} else {
				fmt.Printf("  Saved learning state (epoch %d)\n\n", p.learner.Weights.Epoch)
			}
		}
	}

	p.stats.TotalTime = time.Since(start).Seconds()
	p.stats.TotalCalls = p.stats.Successful + p.stats.Failed
	if p.stats.TotalTime > 0 {
		p.stats.Throughput = float64(p.stats.Successful) / p.stats.TotalTime
	}

	result := &PipelineResult{
		ScoredIdeas:   scored,
		Stats:         p.stats,
		ProviderStats: map[string]ProviderStat{"maxclaw": {Calls: p.stats.TotalCalls, Successes: p.stats.Successful, Failures: p.stats.Failed}},
		Timestamp:     time.Now().Format(time.RFC3339),
		Config:        p.config,
	}

	// Emit final results
	if p.events != nil {
		p.events.EmitProgress(100, "Validation complete!")
		p.events.SaveResults(*result)
	}

	return result, nil
}

// trackCall updates stats for any LLM call result (thread-safe)
func (p *Pipeline) trackCall(success bool) {
	p.mu.Lock()
	if success {
		p.stats.Successful++
	} else {
		p.stats.Failed++
	}
	p.mu.Unlock()
}

// parallelGenerateLandings generates landing pages for all ideas concurrently
func (p *Pipeline) parallelGenerateLandings(ideas []StartupIdea) []*LandingPage {
	landings := make([]*LandingPage, len(ideas))
	var wg sync.WaitGroup
	sem := make(chan struct{}, p.config.MaxConcurrent)

	for i, idea := range ideas {
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int, idea StartupIdea) {
			defer wg.Done()
			defer func() { <-sem }()

			landing := p.generateLandingViaLLM(idea)
			p.trackCall(landing != nil)
			landings[idx] = landing

			if p.config.Verbose {
				fmt.Printf("  [%d/%d] Landing page generated\n", idx+1, len(ideas))
			}
		}(i, idea)
	}
	wg.Wait()

	// Fallback: use local config for any that failed
	for i, l := range landings {
		if l == nil {
			landings[i] = IdeaToLandingConfig(ideas[i])
		}
	}
	return landings
}

// parallelGeneratePersonas generates personas for all ideas concurrently
func (p *Pipeline) parallelGeneratePersonas(ideas []StartupIdea, personasPerIdea int) [][]Persona {
	allPersonas := make([][]Persona, len(ideas))
	var wg sync.WaitGroup
	var personaCounter int32
	maxConc := p.config.MaxConcurrent
	if p.config.EnrichmentMode == "llm-lfm" && maxConc > 5 {
		maxConc = 5
	}
	sem := make(chan struct{}, maxConc)

	if p.events != nil {
		p.events.EmitProgress(25, "Generating personas...")
		p.events.EmitPhase("personas", "active", "Generating personas...")
	}

	for i, idea := range ideas {
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int, idea StartupIdea) {
			defer wg.Done()
			defer func() { <-sem }()

			personas := p.generatePersonas(idea.Description, personasPerIdea)
			p.trackCall(len(personas) > 0)
			allPersonas[idx] = personas

			// Emit each persona
			if p.events != nil {
				for _, persona := range personas {
					personaIdx := int(atomic.AddInt32(&personaCounter, 1))
					profile := PersonaProfile{
						Name:        persona.Name,
						JobRole:     persona.Role,
						Background:  persona.CurrentWorkflow,
						InitialHope: "",
					}
					p.events.SavePersona(profile, personaIdx)
					
					// Update progress
					totalPersonas := len(ideas) * personasPerIdea
					percent := 25 + int(float64(personaIdx)/float64(totalPersonas)*25)
					p.events.EmitProgress(percent, fmt.Sprintf("Generated %d/%d personas", personaIdx, totalPersonas))
				}
			}

			if p.config.Verbose {
				fmt.Printf("  [%d/%d] Generated %d personas\n", idx+1, len(ideas), len(personas))
			}
		}(i, idea)
	}
	wg.Wait()

	if p.events != nil {
		p.events.EmitProgress(50, fmt.Sprintf("%d personas ready", int(personaCounter)))
		p.events.EmitPhase("personas", "complete", fmt.Sprintf("%d generated", int(personaCounter)))
	}

	return allPersonas
}

// parallelSimulate runs all simulations concurrently.
// If allCommittees[i] is non-empty, uses committee mode for idea i; otherwise uses individual persona mode.
func (p *Pipeline) parallelSimulate(ideas []StartupIdea, landings []*LandingPage, allPersonas [][]Persona, allCommittees [][]B2BCommittee, pricePoints []int) [][]SimulationResult {
	allResults := make([][]SimulationResult, len(ideas))
	for i := range allResults {
		cap := 0
		if allCommittees != nil && i < len(allCommittees) && len(allCommittees[i]) > 0 {
			cap = len(allCommittees[i]) * len(pricePoints)
		} else if i < len(allPersonas) {
			cap = len(allPersonas[i]) * len(pricePoints)
		}
		allResults[i] = make([]SimulationResult, 0, cap)
	}

	type committeeJob struct {
		ideaIdx   int
		committee *B2BCommittee
		landing   *LandingPage
		idea      StartupIdea
		price     int
	}
	type individualJob struct {
		ideaIdx int
		persona Persona
		landing *LandingPage
		idea    StartupIdea
		price   int
	}

	var committeeJobs []committeeJob
	var individualJobs []individualJob

	for i, idea := range ideas {
		if i >= len(landings) || landings[i] == nil {
			continue
		}
		useCommittee := allCommittees != nil && i < len(allCommittees) && len(allCommittees[i]) > 0
		if useCommittee {
			for ci := range allCommittees[i] {
				for _, price := range pricePoints {
					committeeJobs = append(committeeJobs, committeeJob{
						ideaIdx: i, committee: &allCommittees[i][ci], landing: landings[i], idea: idea, price: price,
					})
				}
			}
		} else {
			if i >= len(allPersonas) {
				continue
			}
			for _, persona := range allPersonas[i] {
				for _, price := range pricePoints {
					individualJobs = append(individualJobs, individualJob{
						ideaIdx: i, persona: persona, landing: landings[i], idea: idea, price: price,
					})
				}
			}
		}
	}

	totalJobs := len(committeeJobs) + len(individualJobs)

	type indexedResult struct {
		ideaIdx int
		result  SimulationResult
	}

	resultsCh := make(chan indexedResult, totalJobs+1)
	var wg sync.WaitGroup
	sem := make(chan struct{}, p.config.MaxConcurrent)
	var simDone int64

	for _, job := range committeeJobs {
		wg.Add(1)
		sem <- struct{}{}
		go func(j committeeJob) {
			defer wg.Done()
			defer func() { <-sem }()

			committeeResult := p.simulateB2BDecision(j.committee, j.landing, j.idea, j.price)
			simResult := b2bCommitteeToSimResult(committeeResult, j.committee)
			resultsCh <- indexedResult{j.ideaIdx, simResult}

			p.trackCall(simResult.Status != "error")

			done := int(atomic.AddInt64(&simDone, 1))
			if done%10 == 0 || done == totalJobs {
				fmt.Printf("  [%d/%d] simulations complete\n", done, totalJobs)
			}
		}(job)
	}

	for personaIdx, job := range individualJobs {
		wg.Add(1)
		sem <- struct{}{}
		go func(pIdx int, j individualJob) {
			defer wg.Done()
			defer func() { <-sem }()

			result := p.simulatePersonaViaLLM(j.persona, j.landing, j.idea, j.price)
			resultsCh <- indexedResult{j.ideaIdx, result}

			// Emit persona validation event
			if p.events != nil {
				p.events.SavePersonaValidation(pIdx+1, result)
			}

			p.trackCall(result.Status != "error")

			done := int(atomic.AddInt64(&simDone, 1))
			if p.events != nil && totalJobs > 0 {
				percent := 55 + int(float64(done)/float64(totalJobs)*30)
				p.events.EmitProgress(percent, fmt.Sprintf("Validated %d/%d personas", done, totalJobs))
			}
			
			if done%100 == 0 || done == totalJobs {
				fmt.Printf("  [%d/%d] simulations complete\n", done, totalJobs)
			}
		}(personaIdx, job)
	}

	go func() {
		wg.Wait()
		close(resultsCh)
	}()

	var resultMu sync.Mutex
	for ir := range resultsCh {
		resultMu.Lock()
		allResults[ir.ideaIdx] = append(allResults[ir.ideaIdx], ir.result)
		resultMu.Unlock()
	}

	return allResults
}

// scoreAndRank scores each idea's simulation results and ranks them
func (p *Pipeline) scoreAndRank(ideas []StartupIdea, landings []*LandingPage, allResults [][]SimulationResult) []ScoredIdea {
	var scored []ScoredIdea
	pricePoints := p.effectivePricePoints()

	for i, idea := range ideas {
		if i >= len(allResults) {
			continue
		}
		results := allResults[i]
		priceExperiments := make([]PricePointMetrics, 0, len(pricePoints))
		// Default to first configured price point, not hardcoded $29
		selectedPrice := 29
		if len(pricePoints) > 0 {
			selectedPrice = pricePoints[0]
		}
		bestMetrics := ScoreSimulation(results)
		bestComposite := bestMetrics.CompositeScore

		for _, price := range pricePoints {
			filtered := filterResultsByPrice(results, price)
			if len(filtered) == 0 {
				continue
			}
			m := ScoreSimulation(filtered)
			priceExperiments = append(priceExperiments, PricePointMetrics{Price: price, Metrics: m})
			if m.CompositeScore > bestComposite {
				bestComposite = m.CompositeScore
				bestMetrics = m
				selectedPrice = price
			}
		}
		metrics := bestMetrics

		var landing *LandingPage
		if i < len(landings) {
			landing = landings[i]
		}

		scored = append(scored, ScoredIdea{
			Idea:             idea,
			Landing:          landing,
			Metrics:          metrics,
			Sims:             results,
			PriceExperiments: priceExperiments,
			SelectedPrice:    selectedPrice,
		})
	}

	return RankIdeas(scored)
}

func filterResultsByPrice(results []SimulationResult, price int) []SimulationResult {
	if price <= 0 {
		return results
	}
	filtered := make([]SimulationResult, 0, len(results))
	for _, r := range results {
		if r.TestedPrice == price {
			filtered = append(filtered, r)
		}
	}
	return filtered
}

func (p *Pipeline) effectivePricePoints() []int {
	if p.config.PricingMode != "grid" {
		return []int{29}
	}
	if len(p.config.PricePoints) == 0 {
		return []int{9, 19, 29, 49, 99}
	}
	return p.config.PricePoints
}

func (p *Pipeline) deepEvaluateTopIdeas(scored []ScoredIdea) []ScoredIdea {
	if len(scored) == 0 || p.config.DeepTopK <= 0 || p.config.DeepPersonas <= p.config.NumPersonas {
		return scored
	}

	k := p.config.DeepTopK
	if k > len(scored) {
		k = len(scored)
	}

	pricePoints := p.effectivePricePoints()
	for i := 0; i < k; i++ {
		idea := scored[i].Idea
		landing := scored[i].Landing
		if landing == nil {
			landing = IdeaToLandingConfig(idea)
		}
		personas := p.generatePersonas(idea.Description, p.config.DeepPersonas)
		results := p.parallelSimulate([]StartupIdea{idea}, []*LandingPage{landing}, [][]Persona{personas}, nil, pricePoints)
		if len(results) == 0 {
			continue
		}
		updated := p.scoreAndRank([]StartupIdea{idea}, []*LandingPage{landing}, results)
		if len(updated) == 1 {
			scored[i].Metrics = updated[0].Metrics
			scored[i].Sims = updated[0].Sims
			scored[i].PriceExperiments = updated[0].PriceExperiments
			scored[i].SelectedPrice = updated[0].SelectedPrice
		}
	}

	return RankIdeas(scored)
}

func (p *Pipeline) computeCalibrationWarnings(scored []ScoredIdea) []string {
	warnings := make([]string, 0)
	valid, conversions := aggregateCounts(scored)
	if valid == 0 {
		warnings = append(warnings, "No valid persona decisions; simulation is uncalibrated.")
		return warnings
	}
	convRate := float64(conversions) / float64(valid)
	expLow, expHigh := expectedConversionRange(p.config.FunnelStage)
	warnings = append(warnings, fmt.Sprintf("Funnel stage %s expected conversion range %.1f%%-%.1f%%.", p.config.FunnelStage, expLow*100, expHigh*100))
	if convRate < expLow {
		warnings = append(warnings, fmt.Sprintf("Observed conversion %.1f%% is below expected range for %s traffic.", convRate*100, p.config.FunnelStage))
	}
	if convRate > expHigh {
		warnings = append(warnings, fmt.Sprintf("Observed conversion %.1f%% is above expected range for %s traffic.", convRate*100, p.config.FunnelStage))
	}
	if convRate < 0.01 {
		warnings = append(warnings, fmt.Sprintf("Very low aggregate conversion %.1f%%; check simulation assumptions.", convRate*100))
	}
	if convRate > 0.60 {
		warnings = append(warnings, fmt.Sprintf("Very high aggregate conversion %.1f%%; likely optimistic bias.", convRate*100))
	}
	for _, s := range scored {
		width := s.Metrics.ConversionCIHigh - s.Metrics.ConversionCILow
		if width > 0.35 {
			warnings = append(warnings, fmt.Sprintf("High uncertainty for rank #%d (CI width %.2f).", s.Rank, width))
		}
	}
	return warnings
}

func aggregateCounts(scored []ScoredIdea) (int, int) {
	valid := 0
	conversions := 0
	for _, s := range scored {
		valid += s.Metrics.ValidPersonas
		conversions += s.Metrics.Conversions
	}
	return valid, conversions
}

func aggregateConversion(scored []ScoredIdea) float64 {
	valid, conversions := aggregateCounts(scored)
	if valid == 0 {
		return 0
	}
	return float64(conversions) / float64(valid)
}

func expectedConversionRange(stage string) (float64, float64) {
	switch strings.ToLower(stage) {
	case "high_intent", "high-intent", "highintent":
		return 0.06, 0.12
	case "warm":
		return 0.03, 0.07
	default:
		return 0.005, 0.03
	}
}

func (p *Pipeline) effectiveConversionFloor() float64 {
	if p.config.ConversionFloor > 0 {
		return p.config.ConversionFloor
	}
	low, _ := expectedConversionRange(p.config.FunnelStage)
	return low
}

// ── LLM Call Functions ──

// generateLandingViaLLM calls MaxClaw to generate a landing page for an idea
func (p *Pipeline) generateLandingViaLLM(idea StartupIdea) *LandingPage {
	prompt := fmt.Sprintf(`You are a conversion copywriter. Read the product description below and create a high-converting landing page.

PRODUCT:
%s

Think step by step:
1. What is the core painful problem this solves?
2. Who is the exact buyer (role, company type, situation)?
3. What is the #1 outcome they get?
4. What makes this different from alternatives?
5. What is the real pricing model?

Then output ONLY a JSON object — no markdown, no explanation:
{
  "headline": "sharp outcome-focused headline (max 10 words)",
  "subheadline": "one sentence: who it's for + what pain it solves",
  "price": "price as string, e.g. '1000' or '49'",
  "trial_type": "free trial / demo / pilot / none",
  "cta": "action-oriented button text",
  "features": ["specific benefit 1", "specific benefit 2", "specific benefit 3", "specific benefit 4", "specific benefit 5"]
}`,
		idea.Description)

	resp := p.maxclaw.Call(LLMRequest{
		Prompt:    prompt,
		MaxTokens: 3000,
	})
	if resp.Error != nil {
		log.Printf("Landing page generation failed: %v", resp.Error)
		return nil
	}

	if p.config.Verbose {
		if resp.Thinking != "" {
			fmt.Printf("\n── Landing Page Reasoning ──\n%s\n", resp.Thinking)
		}
		fmt.Printf("\n── Landing Page Raw Response ──\n%s\n", resp.Content)
	}

	landing, err := parseLandingPage(resp.Content)
	if p.config.Verbose && landing != nil {
		fmt.Printf("\n── Landing Page Parsed ──\nHeadline: %s\nSubheadline: %s\nPrice: %s\nCTA: %s\nFeatures: %v\n\n",
			landing.Headline, landing.Subheadline, landing.Price, landing.CTA, landing.Features)
	}
	if err != nil && p.config.Debug {
		log.Printf("Landing page parse error: %v | raw: %s", err, truncate(resp.Content, 300))
	}
	return landing
}

// generatePersonas routes to the appropriate persona generation method based on enrichment mode.
// For maxclaw, always uses parallel fan-out (1 request per persona) regardless of count.
func (p *Pipeline) generatePersonas(description string, count int) []Persona {
	switch p.config.EnrichmentMode {
	case "programmatic":
		return p.generatePersonasProgrammatic(description, count)
	case "llm-lfm":
		return p.generatePersonasViaLFM(description, count)
	default: // maxclaw or ""
		return p.generatePersonasBatched(description, count)
	}
}

// generatePersonasBatched generates personas in smaller batches and merges results.
// For maxclaw (batchSize=1) all calls are fanned out concurrently up to MaxConcurrent.
func (p *Pipeline) generatePersonasBatched(description string, targetCount int) []Persona {
	isMaxclaw := p.config.EnrichmentMode == "maxclaw" || p.config.EnrichmentMode == ""

	// Maxclaw returns 1 persona per call — fan out all calls in parallel.
	if isMaxclaw {
		// Request 20% extra to cover duplicates/failures.
		fanOut := int(float64(targetCount) * 1.2)
		if fanOut < targetCount+5 {
			fanOut = targetCount + 5
		}

		concLimit := p.config.MaxConcurrent
		if concLimit <= 0 {
			concLimit = 50
		}

		isB2B := isB2BIdea(description)

		type result struct{ persona *Persona }
		results := make([]result, fanOut)
		sem := make(chan struct{}, concLimit)
		var wg sync.WaitGroup

		for i := 0; i < fanOut; i++ {
			wg.Add(1)
			sem <- struct{}{}
			go func(idx int) {
				defer wg.Done()
				defer func() { <-sem }()
				personas := p.generatePersonaViaMaxClaw(description, idx, fanOut, isB2B)
				if len(personas) > 0 {
					results[idx] = result{persona: &personas[0]}
				}
			}(i)
		}
		wg.Wait()

		seen := make(map[string]bool)
		var allPersonas []Persona
		for _, r := range results {
			if r.persona != nil && !seen[r.persona.Name] {
				seen[r.persona.Name] = true
				allPersonas = append(allPersonas, *r.persona)
				if len(allPersonas) >= targetCount {
					break
				}
			}
		}

		log.Printf("Parallel persona generation (maxclaw): requested %d, got %d (fan-out %d, b2b=%v)", targetCount, len(allPersonas), fanOut, isB2B)
		return allPersonas
	}

	// Non-maxclaw: sequential batching (original behaviour).
	batchSize := 10
	if targetCount < 30 {
		batchSize = 5
	}

	var allPersonas []Persona
	seen := make(map[string]bool)

	for batch := 0; len(allPersonas) < targetCount && batch*batchSize < targetCount*2; batch++ {
		remaining := targetCount - len(allPersonas)
		thisBatch := batchSize
		if remaining < thisBatch {
			thisBatch = remaining
		}

		var batchPersonas []Persona
		switch p.config.EnrichmentMode {
		case "programmatic":
			batchPersonas = p.generatePersonasProgrammatic(description, thisBatch)
		case "llm-lfm":
			batchPersonas = p.generatePersonasViaLFM(description, thisBatch)
		default:
			// Sequential fallback for non-maxclaw modes (maxclaw uses the parallel fan-out above)
			for j := 0; j < thisBatch; j++ {
				ps := p.generatePersonaViaMaxClaw(description, j, thisBatch, isB2BIdea(description))
				batchPersonas = append(batchPersonas, ps...)
			}
		}

		for _, persona := range batchPersonas {
			if !seen[persona.Name] {
				seen[persona.Name] = true
				allPersonas = append(allPersonas, persona)
			}
		}

		if len(batchPersonas) == 0 {
			log.Printf("Warning: batch %d returned no personas, skipping", batch)
		}
	}

	if len(allPersonas) > targetCount {
		allPersonas = allPersonas[:targetCount]
	}

	log.Printf("Batched persona generation: requested %d, got %d", targetCount, len(allPersonas))
	return allPersonas
}

// generatePersonaViaMaxClaw generates a single persona with diversity seeding and B2B awareness.
// idx/total are used to enforce variety across parallel calls.
func (p *Pipeline) generatePersonaViaMaxClaw(description string, idx, total int, isB2B bool) []Persona {
	var financialInstructions, diversityHint string

	if isB2B {
		// Persona archetypes distributed across the fan-out for variety
		archetypes := []string{
			"a small agency owner (5-10 staff) who is price-sensitive and makes decisions alone",
			"a CTO at a mid-size outsourcing firm (20-40 staff) with a real software tooling budget",
			"a QA lead who influences but doesn't control budget",
			"a founder of a bootstrapped mobile dev shop who wears many hats",
			"a project manager at an agency who is frustrated with manual QA",
			"a technical director at a larger agency (50+ staff) evaluating enterprise tools",
			"a solo consultant who delivers mobile apps for clients",
			"an operations head at a growing agency trying to scale without hiring",
		}
		archetype := archetypes[idx%len(archetypes)]

		financialInstructions = `IMPORTANT — this is a B2B purchase. Populate financial fields as BUSINESS financials:
- monthly_income = agency monthly revenue (USD)
- monthly_expenses = monthly operating costs (salaries, office, tools)
- discretionary_budget = monthly budget available for new software tools
- current_subscriptions = existing business software (Jira, TestRail, BrowserStack, etc.)
- savings_months = months of runway / cash reserves
Vary agency size: small ($5k-20k/mo revenue), mid ($20k-80k/mo), larger ($80k+/mo).`

		diversityHint = fmt.Sprintf("Persona slot %d of %d. Generate: %s. Use a realistic name from Russia, Ukraine, Kazakhstan, or Belarus. Vary seniority, company size, geography, and financial health.", idx+1, total, archetype)
	} else {
		diversityHint = fmt.Sprintf("Persona slot %d of %d. Use a globally diverse name. Vary demographics, income level, and personality significantly from typical personas.", idx+1, total)
		financialInstructions = "Populate financial fields as personal finances (after-tax personal income, personal expenses, personal discretionary budget)."
	}

	prompt := fmt.Sprintf(`Generate 1 realistic person who might buy software tools for their work.

%s

%s

Return ONLY a JSON array with exactly 1 persona object. ALL fields required:

[
  {
    "name": "string (realistic full name)",
    "age": int (25-60),
    "role": "string (specific job title)",
    "company_size": "solo" | "startup" | "smb" | "enterprise",
    "experience_years": int,
    "pain_level": int (1-10, how badly they need this),
    "skepticism": int (1-10),
    "current_workflow": "string (how they do this today)",
    "current_tools": "string (tools they currently use)",
    "personality": "early_adopter" | "pragmatic" | "conservative" | "skeptical",
    "archetype": "power_user" | "casual" | "struggling" | "non_user",
    "budget": "tight" | "moderate" | "comfortable",
    "decision_authority": "self" | "needs_approval" | "committee",
    "financial": {
      "monthly_income": int,
      "monthly_expenses": int,
      "discretionary_budget": int,
      "current_subscriptions": [{"name": "string", "monthly_cost": int, "usage": "daily|weekly|rarely|forgot_about_it"}],
      "total_sub_spend": int,
      "savings_months": float,
      "risk_tolerance": "minimal" | "cautious" | "moderate" | "aggressive",
      "recent_purchase_regret": bool
    },
    "daily_life": {
      "wake_time": "HH:MM",
      "sleep_time": "HH:MM",
      "free_hours_per_day": float,
      "top_struggles": ["string", "string", "string"],
      "daily_routine": "string",
      "mental_state": "overwhelmed" | "burned_out" | "coasting" | "focused",
      "discovery_context": "string (how/when they discover tools like this)",
      "attention_span_minutes": int (5-30),
      "current_priorities": ["string", "string", "string"]
    }
  }
]

REALISM RULES: recent_purchase_regret should be true for only ~1 in 5 personas (20%%). Most people are not currently burned by a bad purchase. Mix mental states — most should be "coasting" or "focused", not all "overwhelmed" or "burned_out".

Output ONLY the JSON array. No markdown, no explanation.`, diversityHint, financialInstructions)

	// Retry up to 2 times on parse failures
	for attempt := 0; attempt < 3; attempt++ {
		resp := p.maxclaw.Call(LLMRequest{
			Prompt:    prompt,
			MaxTokens: 4000,
		})

		if resp.Error != nil {
			log.Printf("MaxClaw persona generation attempt %d failed: %v", attempt+1, resp.Error)
			continue
		}

		if p.config.Verbose && resp.Thinking != "" {
			fmt.Printf("\n── Persona Reasoning ──\n%s\n", resp.Thinking)
		}
		if p.config.Verbose {
			fmt.Printf("\n── Persona Raw Response ──\n%s\n", resp.Content)
		}
		if p.config.Debug {
			log.Printf("Persona raw[attempt=%d]: %s", attempt+1, truncate(resp.Content, 500))
		}

		personas, err := parsePersonas(resp.Content, 1)
		if err == nil && len(personas) > 0 {
			if p.config.Verbose {
				for _, persona := range personas {
					fmt.Printf("  ✓ Persona: %s, %d, %s [pain=%d skepticism=%d]\n",
						persona.Name, persona.Age, persona.Role, persona.PainLevel, persona.Skepticism)
				}
			}
			return personas
		}

		log.Printf("MaxClaw persona parse attempt %d failed: %v | raw[:200]: %s", attempt+1, err, truncate(resp.Content, 200))
	}

	return nil
}

// generatePersonasProgrammatic generates core personas programmatically, then enriches daily life via LLM
func (p *Pipeline) generatePersonasProgrammatic(description string, count int) []Persona {
	// Step 1: Generate core personas purely programmatically (no LLM)
	personas := generateCorePersonasProgrammatic(description, count)

	if len(personas) == 0 {
		return nil
	}

	// Step 2: Enrich daily life context via LLM (in parallel batches)
	personas = p.enrichDailyLifeViaLLM(personas, description)

	return personas
}

// generateCorePersonasProgrammatic generates core persona data purely programmatically (no LLM)
func generateCorePersonasProgrammatic(description string, count int) []Persona {
	firstNames := []string{"Alex", "Jordan", "Taylor", "Morgan", "Casey", "Riley", "Quinn", "Avery", "Cameron", "Drew", "Sam", "Jamie", "Dakota", "Reese", "Skyler", "Parker", "Hayden", "Emerson", "Rowan", "Sage"}
	lastInitials := []string{"A", "B", "C", "D", "E", "F", "G", "H", "J", "K", "L", "M", "N", "P", "R", "S", "T", "W"}

	roles := []string{"Software Engineer", "Product Manager", "Designer", "Data Analyst", "Marketing Manager", "Sales Representative", "Customer Success", "Founder", "CTO", "VP Engineering", "DevOps", "Frontend Developer", "Backend Developer"}
	companySizes := []string{"solo", "startup", "smb", "enterprise"}
	personalities := []string{"early_adopter", "pragmatic", "conservative", "skeptical"}
	archetypes := []string{"power_user", "casual", "struggling", "non_user"}
	budgets := []string{"tight", "moderate", "comfortable"}
	decisionAuths := []string{"self", "needs_approval", "committee"}

	personas := make([]Persona, 0, count)
	usedNames := make(map[string]bool)

	for len(personas) < count {
		first := firstNames[rand.Intn(len(firstNames))]
		lastInitial := lastInitials[rand.Intn(len(lastInitials))]
		name := fmt.Sprintf("%s %s.", first, lastInitial)

		if usedNames[name] {
			continue
		}
		usedNames[name] = true

		role := roles[rand.Intn(len(roles))]
		age := 22 + rand.Intn(44) // 22-65

		p := Persona{
			Name:              name,
			Age:               age,
			Role:              role,
			CompanySize:       companySizes[rand.Intn(len(companySizes))],
			ExperienceYears:   rand.Intn(20),
			PainLevel:         1 + rand.Intn(10),
			Skepticism:        1 + rand.Intn(10),
			CurrentWorkflow:   "Various tasks and projects",
			CurrentTools:      "Standard business tools",
			Personality:       personalities[rand.Intn(len(personalities))],
			Archetype:         archetypes[rand.Intn(len(archetypes))],
			Budget:            budgets[rand.Intn(len(budgets))],
			DecisionAuthority: decisionAuths[rand.Intn(len(decisionAuths))],
		}

		// Financial - correlated with role and budget
		var incomeMin, incomeMax int
		roleLower := strings.ToLower(p.Role)
		switch {
		case strings.Contains(roleLower, "student") || strings.Contains(roleLower, "intern"):
			incomeMin, incomeMax = 14000, 38000
		case strings.Contains(roleLower, "founder") || strings.Contains(roleLower, "cto"):
			incomeMin, incomeMax = 60000, 250000
		case strings.Contains(roleLower, "manager") || strings.Contains(roleLower, "director") || strings.Contains(roleLower, "vp"):
			incomeMin, incomeMax = 70000, 200000
		case strings.Contains(roleLower, "engineer") || strings.Contains(roleLower, "developer"):
			incomeMin, incomeMax = 65000, 180000
		default:
			incomeMin, incomeMax = 40000, 120000
		}
		switch p.Budget {
		case "tight":
			incomeMax = int(float64(incomeMax) * 0.7)
		case "comfortable":
			incomeMin = int(float64(incomeMin) * 1.2)
		}
		if p.CompanySize == "enterprise" {
			incomeMin = int(float64(incomeMin) * 1.2)
			incomeMax = int(float64(incomeMax) * 1.25)
		}
		if incomeMax <= incomeMin {
			incomeMax = incomeMin + 15000
		}
		monthlyIncome := incomeMin + rand.Intn(incomeMax-incomeMin)
		if monthlyIncome < 1000 {
			monthlyIncome = 1000
		}

		expenseRatio := 0.7 + rand.Float64()*0.2
		if p.Budget == "tight" {
			expenseRatio = 0.8 + rand.Float64()*0.15
		}
		monthlyExpenses := int(float64(monthlyIncome) * expenseRatio)
		discretionary := monthlyIncome - monthlyExpenses
		if discretionary < 50 {
			discretionary = 50
		}

		numSubs := 2 + rand.Intn(4)
		if p.Budget == "tight" {
			numSubs = 1 + rand.Intn(2)
		}
		subCosts := []int{5, 10, 15, 20, 25, 30, 50}
		subs := make([]Subscription, numSubs)
		totalSubCost := 0
		for j := 0; j < numSubs; j++ {
			subs[j] = Subscription{
				Name:        []string{"Netflix", "Spotify", "Notion", "Slack", "Zoom", "GitHub", "ChatGPT", "Adobe", "Microsoft"}[rand.Intn(9)],
				MonthlyCost: subCosts[rand.Intn(len(subCosts))],
				Usage:       []string{"daily", "weekly", "rarely"}[rand.Intn(3)],
			}
			totalSubCost += subs[j].MonthlyCost
		}

		savingsMonths := rand.Float64() * 12
		if p.Budget == "tight" {
			savingsMonths = rand.Float64() * 3
		}
		riskTolerances := []string{"minimal", "cautious", "moderate", "aggressive"}
		riskIdx := 0
		if savingsMonths > 6 {
			riskIdx = 2 + rand.Intn(2)
		} else if savingsMonths > 2 {
			riskIdx = 1 + rand.Intn(2)
		}

		p.Financial = FinancialProfile{
			MonthlyIncome:        monthlyIncome,
			MonthlyExpenses:      monthlyExpenses,
			DiscretionaryBudget:  discretionary,
			CurrentSubscriptions: subs,
			TotalSubSpend:        totalSubCost,
			SavingsMonths:        savingsMonths,
			RiskTolerance:        riskTolerances[riskIdx],
			RecentPurchaseRegret: rand.Float64() < 0.25,
		}

		// Daily life - will be enriched by LLM later
		p.DailyLife = DailyLife{
			WakeTime:             "07:00",
			SleepTime:            "23:00",
			FreeHoursPerDay:      2.0,
			TopStruggles:         []string{"time", "focus"},
			DailyRoutine:         "work and personal tasks",
			MentalState:          "balanced",
			DiscoveryContext:     "online",
			AttentionSpanMinutes: 15,
			CurrentPriorities:    []string{"work", "health"},
		}

		personas = append(personas, p)
	}

	return personas
}

// enrichDailyLifeViaLLM enriches daily life context for personas via LLM
func (p *Pipeline) enrichDailyLifeViaLLM(personas []Persona, description string) []Persona {
	if len(personas) == 0 {
		return personas
	}

	// Process in batches of 5 to avoid overwhelming the LLM
	batchSize := 5
	var wg sync.WaitGroup
	var mu sync.Mutex
	enriched := make([]Persona, len(personas))

	for i := 0; i < len(personas); i += batchSize {
		end := i + batchSize
		if end > len(personas) {
			end = len(personas)
		}
		batch := personas[i:end]

		wg.Add(1)
		go func(startIdx int, batch []Persona) {
			defer wg.Done()

			// Build prompt for this batch
			names := make([]string, len(batch))
			for j := range batch {
				names[j] = batch[j].Name
			}

			var prompt string
			if len(batch) == 1 {
				prompt = fmt.Sprintf(`Generate daily life context for: "%s". Return ONLY valid JSON object.

{"wake_time": "HH:MM", "sleep_time": "HH:MM", "free_hours_per_day": float, "top_struggles": ["str", "str"], "daily_routine": "brief description", "mental_state": "overwhelmed|burned_out|coasting|focused", "discovery_context": "how they'd find this product", "attention_span_minutes": int, "current_priorities": ["str", "str", "str"]}

JSON only, no markdown.`, names[0])
			} else if len(batch) >= 2 {
				prompt = fmt.Sprintf(`Generate daily life context for these personas. Return ONLY a valid JSON array.

[
  {"name": "%s", "wake_time": "HH:MM", "sleep_time": "HH:MM", "free_hours_per_day": float, "top_struggles": ["str", "str"], "daily_routine": "brief description", "mental_state": "overwhelmed|burned_out|coasting|focused", "discovery_context": "how they'd find this product", "attention_span_minutes": int, "current_priorities": ["str", "str", "str"]},
  {"name": "%s", "wake_time": "HH:MM", "sleep_time": "HH:MM", "free_hours_per_day": float, "top_struggles": ["str", "str"], "daily_routine": "brief description", "mental_state": "overwhelmed|burned_out|coasting|focused", "discovery_context": "how they'd find this product", "attention_span_minutes": int, "current_priorities": ["str", "str", "str"]}
]

For each persona, be realistic about their schedule based on their role. Be diverse. JSON only, no markdown.`,
					names[0], names[1])
			}

			resp := p.maxclaw.Call(LLMRequest{
				Prompt:    prompt,
				MaxTokens: 1500,
			})

			if resp.Error != nil {
				log.Printf("Daily life enrichment failed: %v", resp.Error)
				mu.Lock()
				for j := range batch {
					enriched[startIdx+j] = batch[j]
				}
				mu.Unlock()
				return
			}

			// Parse response
			var dailyLife []map[string]interface{}
			jsonStr := extractJSON(resp.Content)
			if err := json.Unmarshal([]byte(fixJSON(jsonStr)), &dailyLife); err != nil {
				// Try single object
				var single map[string]interface{}
				if err2 := json.Unmarshal([]byte(fixJSON(jsonStr)), &single); err2 == nil {
					dailyLife = []map[string]interface{}{single}
				} else {
					log.Printf("Failed to parse daily life: %v", err)
					mu.Lock()
					for j := range batch {
						enriched[startIdx+j] = batch[j]
					}
					mu.Unlock()
					return
				}
			}

			// Map back to personas
			nameToIdx := make(map[string]int)
			for j := range batch {
				nameToIdx[batch[j].Name] = j
			}

			for _, dl := range dailyLife {
				dlName, ok := dl["name"].(string)
				if !ok {
					continue
				}
				if idx, exists := nameToIdx[dlName]; exists {
					batch[idx].DailyLife.WakeTime = toString(dl["wake_time"])
					batch[idx].DailyLife.SleepTime = toString(dl["sleep_time"])
					batch[idx].DailyLife.FreeHoursPerDay = toFloat(dl["free_hours_per_day"])
					batch[idx].DailyLife.TopStruggles = toStringSlice(dl["top_struggles"])
					batch[idx].DailyLife.DailyRoutine = toString(dl["daily_routine"])
					batch[idx].DailyLife.MentalState = toString(dl["mental_state"])
					batch[idx].DailyLife.DiscoveryContext = toString(dl["discovery_context"])
					batch[idx].DailyLife.AttentionSpanMinutes = toInt(dl["attention_span_minutes"])
					batch[idx].DailyLife.CurrentPriorities = toStringSlice(dl["current_priorities"])
				}
			}

			mu.Lock()
			for j := range batch {
				enriched[startIdx+j] = batch[j]
			}
			mu.Unlock()
		}(i, batch)
	}

	wg.Wait()
	return enriched
}

// generatePersonasViaLFM generates personas via LMStudio qwen/qwen3.5-9b
func (p *Pipeline) generatePersonasViaLFM(description string, count int) []Persona {
	priceEstimate := 29

	// Use batching (5 personas per batch) to stay within context limits on 9B
	batchSize := 5
	var allPersonas []Persona

	for batch := 0; batch*batchSize < count; batch++ {
		remaining := count - batch*batchSize
		batchCount := batchSize
		if remaining < batchSize {
			batchCount = remaining
		}

		prompt := fmt.Sprintf(`Generate %d personas for: "%s" ($%d/month)

Return ONLY valid JSON array. ALL fields required:
[{"name":"str","age":int,"role":"str","company_size":"solo|startup|smb|enterprise","experience_years":int,"pain_level":int,"skepticism":int,"current_workflow":"str","current_tools":"str","personality":"early_adopter|pragmatic|conservative|skeptical","archetype":"power_user|casual|struggling|non_user","budget":"tight|moderate|comfortable","decision_authority":"self|needs_approval|committee","financial":{"monthly_income":int,"monthly_expenses":int,"discretionary_budget":int,"current_subscriptions":[{"name":"str","monthly_cost":int,"usage":"daily|weekly|rarely|forgot_about_it"}],"total_sub_spend":int,"savings_months":float,"risk_tolerance":"minimal|cautious|moderate|aggressive","recent_purchase_regret":bool},"daily_life":{"wake_time":"HH:MM","sleep_time":"HH:MM","free_hours_per_day":float,"top_struggles":["str","str","str"],"daily_routine":"str","mental_state":"overwhelmed|burned_out|coasting|focused","discovery_context":"str","attention_span_minutes":int,"current_priorities":["str","str","str"]}}]

REALISTIC diversity. JSON only, no markdown.`, batchCount, description, priceEstimate)

		// Retry up to 2 times on failures
		var batchPersonas []Persona
		var lastErr error
		for attempt := 0; attempt < 2; attempt++ {
			lfmResp, err := callLFM(prompt)
			if err != nil {
				lastErr = err
				continue
			}

			batchPersonas, err = parsePersonas(lfmResp, batchCount)
			if err == nil && len(batchPersonas) > 0 {
				break
			}
			lastErr = err
		}

		if lastErr != nil {
			log.Printf("LFM batch %d/%d failed after retries: %v", batch+1, (count+batchSize-1)/batchSize, lastErr)
			continue
		}

		allPersonas = append(allPersonas, batchPersonas...)
	}

	return allPersonas
}

// callLFM makes a direct HTTP call to LMStudio qwen/qwen3.5-9b
func callLFM(prompt string) (string, error) {
	payload := map[string]interface{}{
		"model": "qwen/qwen3.5-9b",
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
		"temperature": 0.7,
		"max_tokens":  3000,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal: %w", err)
	}

	req, err := http.NewRequest("POST", "http://192.168.0.23:1234/v1/chat/completions", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 300 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("http call: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("http %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("parse: %w", err)
	}

	if len(result.Choices) == 0 {
		return "", fmt.Errorf("no choices in response")
	}

	return result.Choices[0].Message.Content, nil
}

// simulatePersonaViaLLM simulates a single persona's decision via MaxClaw
func (p *Pipeline) simulatePersonaViaLLM(persona Persona, landing *LandingPage, idea StartupIdea, forcedPrice int) SimulationResult {
	pricing := IdeaPricing{
		MonthlyPrice: 29,
		AnnualPrice:  290,
		TrialDays:    14,
		RequiresCC:   false,
		BillingCycle: "monthly",
	}

	if forcedPrice > 0 {
		pricing.MonthlyPrice = forcedPrice
		pricing.AnnualPrice = forcedPrice * 10
	}

	// Parse price from landing page only when price is not forced
	if forcedPrice <= 0 && landing != nil {
		if price := landing.Price; price != "" {
			var p int
			if _, err := fmt.Sscanf(price, "%d", &p); err == nil && p > 0 {
				pricing.MonthlyPrice = p
				pricing.AnnualPrice = p * 10
			}
		}
	}

	subsJSON, _ := json.Marshal(persona.Financial.CurrentSubscriptions)

	var features string
	if landing != nil {
		features = strings.Join(landing.Features, ", ")
	}

	var headline, subheadline string
	if landing != nil {
		headline = landing.Headline
		subheadline = landing.Subheadline
	} else {
		headline = idea.Description
		subheadline = idea.Reasoning
	}

	biasSection := ""
	if p.config.SimulationBias == "legacy" {
		biasSection = `Most people (95-98%) do NOT convert on first visit. Common reasons include procrastination, comparison shopping, partner consultation, distraction, budget uncertainty, and trust concerns.`
	} else {
		biasSection = `Be behaviorally realistic and balanced: convert only when value, urgency, trust, and affordability are all convincing. Reject when those conditions are not met. Do not force either outcome.`
	}

	isB2B := isB2BIdea(idea.Description)
	var financialSection string
	if isB2B {
		qaHeadcountCost := pricing.MonthlyPrice * 3 // rough: tool should replace ~3x its cost in labor
		financialSection = fmt.Sprintf(`YOUR BUSINESS REALITY:
- Agency monthly revenue: $%d
- Monthly operating costs: $%d
- Monthly software/tools budget: $%d
- Current tools ($%d/month total): %s
- Cash runway: %.1f months
- Risk tolerance: %s
- Recently made a bad purchase decision: %v

EVALUATE THIS AS A BUSINESS DECISION — compare $%d/month against:
- Cost of 1 QA engineer salary (~$2000-4000/month in CIS)
- Time your team wastes on manual testing each sprint
- Client capacity you could add without hiring`,
			persona.Financial.MonthlyIncome, persona.Financial.MonthlyExpenses,
			persona.Financial.DiscretionaryBudget, persona.Financial.TotalSubSpend, string(subsJSON),
			persona.Financial.SavingsMonths, persona.Financial.RiskTolerance,
			persona.Financial.RecentPurchaseRegret,
			pricing.MonthlyPrice)
		_ = qaHeadcountCost
	} else {
		financialSection = fmt.Sprintf(`YOUR FINANCIAL REALITY:
- Monthly income: $%d after tax
- Monthly expenses: $%d
- Discretionary budget: $%d/month (what's left after bills)
- Current subscriptions ($%d/month total): %s
- Savings runway: %.1f months
- Risk tolerance: %s
- Recently regretted a purchase: %v`,
			persona.Financial.MonthlyIncome, persona.Financial.MonthlyExpenses,
			persona.Financial.DiscretionaryBudget, persona.Financial.TotalSubSpend, string(subsJSON),
			persona.Financial.SavingsMonths, persona.Financial.RiskTolerance,
			persona.Financial.RecentPurchaseRegret)
	}

	prompt := fmt.Sprintf(`You are %s, a %d-year-old %s. Simulate YOUR REALISTIC REACTION to this landing page.

%s

YOUR DAILY LIFE:
- Schedule: %s to %s
- Free time: %.1f hours/day
- Mental state: %s
- Top struggles: %s

YOU SPEND %d MINUTES ON THIS LANDING PAGE:
Headline: %s
Subheadline: %s
Price: $%d/month
Trial: %d days %s
Features: %s

BEHAVIOR POLICY:
%s

OUTPUT RULES (STRICT):
- Return exactly one JSON object.
- Do NOT wrap in markdown or code fences.
- Do NOT include any text before or after JSON.
- Use double quotes for all keys and string values.
- Include every required key exactly once.

CRITICAL QUESTION (think honestly):
1. What do I HOPE this solves when I first see the headline?
2. After reading the landing page, does it actually deliver on that hope?

Return ONLY this JSON object (ALL fields required):
{
  "initial_hope": "what you hoped this would solve when you first saw it (1-2 sentences)",
  "hope_met": true/false,
  "hope_gap_reason": "if hope wasn't met, why not? (empty string if met)",
  "converted": true/false,
  "impression_score": 1-10,
  "relevance_score": 1-10,
  "intent_strength": 1-10,
  "friction_points": ["specific objections or blockers"],
  "pricing_reaction": "affordable" | "stretch" | "cant_afford",
  "cpl_equivalent": 5-50,
  "budget_check": "affordable" | "stretch" | "cant_afford",
  "priority_rank": 1-10,
  "time_available": true/false,
  "competing_with": "existing tool or habit",
  "decision_timeline": "impulse" | "this_week" | "next_month" | "never",
  "reasoning": "2-3 sentences explaining your decision as a real human (include procrastination, distractions, or partner consultation if relevant)"
}`,
		persona.Name, persona.Age, persona.Role,
		financialSection,
		persona.DailyLife.WakeTime, persona.DailyLife.SleepTime,
		persona.DailyLife.FreeHoursPerDay, persona.DailyLife.MentalState,
		strings.Join(persona.DailyLife.TopStruggles, ", "),
		persona.DailyLife.AttentionSpanMinutes,
		headline, subheadline,
		pricing.MonthlyPrice,
		pricing.TrialDays, ccText(pricing.RequiresCC),
		features,
		biasSection)

	// Retry up to 3 times on parse failures
	var lastResult SimulationResult
	for attempt := 0; attempt < 3; attempt++ {
		attemptPrompt := prompt
		if attempt > 0 {
			attemptPrompt += "\n\nCRITICAL RETRY: The last output was malformed. Return only a single valid JSON object. No prose."
		}
		resp := p.maxclaw.Call(LLMRequest{
			Prompt:    attemptPrompt,
			MaxTokens: 1000,
		})

		if resp.Error != nil {
			lastResult = SimulationResult{
				PersonaName: persona.Name,
				Converted:   false,
				Status:      "error",
				Reasoning:   fmt.Sprintf("LLM call failed: %v", resp.Error),
			}
			continue
		}

		result := parseSimulation(resp.Content, persona.Name)

		if result.Status != "error" {
			result.Persona = persona
			result.TestedPrice = pricing.MonthlyPrice
			return result
		}

		lastResult = result
		// Parse error - retry with same prompt
	}

	lastResult.Persona = persona
	lastResult.TestedPrice = pricing.MonthlyPrice
	return lastResult
}

// SaveCheckpointPipeline saves pipeline progress
func (p *Pipeline) SaveCheckpointPipeline(scored []ScoredIdea) {
	path := p.config.OutputDir + "/pipeline_checkpoint.json"
	data, err := json.Marshal(scored)
	if err != nil {
		return
	}
	_ = os.WriteFile(path, data, 0644)
}

// isB2BIdea returns true if the idea description or coordinates suggest a B2B product.
func isB2BIdea(description string) bool {
	desc := strings.ToLower(description)
	b2bSignals := []string{
		"agency", "agencies", "outsourc", "enterprise", "b2b", "saas",
		"team", "company", "companies", "business", "businesses",
		"startup", "organization", "client", "workflow", "pipeline",
		"developer tool", "dev tool", "ci/cd", "qa ", "testing",
	}
	for _, signal := range b2bSignals {
		if strings.Contains(desc, signal) {
			return true
		}
	}
	return false
}

// loadPersonasFromResult extracts personas from a previous pipeline result JSON.
// Returns one []Persona per idea slot (matching ideaCount).
func loadPersonasFromResult(path string, ideaCount int) ([][]Persona, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var result PipelineResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	if len(result.ScoredIdeas) == 0 {
		return nil, fmt.Errorf("no scored ideas in result file")
	}

	allPersonas := make([][]Persona, ideaCount)
	for ideaIdx := 0; ideaIdx < ideaCount && ideaIdx < len(result.ScoredIdeas); ideaIdx++ {
		seen := make(map[string]bool)
		var personas []Persona
		for _, sim := range result.ScoredIdeas[ideaIdx].Sims {
			if sim.Persona.Name != "" && !seen[sim.Persona.Name] {
				seen[sim.Persona.Name] = true
				personas = append(personas, sim.Persona)
			}
		}
		allPersonas[ideaIdx] = personas
		log.Printf("Reused %d personas for idea slot %d from %s", len(personas), ideaIdx, path)
	}
	return allPersonas, nil
}

// ── B2B Committee Functions ──

// generateB2BCommittees fans out numCommittees×3 parallel calls (one per role per committee).
func (p *Pipeline) generateB2BCommittees(numCommittees int) []B2BCommittee {
	type memberResult struct {
		committeeIdx int
		member       *B2BCommitteeMember
	}

	roles := []string{"budget_owner", "champion", "technical_evaluator"}
	totalCalls := numCommittees * len(roles)
	resultsCh := make(chan memberResult, totalCalls)
	var wg sync.WaitGroup
	sem := make(chan struct{}, p.config.MaxConcurrent)

	for i := 0; i < numCommittees; i++ {
		for _, role := range roles {
			wg.Add(1)
			sem <- struct{}{}
			go func(idx int, r string) {
				defer wg.Done()
				defer func() { <-sem }()
				member := p.generateB2BCommitteeMember(idx, r, numCommittees)
				resultsCh <- memberResult{committeeIdx: idx, member: member}
			}(i, role)
		}
	}

	go func() {
		wg.Wait()
		close(resultsCh)
	}()

	membersByCommittee := make(map[int][]B2BCommitteeMember)
	for r := range resultsCh {
		if r.member != nil {
			membersByCommittee[r.committeeIdx] = append(membersByCommittee[r.committeeIdx], *r.member)
		}
	}

	companyPrefixes := []string{"Agency", "DevShop", "Studio", "Lab", "TechCo", "Works", "Solutions", "Systems", "Digital", "CodeHouse"}
	var committees []B2BCommittee
	for i := 0; i < numCommittees; i++ {
		members := membersByCommittee[i]
		if len(members) == 0 {
			continue
		}
		suffix := string(rune('A' + i%26))
		if i >= 26 {
			suffix = fmt.Sprintf("%s%d", suffix, i/26)
		}
		companyName := fmt.Sprintf("%s %s", companyPrefixes[i%len(companyPrefixes)], suffix)
		committees = append(committees, B2BCommittee{
			CompanyName: companyName,
			Members:     members,
		})
	}

	log.Printf("Committee generation: requested %d, got %d", numCommittees, len(committees))
	return committees
}

// generateB2BCommitteeMember generates one committee member independently (no product context).
func (p *Pipeline) generateB2BCommitteeMember(committeeIdx int, memberRole string, totalCommittees int) *B2BCommitteeMember {
	var companyArchetype string
	switch {
	case committeeIdx < 3:
		companyArchetype = "small bootstrapped software agency (3-10 staff, $10k-40k/month revenue)"
	case committeeIdx < 6:
		companyArchetype = "mid-size funded tech company (15-50 staff, $50k-200k/month revenue)"
	default:
		companyArchetype = "established software company (50+ staff, $200k+/month revenue)"
	}

	roleDescriptions := map[string]string{
		"budget_owner":        "the person who controls the budget and makes final purchase decisions (CEO, CFO, or CTO with budget authority)",
		"champion":            "the internal advocate who discovers and champions new tools (could be a lead developer, QA lead, or tech-savvy team lead)",
		"technical_evaluator": "the person who evaluates technical fit and integration requirements (senior developer or technical architect)",
	}
	roleDescription := roleDescriptions[memberRole]
	if roleDescription == "" {
		roleDescription = memberRole
	}

	vetoPowerJSON := "false"
	if memberRole == "budget_owner" {
		vetoPowerJSON = "true"
	}

	prompt := fmt.Sprintf(`Generate 1 realistic professional for a software buying committee.

Company type: %s
Their committee role: %s (committee_role field: "%s")
Person slot %d of %d.

Do NOT reference any specific product being evaluated. Generate their background independently.
Use a realistic name from Russia, Ukraine, Kazakhstan, or Belarus.

Return ONLY a single JSON object with ALL fields required:
{
  "name": "string",
  "age": int,
  "role": "string (specific job title)",
  "company_size": "startup" | "smb" | "enterprise",
  "experience_years": int,
  "pain_level": int (1-10),
  "skepticism": int (1-10),
  "current_workflow": "string",
  "current_tools": "string",
  "personality": "early_adopter" | "pragmatic" | "conservative" | "skeptical",
  "archetype": "power_user" | "casual" | "struggling" | "non_user",
  "budget": "tight" | "moderate" | "comfortable",
  "decision_authority": "self" | "needs_approval" | "committee",
  "financial": {
    "monthly_income": int (business budget in USD),
    "monthly_expenses": int,
    "discretionary_budget": int (monthly software/tools budget),
    "current_subscriptions": [{"name": "string", "monthly_cost": int, "usage": "daily|weekly|rarely|forgot_about_it"}],
    "total_sub_spend": int,
    "savings_months": float,
    "risk_tolerance": "minimal" | "cautious" | "moderate" | "aggressive",
    "recent_purchase_regret": bool
  },
  "daily_life": {
    "wake_time": "HH:MM",
    "sleep_time": "HH:MM",
    "free_hours_per_day": float,
    "top_struggles": ["string", "string", "string"],
    "daily_routine": "string",
    "mental_state": "overwhelmed" | "burned_out" | "coasting" | "focused",
    "discovery_context": "string",
    "attention_span_minutes": int,
    "current_priorities": ["string", "string", "string"]
  },
  "committee_role": "%s",
  "personal_goals": ["career/workload/recognition goal 1", "goal 2"],
  "business_goals": ["company goal 1", "company goal 2"],
  "reports_to": "string (manager's role title, or empty string if top decision-maker)",
  "veto_power": %s
}

Output ONLY the JSON object. No markdown, no explanation.`,
		companyArchetype, roleDescription, memberRole,
		committeeIdx+1, totalCommittees,
		memberRole, vetoPowerJSON)

	for attempt := 0; attempt < 3; attempt++ {
		resp := p.maxclaw.Call(LLMRequest{
			Prompt:    prompt,
			MaxTokens: 4000,
		})
		if resp.Error != nil {
			log.Printf("Committee member generation attempt %d failed: %v", attempt+1, resp.Error)
			continue
		}
		member := parseCommitteeMember(resp.Content, memberRole)
		if member != nil {
			if p.config.Verbose {
				fmt.Printf("  ✓ Committee member: %s (%s, %s)\n",
					member.Persona.Name, member.Persona.Role, member.CommitteeRole)
			}
			return member
		}
		log.Printf("Committee member parse attempt %d failed | raw[:200]: %s", attempt+1, truncate(resp.Content, 200))
	}
	return nil
}

// simulateB2BDecision runs a two-phase committee simulation:
// Phase 1: individual evaluations (parallel), Phase 2: group discussion (single LLM call).
func (p *Pipeline) simulateB2BDecision(committee *B2BCommittee, landing *LandingPage, idea StartupIdea, price int) B2BCommitteeResult {
	// Phase 1: individual evaluations in parallel
	individualEvals := make([]SimulationResult, len(committee.Members))
	var wg sync.WaitGroup
	var mu sync.Mutex

	for i, member := range committee.Members {
		wg.Add(1)
		go func(idx int, m B2BCommitteeMember) {
			defer wg.Done()
			result := p.simulatePersonaViaLLM(m.Persona, landing, idea, price)
			mu.Lock()
			individualEvals[idx] = result
			mu.Unlock()
		}(i, member)
	}
	wg.Wait()

	if p.config.Verbose {
		fmt.Printf("\n── Committee: %s — Individual Reactions ──\n", committee.CompanyName)
		for i, m := range committee.Members {
			e := individualEvals[i]
			converted := "NO"
			if e.Converted {
				converted = "YES"
			}
			fmt.Printf("  %s (%s): intent=%d/10, pricing=%s, buy=%s\n",
				m.Persona.Name, m.CommitteeRole, e.IntentStrength, e.PricingReaction, converted)
			if len(e.FrictionPoints) > 0 {
				fmt.Printf("    Concerns: %s\n", strings.Join(e.FrictionPoints, "; "))
			}
		}
	}

	// Phase 2: committee discussion
	var attendeesBuilder strings.Builder
	for i, member := range committee.Members {
		e := individualEvals[i]
		convertedText := "No"
		if e.Converted {
			convertedText = "Yes"
		}
		attendeesBuilder.WriteString(fmt.Sprintf(
			"\n%s (%s, %s):\n  Personal agenda: %s\n  Business priority: %s\n  Initial reaction: %d/10, pricing_reaction=%s\n  Key concerns: %s\n  Would buy alone: %s\n",
			member.Persona.Name, member.Persona.Role, member.CommitteeRole,
			strings.Join(member.PersonalGoals, ", "),
			strings.Join(member.BusinessGoals, ", "),
			e.IntentStrength, e.PricingReaction,
			strings.Join(e.FrictionPoints, "; "),
			convertedText,
		))
	}

	var headline, subheadline, features string
	if landing != nil {
		headline = landing.Headline
		subheadline = landing.Subheadline
		features = strings.Join(landing.Features, ", ")
	} else {
		headline = idea.Description
	}

	discussionPrompt := fmt.Sprintf(`You are observing a buying committee meeting at %s.

They are evaluating:
Headline: %s
Subheadline: %s
Price: $%d/month
Features: %s

--- ATTENDEES ---%s
--- DISCUSSION ---
Simulate their conversation. Who pushes for it? Who blocks it?
What arguments land? Does the champion overcome objections?
What does the budget owner ultimately decide?

Return ONLY this JSON (no markdown, no explanation):
{
  "final_decision": "purchase",
  "converted": true,
  "decision_driver": "name of person who drove the outcome",
  "group_reasoning": "2-3 sentences on what happened in the discussion",
  "vote_breakdown": {"Name1": "yes", "Name2": "no"},
  "key_friction": ["top objections that stuck"],
  "next_step": "what happens after this meeting"
}

Valid values for final_decision: "purchase" | "reject" | "pilot" | "defer"`,
		committee.CompanyName,
		headline, subheadline, price, features,
		attendeesBuilder.String())

	for attempt := 0; attempt < 3; attempt++ {
		attemptPrompt := discussionPrompt
		if attempt > 0 {
			attemptPrompt += "\n\nCRITICAL RETRY: Return only a single valid JSON object. No prose."
		}
		resp := p.maxclaw.Call(LLMRequest{
			Prompt:    attemptPrompt,
			MaxTokens: 1500,
		})
		if resp.Error != nil {
			log.Printf("Committee discussion attempt %d failed: %v", attempt+1, resp.Error)
			continue
		}
		result, err := parseCommitteeDecision(resp.Content)
		if err == nil {
			result.IndividualEvals = individualEvals
			result.TestedPrice = price
			result.Converted = result.FinalDecision == "purchase" || result.FinalDecision == "pilot"
			if p.config.Verbose {
				fmt.Printf("\n── Committee Decision: %s ──\n", committee.CompanyName)
				fmt.Printf("  Final: %s | Driver: %s\n", result.FinalDecision, result.DecisionDriver)
				fmt.Printf("  Reasoning: %s\n", result.GroupReasoning)
				var votes []string
				for name, vote := range result.VoteBreakdown {
					votes = append(votes, fmt.Sprintf("%s:%s", name, vote))
				}
				if len(votes) > 0 {
					fmt.Printf("  Votes: %s\n", strings.Join(votes, ", "))
				}
				fmt.Println()
			}
			return result
		}
		log.Printf("Committee decision parse attempt %d failed: %v | raw[:200]: %s",
			attempt+1, err, truncate(resp.Content, 200))
	}

	// Fallback: majority vote from individual evals
	yesCount := 0
	for _, e := range individualEvals {
		if e.Converted {
			yesCount++
		}
	}
	finalDecision := "reject"
	if yesCount > len(individualEvals)/2 {
		finalDecision = "purchase"
	}
	return B2BCommitteeResult{
		IndividualEvals: individualEvals,
		FinalDecision:   finalDecision,
		Converted:       finalDecision == "purchase",
		GroupReasoning:  "Committee discussion unavailable; majority vote fallback used.",
		KeyFriction:     []string{"discussion simulation failed"},
		TestedPrice:     price,
	}
}

// b2bCommitteeToSimResult converts a B2BCommitteeResult to SimulationResult for scoring.
func b2bCommitteeToSimResult(result B2BCommitteeResult, committee *B2BCommittee) SimulationResult {
	converted := result.FinalDecision == "purchase" || result.FinalDecision == "pilot"

	var totalIntent, totalRelevance, totalImpression float64
	pricingReactions := make(map[string]int)
	for _, e := range result.IndividualEvals {
		totalIntent += float64(e.IntentStrength)
		totalRelevance += float64(e.RelevanceScore)
		totalImpression += float64(e.ImpressionScore)
		pricingReactions[e.PricingReaction]++
	}
	n := len(result.IndividualEvals)
	if n == 0 {
		n = 1
	}

	bestReaction := "stretch"
	bestCount := 0
	for reaction, count := range pricingReactions {
		if count > bestCount {
			bestCount = count
			bestReaction = reaction
		}
	}

	var championPersona Persona
	for _, m := range committee.Members {
		if m.CommitteeRole == "champion" {
			championPersona = m.Persona
			break
		}
	}
	if championPersona.Name == "" && len(committee.Members) > 0 {
		championPersona = committee.Members[0].Persona
	}

	var voteLines []string
	for name, vote := range result.VoteBreakdown {
		voteLines = append(voteLines, fmt.Sprintf("%s:%s", name, vote))
	}
	reasoning := result.GroupReasoning
	if len(voteLines) > 0 {
		reasoning += " [Votes: " + strings.Join(voteLines, ", ") + "]"
	}

	status := "rejected"
	if converted {
		status = "converted"
	}

	return SimulationResult{
		PersonaName:     committee.CompanyName + " Committee",
		Persona:         championPersona,
		Archetype:       "committee",
		ImpressionScore: int(totalImpression / float64(n)),
		RelevanceScore:  int(totalRelevance / float64(n)),
		IntentStrength:  int(totalIntent / float64(n)),
		Converted:       converted,
		Status:          status,
		FrictionPoints:  result.KeyFriction,
		PricingReaction: bestReaction,
		Reasoning:       reasoning,
		TestedPrice:     result.TestedPrice,
	}
}

// parseCommitteeMember parses an LLM response into a B2BCommitteeMember.
func parseCommitteeMember(content string, defaultRole string) *B2BCommitteeMember {
	jsonStr := extractJSON(content)
	fixed := fixJSON(jsonStr)

	// Parse into a flat struct that includes both persona and committee fields
	var raw struct {
		Name              string           `json:"name"`
		Age               int              `json:"age"`
		Role              string           `json:"role"`
		CompanySize       string           `json:"company_size"`
		ExperienceYears   int              `json:"experience_years"`
		PainLevel         int              `json:"pain_level"`
		CurrentWorkflow   string           `json:"current_workflow"`
		Budget            string           `json:"budget"`
		DecisionAuthority string           `json:"decision_authority"`
		Skepticism        int              `json:"skepticism"`
		Personality       string           `json:"personality"`
		Archetype         string           `json:"archetype"`
		CurrentTools      string           `json:"current_tools"`
		Financial         FinancialProfile `json:"financial"`
		DailyLife         DailyLife        `json:"daily_life"`
		CommitteeRole     string           `json:"committee_role"`
		PersonalGoals     []string         `json:"personal_goals"`
		BusinessGoals     []string         `json:"business_goals"`
		ReportsTo         string           `json:"reports_to"`
		VetoPower         bool             `json:"veto_power"`
	}

	if err := json.Unmarshal([]byte(fixed), &raw); err != nil {
		return nil
	}
	if raw.Name == "" || raw.Role == "" {
		return nil
	}

	committeeRole := raw.CommitteeRole
	if committeeRole == "" {
		committeeRole = defaultRole
	}

	persona := Persona{
		Name:              raw.Name,
		Age:               raw.Age,
		Role:              raw.Role,
		CompanySize:       raw.CompanySize,
		ExperienceYears:   raw.ExperienceYears,
		PainLevel:         raw.PainLevel,
		CurrentWorkflow:   raw.CurrentWorkflow,
		Budget:            raw.Budget,
		DecisionAuthority: raw.DecisionAuthority,
		Skepticism:        raw.Skepticism,
		Personality:       raw.Personality,
		Archetype:         raw.Archetype,
		CurrentTools:      raw.CurrentTools,
		Financial:         raw.Financial,
		DailyLife:         raw.DailyLife,
	}

	return &B2BCommitteeMember{
		Persona:       persona,
		CommitteeRole: committeeRole,
		PersonalGoals: raw.PersonalGoals,
		BusinessGoals: raw.BusinessGoals,
		ReportsTo:     raw.ReportsTo,
		VetoPower:     raw.VetoPower,
	}
}

// parseCommitteeDecision parses the committee discussion LLM response.
func parseCommitteeDecision(content string) (B2BCommitteeResult, error) {
	jsonStr := extractJSON(content)
	fixed := fixJSON(jsonStr)

	var raw struct {
		FinalDecision  string            `json:"final_decision"`
		Converted      bool              `json:"converted"`
		DecisionDriver string            `json:"decision_driver"`
		GroupReasoning string            `json:"group_reasoning"`
		VoteBreakdown  map[string]string `json:"vote_breakdown"`
		KeyFriction    []string          `json:"key_friction"`
		NextStep       string            `json:"next_step"`
	}

	if err := json.Unmarshal([]byte(fixed), &raw); err != nil {
		return B2BCommitteeResult{}, fmt.Errorf("json unmarshal: %w", err)
	}

	validDecisions := map[string]bool{"purchase": true, "reject": true, "pilot": true, "defer": true}
	if !validDecisions[raw.FinalDecision] {
		return B2BCommitteeResult{}, fmt.Errorf("invalid final_decision: %q", raw.FinalDecision)
	}

	return B2BCommitteeResult{
		FinalDecision:  raw.FinalDecision,
		Converted:      raw.FinalDecision == "purchase" || raw.FinalDecision == "pilot",
		DecisionDriver: raw.DecisionDriver,
		GroupReasoning: raw.GroupReasoning,
		VoteBreakdown:  raw.VoteBreakdown,
		KeyFriction:    raw.KeyFriction,
		NextStep:       raw.NextStep,
	}, nil
}
