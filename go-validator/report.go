package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

// GenerateJSONReport saves the full pipeline result as JSON
func GenerateJSONReport(result *PipelineResult, path string) error {
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// GenerateHTMLReport generates a dark-theme HTML report
func GenerateHTMLReport(result *PipelineResult, path string) error {
	scored := result.ScoredIdeas
	stats := result.Stats

	// Build top ideas rows
	var rows strings.Builder
	limit := 20
	if len(scored) < limit {
		limit = len(scored)
	}
	for _, s := range scored[:limit] {
		c := s.Idea.Coordinates
		m := s.Metrics

		composite := s.Metrics.CompositeScore
		barWidth := int(composite * 100)
		if barWidth < 2 {
			barWidth = 2
		}
		barColor := "#f44336"
		if barWidth > 50 {
			barColor = "#4caf50"
		} else if barWidth > 30 {
			barColor = "#ff9800"
		}

		convPct := m.ConversionRate * 100

		rows.WriteString(fmt.Sprintf(`
        <tr>
          <td><strong>#%d</strong></td>
          <td>%s</td>
          <td>%s</td>
          <td>%s</td>
          <td>%s</td>
          <td>
            <div style="background:#2d333b;border-radius:4px;overflow:hidden;width:120px;display:inline-block">
              <div style="background:%s;width:%d%%;height:20px;border-radius:4px"></div>
            </div>
            <small> %.3f</small>
          </td>
          <td>%.0f%% (%d/%d) [%.0f-%.0f%%]</td>
          <td>%.1f/10</td>
          <td>$%.1f</td>
          <td>$%d</td>
        </tr>`,
			s.Rank,
			titleCase(c["category"]),
			titleCase(c["audience"]),
			titleCase(c["technology"]),
			titleCase(c["monetization"]),
			barColor, barWidth, composite,
			convPct, m.Conversions, m.TotalPersonas, m.ConversionCILow*100, m.ConversionCIHigh*100,
			m.AvgRelevance,
			m.SimulatedCPL,
			s.SelectedPrice,
		))
	}

	// Build phase timing rows
	var phaseRows strings.Builder
	for phase, t := range stats.PhaseTimes {
		phaseRows.WriteString(fmt.Sprintf(
			"<tr><td>%s</td><td>%.1fs</td></tr>\n",
			titleCase(phase), t,
		))
	}

	// Build learning history
	var learningSection strings.Builder
	if result.Config.Epochs > 0 {
		learningSection.WriteString("<h2>Learning History</h2><table>")
		learningSection.WriteString("<tr><th>Epoch</th><th>Ideas</th><th>Avg Conversion</th><th>Timestamp</th></tr>")
		// Note: learner history not included in result directly; just show epoch count
		learningSection.WriteString("</table>")
	}

	html := fmt.Sprintf(`<!DOCTYPE html>
<html><head><meta charset="utf-8">
<title>Startup Factory Results</title>
<style>
  body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif;
         max-width: 1400px; margin: 0 auto; padding: 20px;
         background: #0d1117; color: #c9d1d9; }
  h1, h2 { color: #58a6ff; }
  h1 { border-bottom: 1px solid #30363d; padding-bottom: 16px; }
  .stats { display: grid; grid-template-columns: repeat(auto-fit, minmax(160px, 1fr));
           gap: 16px; margin: 20px 0; }
  .stat { background: #161b22; border: 1px solid #30363d; border-radius: 8px;
          padding: 16px; text-align: center; }
  .stat .value { font-size: 2em; font-weight: bold; color: #58a6ff; }
  .stat .label { color: #8b949e; font-size: 0.85em; margin-top: 4px; }
  table { width: 100%%; border-collapse: collapse; margin: 16px 0; font-size: 0.9em; }
  th, td { padding: 10px 12px; border: 1px solid #30363d; text-align: left; }
  th { background: #161b22; color: #58a6ff; font-weight: 600; }
  tr:nth-child(even) { background: #161b22; }
  tr:hover { background: #1c2128; }
  .success { color: #3fb950; }
  .fail { color: #f85149; }
  .meta { color: #8b949e; font-size: 0.85em; margin-bottom: 24px; }
  .formula { background: #161b22; border: 1px solid #30363d; border-radius: 8px;
             padding: 12px 16px; margin: 8px 0; color: #8b949e; font-family: monospace; }
</style></head><body>

<h1>Startup Factory Results</h1>
<p class="meta">Run: %s | %d ideas × %d personas | %s mode</p>

<div class="stats">
  <div class="stat"><div class="value">%d</div><div class="label">Ideas Tested</div></div>
  <div class="stat"><div class="value">%d</div><div class="label">Personas Each</div></div>
  <div class="stat"><div class="value">%d</div><div class="label">Total LLM Calls</div></div>
  <div class="stat"><div class="value">%.0fs</div><div class="label">Total Time</div></div>
  <div class="stat"><div class="value success">%d</div><div class="label">Successful</div></div>
  <div class="stat"><div class="value fail">%d</div><div class="label">Failed</div></div>
  <div class="stat"><div class="value">%.1f/s</div><div class="label">Throughput</div></div>
</div>

<h2>Phase Timing</h2>
<table><tr><th>Phase</th><th>Time</th></tr>
%s
</table>

<h2>Top %d Ideas (by confidence-adjusted composite score)</h2>
<div class="formula">
  Composite = Conversion×0.5 + Relevance×0.25 + Impression×0.15 + (1-CPL/100)×0.1 - uncertainty_penalty
</div>
<table>
  <tr><th>#</th><th>Category</th><th>Audience</th><th>Technology</th><th>Monetization</th>
      <th>Score</th><th>Conversion</th><th>Relevance</th><th>Sim CPL</th><th>Best Price</th></tr>
  %s
</table>

%s

<p class="meta">Generated by Startup Factory | %s</p>
</body></html>`,
		result.Timestamp,
		len(scored), result.Config.NumPersonas, result.Config.Mode,
		len(scored), result.Config.NumPersonas,
		stats.TotalCalls, stats.TotalTime,
		stats.Successful, stats.Failed, stats.Throughput,
		phaseRows.String(),
		limit,
		rows.String(),
		learningSection.String(),
		time.Now().Format(time.RFC3339),
	)

	return os.WriteFile(path, []byte(html), 0644)
}

// PrintSummary prints a concise terminal summary
func PrintSummary(result *PipelineResult) {
	stats := result.Stats
	scored := result.ScoredIdeas

	fmt.Printf("\n%s\n", strings.Repeat("=", 70))
	fmt.Println("RESULTS SUMMARY")
	fmt.Printf("%s\n\n", strings.Repeat("=", 70))

	fmt.Printf("Runtime:     %.1f minutes\n", stats.TotalTime/60)
	fmt.Printf("Ideas tested: %d (requested %d)\n", len(scored), result.Config.NumIdeas)
	fmt.Printf("LLM Calls:   %d (%.1f/s)\n", stats.TotalCalls, stats.Throughput)
	fmt.Printf("LLM Success: %d / %d (%.0f%%)\n",
		stats.Successful, stats.TotalCalls,
		float64(stats.Successful)/float64(max1(stats.TotalCalls, 1))*100)
	if stats.ExpectedConvHigh > 0 {
		fmt.Printf("Expected Conv (%s): %.1f%%-%.1f%% | Observed: %.1f%%\n",
			result.Config.FunnelStage,
			stats.ExpectedConvLow*100,
			stats.ExpectedConvHigh*100,
			stats.ObservedConversion*100)
	}

	// Calculate overall persona decision stats
	totalConverted := 0
	totalRejected := 0
	totalErrors := 0
	totalPersonas := 0
	for _, s := range scored {
		totalConverted += s.Metrics.Conversions
		totalRejected += s.Metrics.Rejections
		totalErrors += s.Metrics.Errors
		totalPersonas += s.Metrics.TotalPersonas
	}
	validTotal := totalConverted + totalRejected
	validPct := 0.0
	convPct := 0.0
	if totalPersonas > 0 {
		validPct = float64(validTotal) / float64(totalPersonas) * 100
	}
	if validTotal > 0 {
		convPct = float64(totalConverted) / float64(validTotal) * 100
	}

	fmt.Printf("\nPersona Decisions:\n")
	fmt.Printf("  Valid: %d / %d (%.0f%%)\n", validTotal, totalPersonas, validPct)
	fmt.Printf("    Converted: %d (%.1f%% of valid)\n", totalConverted, convPct)
	fmt.Printf("    Rejected:  %d (%.1f%% of valid)\n", totalRejected, 100-convPct)
	fmt.Printf("  Errors: %d (parse/LLM failures)\n", totalErrors)

	if len(result.ProviderStats) > 0 {
		fmt.Println("\nProvider Stats:")
		for name, stat := range result.ProviderStats {
			successRate := 0.0
			if stat.Calls > 0 {
				successRate = float64(stat.Successes) / float64(stat.Calls) * 100
			}
			fmt.Printf("  %s: %d calls (%.0f%% success)\n", name, stat.Calls, successRate)
		}
	}

	if len(stats.LimiterStats) > 0 {
		fmt.Println("\nAdaptive Concurrency:")
		for _, s := range stats.LimiterStats {
			fmt.Printf("  %s\n", s)
		}
	}

	if len(stats.CalibrationWarnings) > 0 {
		fmt.Println("\nCalibration warnings:")
		for _, w := range stats.CalibrationWarnings {
			fmt.Printf("  - %s\n", w)
		}
	}

	fmt.Println("\nPhase times:")
	for phase, t := range stats.PhaseTimes {
		fmt.Printf("  %-25s %.1fs\n", titleCase(phase)+":", t)
	}

	top := 10
	if len(scored) < top {
		top = len(scored)
	}

	fmt.Printf("\nTop %d Ideas:\n\n", top)
	for _, s := range scored[:top] {
		c := s.Idea.Coordinates
		composite := s.Metrics.CompositeScore

		// Show conversion rate only from valid personas (exclude errors)
		convPct := 0.0
		if s.Metrics.ValidPersonas > 0 {
			convPct = float64(s.Metrics.Conversions) / float64(s.Metrics.ValidPersonas) * 100
		}

		fmt.Printf("  #%d Score:%.3f Conv:%.0f%% [%.0f-%.0f%%] | Price:$%d | Stability:%.0f%% | %d✓ %d✗ %d⚠ | %s → %s | %s\n",
			s.Rank, composite, convPct,
			s.Metrics.ConversionCILow*100, s.Metrics.ConversionCIHigh*100, s.SelectedPrice, s.RankStability*100,
			s.Metrics.Conversions, s.Metrics.Rejections, s.Metrics.Errors,
			titleCase(c["category"]), titleCase(c["audience"]),
			titleCase(c["technology"]))
		fmt.Printf("     %s\n", s.Idea.Reasoning)
	}
}

func max1(a, b int) int {
	if a > b {
		return a
	}
	return b
}
