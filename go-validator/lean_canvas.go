package main

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
)

// canvasToDescription converts a LeanCanvas into a prose description for the LLM prompts
func canvasToDescription(c LeanCanvas) string {
	var sb strings.Builder
	if c.HighLevelConcept != "" {
		sb.WriteString(c.HighLevelConcept + ". ")
	}
	if len(c.Problem) > 0 {
		sb.WriteString("Problems solved: " + strings.Join(c.Problem, "; ") + ". ")
	}
	if len(c.Solution) > 0 {
		sb.WriteString("Solution: " + strings.Join(c.Solution, "; ") + ". ")
	}
	if c.UniqueValueProp != "" {
		sb.WriteString("Value: " + c.UniqueValueProp + ". ")
	}
	if len(c.CustomerSegments) > 0 {
		sb.WriteString("For: " + strings.Join(c.CustomerSegments, ", ") + ". ")
	}
	if len(c.RevenueStreams) > 0 {
		sb.WriteString("Revenue: " + strings.Join(c.RevenueStreams, ", ") + ".")
	}
	return sb.String()
}

// generateLeanCanvasViaLLM generates a LeanCanvas from a free-form idea description
func (p *Pipeline) generateLeanCanvasViaLLM(ideaText string) *LeanCanvas {
	prompt := fmt.Sprintf(`You are a startup strategist. A founder has this idea:

"%s"

Generate a complete Lean Canvas. Be specific and concrete — no generic filler.

Return ONLY this JSON object (no markdown, no explanation):
{
  "problem": ["top problem 1", "problem 2", "problem 3"],
  "existing_alternatives": ["what people use today to solve this"],
  "solution": ["feature/approach 1", "feature 2", "feature 3"],
  "unique_value_prop": "one clear differentiating sentence",
  "high_level_concept": "X for Y (analogy, e.g. 'Grammarly for pitch decks')",
  "unfair_advantage": "what cannot be easily copied or bought",
  "customer_segments": ["specific ICP 1", "ICP 2"],
  "early_adopters": ["who will be first to buy and why"],
  "channels": ["distribution channel 1", "channel 2"],
  "key_metrics": ["metric 1", "metric 2", "metric 3"],
  "cost_structure": ["main cost 1", "cost 2"],
  "revenue_streams": ["revenue model 1", "revenue model 2"]
}`, ideaText)

	for attempt := 0; attempt < 3; attempt++ {
		resp := p.maxclaw.Call(LLMRequest{
			Prompt:    prompt,
			MaxTokens: 2000,
		})
		if resp.Error != nil {
			log.Printf("Canvas generation attempt %d failed: %v", attempt+1, resp.Error)
			continue
		}
		canvas, err := parseLeanCanvas(resp.Content)
		if err == nil && canvas != nil {
			return canvas
		}
		log.Printf("Canvas parse attempt %d failed: %v | raw[:200]: %s", attempt+1, err, truncate(resp.Content, 200))
	}
	return nil
}

func parseLeanCanvas(content string) (*LeanCanvas, error) {
	jsonStr := extractJSON(content)
	fixed := fixJSON(jsonStr)
	var c LeanCanvas
	if err := json.Unmarshal([]byte(fixed), &c); err != nil {
		return nil, err
	}
	if c.UniqueValueProp == "" && len(c.Problem) == 0 {
		return nil, fmt.Errorf("canvas missing required fields")
	}
	return &c, nil
}

// renderLandingHTML renders a LandingPage struct to a standalone HTML page.
// Picks one of 5 visual styles based on the headline to ensure variety.
func renderLandingHTML(lp LandingPage) string {
	featuresHTML := ""
	for _, f := range lp.Features {
		featuresHTML += fmt.Sprintf(`<li>%s</li>`, f)
	}
	trial := lp.TrialType
	if trial == "" {
		trial = "Free trial available"
	}

	// Pick style from headline hash for deterministic but varied output
	styleIdx := 0
	for _, c := range lp.Headline {
		styleIdx += int(c)
	}
	styleIdx = styleIdx % 5

	type theme struct {
		heroBg, ctaColor, ctaText, accentColor, featureBg, featureBorder string
	}
	themes := []theme{
		{"linear-gradient(135deg,#667eea,#764ba2)", "#fff", "#764ba2", "#667eea", "#f8f9ff", "#667eea"},
		{"linear-gradient(135deg,#11998e,#38ef7d)", "#fff", "#0d7a6a", "#11998e", "#f0fff8", "#11998e"},
		{"linear-gradient(135deg,#f7971e,#ffd200)", "#fff", "#c47a00", "#f7971e", "#fffdf0", "#f7971e"},
		{"linear-gradient(135deg,#e53935,#e91e63)", "#fff", "#c2185b", "#e91e63", "#fff5f7", "#e91e63"},
		{"linear-gradient(135deg,#1a1a2e,#16213e)", "#c9a96e", "#1a1a2e", "#c9a96e", "#f9f8f5", "#c9a96e"},
	}
	t := themes[styleIdx]

	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>%s</title>
<style>
*{margin:0;padding:0;box-sizing:border-box}
body{font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;color:#1a1a2e;line-height:1.6}
.hero{background:%s;color:#fff;padding:80px 24px;text-align:center}
.hero h1{font-size:clamp(28px,5vw,52px);font-weight:800;margin-bottom:16px;line-height:1.2}
.hero p{font-size:clamp(16px,2vw,22px);opacity:.9;max-width:700px;margin:0 auto 32px}
.badge{display:inline-block;background:rgba(255,255,255,.15);border:1px solid rgba(255,255,255,.3);border-radius:20px;padding:6px 16px;font-size:14px;margin-bottom:20px}
.cta-btn{background:%s;color:%s;padding:16px 40px;border-radius:10px;font-size:18px;font-weight:700;border:none;cursor:pointer;display:inline-block;text-decoration:none;transition:transform .2s,box-shadow .2s;box-shadow:0 4px 20px rgba(0,0,0,.2)}
.cta-btn:hover{transform:translateY(-2px);box-shadow:0 8px 30px rgba(0,0,0,.3)}
.price-tag{font-size:32px;font-weight:800;margin:20px 0 8px;opacity:.95}
.trial{font-size:14px;opacity:.7;margin-bottom:28px}
.features{max-width:900px;margin:60px auto;padding:0 24px}
.features h2{text-align:center;font-size:28px;margin-bottom:40px;color:#333}
.features ul{list-style:none;display:grid;grid-template-columns:repeat(auto-fit,minmax(280px,1fr));gap:16px}
.features li{background:%s;border-left:4px solid %s;padding:16px 20px;border-radius:8px;font-size:15px;color:#444}
.features li::before{content:"✓ ";color:%s;font-weight:700}
footer{text-align:center;padding:30px;color:#888;font-size:13px}
</style>
</head>
<body>
<div class="hero">
  <div class="badge">%s</div>
  <h1>%s</h1>
  <p>%s</p>
  <div class="price-tag">$%s/mo</div>
  <div class="trial">%s</div>
  <a href="#" class="cta-btn">%s</a>
</div>
<div class="features">
  <h2>What you get</h2>
  <ul>%s</ul>
</div>
<footer>Simple pricing. Cancel anytime.</footer>
</body>
</html>`, lp.Headline, t.heroBg, t.ctaColor, t.ctaText,
		t.featureBg, t.featureBorder, t.accentColor,
		lp.TrialType, lp.Headline, lp.Subheadline, lp.Price, trial, lp.CTA, featuresHTML)
}
