package nl

import (
	"context"
	"strings"
	"testing"
)

// fakeCompleter returns a scripted sequence of responses, one per call.
type fakeCompleter struct {
	responses []string
	calls     int
	lastMsgs  []Message
}

func (f *fakeCompleter) Complete(_ context.Context, _ string, messages []Message) (string, error) {
	f.lastMsgs = messages
	r := f.responses[f.calls]
	f.calls++
	return r, nil
}

func testCatalog() *Catalog {
	return &Catalog{
		Biomes: map[string][]string{
			"temperate_forest": {"base_ground", "ground", "mountain", "ruins", "water"},
			"desert":           {"base_ground", "ground", "mountain", "water"},
		},
		POINames: []string{"altar", "broken_pillar"},
	}
}

const validIR = `{
  "map": {"width": 40, "length": 40, "name": "t", "biome": "temperate_forest"},
  "zones": [
    {"id": "clearing", "anchor": {"x": 20, "y": 20}, "relative_size": 0.6, "elevation": "flat"},
    {"id": "pond", "anchor": {"x": 20, "y": 30}, "relative_size": 0.2, "elevation": "depression", "relief_override": "water"}
  ]
}`

func TestInterpretFirstTry(t *testing.T) {
	fc := &fakeCompleter{responses: []string{validIR}}
	res, err := NewInterpreter(fc, testCatalog()).Interpret(context.Background(), "une clairière avec une mare", Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.IR.Map.Biome != "temperate_forest" || len(res.IR.Zones) != 2 {
		t.Fatalf("unexpected IR: %+v", res.IR.Map)
	}
	if len(res.Attempts) != 1 {
		t.Errorf("expected 1 attempt, got %d", len(res.Attempts))
	}
}

func TestInterpretStripsCodeFence(t *testing.T) {
	fenced := "```json\n" + validIR + "\n```"
	fc := &fakeCompleter{responses: []string{fenced}}
	res, err := NewInterpreter(fc, testCatalog()).Interpret(context.Background(), "x", Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.IR == nil {
		t.Fatal("expected IR from fenced response")
	}
}

func TestInterpretRetriesThenSucceeds(t *testing.T) {
	fc := &fakeCompleter{responses: []string{"sorry, here is nothing useful", validIR}}
	res, err := NewInterpreter(fc, testCatalog()).Interpret(context.Background(), "x", Options{MaxRetries: 2})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Attempts) != 2 {
		t.Fatalf("expected 2 attempts, got %d", len(res.Attempts))
	}
	if res.Attempts[0].Error == "" {
		t.Error("first attempt should have recorded an error")
	}
	// The retry conversation should include the assistant's bad output and a
	// corrective user turn.
	if len(fc.lastMsgs) != 3 {
		t.Errorf("expected 3 messages on retry, got %d", len(fc.lastMsgs))
	}
}

func TestInterpretRejectsUnknownRelief(t *testing.T) {
	bad := `{"map":{"width":30,"length":30,"name":"t","biome":"desert"},
	         "zones":[{"id":"z","anchor":{"x":15,"y":15},"relative_size":0.9,"relief_override":"ruins"}]}`
	fc := &fakeCompleter{responses: []string{bad, validIR}}
	res, err := NewInterpreter(fc, testCatalog()).Interpret(context.Background(), "x", Options{MaxRetries: 1})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(res.Attempts[0].Error, "relief_override") {
		t.Errorf("expected relief_override rejection, got %q", res.Attempts[0].Error)
	}
}

func TestInterpretFailsAfterRetries(t *testing.T) {
	fc := &fakeCompleter{responses: []string{"junk", "still junk", "no json here"}}
	_, err := NewInterpreter(fc, testCatalog()).Interpret(context.Background(), "x", Options{MaxRetries: 2})
	if err == nil {
		t.Fatal("expected failure after exhausting retries")
	}
}

func TestForcedBiomeEnforced(t *testing.T) {
	// Model returns temperate_forest but desert was forced -> rejected, then it
	// complies.
	desert := `{"map":{"width":30,"length":30,"name":"t","biome":"desert"},
	            "zones":[{"id":"z","anchor":{"x":15,"y":15},"relative_size":0.9,"elevation":"flat"}]}`
	fc := &fakeCompleter{responses: []string{validIR, desert}}
	res, err := NewInterpreter(fc, testCatalog()).Interpret(context.Background(), "x",
		Options{ForcedBiome: "desert", MaxRetries: 1})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.IR.Map.Biome != "desert" {
		t.Errorf("expected desert, got %q", res.IR.Map.Biome)
	}
	if !strings.Contains(res.Attempts[0].Error, "desert") {
		t.Errorf("expected forced-biome rejection, got %q", res.Attempts[0].Error)
	}
}

func TestExtractJSON(t *testing.T) {
	cases := map[string]string{
		`{"a":1}`:                 `{"a":1}`,
		"prefix {\"a\":1} suffix": `{"a":1}`,
		"```json\n{\"a\":1}\n```": `{"a":1}`,
		"```\n{\"a\":1}\n```":     `{"a":1}`,
		"no braces here":          "",
	}
	for in, want := range cases {
		if got := extractJSON(in); got != want {
			t.Errorf("extractJSON(%q) = %q, want %q", in, got, want)
		}
	}
}
