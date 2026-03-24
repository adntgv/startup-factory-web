package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

const (
	weightBuildability    = 0.15 // was 0.30 — coding is easy now
	weightMarketSize      = 0.15 // was 0.25 — raw TAM is a vanity metric
	weightMonetization    = 0.20 // was 0.25 — stays high
	weightCompetition     = 0.20 // was 0.10 — bloodbath risk is first-order
	weightUniqueAdvantage = 0.10 // unchanged
	weightDistribution    = 0.15 // NEW — "where do first 50 users come from?"
	weightRetention       = 0.05 // NEW — moat lives in habits not features
)

var bloodbathCategories = map[string]struct{}{
	"ai_productivity": {}, "lifestyle": {}, "health_fitness": {}, "social": {},
}
var bloodbathEscapeAudiences = map[string]struct{}{
	"developers": {}, "businesses": {}, "athletes": {}, "entrepreneurs": {},
}
var bloodbathEscapeInteractions = map[string]struct{}{
	"collaboration": {}, "creator_platform": {}, "marketplace": {},
}

// LoadProfile loads and parses profile.json
func LoadProfile(path string) (*FounderProfile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading profile: %w", err)
	}
	var profile FounderProfile
	if err := json.Unmarshal(data, &profile); err != nil {
		return nil, fmt.Errorf("parsing profile: %w", err)
	}
	if profile.ScoringWeights == nil {
		profile.ScoringWeights = map[string]float64{
			"buildability":           weightBuildability,
			"market_size":            weightMarketSize,
			"monetization_potential": weightMonetization,
			"competition_level":      weightCompetition,
			"unique_advantage":       weightUniqueAdvantage,
			"distribution_clarity":   weightDistribution,
			"retention_signal":       weightRetention,
		}
	}
	return &profile, nil
}

// IsViable checks if a coordinate set passes hard constraints
// Returns (is_viable, reason_if_not)
func IsViable(coords map[string]string, profile *FounderProfile) (bool, string) {
	tech := coords["technology"]
	dist := coords["distribution"]
	interaction := coords["interaction_model"]
	content := coords["content_format"]
	monetization := coords["monetization"]
	category := coords["category"]

	// Technology constraints
	avoidTech := setOf(profile.Preferences.AvoidTechnology)
	if _, bad := avoidTech[tech]; bad {
		return false, fmt.Sprintf("Technology %s in avoid list", tech)
	}

	// Distribution constraints: hardware-dependent platforms
	switch dist {
	case "smart_tv", "car_infotainment", "smartwatch":
		return false, fmt.Sprintf("Distribution %s requires hardware access", dist)
	}

	// Interaction model constraints
	avoidInteraction := setOf(profile.Preferences.AvoidInteraction)
	if _, bad := avoidInteraction[interaction]; bad {
		return false, fmt.Sprintf("Interaction %s not viable", interaction)
	}

	// Content format constraints
	if content == "ar_vr" || content == "games" {
		return false, fmt.Sprintf("Content format %s requires specialized skills", content)
	}

	// Monetization constraints
	avoidMon := setOf(profile.Preferences.AvoidMonetization)
	if _, bad := avoidMon[monetization]; bad {
		return false, fmt.Sprintf("Monetization %s not preferred", monetization)
	}

	// Category constraints
	if category == "gaming" && content != "text" {
		return false, "Gaming requires game dev skills (not available)"
	}

	// Network effects constraint for social/marketplace
	if (category == "social" || category == "ecommerce") &&
		interaction == "many_to_many" && tech != "ai_llms" {
		return false, "Network effects products need significant user base"
	}

	return true, ""
}

// distributionClarityScore scores "first 50 users" channel clarity (0-1)
func distributionClarityScore(coords map[string]string) float64 {
	dist := coords["distribution"]
	audience := coords["audience"]
	category := coords["category"]
	interaction := coords["interaction_model"]
	tech := coords["technology"]

	score := 0.0

	if dist == "browser_extension" {
		score += 0.4
	}
	if (audience == "developers") && (dist == "web_platform" || dist == "browser_extension") {
		score += 0.3
	}
	if category == "education" && dist == "web_platform" {
		score += 0.3 // SEO intent
	}
	if (audience == "businesses" || audience == "professionals" || audience == "entrepreneurs") &&
		(interaction == "collaboration" || interaction == "ai_assistant") {
		score += 0.25
	}
	if audience == "creators" && interaction == "creator_platform" {
		score += 0.3
	}
	if dist == "messaging_bot" {
		score += 0.25
	}
	if tech == "ai_llms" && audience == "developers" {
		score += 0.2
	}

	return clamp01(score)
}

// retentionSignalScore scores sticky habit potential (0-1)
func retentionSignalScore(coords map[string]string) float64 {
	engagement := coords["engagement_mechanics"]
	interaction := coords["interaction_model"]
	dataSource := coords["data_source"]
	category := coords["category"]

	score := 0.0

	switch engagement {
	case "streaks", "quests":
		score += 0.35
	case "reputation", "levels":
		score += 0.25
	case "rewards", "badges":
		score += 0.15
	}

	switch interaction {
	case "ai_assistant":
		score += 0.25
	case "community_forum", "collaboration":
		score += 0.2
	}

	switch dataSource {
	case "user_generated", "sensors_wearables":
		score += 0.2
	}

	switch category {
	case "finance", "health_fitness":
		score += 0.1
	}

	return clamp01(score)
}

// bloodbathPenalty returns 0.4 if category is in bloodbath set with no escape
func bloodbathPenalty(coords map[string]string) float64 {
	category := coords["category"]
	audience := coords["audience"]
	interaction := coords["interaction_model"]
	vp := coords["value_proposition"]

	if _, inBloodbath := bloodbathCategories[category]; !inBloodbath {
		return 0
	}

	// Escape conditions
	if _, ok := bloodbathEscapeAudiences[audience]; ok {
		return 0
	}
	if _, ok := bloodbathEscapeInteractions[interaction]; ok {
		return 0
	}
	// Specific VP escape
	if vp == "earn_money" || vp == "save_money" {
		return 0
	}

	return 0.4
}

// narrowWedgeBonus rewards specificity
func narrowWedgeBonus(coords map[string]string) float64 {
	geo := coords["geography"]
	dist := coords["distribution"]
	audience := coords["audience"]
	interaction := coords["interaction_model"]

	bonus := 0.0

	if geo != "global" && geo != "" {
		bonus += 0.15
	}

	switch dist {
	case "browser_extension", "messaging_bot", "desktop_software":
		bonus += 0.15
	}

	if (audience == "developers" || audience == "athletes" || audience == "investors") &&
		(interaction == "collaboration" || interaction == "ai_assistant") {
		bonus += 0.2
	}

	return clamp01(bonus)
}

// AssignStrategicPosture returns one of four postures based on idea coordinates
func AssignStrategicPosture(coords map[string]string) string {
	audience := coords["audience"]
	dist := coords["distribution"]
	geo := coords["geography"]
	interaction := coords["interaction_model"]
	mon := coords["monetization"]

	// 1. best_free: developers + web/browser/bot distribution
	if audience == "developers" &&
		(dist == "web_platform" || dist == "browser_extension" || dist == "messaging_bot") {
		return "best_free"
	}

	// 2. best_distribution: strong distribution clarity + global/us geography
	if distributionClarityScore(coords) >= 0.6 && (geo == "global" || geo == "us") {
		return "best_distribution"
	}

	// 3. best_premium: professionals/businesses + subscription/saas + ai/collab
	if (audience == "professionals" || audience == "businesses") &&
		(mon == "subscription" || mon == "saas_license") &&
		(interaction == "ai_assistant" || interaction == "collaboration") {
		return "best_premium"
	}

	// 4. best_niche: non-global geo or niche audience (default fallback)
	return "best_niche"
}

// ScoreFit scores how well an idea fits the founder profile (0-1 scale)
// Returns (total_score, breakdown_by_factor)
func ScoreFit(coords map[string]string, profile *FounderProfile) (float64, map[string]float64) {
	scores := make(map[string]float64)

	tech := coords["technology"]
	dist := coords["distribution"]
	interaction := coords["interaction_model"]
	geo := coords["geography"]
	audience := coords["audience"]
	category := coords["category"]
	mon := coords["monetization"]
	vp := coords["value_proposition"]

	// 1. Buildability (15%)
	buildability := 0.0
	if tech == "ai_llms" {
		buildability += 0.4
	}
	switch dist {
	case "web_platform", "browser_extension":
		buildability += 0.3
	case "messaging_bot":
		buildability += 0.2
	}
	if interaction == "ai_assistant" {
		buildability += 0.3
	}
	scores["buildability"] = clamp01(buildability)

	// 2. Market size (15%)
	market := 0.0
	switch geo {
	case "global":
		market += 0.4
	case "us", "europe":
		market += 0.3
	case "middle_east":
		market += 0.2
	}
	switch audience {
	case "professionals", "creators", "developers", "entrepreneurs":
		market += 0.3
	case "businesses":
		market += 0.4
	}
	switch category {
	case "ai_productivity", "education", "finance":
		market += 0.3
	}
	scores["market_size"] = clamp01(market)

	// 3. Monetization potential (20%)
	monScore := 0.0
	switch mon {
	case "subscription":
		monScore += 0.5
	case "saas_license":
		monScore += 0.4
	case "in_app_purchases":
		monScore += 0.3
	}
	if (audience == "businesses" || audience == "professionals") &&
		(mon == "subscription" || mon == "saas_license") {
		monScore += 0.3
	}
	scores["monetization_potential"] = clamp01(monScore)

	// 4. Competition level (20%) – inverse: lower competition = higher score
	competition := 0.0
	if geo == "middle_east" && category == "education" {
		competition += 0.4
	}
	if tech == "ai_llms" && category != "ai_productivity" {
		competition += 0.3
	}
	if (audience == "students" || audience == "professionals") &&
		(geo == "middle_east" || geo == "global") &&
		(category == "education" || category == "health_fitness" || category == "finance") {
		competition += 0.3
	}
	competition += narrowWedgeBonus(coords)
	competition -= bloodbathPenalty(coords)
	scores["competition_level"] = clamp01(competition)

	// 5. Unique advantage (10%)
	advantage := 0.0
	if dist == "browser_extension" {
		advantage += 0.4
	}
	if (geo == "middle_east" || geo == "global") && (category == "education" || category == "lifestyle") {
		advantage += 0.3
	}
	if tech == "ai_llms" && (vp == "save_time" || vp == "increase_productivity") {
		advantage += 0.3
	}
	scores["unique_advantage"] = clamp01(advantage)

	// 6. Distribution clarity (15%) — NEW
	scores["distribution_clarity"] = distributionClarityScore(coords)

	// 7. Retention signal (5%) — NEW
	scores["retention_signal"] = retentionSignalScore(coords)

	// Weighted total
	total := 0.0
	weights := profile.ScoringWeights
	if len(weights) == 0 {
		weights = map[string]float64{
			"buildability":           weightBuildability,
			"market_size":            weightMarketSize,
			"monetization_potential": weightMonetization,
			"competition_level":      weightCompetition,
			"unique_advantage":       weightUniqueAdvantage,
			"distribution_clarity":   weightDistribution,
			"retention_signal":       weightRetention,
		}
	}
	for factor, score := range scores {
		total += score * weights[factor]
	}

	return total, scores
}

// GenerateReasoning generates a 7-question framework answer for a fit score
func GenerateReasoning(coords map[string]string, breakdown map[string]float64) string {
	audience := coords["audience"]
	geo := coords["geography"]
	vp := coords["value_proposition"]
	engagement := coords["engagement_mechanics"]

	posture := AssignStrategicPosture(coords)
	channel := deriveFirstChannel(coords)
	differentiator := deriveDifferentiator(coords, breakdown)
	retentionHook := deriveRetentionHook(coords)
	moat := deriveMoat(coords, breakdown)
	asset := deriveCompoundingAsset(coords)
	hope := deriveInitialHope(coords)

	lines := []string{
		fmt.Sprintf("[WHO] %s in %s focused on %s", audience, geo, vp),
		fmt.Sprintf("[CHANNEL] %s", channel),
		fmt.Sprintf("[HOPE] %s", hope),
		fmt.Sprintf("[WHY THIS] %s", differentiator),
		fmt.Sprintf("[RETENTION] %s via %s", engagement, retentionHook),
		fmt.Sprintf("[MOAT] %s", moat),
		fmt.Sprintf("[ASSET] %s", asset),
		fmt.Sprintf("[POSTURE] %s", posture),
	}

	if bloodbathPenalty(coords) > 0 {
		lines = append(lines, "[WARNING] Bloodbath category — differentiation required to survive")
	}

	return strings.Join(lines, "\n")
}

// deriveFirstChannel maps audience+distribution+category to a channel description
func deriveFirstChannel(coords map[string]string) string {
	audience := coords["audience"]
	dist := coords["distribution"]
	category := coords["category"]

	switch {
	case audience == "developers" && dist == "browser_extension":
		return "Chrome Web Store + dev communities (HN, Product Hunt, Reddit r/programming)"
	case audience == "developers" && (dist == "web_platform" || dist == "messaging_bot"):
		return "Developer communities: GitHub, HN, dev Discords, Twitter/X"
	case audience == "businesses" || audience == "professionals":
		return "LinkedIn outreach + cold email to target job titles"
	case category == "education" && dist == "web_platform":
		return "SEO + long-tail educational content (intent-driven inbound)"
	case audience == "creators":
		return "Creator communities: YouTube, TikTok, Instagram DMs + collabs"
	case dist == "messaging_bot":
		return "Slack/Discord app directories + community demos"
	case audience == "entrepreneurs":
		return "Founder communities: Indie Hackers, Product Hunt, startup Slacks"
	default:
		return fmt.Sprintf("%s distribution + organic social", dist)
	}
}

// deriveDifferentiator maps tech+distribution+scores to why-choose-this
func deriveDifferentiator(coords map[string]string, breakdown map[string]float64) string {
	tech := coords["technology"]
	dist := coords["distribution"]
	vp := coords["value_proposition"]

	switch {
	case dist == "browser_extension":
		return fmt.Sprintf("Zero-install, in-context delivery — %s happens where the user already is", vp)
	case tech == "ai_llms" && breakdown["unique_advantage"] > 0.5:
		return fmt.Sprintf("AI-native %s with defensible data loop", vp)
	case breakdown["competition_level"] > 0.6:
		return fmt.Sprintf("Low-competition wedge: first mover advantage in narrow segment for %s", vp)
	case dist == "messaging_bot":
		return fmt.Sprintf("Meets users in their existing messaging workflow — no new app habit required for %s", vp)
	default:
		return fmt.Sprintf("%s with %s as primary differentiator", vp, tech)
	}
}

// deriveRetentionHook maps dataSource+interaction+engagement to retention mechanism
func deriveRetentionHook(coords map[string]string) string {
	dataSource := coords["data_source"]
	interaction := coords["interaction_model"]
	engagement := coords["engagement_mechanics"]

	switch {
	case interaction == "ai_assistant" && dataSource == "user_generated":
		return "personalized AI that gets smarter with usage (data flywheel)"
	case interaction == "ai_assistant":
		return "AI assistant improves with each session — switching means starting over"
	case engagement == "streaks" || engagement == "quests":
		return "daily habit loop with streak/quest mechanics"
	case engagement == "reputation" || engagement == "levels":
		return "reputation/level progression that can't be exported"
	case engagement == "rewards" || engagement == "badges":
		return "reward accumulation and status signaling"
	case engagement == "social_challenges":
		return "social accountability loop — peers keep users coming back"
	case engagement == "notifications":
		return "push-triggered re-engagement on relevant events"
	case interaction == "collaboration":
		return "team dependency — churn means the whole team loses context"
	case interaction == "community_forum":
		return "community gravity — answers, connections, and reputation built up over time"
	case dataSource == "sensors_wearables":
		return "continuous biometric data accumulation (history = lock-in)"
	case dataSource == "user_generated":
		return "user's own content/data accumulates — high switching cost"
	default:
		return "ongoing value delivery through repeated use"
	}
}

// deriveMoat maps distribution+audience+scores to hard-to-copy element
func deriveMoat(coords map[string]string, breakdown map[string]float64) string {
	dist := coords["distribution"]
	audience := coords["audience"]
	dataSource := coords["data_source"]

	switch {
	case dist == "browser_extension" && breakdown["unique_advantage"] > 0.4:
		return "Browser real-estate + UX context switching cost (users won't uninstall if embedded in workflow)"
	case audience == "developers" && breakdown["distribution_clarity"] > 0.5:
		return "Developer word-of-mouth + GitHub/toolchain integration (high switching cost)"
	case audience == "businesses" && breakdown["monetization_potential"] > 0.6:
		return "B2B contract stickiness + admin/team onboarding investment"
	case dataSource == "user_generated":
		return "Proprietary user data corpus — competitors start from zero"
	case dataSource == "sensors_wearables":
		return "Historical biometric data creates personalization gap competitors can't close"
	default:
		return fmt.Sprintf("Distribution advantage via %s — hard to replicate channel", dist)
	}
}

// deriveCompoundingAsset maps category+audience+distribution to lasting asset
func deriveCompoundingAsset(coords map[string]string) string {
	category := coords["category"]
	audience := coords["audience"]
	dist := coords["distribution"]
	dataSource := coords["data_source"]

	switch {
	case dataSource == "user_generated":
		return "User-generated dataset compounds with each new user — training data moat"
	case category == "education":
		return "Curriculum content library + learner outcome data"
	case category == "finance":
		return "Financial history + behavioral spending model per user"
	case audience == "developers" && dist == "browser_extension":
		return "Extension install base + GitHub integration data"
	case category == "health_fitness":
		return "Longitudinal health data + habit graph per user"
	case audience == "businesses":
		return "Org knowledge graph + workflow automation history"
	default:
		return fmt.Sprintf("Domain-specific %s data accumulated through %s engagement", category, audience)
	}
}

// deriveInitialHope maps value_proposition+category to what users hope for at first glance
func deriveInitialHope(coords map[string]string) string {
	vp := coords["value_proposition"]
	category := coords["category"]
	audience := coords["audience"]

	switch {
	case vp == "save_time":
		return fmt.Sprintf("'This will finally automate %s and give me back hours every week'", category)
	case vp == "save_money":
		return "'This will cut costs / help me earn more without extra work'"
	case vp == "learn_skills":
		return "'This will teach me the skill I've been putting off learning'"
	case vp == "connect_people":
		return "'This will help me find/reach the right people easily'"
	case vp == "organize_life":
		return "'This will get my chaotic %s under control'" // category
	case vp == "stay_healthy":
		return "'This will help me stick to habits I keep failing at'"
	case vp == "entertainment":
		return "'This will be fun and worth my time'"
	case vp == "status_identity":
		return "'This will make me look better / more capable to others'"
	case audience == "businesses":
		return fmt.Sprintf("'This will solve our %s bottleneck and scale the team'", category)
	default:
		return fmt.Sprintf("'This will make %s significantly easier'", vp)
	}
}

// setOf creates a string set from a slice
func setOf(s []string) map[string]struct{} {
	m := make(map[string]struct{}, len(s))
	for _, v := range s {
		m[v] = struct{}{}
	}
	return m
}

// clamp01 clamps a float to [0, 1]
func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
