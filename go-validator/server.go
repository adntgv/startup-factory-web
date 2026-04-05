package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// runServer starts the HTTP server on the given port. DATABASE_URL is read from env.
func runServer(port string) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL env var required for server mode")
	}
	ctx := context.Background()
	db, err := OpenDB(ctx, dbURL)
	if err != nil {
		log.Fatalf("DB connect failed: %v", err)
	}
	log.Printf("Database connected, migrations applied")

	mux := http.NewServeMux()

	// Static files from embedded web/ directory
	webSub, _ := fs.Sub(webFS, "web")
	mux.Handle("/", http.FileServer(http.FS(webSub)))

	// Auth
	mux.HandleFunc("/api/auth/register", handleRegister(db))
	mux.HandleFunc("/api/auth/login", handleLogin(db))
	mux.HandleFunc("/api/auth/logout", handleLogout(db))
	mux.HandleFunc("/api/auth/me", requireAuth(db, handleMe(db)))

	// Runs
	mux.HandleFunc("/api/runs", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			requireAuth(db, handleListRuns(db))(w, r)
		case http.MethodPost:
			requireAuth(db, handleCreateRun(db))(w, r)
		default:
			http.Error(w, "method not allowed", 405)
		}
	})
	mux.HandleFunc("/api/runs/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if strings.HasSuffix(path, "/canvas") {
			requireAuth(db, handleCanvas(db))(w, r)
		} else if strings.HasSuffix(path, "/landing") {
			requireAuth(db, handleLanding(db))(w, r)
		} else if strings.HasSuffix(path, "/simulate") {
			requireAuth(db, handleSimulate(db))(w, r)
		} else {
			requireAuth(db, handleGetRun(db))(w, r)
		}
	})

	addr := ":" + port
	log.Printf("Startup Factory server listening on %s", addr)
	srv := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  5 * time.Minute,
		WriteTimeout: 10 * time.Minute,
		IdleTimeout:  2 * time.Minute,
	}
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}

// extractRunID extracts the run ID from paths like /api/runs/42/canvas
func extractRunID(path string) int64 {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	for i, p := range parts {
		if p == "runs" && i+1 < len(parts) {
			id, _ := strconv.ParseInt(parts[i+1], 10, 64)
			return id
		}
	}
	return 0
}

// ── JSON helpers ─────────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, code int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func readJSON(r *http.Request, v interface{}) error {
	return json.NewDecoder(r.Body).Decode(v)
}

// ── Auth handlers ─────────────────────────────────────────────────────────────

func handleRegister(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", 405)
			return
		}
		var body struct {
			Email    string `json:"email"`
			Password string `json:"password"`
		}
		if err := readJSON(r, &body); err != nil {
			writeJSON(w, 400, map[string]string{"error": "invalid JSON"})
			return
		}
		if body.Email == "" {
			writeJSON(w, 400, map[string]string{"error": "email required"})
			return
		}
		if len(body.Password) < 8 {
			writeJSON(w, 400, map[string]string{"error": "password must be at least 8 characters"})
			return
		}
		hash, err := hashPassword(body.Password)
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": "internal error"})
			return
		}
		userID, err := db.CreateUser(r.Context(), body.Email, hash)
		if err != nil {
			writeJSON(w, 409, map[string]string{"error": "email already registered"})
			return
		}
		token, err := generateSessionToken()
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": "internal error"})
			return
		}
		expiresAt := time.Now().Add(sessionDuration)
		if err := db.CreateSession(r.Context(), token, userID, expiresAt); err != nil {
			writeJSON(w, 500, map[string]string{"error": "internal error"})
			return
		}
		http.SetCookie(w, &http.Cookie{
			Name:     sessionCookie,
			Value:    token,
			Expires:  expiresAt,
			HttpOnly: true,
			Path:     "/",
			SameSite: http.SameSiteLaxMode,
		})
		writeJSON(w, 201, map[string]interface{}{"user_id": userID})
	}
}

func handleLogin(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", 405)
			return
		}
		var body struct {
			Email    string `json:"email"`
			Password string `json:"password"`
		}
		if err := readJSON(r, &body); err != nil {
			writeJSON(w, 400, map[string]string{"error": "invalid JSON"})
			return
		}
		if body.Email == "" {
			writeJSON(w, 400, map[string]string{"error": "email required"})
			return
		}
		if len(body.Password) < 8 {
			writeJSON(w, 400, map[string]string{"error": "password must be at least 8 characters"})
			return
		}
		userID, hash, err := db.GetUserByEmail(r.Context(), body.Email)
		if err != nil || !checkPassword(hash, body.Password) {
			writeJSON(w, 401, map[string]string{"error": "invalid credentials"})
			return
		}
		token, err := generateSessionToken()
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": "internal error"})
			return
		}
		expiresAt := time.Now().Add(sessionDuration)
		if err := db.CreateSession(r.Context(), token, userID, expiresAt); err != nil {
			writeJSON(w, 500, map[string]string{"error": "internal error"})
			return
		}
		http.SetCookie(w, &http.Cookie{
			Name:     sessionCookie,
			Value:    token,
			Expires:  expiresAt,
			HttpOnly: true,
			Path:     "/",
			SameSite: http.SameSiteLaxMode,
		})
		writeJSON(w, 200, map[string]interface{}{"user_id": userID})
	}
}

func handleLogout(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(sessionCookie)
		if err == nil {
			db.DeleteSession(r.Context(), cookie.Value)
		}
		http.SetCookie(w, &http.Cookie{
			Name:    sessionCookie,
			Value:   "",
			Expires: time.Unix(0, 0),
			MaxAge:  -1,
			Path:    "/",
		})
		writeJSON(w, 200, map[string]bool{"ok": true})
	}
}

func handleMe(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := getUserID(r)
		email, err := db.GetUserByID(r.Context(), userID)
		if err != nil {
			writeJSON(w, 404, map[string]string{"error": "user not found"})
			return
		}
		writeJSON(w, 200, map[string]interface{}{
			"user_id": userID,
			"email":   email,
		})
	}
}

// ── Run handlers ──────────────────────────────────────────────────────────────

// generateRunSummary calls the LLM to produce a strategic analysis of all persona reasoning.
func generateRunSummary(pipe *Pipeline, personas []Persona, results []SimulationResult, metrics SimulationMetrics) string {
	if len(results) == 0 {
		return ""
	}

	// Build compact summary of each persona's verdict
	type entry struct {
		Name      string
		Role      string
		PainLevel int
		Converted bool
		Reasoning string
	}
	entries := make([]entry, 0, len(results))
	for i, r := range results {
		name, role := fmt.Sprintf("Persona %d", i+1), ""
		painLevel := 0
		if i < len(personas) {
			name = personas[i].Name
			role = personas[i].Role
			painLevel = personas[i].PainLevel
		}
		entries = append(entries, entry{name, role, painLevel, r.Converted, r.Reasoning})
	}

	// Summarise in ~1500 chars so the prompt stays small
	var sb strings.Builder
	for _, e := range entries {
		verdict := "REJECTED"
		if e.Converted {
			verdict = "CONVERTED"
		}
		sb.WriteString(fmt.Sprintf("- %s (%s, pain=%d): %s — %s\n", e.Name, e.Role, e.PainLevel, verdict, truncate(e.Reasoning, 120)))
	}

	prompt := fmt.Sprintf(`You are a startup advisor. Below are the reactions of %d simulated personas to a product landing page.

Conversion rate: %.1f%% (%d converted, %d rejected)

PERSONA VERDICTS:
%s

Write a concise strategic analysis (4–6 paragraphs, no bullet lists) covering:
1. Who converted and why — what pain levels, roles, and mental states drove conversion
2. Who rejected and their core objections — what stopped them
3. The single biggest friction theme across all rejections
4. Specific, actionable changes to the landing page or product positioning that would lift conversion
5. Honest verdict: is this idea worth pursuing, pivoting, or killing — and why

Be direct and specific. No generic startup advice.`,
		len(results),
		metrics.ConversionRate*100,
		metrics.Conversions,
		metrics.Rejections,
		sb.String(),
	)

	resp := pipe.maxclaw.Call(LLMRequest{Prompt: prompt, MaxTokens: 2000})
	if resp.Error != nil {
		log.Printf("[summary] LLM error: %v", resp.Error)
		return ""
	}
	return strings.TrimSpace(resp.Content)
}

func newWebPipeline() *Pipeline {
	return &Pipeline{
		maxclaw: NewMaxClawProvider(),
		config: PipelineConfig{
			MaxConcurrent:  50,
			EnrichmentMode: "",
			SimulationBias: "neutral",
		},
		stats: PipelineStats{PhaseTimes: make(map[string]float64)},
	}
}

func handleCreateRun(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", 405)
			return
		}
		userID := getUserID(r)
		var body struct {
			IdeaText string `json:"idea_text"`
		}
		if err := readJSON(r, &body); err != nil {
			writeJSON(w, 400, map[string]string{"error": "invalid JSON"})
			return
		}
		if strings.TrimSpace(body.IdeaText) == "" {
			writeJSON(w, 400, map[string]string{"error": "idea_text required"})
			return
		}

		runID, err := db.CreateRun(r.Context(), userID, body.IdeaText)
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": "failed to create run"})
			return
		}

		// Generate canvas synchronously
		pipe := newWebPipeline()
		canvas := pipe.generateLeanCanvasViaLLM(body.IdeaText)
		if canvas == nil {
			db.UpdateRunStatus(r.Context(), runID, "failed")
			writeJSON(w, 500, map[string]string{"error": "canvas generation failed"})
			return
		}

		canvasJSON, err := json.Marshal(canvas)
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": "internal error"})
			return
		}
		if err := db.UpdateRunCanvas(r.Context(), runID, canvasJSON); err != nil {
			writeJSON(w, 500, map[string]string{"error": "internal error"})
			return
		}
		db.UpdateRunStatus(r.Context(), runID, "canvas_ready")

		writeJSON(w, 201, map[string]interface{}{
			"run_id": runID,
			"canvas": canvas,
		})
	}
}

func handleListRuns(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", 405)
			return
		}
		userID := getUserID(r)
		runs, err := db.ListRuns(r.Context(), userID)
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": "internal error"})
			return
		}
		type runSummary struct {
			ID        int64     `json:"id"`
			IdeaText  string    `json:"idea_text"`
			Status    string    `json:"status"`
			CreatedAt time.Time `json:"created_at"`
		}
		out := make([]runSummary, 0, len(runs))
		for _, r := range runs {
			out = append(out, runSummary{
				ID:        r.ID,
				IdeaText:  r.IdeaText,
				Status:    r.Status,
				CreatedAt: r.CreatedAt,
			})
		}
		writeJSON(w, 200, out)
	}
}

func handleGetRun(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", 405)
			return
		}
		userID := getUserID(r)
		runID := extractRunID(r.URL.Path)
		if runID == 0 {
			writeJSON(w, 400, map[string]string{"error": "invalid run id"})
			return
		}
		run, err := db.GetRun(r.Context(), runID, userID)
		if err != nil || run == nil {
			writeJSON(w, 404, map[string]string{"error": "run not found"})
			return
		}
		resp := map[string]interface{}{
			"id":          run.ID,
			"idea_text":   run.IdeaText,
			"canvas":      run.Canvas,
			"landing":     run.Landing,
			"status":      run.Status,
			"personas":    run.Personas,
			"validations": run.Validations,
			"results":     run.Results,
			"error_msg":   run.ErrorMsg,
			"created_at":  run.CreatedAt,
			"updated_at":  run.UpdatedAt,
		}
		// Render landing HTML server-side so reload shows the preview
		if run.Landing != nil {
			var lp LandingPage
			if json.Unmarshal(run.Landing, &lp) == nil {
				resp["landing_html"] = renderLandingHTML(lp)
			}
		}
		writeJSON(w, 200, resp)
	}
}

func handleCanvas(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			http.Error(w, "method not allowed", 405)
			return
		}
		userID := getUserID(r)
		runID := extractRunID(r.URL.Path)
		if runID == 0 {
			writeJSON(w, 400, map[string]string{"error": "invalid run id"})
			return
		}
		run, err := db.GetRun(r.Context(), runID, userID)
		if err != nil || run == nil {
			writeJSON(w, 404, map[string]string{"error": "run not found"})
			return
		}

		var body struct {
			Canvas LeanCanvas `json:"canvas"`
		}
		if err := readJSON(r, &body); err != nil {
			writeJSON(w, 400, map[string]string{"error": "invalid JSON"})
			return
		}
		canvasJSON, err := json.Marshal(body.Canvas)
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": "internal error"})
			return
		}
		if err := db.UpdateRunCanvas(r.Context(), runID, canvasJSON); err != nil {
			writeJSON(w, 500, map[string]string{"error": "internal error"})
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	}
}

func handleLanding(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", 405)
			return
		}
		userID := getUserID(r)
		runID := extractRunID(r.URL.Path)
		if runID == 0 {
			writeJSON(w, 400, map[string]string{"error": "invalid run id"})
			return
		}
		run, err := db.GetRun(r.Context(), runID, userID)
		if err != nil || run == nil {
			writeJSON(w, 404, map[string]string{"error": "run not found"})
			return
		}
		if run.Canvas == nil {
			writeJSON(w, 400, map[string]string{"error": "generate canvas first"})
			return
		}

		var canvas LeanCanvas
		if err := json.Unmarshal(run.Canvas, &canvas); err != nil {
			writeJSON(w, 400, map[string]string{"error": "invalid canvas"})
			return
		}

		canvasDesc := canvasToDescription(canvas)
		pipe := newWebPipeline()
		idea := StartupIdea{Description: canvasDesc}
		landing := pipe.generateLandingViaLLM(idea)
		if landing == nil {
			writeJSON(w, 500, map[string]string{"error": "landing generation failed"})
			return
		}

		landingJSON, err := json.Marshal(landing)
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": "internal error"})
			return
		}
		if err := db.UpdateRunLanding(r.Context(), runID, landingJSON); err != nil {
			writeJSON(w, 500, map[string]string{"error": "internal error"})
			return
		}
		db.UpdateRunStatus(r.Context(), runID, "landing_ready")

		html := renderLandingHTML(*landing)
		writeJSON(w, 200, map[string]interface{}{
			"landing": landing,
			"html":    html,
		})
	}
}

func handleSimulate(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := getUserID(r)
		runID := extractRunID(r.URL.Path)
		if runID == 0 {
			http.Error(w, `{"error":"invalid run id"}`, 400)
			return
		}

		run, err := db.GetRun(r.Context(), runID, userID)
		if err != nil || run == nil {
			http.Error(w, `{"error":"run not found"}`, 404)
			return
		}
		if run.Landing == nil {
			http.Error(w, `{"error":"generate landing page first"}`, 400)
			return
		}

		// Parse canvas and landing from DB
		var canvas LeanCanvas
		if err := json.Unmarshal(run.Canvas, &canvas); err != nil {
			http.Error(w, `{"error":"invalid canvas"}`, 400)
			return
		}
		var landing LandingPage
		if err := json.Unmarshal(run.Landing, &landing); err != nil {
			http.Error(w, `{"error":"invalid landing"}`, 400)
			return
		}

		// Setup SSE
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", 500)
			return
		}

		emit := func(eventType string, payload interface{}) {
			data, _ := json.Marshal(map[string]interface{}{
				"type":    eventType,
				"payload": payload,
			})
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}

		// Mark as simulating
		db.UpdateRunStatus(r.Context(), runID, "simulating")

		emit("progress", map[string]interface{}{"percent": 5, "message": "Starting..."})

		pipe := newWebPipeline()
		canvasDesc := canvasToDescription(canvas)
		isB2B := isB2BIdea(canvasDesc)
		idea := StartupIdea{Description: canvasDesc}

		// Phase 1: Generate 100 personas in parallel, stream each one
		emit("progress", map[string]interface{}{"percent": 10, "message": "Generating personas..."})

		const targetPersonas = 100
		fanOut := 125 // generate extra to cover failures/dupes
		concLimit := 50

		type personaSlot struct{ p *Persona }
		personaCh := make(chan personaSlot, fanOut)
		var wg sync.WaitGroup
		sem := make(chan struct{}, concLimit)

		for i := 0; i < fanOut; i++ {
			wg.Add(1)
			sem <- struct{}{}
			go func(idx int) {
				defer wg.Done()
				defer func() { <-sem }()
				ps := pipe.generatePersonaViaMaxClaw(canvasDesc, idx, fanOut, isB2B)
				if len(ps) > 0 {
					personaCh <- personaSlot{p: &ps[0]}
				} else {
					personaCh <- personaSlot{}
				}
			}(i)
		}
		go func() { wg.Wait(); close(personaCh) }()

		seen := map[string]bool{}
		var personas []Persona
		var personaMu sync.Mutex
		var personaCount int64

		for slot := range personaCh {
			if slot.p == nil {
				continue
			}
			personaMu.Lock()
			if !seen[slot.p.Name] && int64(len(personas)) < targetPersonas {
				seen[slot.p.Name] = true
				personas = append(personas, *slot.p)
				idx := len(personas)
				personaMu.Unlock()

				atomic.AddInt64(&personaCount, 1)
				pct := 10 + int(float64(idx)/float64(targetPersonas)*30)
				emit("persona", map[string]interface{}{
					"id":         idx,
					"name":       slot.p.Name,
					"role":       slot.p.Role,
					"age":        slot.p.Age,
					"archetype":  slot.p.Archetype,
					"pain_level": slot.p.PainLevel,
				})
				emit("progress", map[string]interface{}{"percent": pct, "message": fmt.Sprintf("Generated %d/%d personas", idx, targetPersonas)})
			} else {
				personaMu.Unlock()
			}
		}

		if len(personas) == 0 {
			db.UpdateRunStatus(r.Context(), runID, "failed")
			emit("error", map[string]interface{}{"message": "persona generation failed"})
			return
		}

		emit("progress", map[string]interface{}{"percent": 40, "message": fmt.Sprintf("%d personas ready, running evaluations...", len(personas))})

		// Phase 2: Simulate all personas in parallel, stream each result
		type simResult struct {
			idx    int
			result SimulationResult
		}
		simCh := make(chan simResult, len(personas))
		var wg2 sync.WaitGroup
		sem2 := make(chan struct{}, concLimit)

		for i, persona := range personas {
			wg2.Add(1)
			sem2 <- struct{}{}
			go func(idx int, p Persona) {
				defer wg2.Done()
				defer func() { <-sem2 }()
				result := pipe.simulatePersonaViaLLM(p, &landing, idea, 0)
				simCh <- simResult{idx: idx, result: result}
			}(i, persona)
		}
		go func() { wg2.Wait(); close(simCh) }()

		var allResults []SimulationResult
		simDone := 0
		for sr := range simCh {
			allResults = append(allResults, sr.result)
			simDone++

			decision := "reject"
			if sr.result.Converted {
				decision = "convert"
			}
			pct := 40 + int(float64(simDone)/float64(len(personas))*55)
			emit("persona_validation", map[string]interface{}{
				"personaId": sr.idx + 1,
				"validation": map[string]interface{}{
					"decision":          decision,
					"converted":         sr.result.Converted,
					"intent_strength":   sr.result.IntentStrength,
					"impression_score":  sr.result.ImpressionScore,
					"relevance_score":   sr.result.RelevanceScore,
					"pricing_reaction":  sr.result.PricingReaction,
					"friction_points":   sr.result.FrictionPoints,
					"reasoning":         sr.result.Reasoning,
					"decision_timeline": sr.result.DecisionTimeline,
				},
			})
			emit("progress", map[string]interface{}{"percent": pct, "message": fmt.Sprintf("Evaluated %d/%d personas", simDone, len(personas))})
		}

		// Compute aggregate metrics
		metrics := ScoreSimulation(allResults)

		// Persist to DB
		personasJSON, _ := json.Marshal(personas)
		validationsJSON, _ := json.Marshal(allResults)
		resultsJSON, _ := json.Marshal(metrics)
		db.UpdateRunResults(r.Context(), runID, personasJSON, validationsJSON, resultsJSON, "done")

		emit("progress", map[string]interface{}{"percent": 100, "message": "Generating analysis…"})

		// Grand summary: LLM analyzes all persona reasoning
		summary := generateRunSummary(pipe, personas, allResults, metrics)
		if summary != "" {
			emit("summary", map[string]interface{}{"text": summary})
			// Persist summary into results JSON
			var resultsMap map[string]interface{}
			if json.Unmarshal(resultsJSON, &resultsMap) == nil {
				resultsMap["summary"] = summary
				if updated, err := json.Marshal(resultsMap); err == nil {
					db.UpdateRunResults(r.Context(), runID, personasJSON, validationsJSON, updated, "done")
				}
			}
		}

		emit("progress", map[string]interface{}{"percent": 100, "message": "Complete!"})
		emit("results", map[string]interface{}{
			"run_id":          runID,
			"conversion_rate": metrics.ConversionRate,
			"conversions":     metrics.Conversions,
			"rejections":      metrics.Rejections,
			"total":           metrics.TotalPersonas,
			"intent_strength": metrics.IntentStrength,
			"friction_score":  metrics.FrictionScore,
			"composite_score": metrics.CompositeScore,
			"avg_impression":  metrics.AvgImpression,
			"avg_relevance":   metrics.AvgRelevance,
			"ci_low":          metrics.ConversionCILow,
			"ci_high":         metrics.ConversionCIHigh,
		})
	}
}
