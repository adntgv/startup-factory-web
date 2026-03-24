package main

import (
	"fmt"
	"math/rand"
	"strings"
)

// TopologyOrder defines deterministic key ordering for 12 dimensions
var TopologyOrder = []string{
	"category",
	"audience",
	"value_proposition",
	"interaction_model",
	"content_format",
	"technology",
	"monetization",
	"engagement_mechanics",
	"network_structure",
	"distribution",
	"geography",
	"data_source",
}

// Topology defines all 12 dimensions of the startup idea space
var Topology = map[string]Dimension{
	"category": {
		Name: "category",
		Options: []string{
			"gaming", "social", "entertainment", "dating", "ecommerce",
			"finance", "health_fitness", "education", "ai_productivity",
			"travel", "food_delivery", "lifestyle",
		},
	},
	"audience": {
		Name: "audience",
		Options: []string{
			"kids", "students", "professionals", "creators", "developers",
			"entrepreneurs", "parents", "seniors", "gamers", "athletes",
			"investors", "businesses",
		},
	},
	"value_proposition": {
		Name: "value_proposition",
		Options: []string{
			"save_time", "make_money", "learn_skills", "improve_health",
			"find_relationships", "build_community", "express_creativity",
			"increase_productivity", "entertainment",
		},
	},
	"interaction_model": {
		Name: "interaction_model",
		Options: []string{
			"feed", "chat", "multiplayer", "marketplace", "ai_assistant",
			"collaboration", "community_forum", "livestream", "creator_platform",
		},
	},
	"content_format": {
		Name: "content_format",
		Options: []string{
			"short_video", "long_video", "audio", "text", "images",
			"livestream", "interactive_simulation", "games", "ar_vr",
		},
	},
	"technology": {
		Name: "technology",
		Options: []string{
			"ai_llms", "ar_vr", "blockchain", "computer_vision",
			"voice_interface", "wearables", "iot", "robotics",
		},
	},
	"monetization": {
		Name: "monetization",
		Options: []string{
			"subscription", "ads", "in_app_purchases", "marketplace_commission",
			"transaction_fees", "saas_license", "creator_revenue_share", "digital_goods",
		},
	},
	"engagement_mechanics": {
		Name: "engagement_mechanics",
		Options: []string{
			"streaks", "rewards", "levels", "leaderboards",
			"badges", "quests", "reputation", "social_challenges",
		},
	},
	"network_structure": {
		Name: "network_structure",
		Options: []string{
			"one_to_one", "one_to_many", "many_to_many",
			"peer_to_peer", "two_sided_marketplace", "creator_fan",
		},
	},
	"distribution": {
		Name: "distribution",
		Options: []string{
			"mobile_app", "web_platform", "desktop_software", "browser_extension",
			"messaging_bot", "smart_tv", "smartwatch", "voice_assistant", "car_infotainment",
		},
	},
	"geography": {
		Name: "geography",
		Options: []string{
			"global", "us", "europe", "india", "china",
			"southeast_asia", "africa", "latin_america", "middle_east",
		},
	},
	"data_source": {
		Name: "data_source",
		Options: []string{
			"user_generated", "sensors_wearables", "financial_data", "gps_location",
			"camera_input", "voice_input", "ai_generated", "external_apis",
		},
	},
}

// RandomCoordinates generates a random point in the topology space
func RandomCoordinates() map[string]string {
	coords := make(map[string]string, len(Topology))
	for _, name := range TopologyOrder {
		dim := Topology[name]
		coords[name] = dim.Options[rand.Intn(len(dim.Options))]
	}
	return coords
}

// WeightedRandomCoordinates generates coordinates biased by learned weights
// Uses 70% exploit (weighted) / 30% explore (uniform) strategy
func WeightedRandomCoordinates(weights *DimensionWeights) map[string]string {
	coords := make(map[string]string, len(Topology))
	for _, name := range TopologyOrder {
		dim := Topology[name]
		if weights != nil {
			if dimWeights, ok := weights.Weights[name]; ok && len(dimWeights) > 0 {
				if rand.Float64() < 0.70 {
					// Exploit: weighted choice
					coords[name] = weightedChoice(dim.Options, dimWeights)
					continue
				}
			}
		}
		// Explore: uniform random
		coords[name] = dim.Options[rand.Intn(len(dim.Options))]
	}
	return coords
}

// weightedChoice selects from options using weight map (softmax-like)
func weightedChoice(options []string, weights map[string]float64) string {
	total := 0.0
	for _, opt := range options {
		w := weights[opt]
		if w <= 0 {
			w = 0.01 // small base weight so all options remain possible
		}
		total += w
	}
	r := rand.Float64() * total
	cumulative := 0.0
	for _, opt := range options {
		w := weights[opt]
		if w <= 0 {
			w = 0.01
		}
		cumulative += w
		if r <= cumulative {
			return opt
		}
	}
	return options[0]
}

// DescribeIdea generates a human-readable description from coordinates
func DescribeIdea(coords map[string]string) string {
	cat := titleCase(coords["category"])
	aud := coords["audience"]
	vp := titleCase(coords["value_proposition"])
	im := titleCase(coords["interaction_model"])
	dist := coords["distribution"]
	mon := titleCase(coords["monetization"])
	geo := titleCase(coords["geography"])

	return fmt.Sprintf("%s for %s | %s | %s via %s | %s | %s market",
		cat, aud, vp, im, dist, mon, geo)
}

// titleCase converts snake_case to Title Case
func titleCase(s string) string {
	parts := strings.Split(s, "_")
	for i, p := range parts {
		if len(p) > 0 {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, " ")
}
