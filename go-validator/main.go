package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	// ── Flags ──
	mode := flag.String("mode", "pipeline", "pipeline|generate|learn|insights")
	numIdeas := flag.Int("ideas", 100, "Ideas per batch")
	numPersonas := flag.Int("personas", 100, "Personas per idea")
	minScore := flag.Float64("min-score", 0.5, "Min idea fit score")
	maxConcurrent := flag.Int("max-concurrent", 500, "Max concurrent goroutines")
	profilePath := flag.String("profile", "../profile.json", "Profile JSON path")
	outputDir := flag.String("output", "results", "Output directory")
	epochs := flag.Int("epochs", 1, "Learning epochs to run")
	resume := flag.Bool("resume", false, "Resume from checkpoint")
	verbose := flag.Bool("verbose", false, "Verbose output")
	debug := flag.Bool("debug", false, "Enable debug logging and persona parse dumps")
	enrichmentMode := flag.String("enrichment-mode", "maxclaw", "Enrichment mode: maxclaw|programmatic|llm-lfm")
	learningMode := flag.String("learning-mode", "hybrid", "Learning mode: llm_explain|score_only|hybrid|holdout")
	pricingMode := flag.String("pricing-mode", "grid", "Pricing mode: fixed|grid")
	sampleMode := flag.String("sample-mode", "two_stage", "Sampling mode: single_stage|two_stage")
	simulationBias := flag.String("simulation-bias", "neutral", "Simulation prompt bias: legacy|neutral")
	funnelStage := flag.String("funnel-stage", "cold", "Traffic stage for calibration: cold|warm|high_intent")
	conversionFloor := flag.Float64("conversion-floor", 0, "Minimum acceptable conversion before learning updates (0=auto by stage)")
	enforceCalibration := flag.Bool("enforce-calibration", true, "Skip learning updates when below conversion floor")
	ciThreshold := flag.Float64("ci-threshold", 0.55, "Minimum holdout agreement ratio to apply learning")
	deepPersonas := flag.Int("deep-personas", 60, "Personas per idea in deep evaluation stage")
	deepTopK := flag.Int("deep-top-k", 3, "Top ideas to deep-evaluate in two-stage mode")
	pricePointsRaw := flag.String("price-points", "9,19,29,49,99", "Comma-separated price points for grid pricing")
	seed := flag.Int64("seed", time.Now().UnixNano(), "Random seed for reproducible runs")
	pinnedIdeaPath := flag.String("pinned-idea", "", "Path to a JSON file containing a StartupIdea to validate directly")
	reusePersonasFrom := flag.String("reuse-personas", "", "Path to a previous pipeline result JSON; reuses its personas and skips persona generation")
	b2bMode := flag.String("b2b-mode", "auto", "B2B simulation mode: individual|committee|auto")
	flag.Parse()
	pricePoints := parsePricePoints(*pricePointsRaw)

	var pinnedIdeas []StartupIdea
	if *pinnedIdeaPath != "" {
		data, err := os.ReadFile(*pinnedIdeaPath)
		if err != nil {
			log.Fatalf("Failed to read pinned idea file: %v", err)
		}
		var idea StartupIdea
		if err := json.Unmarshal(data, &idea); err != nil {
			log.Fatalf("Failed to parse pinned idea JSON: %v", err)
		}
		pinnedIdeas = []StartupIdea{idea}
	}

	config := PipelineConfig{
		PinnedIdeas:        pinnedIdeas,
		ReusePersonasFrom:  *reusePersonasFrom,
		NumIdeas:           *numIdeas,
		NumPersonas:        *numPersonas,
		DeepPersonas:       *deepPersonas,
		DeepTopK:           *deepTopK,
		MinScore:           *minScore,
		MaxConcurrent:      *maxConcurrent,
		ProfilePath:        *profilePath,
		OutputDir:          *outputDir,
		PricePoints:        pricePoints,
		Epochs:             *epochs,
		Resume:             *resume,
		Verbose:            *verbose,
		Debug:              *debug,
		Mode:               *mode,
		EnrichmentMode:     *enrichmentMode,
		LearningMode:       *learningMode,
		PricingMode:        *pricingMode,
		SampleMode:         *sampleMode,
		SimulationBias:     *simulationBias,
		FunnelStage:        *funnelStage,
		ConversionFloor:    *conversionFloor,
		EnforceCalibration: *enforceCalibration,
		CIThreshold:        *ciThreshold,
		Seed:               *seed,
		B2BMode:            *b2bMode,
	}

	// Ensure results dir exists
	os.MkdirAll(config.OutputDir, 0755)

	switch *mode {
	case "generate":
		runGenerate(config)
	case "insights":
		runInsights(config)
	case "learn":
		runLearn(config)
	default:
		runPipeline(config)
	}
}

// runGenerate generates ideas without LLM (instant, for inspection)
func runGenerate(config PipelineConfig) {
	fmt.Printf("Generating %d ideas (min score: %.2f)...\n\n", config.NumIdeas, config.MinScore)

	profile, err := LoadProfile(config.ProfilePath)
	if err != nil {
		log.Fatalf("Failed to load profile: %v", err)
	}

	ideas := GenerateIdeas(profile, config.NumIdeas, config.MinScore)

	fmt.Printf("\nTop 20 ideas:\n\n")
	top := 20
	if len(ideas) < top {
		top = len(ideas)
	}
	for i, idea := range ideas[:top] {
		fmt.Printf("%2d. Score: %.3f | %s\n", i+1, idea.FitScore, idea.Description)
		fmt.Printf("    %s\n", idea.Reasoning)
		fmt.Println()
	}
	fmt.Printf("Total: %d ideas\n", len(ideas))
}

// runInsights prints learning history and current weights
func runInsights(config PipelineConfig) {
	learnerPath := config.OutputDir + "/learning_state.json"
	learner := NewLearner(learnerPath)
	learner.PrintInsights()
}

// runLearn runs multiple epochs, learning after each
func runLearn(config PipelineConfig) {
	fmt.Printf("Running %d learning epochs...\n", config.Epochs)

	for epoch := 1; epoch <= config.Epochs; epoch++ {
		fmt.Printf("\n%s\nEPOCH %d / %d\n%s\n",
			repeatStr("=", 70), epoch, config.Epochs, repeatStr("=", 70))

		result, err := runSinglePipeline(config)
		if err != nil {
			log.Printf("Epoch %d failed: %v", epoch, err)
			continue
		}

		// Save outputs
		ts := time.Now().Unix()
		jsonPath := fmt.Sprintf("%s/epoch_%d_%d.json", config.OutputDir, epoch, ts)
		htmlPath := fmt.Sprintf("%s/epoch_%d_%d.html", config.OutputDir, epoch, ts)

		if err := GenerateJSONReport(result, jsonPath); err != nil {
			log.Printf("Warning: JSON save failed: %v", err)
		}
		if err := GenerateHTMLReport(result, htmlPath); err != nil {
			log.Printf("Warning: HTML save failed: %v", err)
		}

		PrintSummary(result)
		fmt.Printf("\nResults: %s\n", jsonPath)
	}

	fmt.Println("\nLearning complete. Run with -mode insights to view learned patterns.")
}

// runPipeline runs the full pipeline once
func runPipeline(config PipelineConfig) {
	result, err := runSinglePipeline(config)
	if err != nil {
		log.Fatalf("Pipeline failed: %v", err)
	}

	// Save outputs
	ts := time.Now().Unix()
	jsonPath := fmt.Sprintf("%s/pipeline_%d.json", config.OutputDir, ts)
	htmlPath := fmt.Sprintf("%s/pipeline_%d.html", config.OutputDir, ts)

	if err := GenerateJSONReport(result, jsonPath); err != nil {
		log.Printf("Warning: JSON save failed: %v", err)
	} else {
		fmt.Printf("\nJSON: %s\n", jsonPath)
	}

	if err := GenerateHTMLReport(result, htmlPath); err != nil {
		log.Printf("Warning: HTML save failed: %v", err)
	} else {
		fmt.Printf("HTML: %s\n", htmlPath)
	}

	PrintSummary(result)
}

// runSinglePipeline creates and runs a pipeline, returning results
func runSinglePipeline(config PipelineConfig) (*PipelineResult, error) {
	pipeline, err := NewPipeline(config)
	if err != nil {
		return nil, fmt.Errorf("creating pipeline: %w", err)
	}
	return pipeline.Run()
}

func repeatStr(s string, n int) string {
	result := make([]byte, 0, len(s)*n)
	for i := 0; i < n; i++ {
		result = append(result, s...)
	}
	return string(result)
}

func parsePricePoints(raw string) []int {
	parts := strings.Split(raw, ",")
	prices := make([]int, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		v, err := strconv.Atoi(p)
		if err != nil || v <= 0 {
			continue
		}
		prices = append(prices, v)
	}
	if len(prices) == 0 {
		return []int{29}
	}
	return prices
}
