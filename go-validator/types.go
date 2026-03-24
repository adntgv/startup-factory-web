package main

import "time"

// ── New types for the parallel pipeline ──

// Dimension represents one axis of the startup idea space
type Dimension struct {
	Name    string
	Options []string
}

// ProfilePreferences from profile.json preferences section
type ProfilePreferences struct {
	TargetCustomer    string   `json:"target_customer"`
	BusinessModel     []string `json:"business_model"`
	AvoidTechnology   []string `json:"avoid_technology"`
	AvoidIndustries   []string `json:"avoid_industries"`
	AvoidInteraction  []string `json:"avoid_interaction"`
	AvoidMonetization []string `json:"avoid_monetization"`
	FavorTechnology   []string `json:"favor_technology"`
	FavorDistribution []string `json:"favor_distribution"`
	FavorMonetization []string `json:"favor_monetization"`
	FavorAudience     []string `json:"favor_audience"`
	FavorGeography    []string `json:"favor_geography"`
}

// ProfileConstraints from profile.json constraints section
type ProfileConstraints struct {
	Hard []string `json:"hard"`
	Soft []string `json:"soft"`
}

// FounderProfile loaded from profile.json
type FounderProfile struct {
	Strengths      []string               `json:"strengths"`
	Limitations    []string               `json:"limitations"`
	Conditions     map[string]interface{} `json:"conditions"`
	Preferences    ProfilePreferences     `json:"preferences"`
	Constraints    ProfileConstraints     `json:"constraints"`
	ScoringWeights map[string]float64     `json:"scoring_weights"`
}

// StartupIdea is a point in the topology space with scoring
type StartupIdea struct {
	Coordinates      map[string]string     `json:"coordinates"`
	FitScore         float64               `json:"fit_score"`
	FitBreakdown     map[string]float64    `json:"fit_breakdown"`
	Reasoning        string                `json:"reasoning"`
	Description      string                `json:"description"`
	ICP              *IdealCustomerProfile `json:"icp,omitempty"`
	StrategicPosture string                `json:"strategic_posture,omitempty"` // best_free | best_premium | best_niche | best_distribution
}

// IdealCustomerProfile - ad platform style targeting parameters
type IdealCustomerProfile struct {
	TargetAudience  string   `json:"target_audience"`  // from idea coordinates: audience
	TargetGeography string   `json:"target_geography"` // from idea coordinates: geography
	AgeRange        string   `json:"age_range"`        // derived from audience (e.g., "18-24", "25-34")
	Interests       []string `json:"interests"`        // derived from category/audience
	JobRoles        []string `json:"job_roles"`        // derived from audience
}

// SimulationMetrics holds aggregate metrics for one idea's simulations
type SimulationMetrics struct {
	Score              float64 `json:"score"`
	CompositeScore     float64 `json:"composite_score"`
	ConversionRate     float64 `json:"conversion_rate"`
	ConversionCILow    float64 `json:"conversion_ci_low"`
	ConversionCIHigh   float64 `json:"conversion_ci_high"`
	UncertaintyPenalty float64 `json:"uncertainty_penalty"`
	SimulatedCPL       float64 `json:"simulated_cpl"`
	IntentStrength     float64 `json:"intent_strength"`
	FrictionScore      float64 `json:"friction_score"`
	AvgImpression      float64 `json:"avg_impression"`
	AvgRelevance       float64 `json:"avg_relevance"`
	Conversions        int     `json:"conversions"`
	Rejections         int     `json:"rejections"`
	Errors             int     `json:"errors"`
	TotalPersonas      int     `json:"total_personas"`
	ValidPersonas      int     `json:"valid_personas"` // conversions + rejections (excludes errors)
}

type PricePointMetrics struct {
	Price   int               `json:"price"`
	Metrics SimulationMetrics `json:"metrics"`
}

// DimensionWeights tracks learned sampling weights per dimension
type DimensionWeights struct {
	Weights map[string]map[string]float64 `json:"weights"` // dim → option → weight
	Epoch   int                           `json:"epoch"`
}

// DynamicTopology extends the static topology with learned dimensions
type DynamicTopology struct {
	Base    map[string]Dimension `json:"-"`
	Learned map[string]Dimension `json:"learned_dimensions"`
	Order   []string             `json:"dimension_order"`
}

// DecisionAnalysis is the result of analyzing one persona's decision
type DecisionAnalysis struct {
	Persona            string   `json:"persona"`
	Decision           string   `json:"decision"`
	PositiveDimensions []string `json:"positive_dimensions"`
	NegativeDimensions []string `json:"negative_dimensions"`
	MissingFactors     []string `json:"missing_factors"`
}

// SuggestedDimension is a new dimension proposed by the LLM analysis
type SuggestedDimension struct {
	Name         string   `json:"name"`
	Reason       string   `json:"reason"`
	Options      []string `json:"options"`
	MentionCount int      `json:"mention_count"`
}

// PipelineConfig configures the pipeline run
type PipelineConfig struct {
	PinnedIdeas        []StartupIdea `json:"pinned_ideas,omitempty"`  // If set, skip idea generation and use these
	ReusePersonasFrom  string        `json:"reuse_personas_from,omitempty"` // Path to a previous result JSON; reuses its personas, skips phases 2+3
	NumIdeas           int           `json:"num_ideas"`
	NumPersonas        int      `json:"num_personas"`
	DeepPersonas       int      `json:"deep_personas"`
	DeepTopK           int      `json:"deep_top_k"`
	MinScore           float64  `json:"min_score"`
	ProfilePath        string   `json:"profile_path"`
	OutputDir          string   `json:"output_dir"`
	Providers          []string `json:"providers"`
	PricePoints        []int    `json:"price_points"`
	MaxConcurrent      int      `json:"max_concurrent"`
	Resume             bool     `json:"resume"`
	Verbose            bool     `json:"verbose"`
	Debug              bool     `json:"debug"`
	Epochs             int      `json:"epochs"`
	Mode               string   `json:"mode"`
	EnrichmentMode     string   `json:"enrichment_mode"` // maxclaw|programmatic|llm-lfm
	LearningMode       string   `json:"learning_mode"`   // llm_explain|score_only|hybrid|holdout
	PricingMode        string   `json:"pricing_mode"`    // fixed|grid
	SampleMode         string   `json:"sample_mode"`     // single_stage|two_stage
	SimulationBias     string   `json:"simulation_bias"` // legacy|neutral
	FunnelStage        string   `json:"funnel_stage"`    // cold|warm|high_intent
	B2BMode            string   `json:"b2b_mode"`        // "individual" | "committee" | "auto" (default "auto")
	ConversionFloor    float64  `json:"conversion_floor"`
	EnforceCalibration bool     `json:"enforce_calibration"`
	CIThreshold        float64  `json:"ci_threshold"`
	Seed               int64    `json:"seed"`
}

// ScoredIdea is an idea with full simulation results and ranking
type ScoredIdea struct {
	Rank             int                 `json:"rank"`
	Idea             StartupIdea         `json:"idea"`
	Landing          *LandingPage        `json:"landing_page"`
	Metrics          SimulationMetrics   `json:"metrics"`
	Sims             []SimulationResult  `json:"simulations"`
	PriceExperiments []PricePointMetrics `json:"price_experiments,omitempty"`
	SelectedPrice    int                 `json:"selected_price"`
	RankStability    float64             `json:"rank_stability"`
}

// PipelineStats holds overall run statistics
type PipelineStats struct {
	TotalCalls          int                `json:"total_calls"`
	Successful          int                `json:"successful"`
	Failed              int                `json:"failed"`
	TotalTime           float64            `json:"total_time_seconds"`
	Throughput          float64            `json:"throughput_calls_per_sec"`
	PhaseTimes          map[string]float64 `json:"phase_times"`
	LimiterStats        []string           `json:"limiter_stats,omitempty"`
	CalibrationWarnings []string           `json:"calibration_warnings,omitempty"`
	ObservedConversion  float64            `json:"observed_conversion"`
	ExpectedConvLow     float64            `json:"expected_conv_low"`
	ExpectedConvHigh    float64            `json:"expected_conv_high"`
	LearningGatePassed  bool               `json:"learning_gate_passed"`
}

// PipelineResult is the full output of a pipeline run
type PipelineResult struct {
	ScoredIdeas   []ScoredIdea            `json:"scored_ideas"`
	Stats         PipelineStats           `json:"stats"`
	ProviderStats map[string]ProviderStat `json:"provider_stats"`
	Timestamp     string                  `json:"timestamp"`
	Config        PipelineConfig          `json:"config"`
}

// LearningEpoch records what was learned in one epoch
type LearningEpoch struct {
	Epoch         int                 `json:"epoch"`
	IdeasTested   int                 `json:"ideas_tested"`
	AvgConversion float64             `json:"avg_conversion"`
	WeightDeltas  map[string]float64  `json:"weight_deltas"`
	NewDimensions []string            `json:"new_dimensions"`
	NewOptions    map[string][]string `json:"new_options"`
	Timestamp     string              `json:"timestamp"`
}

// Config holds validation configuration
type Config struct {
	WorkersPerProvider  int
	Providers           []string
	Iterations          int
	IdeasPerIteration   int
	PersonasPerIdea     int
	Timeout             time.Duration
	GenerationProvider  string   // Preferred provider for persona/landing generation
	SimulationProviders []string // Providers for simulations
}

// LLMRequest represents a request to an LLM provider
type LLMRequest struct {
	Prompt      string
	MaxTokens   int
	Temperature float64
	TaskType    string // "personas", "landing", "simulation"
	TaskID      string
}

// LLMResponse represents a response from an LLM provider
type LLMResponse struct {
	Content  string
	Thinking string // reasoning/thinking block content (if any)
	Provider string
	Latency  float64
	Error    error
}

// Subscription represents a subscription service
type Subscription struct {
	Name        string `json:"name"`
	MonthlyCost int    `json:"monthly_cost"`
	Usage       string `json:"usage"` // "daily", "weekly", "rarely", "forgot_about_it"
}

// FinancialProfile represents financial constraints
type FinancialProfile struct {
	MonthlyIncome        int            `json:"monthly_income"`       // After tax, USD
	MonthlyExpenses      int            `json:"monthly_expenses"`     // Rent, food, bills
	DiscretionaryBudget  int            `json:"discretionary_budget"` // What's left for wants
	CurrentSubscriptions []Subscription `json:"current_subscriptions"`
	TotalSubSpend        int            `json:"total_sub_spend"`        // Sum of all subs
	SavingsMonths        float64        `json:"savings_months"`         // Emergency runway
	RiskTolerance        string         `json:"risk_tolerance"`         // "minimal", "cautious", "moderate", "aggressive"
	RecentPurchaseRegret bool           `json:"recent_purchase_regret"` // Burned by a bad purchase recently?
}

// DailyLife represents daily life context
type DailyLife struct {
	WakeTime             string   `json:"wake_time"`              // "6:30 AM"
	SleepTime            string   `json:"sleep_time"`             // "11:30 PM"
	FreeHoursPerDay      float64  `json:"free_hours_per_day"`     // Actual unstructured time
	TopStruggles         []string `json:"top_struggles"`          // Ranked top 3
	DailyRoutine         string   `json:"daily_routine"`          // Brief hourly breakdown
	MentalState          string   `json:"mental_state"`           // "overwhelmed", "coasting", "focused", "burned_out"
	DiscoveryContext     string   `json:"discovery_context"`      // "scrolling Twitter at 11pm", "colleague Slack mention"
	AttentionSpanMinutes int      `json:"attention_span_minutes"` // How long they'd read a landing page
	CurrentPriorities    []string `json:"current_priorities"`     // Top 3 life priorities right now
}

// Persona represents a user persona
type Persona struct {
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
}

// LandingPage represents a landing page configuration
type LandingPage struct {
	Headline    string   `json:"headline"`
	Subheadline string   `json:"subheadline"`
	Price       string   `json:"price"`
	TrialType   string   `json:"trial_type"`
	CTA         string   `json:"cta"`
	Features    []string `json:"features"`
}

// IdeaPricing represents pricing for an idea
type IdeaPricing struct {
	MonthlyPrice    int    `json:"monthly_price"`
	AnnualPrice     int    `json:"annual_price"` // Annual option (discounted)
	TrialDays       int    `json:"trial_days"`
	RequiresCC      bool   `json:"requires_cc"`      // Credit card for trial?
	BillingCycle    string `json:"billing_cycle"`    // "monthly", "annual", "one-time"
	Category        string `json:"category"`         // "dev_tool", "productivity", "health", etc.
	ValueProp       string `json:"value_prop"`       // One-line "why pay"
	CompetitorPrice int    `json:"competitor_price"` // What alternatives cost
}

// SimulationResult represents the result of a persona simulation
type SimulationResult struct {
	PersonaName      string   `json:"persona_name"`
	Persona          Persona  `json:"persona,omitempty"`
	Archetype        string   `json:"archetype"`
	ImpressionScore  int      `json:"impression_score"`
	RelevanceScore   int      `json:"relevance_score"`
	IntentStrength   int      `json:"intent_strength"`
	Converted        bool     `json:"converted"`
	Status           string   `json:"status"` // "converted", "rejected", "error"
	Skepticism       string   `json:"skepticism"`
	FrictionPoints   []string `json:"friction_points"`
	PricingReaction  string   `json:"pricing_reaction"`
	Reasoning        string   `json:"reasoning"`
	CPLEquivalent    float64  `json:"cpl_equivalent"`
	BudgetCheck      string   `json:"budget_check"`      // "affordable", "stretch", "cant_afford"
	PriorityRank     int      `json:"priority_rank"`     // Where does this fall in their priorities? 1-10
	TimeAvailable    bool     `json:"time_available"`    // Do they have time to use this?
	CompetingWith    string   `json:"competing_with"`    // What existing tool/habit this competes with
	DecisionTimeline string   `json:"decision_timeline"` // "impulse", "this_week", "next_month", "never"
	TestedPrice      int      `json:"tested_price,omitempty"`
	InitialHope      string   `json:"initial_hope"`       // What does the user hope this solves when they first see it?
	HopeMet          bool     `json:"hope_met"`           // Did the product deliver on that initial hope?
	HopeGapReason    string   `json:"hope_gap_reason"`    // If hope wasn't met, why? (empty if met)
}

// IdeaResult represents validation results for one idea
type IdeaResult struct {
	Description    string             `json:"description"`
	Coordinates    map[string]int     `json:"coordinates"`
	FitScore       float64            `json:"fit_score"`
	ConversionRate float64            `json:"conversion_rate"`
	Signups        int                `json:"signups"`
	TotalPersonas  int                `json:"total_personas"`
	Score          float64            `json:"score"`
	RuntimeSeconds float64            `json:"runtime_seconds"`
	Simulations    []SimulationResult `json:"simulations"`
	LandingPage    *LandingPage       `json:"landing_page"`
}

// ValidationResults holds all validation results
type ValidationResults struct {
	RuntimeSeconds float64                 `json:"runtime_seconds"`
	Ideas          []IdeaResult            `json:"ideas"`
	ProviderStats  map[string]ProviderStat `json:"provider_stats"`
	Timestamp      string                  `json:"timestamp"`
}

// ProviderStat tracks provider performance
type ProviderStat struct {
	Calls     int     `json:"calls"`
	Successes int     `json:"successes"`
	Failures  int     `json:"failures"`
	TotalTime float64 `json:"total_time"`
}

// Task represents a unit of work for the worker pool
type Task struct {
	Type     string // "persona_gen", "landing_gen", "simulation"
	Request  LLMRequest
	ResultCh chan<- TaskResult
}

// TaskResult contains the result of a task
type TaskResult struct {
	Content  string
	Provider string
	Latency  float64
	Error    error
}

// AdversarialArchetype represents a hardcoded skeptical persona
type AdversarialArchetype struct {
	Name         string
	PromptPrefix string // Injected into simulation prompt
}

// B2BCommitteeMember wraps a Persona with committee-specific context
type B2BCommitteeMember struct {
	Persona       Persona  `json:"persona"`
	CommitteeRole string   `json:"committee_role"` // "champion" | "budget_owner" | "technical_evaluator"
	PersonalGoals []string `json:"personal_goals"` // what THEY personally want (career, workload, recognition)
	BusinessGoals []string `json:"business_goals"` // what they want for the company
	ReportsTo     string   `json:"reports_to"`     // role title of their manager (or "" if top)
	VetoPower     bool     `json:"veto_power"`     // can they unilaterally kill the deal?
}

// B2BCommittee is the buying group for one simulation unit
type B2BCommittee struct {
	CompanyName string               `json:"company_name"`
	Members     []B2BCommitteeMember `json:"members"`
}

// B2BCommitteeResult is the output of one committee simulation
type B2BCommitteeResult struct {
	IndividualEvals []SimulationResult `json:"individual_evals"`
	FinalDecision   string             `json:"final_decision"` // "purchase" | "reject" | "pilot" | "defer"
	Converted       bool               `json:"converted"`
	DecisionDriver  string             `json:"decision_driver"` // name of person who drove outcome
	GroupReasoning  string             `json:"group_reasoning"` // what happened in the discussion
	VoteBreakdown   map[string]string  `json:"vote_breakdown"`  // name → "yes" | "no" | "maybe"
	KeyFriction     []string           `json:"key_friction"`
	NextStep        string             `json:"next_step"`
	TestedPrice     int                `json:"tested_price"`
}

// AdversarialArchetypes defines hardcoded skeptical personas (just 2 to avoid over-biasing toward "no")
var AdversarialArchetypes = []AdversarialArchetype{
	{
		Name:         "Cash-Strapped Freelancer",
		PromptPrefix: `You are a freelance designer/developer with irregular income ($2k-$6k/month). This month was $2.5k. You have $800 in subscriptions you keep meaning to cancel (Adobe CC, Figma, Notion, Slack, etc). Your savings are 1.5 months of expenses. You're anxious about money and only buy things that directly generate revenue.`,
	},
	{
		Name:         "Overworked Founder",
		PromptPrefix: `You are a bootstrapped startup founder with 18 months of runway left. Every $50/month matters because it's YOUR money. You work 70hr weeks and have no time to learn new tools. You've built scrappy internal solutions for most problems. You're skeptical of anything that isn't immediately solving your #1 problem (getting more customers). You see 20 product pitches a day.`,
	},
}
