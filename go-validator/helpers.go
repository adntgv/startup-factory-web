package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ccText returns text about credit card requirement
func ccText(requiresCC bool) string {
	if requiresCC {
		return "(credit card required)"
	}
	return "(no credit card needed)"
}

// extractJSON extracts JSON from markdown code blocks or raw content
func extractJSON(content string) string {
	// Strip <think>...</think> blocks (MaxClaw/MiniMax wraps responses in thinking tags)
	if idx := strings.Index(content, "</think>"); idx != -1 {
		content = strings.TrimSpace(content[idx+8:])
	} else if strings.HasPrefix(strings.TrimSpace(content), "<think>") {
		// Thinking block not closed = response truncated, try to find JSON anyway
		// Look for JSON after last complete sentence in thinking
	}

	if strings.Contains(content, "```json") {
		parts := strings.Split(content, "```json")
		if len(parts) > 1 {
			jsonPart := strings.Split(parts[1], "```")[0]
			return strings.TrimSpace(jsonPart)
		}
	} else if strings.Contains(content, "```") {
		parts := strings.Split(content, "```")
		if len(parts) >= 3 {
			return strings.TrimSpace(parts[1])
		}
	}

	// Find first valid JSON array/object and ignore trailing text.
	for i := 0; i < len(content); i++ {
		if content[i] != '{' && content[i] != '[' {
			continue
		}
		if extracted, ok := decodeJSONPrefix(content[i:]); ok {
			return extracted
		}
	}

	return strings.TrimSpace(content)
}

func decodeJSONPrefix(s string) (string, bool) {
	dec := json.NewDecoder(strings.NewReader(s))
	dec.UseNumber()

	var v interface{}
	if err := dec.Decode(&v); err != nil {
		return "", false
	}

	offset := dec.InputOffset()
	if offset <= 0 {
		return "", false
	}

	return strings.TrimSpace(s[:offset]), true
}

// fixJSON removes common JSON issues like trailing commas
func fixJSON(s string) string {
	s = strings.ReplaceAll(s, ",]", "]")
	s = strings.ReplaceAll(s, ",}", "}")
	return s
}

// parsePersonas parses LLM response into personas
func parsePersonas(content string, expectedCount int) ([]Persona, error) {
	jsonStr := extractJSON(content)

	var personas []Persona
	if err := json.Unmarshal([]byte(jsonStr), &personas); err == nil {
		if len(personas) == 0 {
			return nil, fmt.Errorf("no personas parsed")
		}
		if expectedCount > 0 && len(personas) < expectedCount {
			log.Printf("Warning: partial persona generation: got %d, expected %d", len(personas), expectedCount)
		}
		return personas, nil
	}

	clean := fixJSON(jsonStr)
	var raw []map[string]interface{}
	if err := json.Unmarshal([]byte(clean), &raw); err != nil {
		// Accept wrapped object forms, e.g. {"personas": [...]}.
		var wrapped map[string]interface{}
		if err2 := json.Unmarshal([]byte(clean), &wrapped); err2 == nil {
			candidates := []string{"personas", "persona", "data", "results", "items"}
			for _, key := range candidates {
				if arr, ok := wrapped[key].([]interface{}); ok {
					for _, item := range arr {
						if m, ok := item.(map[string]interface{}); ok {
							personas = append(personas, mapToPersona(m))
						}
					}
					if len(personas) > 0 {
						if expectedCount > 0 && len(personas) < expectedCount {
							log.Printf("Warning: partial persona generation: got %d, expected %d", len(personas), expectedCount)
						}
						return personas, nil
					}
				}
				// Handle single-object value, e.g. {"persona": {...}}
				if obj, ok := wrapped[key].(map[string]interface{}); ok && looksLikePersonaObject(obj) {
					personas = append(personas, mapToPersona(obj))
					return personas, nil
				}
			}

			// Single persona object fallback.
			if looksLikePersonaObject(wrapped) {
				personas = append(personas, mapToPersona(wrapped))
				if expectedCount > 0 && len(personas) < expectedCount {
					log.Printf("Warning: partial persona generation: got %d, expected %d", len(personas), expectedCount)
				}
				return personas, nil
			}
		}
		return nil, fmt.Errorf("failed to parse personas: %w", err)
	}
	for _, r := range raw {
		p := mapToPersona(r)
		personas = append(personas, p)
	}
	if len(personas) == 0 {
		return nil, fmt.Errorf("no personas parsed")
	}
	if expectedCount > 0 && len(personas) < expectedCount {
		log.Printf("Warning: partial persona generation: got %d, expected %d", len(personas), expectedCount)
	}
	return personas, nil
}

func looksLikePersonaObject(m map[string]interface{}) bool {
	_, hasName := m["name"]
	_, hasRole := m["role"]
	_, hasAge := m["age"]
	return hasName && hasRole && hasAge
}

func isLikelyTruncatedThink(content string) bool {
	trimmed := strings.TrimSpace(content)
	return strings.HasPrefix(trimmed, "<think>") && !strings.Contains(trimmed, "</think>")
}

func writePersonaDebugDump(outputDir, reason, prompt, raw string) (string, error) {
	if outputDir == "" {
		outputDir = "results"
	}
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return "", fmt.Errorf("create debug dir: %w", err)
	}

	path := filepath.Join(outputDir, fmt.Sprintf("debug_persona_%d.txt", time.Now().UnixNano()))
	var b bytes.Buffer
	b.WriteString("reason: ")
	b.WriteString(reason)
	b.WriteString("\n")
	b.WriteString("timestamp: ")
	b.WriteString(time.Now().Format(time.RFC3339))
	b.WriteString("\n\n=== PROMPT ===\n")
	b.WriteString(prompt)
	b.WriteString("\n\n=== RAW RESPONSE ===\n")
	b.WriteString(raw)
	b.WriteString("\n")

	if err := os.WriteFile(path, b.Bytes(), 0644); err != nil {
		return "", fmt.Errorf("write debug dump: %w", err)
	}
	return path, nil
}

// parseLandingPage parses LLM response into landing page
func parseLandingPage(content string) (*LandingPage, error) {
	jsonStr := extractJSON(content)

	var landing LandingPage
	if err := json.Unmarshal([]byte(jsonStr), &landing); err == nil {
		return &landing, nil
	}

	var landings []LandingPage
	if err := json.Unmarshal([]byte(jsonStr), &landings); err == nil && len(landings) > 0 {
		return &landings[0], nil
	}

	var raw interface{}
	if err := json.Unmarshal([]byte(jsonStr), &raw); err == nil {
		if arr, ok := raw.([]interface{}); ok && len(arr) > 0 {
			if obj, ok := arr[0].(map[string]interface{}); ok {
				b, _ := json.Marshal(obj)
				var l LandingPage
				if err := json.Unmarshal(b, &l); err == nil {
					return &l, nil
				}
			}
		}
	}

	return &LandingPage{
		Headline:    "Transform Your Workflow",
		Subheadline: "AI-powered solution for your needs",
		Price:       "29",
		TrialType:   "14-day free trial",
		CTA:         "Start Free Trial",
		Features:    []string{"AI-powered automation", "Easy setup", "24/7 support"},
	}, nil
}

// parseSimulation parses LLM response into simulation result
func parseSimulation(content string, personaName string) SimulationResult {
	jsonStr := extractJSON(content)

	var simData struct {
		Converted        bool     `json:"converted"`
		ImpressionScore  int      `json:"impression_score"`
		RelevanceScore   int      `json:"relevance_score"`
		IntentStrength   int      `json:"intent_strength"`
		FrictionPoints   []string `json:"friction_points"`
		PricingReaction  string   `json:"pricing_reaction"`
		CPLEquivalent    float64  `json:"cpl_equivalent"`
		Reasoning        string   `json:"reasoning"`
		BudgetCheck      string   `json:"budget_check"`
		PriorityRank     int      `json:"priority_rank"`
		TimeAvailable    bool     `json:"time_available"`
		CompetingWith    string   `json:"competing_with"`
		DecisionTimeline string   `json:"decision_timeline"`
	}

	if err := json.Unmarshal([]byte(jsonStr), &simData); err != nil {
		// Accept array/object variants and minor JSON issues.
		recovered, recErr := recoverSimulationFromLooseJSON(jsonStr)
		if recErr == nil {
			simData = recovered
		} else {
			// Last fallback: conservative text extraction. Never infer conversions from malformed output.
			if heuristic, ok := recoverSimulationHeuristic(content); ok {
				heuristic.PersonaName = personaName
				return heuristic
			}

			// NEVER count parse errors as conversions - this inflates metrics
			return SimulationResult{
				PersonaName:    personaName,
				Converted:      false,
				Status:         "error",
				RelevanceScore: 0,
				Reasoning:      fmt.Sprintf("Parse error: %v", err),
			}
		}
	}

	// Default reasonable values if not provided
	if simData.RelevanceScore == 0 {
		simData.RelevanceScore = 5
	}
	if simData.ImpressionScore == 0 {
		simData.ImpressionScore = 5
	}

	// Determine status based on conversion and reasoning quality
	status := "rejected"
	if simData.Converted {
		status = "converted"
	}

	return SimulationResult{
		PersonaName:      personaName,
		Converted:        simData.Converted,
		Status:           status,
		ImpressionScore:  simData.ImpressionScore,
		RelevanceScore:   simData.RelevanceScore,
		IntentStrength:   simData.IntentStrength,
		FrictionPoints:   simData.FrictionPoints,
		PricingReaction:  simData.PricingReaction,
		CPLEquivalent:    simData.CPLEquivalent,
		Reasoning:        simData.Reasoning,
		BudgetCheck:      simData.BudgetCheck,
		PriorityRank:     simData.PriorityRank,
		TimeAvailable:    simData.TimeAvailable,
		CompetingWith:    simData.CompetingWith,
		DecisionTimeline: simData.DecisionTimeline,
	}
}

func recoverSimulationFromLooseJSON(jsonStr string) (struct {
	Converted        bool     `json:"converted"`
	ImpressionScore  int      `json:"impression_score"`
	RelevanceScore   int      `json:"relevance_score"`
	IntentStrength   int      `json:"intent_strength"`
	FrictionPoints   []string `json:"friction_points"`
	PricingReaction  string   `json:"pricing_reaction"`
	CPLEquivalent    float64  `json:"cpl_equivalent"`
	Reasoning        string   `json:"reasoning"`
	BudgetCheck      string   `json:"budget_check"`
	PriorityRank     int      `json:"priority_rank"`
	TimeAvailable    bool     `json:"time_available"`
	CompetingWith    string   `json:"competing_with"`
	DecisionTimeline string   `json:"decision_timeline"`
}, error) {
	var out struct {
		Converted        bool     `json:"converted"`
		ImpressionScore  int      `json:"impression_score"`
		RelevanceScore   int      `json:"relevance_score"`
		IntentStrength   int      `json:"intent_strength"`
		FrictionPoints   []string `json:"friction_points"`
		PricingReaction  string   `json:"pricing_reaction"`
		CPLEquivalent    float64  `json:"cpl_equivalent"`
		Reasoning        string   `json:"reasoning"`
		BudgetCheck      string   `json:"budget_check"`
		PriorityRank     int      `json:"priority_rank"`
		TimeAvailable    bool     `json:"time_available"`
		CompetingWith    string   `json:"competing_with"`
		DecisionTimeline string   `json:"decision_timeline"`
	}

	// Try object path first with cleaned JSON.
	clean := fixJSON(jsonStr)
	if err := json.Unmarshal([]byte(clean), &out); err == nil {
		return out, nil
	}

	// Array path: take first object.
	var arr []map[string]interface{}
	if err := json.Unmarshal([]byte(clean), &arr); err == nil && len(arr) > 0 {
		m := arr[0]
		out.Converted = toBool(m["converted"])
		out.ImpressionScore = toInt(m["impression_score"])
		out.RelevanceScore = toInt(m["relevance_score"])
		out.IntentStrength = toInt(m["intent_strength"])
		out.FrictionPoints = toStringSlice(m["friction_points"])
		out.PricingReaction = toString(m["pricing_reaction"])
		out.CPLEquivalent = toFloat(m["cpl_equivalent"])
		out.Reasoning = toString(m["reasoning"])
		out.BudgetCheck = toString(m["budget_check"])
		out.PriorityRank = toInt(m["priority_rank"])
		out.TimeAvailable = toBool(m["time_available"])
		out.CompetingWith = toString(m["competing_with"])
		out.DecisionTimeline = toString(m["decision_timeline"])
		return out, nil
	}

	return out, fmt.Errorf("unable to recover simulation json")
}

func recoverSimulationHeuristic(content string) (SimulationResult, bool) {
	l := strings.ToLower(content)
	if !strings.Contains(l, "converted") && !strings.Contains(l, "reason") {
		return SimulationResult{}, false
	}

	converted := strings.Contains(l, "\"converted\": true") || strings.Contains(l, "converted: true")
	reason := "Recovered from malformed JSON"
	if converted {
		reason = "Recovered from malformed JSON; conversion indicated"
	}

	status := "rejected"
	if converted {
		status = "converted"
	}

	return SimulationResult{
		Converted:        converted,
		Status:           status,
		ImpressionScore:  5,
		RelevanceScore:   5,
		IntentStrength:   5,
		FrictionPoints:   []string{"malformed_response"},
		PricingReaction:  "stretch",
		CPLEquivalent:    25,
		Reasoning:        reason,
		BudgetCheck:      "stretch",
		PriorityRank:     5,
		TimeAvailable:    false,
		CompetingWith:    "unknown",
		DecisionTimeline: "next_month",
	}, true
}

func toString(v interface{}) string {
	switch t := v.(type) {
	case string:
		return t
	case json.Number:
		return t.String()
	case float64:
		return fmt.Sprintf("%.0f", t)
	default:
		return ""
	}
}

func toInt(v interface{}) int {
	switch t := v.(type) {
	case float64:
		return int(t)
	case int:
		return t
	case json.Number:
		i, _ := t.Int64()
		return int(i)
	case string:
		var i int
		fmt.Sscanf(t, "%d", &i)
		return i
	default:
		return 0
	}
}

func toFloat(v interface{}) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case json.Number:
		f, _ := t.Float64()
		return f
	case string:
		var f float64
		fmt.Sscanf(t, "%f", &f)
		return f
	default:
		return 0
	}
}

func toBool(v interface{}) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		t = strings.ToLower(strings.TrimSpace(t))
		return t == "true" || t == "yes"
	default:
		return false
	}
}

func toStringSlice(v interface{}) []string {
	var out []string
	switch t := v.(type) {
	case []interface{}:
		for _, item := range t {
			out = append(out, toString(item))
		}
	case []string:
		return t
	}
	return out
}

// mapToPersona coerces loose JSON into Persona struct
func mapToPersona(m map[string]interface{}) Persona {
	p := Persona{}

	p.Name = toString(m["name"])
	p.Age = toInt(m["age"])
	p.Role = toString(m["role"])
	p.CompanySize = toString(m["company_size"])
	p.ExperienceYears = toInt(m["experience_years"])
	p.PainLevel = toInt(m["pain_level"])
	p.Skepticism = toInt(m["skepticism"])
	p.CurrentWorkflow = toString(m["current_workflow"])
	p.CurrentTools = toString(m["current_tools"])
	p.Personality = toString(m["personality"])
	p.Archetype = toString(m["archetype"])
	p.Budget = toString(m["budget"])
	p.DecisionAuthority = toString(m["decision_authority"])

	if fin, ok := m["financial"].(map[string]interface{}); ok {
		p.Financial.MonthlyIncome = toInt(fin["monthly_income"])
		p.Financial.MonthlyExpenses = toInt(fin["monthly_expenses"])
		p.Financial.DiscretionaryBudget = toInt(fin["discretionary_budget"])
		p.Financial.TotalSubSpend = toInt(fin["total_sub_spend"])
		p.Financial.SavingsMonths = toFloat(fin["savings_months"])
		p.Financial.RiskTolerance = toString(fin["risk_tolerance"])
		p.Financial.RecentPurchaseRegret = toBool(fin["recent_purchase_regret"])
		if subs, ok := fin["current_subscriptions"].([]interface{}); ok {
			for _, s := range subs {
				if sm, ok := s.(map[string]interface{}); ok {
					p.Financial.CurrentSubscriptions = append(p.Financial.CurrentSubscriptions, Subscription{
						Name:        toString(sm["name"]),
						MonthlyCost: toInt(sm["monthly_cost"]),
						Usage:       toString(sm["usage"]),
					})
				}
			}
		}
	}

	if dl, ok := m["daily_life"].(map[string]interface{}); ok {
		p.DailyLife.WakeTime = toString(dl["wake_time"])
		p.DailyLife.SleepTime = toString(dl["sleep_time"])
		p.DailyLife.FreeHoursPerDay = toFloat(dl["free_hours_per_day"])
		p.DailyLife.DailyRoutine = toString(dl["daily_routine"])
		p.DailyLife.MentalState = toString(dl["mental_state"])
		p.DailyLife.DiscoveryContext = toString(dl["discovery_context"])
		p.DailyLife.AttentionSpanMinutes = toInt(dl["attention_span_minutes"])
		p.DailyLife.TopStruggles = toStringSlice(dl["top_struggles"])
		p.DailyLife.CurrentPriorities = toStringSlice(dl["current_priorities"])
	}

	return p
}

// Checkpoint stores progress for resume capability
type Checkpoint struct {
	CompletedIdeas []IdeaResult `json:"completed_ideas"`
	Timestamp      string       `json:"timestamp"`
}

func saveCheckpoint(ideas []IdeaResult, path string) {
	checkpoint := Checkpoint{
		CompletedIdeas: ideas,
		Timestamp:      time.Now().Format(time.RFC3339),
	}
	data, err := json.Marshal(checkpoint)
	if err != nil {
		log.Printf("Warning: failed to marshal checkpoint: %v", err)
		return
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		log.Printf("Warning: failed to save checkpoint: %v", err)
	}
}

func loadCheckpoint(path string) Checkpoint {
	data, err := os.ReadFile(path)
	if err != nil {
		return Checkpoint{}
	}
	var checkpoint Checkpoint
	if err := json.Unmarshal(data, &checkpoint); err != nil {
		log.Printf("Warning: failed to parse checkpoint: %v", err)
		return Checkpoint{}
	}
	return checkpoint
}
