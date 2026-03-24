package main

import (
	"fmt"
	"math/rand"
	"sort"
)

// GenerateIdeas generates viable ideas by random sampling + filtering
func GenerateIdeas(profile *FounderProfile, count int, minScore float64) []StartupIdea {
	return generateIdeasInternal(profile, nil, count, minScore)
}

// GenerateWeightedIdeas generates ideas biased by learned dimension weights
// Uses 70% exploit (weighted sampling) / 30% explore (uniform sampling)
func GenerateWeightedIdeas(profile *FounderProfile, weights *DimensionWeights, count int, minScore float64) []StartupIdea {
	return generateIdeasInternal(profile, weights, count, minScore)
}

func generateIdeasInternal(profile *FounderProfile, weights *DimensionWeights, count int, minScore float64) []StartupIdea {
	var ideas []StartupIdea
	// At ~1% acceptance rate, need ~100x samples. Use 1000x with a 10k floor.
	maxAttempts := count * 1000
	if maxAttempts < 10000 {
		maxAttempts = 10000
	}
	attempts := 0

	for len(ideas) < count && attempts < maxAttempts {
		attempts++

		var coords map[string]string
		if weights != nil {
			coords = WeightedRandomCoordinates(weights)
		} else {
			coords = RandomCoordinates()
		}

		viable, _ := IsViable(coords, profile)
		if !viable {
			continue
		}

		fitScore, breakdown := ScoreFit(coords, profile)
		if fitScore < minScore {
			continue
		}

		reasoning := GenerateReasoning(coords, breakdown)
		desc := DescribeIdea(coords)

		ideas = append(ideas, StartupIdea{
			Coordinates:      coords,
			FitScore:         fitScore,
			FitBreakdown:     breakdown,
			Reasoning:        reasoning,
			Description:      desc,
			ICP:              generateICP(coords),
			StrategicPosture: AssignStrategicPosture(coords),
		})

		if len(ideas)%10 == 0 {
			fmt.Printf("   Found %d/%d viable ideas (attempt %d)...\n", len(ideas), count, attempts)
		}
	}

	// Sort by fit score descending
	sort.Slice(ideas, func(i, j int) bool {
		return ideas[i].FitScore > ideas[j].FitScore
	})

	fmt.Printf("   Generated %d ideas from %d attempts (%.1f%% acceptance)\n",
		len(ideas), attempts, float64(len(ideas))/float64(attempts)*100)

	if len(ideas) > count {
		return ideas[:count]
	}
	return ideas
}

// IdeaToLandingConfig generates a basic landing page from idea coordinates
// without requiring an LLM call (used as fallback or for generate-only mode)
func IdeaToLandingConfig(idea StartupIdea) *LandingPage {
	c := idea.Coordinates

	category := titleCase(c["category"])
	audience := c["audience"]
	valueProp := c["value_proposition"]
	technology := c["technology"]
	monetization := c["monetization"]
	interaction := c["interaction_model"]

	// Determine price from monetization model
	price := monetizationPrice(monetization)

	trialType := "14-day free trial"
	cta := "Start Free Trial"
	if monetization == "ads" {
		price = "0"
		trialType = "Free forever"
		cta = "Get Started Free"
	}

	headline := fmt.Sprintf("%s Tool That Helps %s %s",
		category, titleCase(audience), titleCase(valueProp))
	subheadline := fmt.Sprintf("AI-powered %s for %s who need to %s",
		titleCase(technology), audience, titleCase(valueProp))

	features := []string{
		fmt.Sprintf("AI-powered %s automation", titleCase(valueProp)),
		fmt.Sprintf("Built for %s", titleCase(audience)),
		"Works with your existing tools",
		fmt.Sprintf("%s interface", titleCase(interaction)),
		"Real-time analytics and insights",
	}

	return &LandingPage{
		Headline:    headline,
		Subheadline: subheadline,
		Price:       price,
		TrialType:   trialType,
		CTA:         cta,
		Features:    features,
	}
}

func monetizationPrice(mon string) string {
	prices := map[string]string{
		"subscription":           "29",
		"saas_license":           "49",
		"in_app_purchases":       "9",
		"ads":                    "0",
		"marketplace_commission": "0",
		"transaction_fees":       "0",
		"creator_revenue_share":  "0",
		"digital_goods":          "9",
	}
	if p, ok := prices[mon]; ok {
		return p
	}
	return "29"
}

// shuffleIdeas randomly shuffles ideas for diversity in weighted generation
func shuffleIdeas(ideas []StartupIdea) {
	rand.Shuffle(len(ideas), func(i, j int) {
		ideas[i], ideas[j] = ideas[j], ideas[i]
	})
}

// generateICP creates targeting parameters from idea coordinates
func generateICP(coords map[string]string) *IdealCustomerProfile {
	audience := coords["audience"]
	category := coords["category"]
	geography := coords["geography"]

	icp := &IdealCustomerProfile{
		TargetAudience:  audience,
		TargetGeography: geography,
	}

	// Map audience to job roles
	roleMap := map[string][]string{
		"developers":    {"Software Engineer", "Developer", "DevOps"},
		"designers":     {"Designer", "UI/UX"},
		"marketers":     {"Marketing Manager", "Content Marketer"},
		"founders":      {"Founder", "CEO", "CTO"},
		"students":      {"Student", "Learner"},
		"professionals": {"Manager", "Professional"},
		"businesses":    {"Business Owner", "Executive"},
		"athletes":      {"Athlete", "Fitness Professional"},
		"seniors":       {"Retired Professional"},
		"parents":       {"Parent"},
		"creators":      {"Content Creator", "Influencer"},
		"entrepreneurs": {"Entrepreneur", "Founder"},
	}
	if roles, ok := roleMap[audience]; ok {
		icp.JobRoles = roles
	}

	// Map audience to age range
	ageMap := map[string]string{
		"students":      "18-24",
		"developers":    "22-35",
		"seniors":       "60-75",
		"creators":      "18-35",
		"parents":       "28-45",
		"athletes":      "18-45",
		"entrepreneurs": "25-50",
	}
	if ar, ok := ageMap[audience]; ok {
		icp.AgeRange = ar
	} else {
		icp.AgeRange = "25-45"
	}

	// Map category to interests
	interestMap := map[string][]string{
		"ai_productivity": {"Technology", "AI", "Productivity"},
		"health_fitness":  {"Fitness", "Health", "Wellness"},
		"education":       {"Education", "Learning"},
		"finance":         {"Finance", "Investing"},
		"ecommerce":       {"Shopping", "Retail"},
		"social":          {"Social Media", "Networking"},
		"entertainment":   {"Entertainment", "Gaming"},
		"food_delivery":   {"Food", "Cooking"},
		"travel":          {"Travel", "Adventure"},
		"dating":          {"Dating", "Relationships"},
		"lifestyle":       {"Lifestyle", "Fashion"},
	}
	if interests, ok := interestMap[category]; ok {
		icp.Interests = interests
	} else {
		icp.Interests = []string{"Technology", "Business"}
	}

	return icp
}
