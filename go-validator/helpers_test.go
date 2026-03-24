package main

import "testing"

func TestExtractJSON_JSONFollowedByExtraText(t *testing.T) {
	content := "Here is output:\n{\"ok\":true}\nThis is extra commentary"
	got := extractJSON(content)
	want := "{\"ok\":true}"
	if got != want {
		t.Fatalf("extractJSON mismatch\nwant: %q\n got: %q", want, got)
	}
}

func TestParsePersonas_AcceptsPartialGeneration(t *testing.T) {
	content := `[{"name":"Alex","age":30,"role":"Engineer"}]`

	personas, err := parsePersonas(content, 10)
	if err != nil {
		t.Fatalf("expected partial persona parse to succeed, got error: %v", err)
	}
	if len(personas) != 1 {
		t.Fatalf("expected 1 persona, got %d", len(personas))
	}
}

func TestParseSimulation_RecoversArrayPayload(t *testing.T) {
	content := `[{"converted":false,"impression_score":6,"relevance_score":7,"intent_strength":4,"friction_points":["price"],"pricing_reaction":"stretch","cpl_equivalent":22,"reasoning":"too expensive","budget_check":"stretch","priority_rank":6,"time_available":false,"competing_with":"notion","decision_timeline":"next_month"}]`
	res := parseSimulation(content, "Ava")
	if res.Status == "error" {
		t.Fatalf("expected array payload recovery, got error: %+v", res)
	}
	if res.RelevanceScore != 7 || res.Converted {
		t.Fatalf("unexpected parsed result: %+v", res)
	}
}

func TestParseSimulation_HeuristicFallback(t *testing.T) {
	content := `{"converted": true, "reasoning": "looks good"`
	res := parseSimulation(content, "Ben")
	if res.Status == "error" {
		t.Fatalf("expected heuristic recovery, got error")
	}
	if !res.Converted {
		t.Fatalf("expected converted=true from heuristic recovery")
	}
}

func TestParsePersonas_WrappedObjectArray(t *testing.T) {
	content := `{"personas":[{"name":"Alex","age":30,"role":"Engineer"},{"name":"Sam","age":34,"role":"PM"}]}`
	personas, err := parsePersonas(content, 2)
	if err != nil {
		t.Fatalf("expected wrapped persona parse to succeed: %v", err)
	}
	if len(personas) != 2 {
		t.Fatalf("expected 2 personas, got %d", len(personas))
	}
}
