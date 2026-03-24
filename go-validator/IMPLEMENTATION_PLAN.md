# Realistic Persona Constraints - Implementation Plan

## Problem
Current conversion: ~75% (LLM personas are too agreeable). Target: 20-30%.

Root cause: `simulatePersona()` prompt is too simple — just name/age/role seeing a landing page. No financial reality, no life context, no competing priorities.

---

## Phase 1: New Data Structures (`types.go`)

### 1.1 Add `FinancialProfile` struct

```go
type FinancialProfile struct {
    MonthlyIncome       int      `json:"monthly_income"`        // After tax, USD
    MonthlyExpenses     int      `json:"monthly_expenses"`      // Rent, food, bills
    DiscretionaryBudget int      `json:"discretionary_budget"`  // What's left for wants
    CurrentSubscriptions []Subscription `json:"current_subscriptions"`
    TotalSubSpend       int      `json:"total_sub_spend"`       // Sum of all subs
    SavingsMonths       float64  `json:"savings_months"`        // Emergency runway
    RiskTolerance       string   `json:"risk_tolerance"`        // "minimal", "cautious", "moderate", "aggressive"
    RecentPurchaseRegret bool    `json:"recent_purchase_regret"` // Burned by a bad purchase recently?
}

type Subscription struct {
    Name      string `json:"name"`
    MonthlyCost int  `json:"monthly_cost"`
    Usage     string `json:"usage"` // "daily", "weekly", "rarely", "forgot_about_it"
}
```

### 1.2 Add `DailyLife` struct

```go
type DailyLife struct {
    WakeTime        string   `json:"wake_time"`          // "6:30 AM"
    SleepTime       string   `json:"sleep_time"`         // "11:30 PM"
    FreeHoursPerDay float64  `json:"free_hours_per_day"` // Actual unstructured time
    TopStruggles    []string `json:"top_struggles"`      // Ranked top 3
    DailyRoutine    string   `json:"daily_routine"`      // Brief hourly breakdown
    MentalState     string   `json:"mental_state"`       // "overwhelmed", "coasting", "focused", "burned_out"
    DiscoveryContext string  `json:"discovery_context"`  // "scrolling Twitter at 11pm", "colleague Slack mention"
    AttentionSpanMinutes int `json:"attention_span_minutes"` // How long they'd read a landing page
    CurrentPriorities []string `json:"current_priorities"` // Top 3 life priorities right now
}
```

### 1.3 Update `Persona` struct — add embedded fields

```go
type Persona struct {
    // ... existing fields stay ...
    Financial    FinancialProfile `json:"financial"`
    DailyLife    DailyLife        `json:"daily_life"`
}
```

### 1.4 Add `IdeaPricing` struct

```go
type IdeaPricing struct {
    MonthlyPrice    int    `json:"monthly_price"`
    AnnualPrice     int    `json:"annual_price"`       // Annual option (discounted)
    TrialDays       int    `json:"trial_days"`
    RequiresCC      bool   `json:"requires_cc"`        // Credit card for trial?
    BillingCycle    string `json:"billing_cycle"`       // "monthly", "annual", "one-time"
    Category        string `json:"category"`            // "dev_tool", "productivity", "health", etc.
    ValueProp       string `json:"value_prop"`          // One-line "why pay"
    CompetitorPrice int    `json:"competitor_price"`    // What alternatives cost
}
```

### 1.5 Add `DecisionFactors` to `SimulationResult`

```go
type SimulationResult struct {
    // ... existing fields ...
    BudgetCheck      string `json:"budget_check"`       // "affordable", "stretch", "cant_afford"
    PriorityRank     int    `json:"priority_rank"`       // Where does this fall in their priorities? 1-10
    TimeAvailable    bool   `json:"time_available"`      // Do they have time to use this?
    CompetingWith    string `json:"competing_with"`      // What existing tool/habit this competes with
    DecisionTimeline string `json:"decision_timeline"`   // "impulse", "this_week", "next_month", "never"
}
```

### 1.6 Add `AdversarialPersona` — hardcoded skeptics

```go
type AdversarialArchetype struct {
    Name           string
    PromptPrefix   string  // Injected into simulation prompt
}

var AdversarialArchetypes = []AdversarialArchetype{
    {
        Name: "Broke College Student",
        PromptPrefix: `You are a college student with $200/month discretionary budget after tuition/rent/food. You already subscribe to Spotify ($5.99 student), ChatGPT Plus ($20), and your phone plan. Every dollar matters. You've been burned by "free trials" that auto-charged. You discover products while procrastinating at 1am.`,
    },
    {
        Name: "Burned-Out Developer",
        PromptPrefix: `You are a senior developer making $150k but working 55hr weeks. You're exhausted and cynical about new tools — you've tried dozens that promised to "10x your workflow" and abandoned them all within 2 weeks. You have 0 free hours and your partner is upset about screen time. You already pay for GitHub Copilot, JetBrains, multiple cloud services.`,
    },
    {
        Name: "Cash-Strapped Freelancer",
        PromptPrefix: `You are a freelance designer/developer with irregular income ($2k-$6k/month). This month was $2.5k. You have $800 in subscriptions you keep meaning to cancel (Adobe CC, Figma, Notion, Slack, etc). Your savings are 1.5 months of expenses. You're anxious about money and only buy things that directly generate revenue.`,
    },
    {
        Name: "Cautious Mid-Manager",
        PromptPrefix: `You are a mid-level manager at a 500-person company. You can't expense tools without VP approval (takes 3 weeks). You've been burned proposing tools that the team didn't adopt, hurting your credibility. You need ROI proof, case studies from similar companies, and SOC2 compliance before even considering something. Your budget for the quarter is already allocated.`,
    },
    {
        Name: "Overworked Founder",
        PromptPrefix: `You are a bootstrapped startup founder with 18 months of runway left. Every $50/month matters because it's YOUR money. You work 70hr weeks and have no time to learn new tools. You've built scrappy internal solutions for most problems. You're skeptical of anything that isn't immediately solving your #1 problem (getting more customers). You see 20 product pitches a day.`,
    },
}
```

---

## Phase 2: Pricing Generation (`validator.go`)

### 2.1 New function: `determinePricing()`

Add after `generateLandingPage()`. Instead of LLM-generating price (which is random), use category-based rules:

```go
func determinePricing(description string, landing *LandingPage) IdeaPricing {
    // Parse category from description keywords
    category := categorizeIdea(description)
    
    pricing := categoryPricingDefaults[category] // map lookup
    
    // Override with landing page price if parseable
    if parsed, err := strconv.Atoi(landing.Price); err == nil {
        pricing.MonthlyPrice = parsed
    }
    
    return pricing
}

var categoryPricingDefaults = map[string]IdeaPricing{
    "dev_tool":      {MonthlyPrice: 29, AnnualPrice: 290, TrialDays: 14, RequiresCC: false},
    "productivity":  {MonthlyPrice: 12, AnnualPrice: 120, TrialDays: 7, RequiresCC: false},
    "health":        {MonthlyPrice: 15, AnnualPrice: 150, TrialDays: 7, RequiresCC: false},
    "education":     {MonthlyPrice: 25, AnnualPrice: 250, TrialDays: 14, RequiresCC: false},
    "enterprise":    {MonthlyPrice: 99, AnnualPrice: 990, TrialDays: 14, RequiresCC: true},
    "consumer":      {MonthlyPrice: 9,  AnnualPrice: 90,  TrialDays: 7, RequiresCC: false},
    "default":       {MonthlyPrice: 19, AnnualPrice: 190, TrialDays: 7, RequiresCC: false},
}

func categorizeIdea(description string) string {
    desc := strings.ToLower(description)
    switch {
    case strings.Contains(desc, "developer") || strings.Contains(desc, "code") || strings.Contains(desc, "api"):
        return "dev_tool"
    case strings.Contains(desc, "health") || strings.Contains(desc, "fitness"):
        return "health"
    case strings.Contains(desc, "education") || strings.Contains(desc, "learn"):
        return "education"
    case strings.Contains(desc, "enterprise") || strings.Contains(desc, "team"):
        return "enterprise"
    case strings.Contains(desc, "productiv"):
        return "productivity"
    default:
        return "default"
    }
}
```

---

## Phase 3: Enhanced Persona Generation Prompt (`validator.go`)

### 3.1 Replace `generatePersonas()` prompt

```go
prompt := fmt.Sprintf(`Generate %d realistic user personas for this product idea:
"%s"

Price: $%d/month

Each persona MUST include realistic financial and daily life constraints.

Return a JSON array where each persona has:
- name, age, role, company_size, experience_years
- pain_level (1-10), skepticism (1-10)
- current_workflow, current_tools, personality, archetype
- budget: "tight" | "moderate" | "comfortable"
- decision_authority: "self" | "needs_approval" | "committee"
- financial: {
    monthly_income (int, after-tax USD),
    monthly_expenses (int),
    discretionary_budget (int, income - expenses),
    current_subscriptions: [{name, monthly_cost, usage}] (3-8 real subscriptions),
    total_sub_spend (int),
    savings_months (float, emergency runway),
    risk_tolerance: "minimal" | "cautious" | "moderate" | "aggressive",
    recent_purchase_regret (bool)
  }
- daily_life: {
    wake_time, sleep_time,
    free_hours_per_day (float, realistic: 0.5-3),
    top_struggles: [3 strings, ranked],
    daily_routine (brief hourly breakdown),
    mental_state: "overwhelmed" | "burned_out" | "coasting" | "focused",
    discovery_context (how/when they find this product),
    attention_span_minutes (int, 1-5 for landing pages),
    current_priorities: [3 strings, life priorities]
  }

IMPORTANT RULES:
- At least 40%% of personas should have discretionary_budget < $%d (can't afford the product easily)
- At least 30%% should have mental_state "overwhelmed" or "burned_out"
- At least 30%% should already have 5+ subscriptions they're not fully using
- Mix of decision_authority types
- free_hours_per_day should average 1.5 (most people are busy)
- Subscriptions should be REAL products (Netflix, Spotify, Slack, Notion, GitHub Copilot, etc.)

Output JSON array directly, no markdown.`, count, description, pricing.MonthlyPrice, pricing.MonthlyPrice*2)
```

---

## Phase 4: Multi-Factor Simulation Prompt (`validator.go`)

### 4.1 Replace `simulatePersona()` with realistic decision model

```go
func (v *Validator) simulatePersona(persona Persona, landing *LandingPage, description string, pricing IdeaPricing) SimulationResult {
    
    subsJSON, _ := json.Marshal(persona.Financial.CurrentSubscriptions)
    
    prompt := fmt.Sprintf(`You are %s, a %d-year-old %s. 

YOUR FINANCIAL REALITY:
- Monthly income: $%d after tax
- Monthly expenses: $%d  
- Discretionary budget: $%d/month
- Current subscriptions ($%d/month total): %s
- Savings runway: %.1f months
- Risk tolerance: %s
- Recently regretted a purchase: %v

YOUR DAILY LIFE:
- Schedule: %s to %s (%s)
- Free time: %.1f hours/day
- Mental state: %s
- Top 3 struggles: %s
- Current priorities: %s
- How you found this: %s

YOU SEE THIS LANDING PAGE (spending %d minutes max reading it):
Headline: %s
Subheadline: %s  
Price: $%d/month ($%d/year)
Trial: %d days %s
Features: %s

DECISION PROCESS — Think through each factor honestly:
1. BUDGET CHECK: Can I actually afford $%d/month? What would I cut?
2. PRIORITY CHECK: Does this solve my #1 struggle, or a minor annoyance?
3. TIME CHECK: Do I have time to set this up and actually use it?
4. TRUST CHECK: Do I trust this enough to give my card/email? What's the catch?
5. COMPETING SOLUTIONS: Am I already solving this with something else (even imperfectly)?
6. MENTAL STATE: Am I in the right headspace to evaluate and onboard a new tool right now?

Reply with JSON:
{
  "converted": true/false,
  "budget_check": "affordable" | "stretch" | "cant_afford",
  "priority_rank": 1-10 (1 = this is my #1 problem, 10 = irrelevant),
  "time_available": true/false,
  "competing_with": "what existing solution they'd stick with",
  "decision_timeline": "impulse" | "this_week" | "next_month" | "never",
  "relevance_score": 1-10,
  "reasoning": "2-3 sentence honest internal monologue about why they would/wouldn't sign up"
}

IMPORTANT: Most real people do NOT sign up for things they see. Default to skepticism. Only convert if ALL of these are true:
- You can genuinely afford it without stress
- It solves a top-2 struggle
- You have time to use it
- Nothing else already solves this
- You're not too overwhelmed to deal with it right now`,
        persona.Name, persona.Age, persona.Role,
        persona.Financial.MonthlyIncome, persona.Financial.MonthlyExpenses,
        persona.Financial.DiscretionaryBudget, persona.Financial.TotalSubSpend, string(subsJSON),
        persona.Financial.SavingsMonths, persona.Financial.RiskTolerance,
        persona.Financial.RecentPurchaseRegret,
        persona.DailyLife.WakeTime, persona.DailyLife.SleepTime, persona.DailyLife.DailyRoutine,
        persona.DailyLife.FreeHoursPerDay, persona.DailyLife.MentalState,
        strings.Join(persona.DailyLife.TopStruggles, ", "),
        strings.Join(persona.DailyLife.CurrentPriorities, ", "),
        persona.DailyLife.DiscoveryContext,
        persona.DailyLife.AttentionSpanMinutes,
        landing.Headline, landing.Subheadline,
        pricing.MonthlyPrice, pricing.AnnualPrice,
        pricing.TrialDays, ccText(pricing.RequiresCC),
        strings.Join(landing.Features, ", "),
        pricing.MonthlyPrice,
    )
    // ... rest of submit + parse logic
}

func ccText(requiresCC bool) string {
    if requiresCC {
        return "(credit card required)"
    }
    return "(no credit card needed)"
}
```

### 4.2 Add adversarial simulation round

In `testIdea()`, after normal simulations, add:

```go
// Run adversarial personas (5 hardcoded skeptics)
adversarialResults := v.runAdversarialSimulations(landing, description, pricing)
simResults = append(simResults, adversarialResults...)
```

```go
func (v *Validator) runAdversarialSimulations(landing *LandingPage, description string, pricing IdeaPricing) []SimulationResult {
    var wg sync.WaitGroup
    resultsCh := make(chan SimulationResult, len(AdversarialArchetypes))
    
    for _, archetype := range AdversarialArchetypes {
        wg.Add(1)
        go func(a AdversarialArchetype) {
            defer wg.Done()
            
            prompt := fmt.Sprintf(`%s

You see this landing page:
Headline: %s
Subheadline: %s
Price: $%d/month ($%d/year) 
Trial: %d days %s
Features: %s

Would you sign up? Consider your financial situation, time, current tools, and mental state.

Reply with JSON:
{
  "converted": true/false,
  "budget_check": "affordable" | "stretch" | "cant_afford",
  "priority_rank": 1-10,
  "time_available": true/false,
  "competing_with": "...",
  "decision_timeline": "impulse" | "this_week" | "next_month" | "never",
  "relevance_score": 1-10,
  "reasoning": "honest 2-3 sentence reaction"
}

Default to NOT signing up. Only sign up if this genuinely solves a critical problem AND you can afford it.`,
                a.PromptPrefix,
                landing.Headline, landing.Subheadline,
                pricing.MonthlyPrice, pricing.AnnualPrice,
                pricing.TrialDays, ccText(pricing.RequiresCC),
                strings.Join(landing.Features, ", "))
            
            result := v.pool.Submit("simulation", LLMRequest{
                Prompt: prompt, MaxTokens: 500, Temperature: 0.7, TaskType: "simulation",
            })
            
            sim := parseSimulation(result.Content, a.Name)
            sim.Archetype = a.Name
            resultsCh <- sim
        }(archetype)
    }
    
    go func() { wg.Wait(); close(resultsCh) }()
    
    var results []SimulationResult
    for sim := range resultsCh {
        results = append(results, sim)
    }
    return results
}
```

---

## Phase 5: Updated `parseSimulation()` 

Extend to capture new fields:

```go
var simData struct {
    Converted        bool   `json:"converted"`
    RelevanceScore   int    `json:"relevance_score"`
    Reasoning        string `json:"reasoning"`
    BudgetCheck      string `json:"budget_check"`
    PriorityRank     int    `json:"priority_rank"`
    TimeAvailable    bool   `json:"time_available"`
    CompetingWith    string `json:"competing_with"`
    DecisionTimeline string `json:"decision_timeline"`
}
```

---

## Implementation Sequence

| Step | File | What | Effort |
|------|------|------|--------|
| 1 | `types.go` | Add FinancialProfile, DailyLife, IdeaPricing, AdversarialArchetype structs + defaults | 30 min |
| 2 | `validator.go` | Add `determinePricing()` + `categorizeIdea()` | 15 min |
| 3 | `validator.go` | Update `generatePersonas()` prompt | 20 min |
| 4 | `validator.go` | Rewrite `simulatePersona()` with multi-factor prompt | 30 min |
| 5 | `validator.go` | Add `runAdversarialSimulations()` | 20 min |
| 6 | `validator.go` | Update `testIdea()` to wire pricing + adversarial round | 15 min |
| 7 | `validator.go` | Update `parseSimulation()` for new fields | 10 min |
| 8 | `types.go` | Add new fields to SimulationResult | 5 min |
| **Total** | | | **~2.5 hours** |

**Order matters:** Do 1 → 8 sequentially. Each step builds on the previous.

---

## Testing Strategy

### Test 1: Struct compilation
```bash
go build ./...  # Must compile with no errors
```

### Test 2: Pricing categorization
Run `categorizeIdea()` on 10 sample descriptions, verify correct categories.

### Test 3: Persona generation quality
Generate 5 personas for a $29/month dev tool. Verify:
- At least 2 have `discretionary_budget < $58`
- At least 1 has `mental_state: "burned_out"` or `"overwhelmed"`
- All have realistic subscriptions (real product names)
- `free_hours_per_day` averages < 2.0

### Test 4: Conversion rate validation
Run full pipeline on 3 dummy ideas. Check:
- Overall conversion: 20-35%
- Adversarial personas: <10% conversion
- Budget-constrained personas: <15% conversion
- No persona converts with `budget_check: "cant_afford"`

### Test 5: Regression — compare before/after
Run same 5 ideas with old prompts vs new prompts. Document conversion drop.

---

## Expected Conversion Impact

| Metric | Before | After | Why |
|--------|--------|-------|-----|
| Overall conversion | ~75% | 20-30% | Financial + life constraints filter out agreeable responses |
| Budget-constrained personas | N/A | 5-15% | Can't afford = won't buy |
| Overwhelmed/burned-out | N/A | 10-20% | No bandwidth for new tools |
| Adversarial personas | N/A | 0-10% | Designed to be hard to convert |
| High-fit personas | ~90% | 40-60% | Even good fits have competing priorities |

**Key insight:** The 75%→25% drop comes from 3 compounding filters:
1. **Budget filter** (~40% can't easily afford → cuts 30% of conversions)
2. **Priority filter** (~60% have bigger problems → cuts 40% of remaining)  
3. **Time/energy filter** (~50% too busy/burned out → cuts 30% of remaining)

0.70 × 0.60 × 0.70 ≈ 0.29 → ~29% of otherwise-willing personas convert

---

## Files Changed Summary

| File | Changes |
|------|---------|
| `types.go` | +FinancialProfile, +Subscription, +DailyLife, +IdeaPricing, +AdversarialArchetype, +fields on SimulationResult, +fields on Persona |
| `validator.go` | +determinePricing(), +categorizeIdea(), +ccText(), +runAdversarialSimulations(), rewrite generatePersonas() prompt, rewrite simulatePersona() prompt, update testIdea() flow, update parseSimulation() |
| `workers.go` | No changes |
| `workers_v2.go` | No changes |
| `main.go` | No changes |
| `providers.go` | No changes |

---

## 2026-03-11 Persona Parsing + Prompt Robustness Notes

- Added `--debug` CLI flag and `PipelineConfig.Debug` wiring to gate persona parse debug dumps.
- Persona generation is now two-phase in `pipeline.go`:
  1. Generate core persona schema only.
  2. Enrich each persona in parallel with `financial` + `daily_life` JSON.
- Added truncation detection for MaxClaw think blocks (`<think>` without `</think>`) and one retry with higher `max_tokens` for core persona generation.
- Updated MaxClaw token handling to allow larger requests (capped at 16000) while honoring caller-provided token budgets.
- `extractJSON` now decodes the first valid JSON prefix so JSON followed by extra text still parses.
- `parsePersonas` now accepts partial persona batches as long as at least one persona is parsed; warnings are logged for partial results.
- When persona parse/enrichment parse fails and `--debug` is enabled, raw prompts/responses are written to `results/debug_persona_<ts>.txt`.
