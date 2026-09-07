package nl

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kirkanoskun/antoine-talespire-map-generator/internal/ir"
)

// Message is one turn in the conversation with the model.
type Message struct {
	Role string // "user" or "assistant"
	Text string
}

// Completer abstracts the model call so the interpreter can be tested offline.
// It receives a system prompt and the conversation so far, and returns the
// model's raw text response.
type Completer interface {
	Complete(ctx context.Context, system string, messages []Message) (string, error)
}

// Options tune a single interpretation.
type Options struct {
	Width       int    // default 50
	Length      int    // default 50
	Name        string // optional map name hint
	ForcedBiome string // optional: force the map biome
	MaxRetries  int    // additional attempts after the first (default 2)
}

// Attempt records one model response and why it was accepted or rejected.
type Attempt struct {
	Raw   string // the model's raw output
	Error string // validation error, empty if this attempt succeeded
}

// Result is the outcome of Interpret.
type Result struct {
	IR       *ir.IR
	Attempts []Attempt // every attempt in order; last one succeeded
}

// Interpreter turns descriptions into validated IR via a Completer.
type Interpreter struct {
	completer Completer
	catalog   *Catalog
}

// NewInterpreter builds an interpreter over the given completer and catalog.
func NewInterpreter(completer Completer, catalog *Catalog) *Interpreter {
	return &Interpreter{completer: completer, catalog: catalog}
}

// Interpret runs the description → IR pipeline: it prompts the model, validates
// the output strictly (schema + biome/relief coherence), and on failure feeds
// the exact error back and retries up to opts.MaxRetries additional times.
func (in *Interpreter) Interpret(ctx context.Context, description string, opts Options) (*Result, error) {
	if strings.TrimSpace(description) == "" {
		return nil, fmt.Errorf("description is empty")
	}
	if opts.Width <= 0 {
		opts.Width = 50
	}
	if opts.Length <= 0 {
		opts.Length = 50
	}
	if opts.MaxRetries < 0 {
		opts.MaxRetries = 0
	}
	if opts.ForcedBiome != "" && in.catalog.Biomes[opts.ForcedBiome] == nil {
		return nil, fmt.Errorf("forced biome %q is not in the catalogue", opts.ForcedBiome)
	}

	messages := []Message{{
		Role: "user",
		Text: userPrompt(description, opts.Width, opts.Length, opts.Name, opts.ForcedBiome),
	}}
	return in.run(ctx, messages, opts)
}

// Adjust applies a conversational follow-up ("move the pond further south") to
// an existing IR. It hands the model the current IR plus the instruction and
// asks for the full updated IR, then validates and retries exactly like
// Interpret. This is the in-memory iteration loop of the brief (5.8): a targeted
// edit of the current IR rather than a fresh generation.
func (in *Interpreter) Adjust(ctx context.Context, current *ir.IR, message string, opts Options) (*Result, error) {
	if current == nil {
		return nil, fmt.Errorf("no current IR to adjust")
	}
	if strings.TrimSpace(message) == "" {
		return nil, fmt.Errorf("adjustment message is empty")
	}
	if opts.Width <= 0 {
		opts.Width = current.Map.Width
	}
	if opts.Length <= 0 {
		opts.Length = current.Map.Length
	}
	if opts.MaxRetries < 0 {
		opts.MaxRetries = 0
	}

	currentJSON, err := json.MarshalIndent(current, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshalling current IR: %w", err)
	}
	messages := []Message{{Role: "user", Text: adjustPrompt(string(currentJSON), message)}}
	return in.run(ctx, messages, opts)
}

// run executes the validate-and-retry loop over a seeded conversation.
func (in *Interpreter) run(ctx context.Context, messages []Message, opts Options) (*Result, error) {
	// Non-interactive: the API path is a single-shot call whose response must be
	// JSON for the validate loop, so it never asks clarifying questions.
	system := in.catalog.systemPrompt(false)
	res := &Result{}
	for attempt := 0; attempt <= opts.MaxRetries; attempt++ {
		raw, err := in.completer.Complete(ctx, system, messages)
		if err != nil {
			return res, fmt.Errorf("model call failed on attempt %d: %w", attempt+1, err)
		}

		doc, verr := in.validate(raw, opts)
		res.Attempts = append(res.Attempts, Attempt{Raw: raw, Error: errString(verr)})
		if verr == nil {
			res.IR = doc
			return res, nil
		}

		// Feed the failure back and try again.
		messages = append(messages,
			Message{Role: "assistant", Text: raw},
			Message{Role: "user", Text: fmt.Sprintf(
				"That response was rejected: %s\nReturn a corrected JSON document only — no prose, no code fences.", verr)},
		)
	}

	return res, fmt.Errorf("failed to obtain a valid IR after %d attempt(s); last error: %s",
		len(res.Attempts), res.Attempts[len(res.Attempts)-1].Error)
}

// validate parses and structurally validates the raw model output, then checks
// biome/relief coherence against the catalogue.
func (in *Interpreter) validate(raw string, opts Options) (*ir.IR, error) {
	jsonText := extractJSON(raw)
	if jsonText == "" {
		return nil, fmt.Errorf("no JSON object found in the response")
	}
	doc, err := ir.Parse([]byte(jsonText))
	if err != nil {
		return nil, err
	}
	if opts.ForcedBiome != "" && doc.Map.Biome != opts.ForcedBiome {
		return nil, fmt.Errorf("map.biome must be %q as instructed, got %q", opts.ForcedBiome, doc.Map.Biome)
	}
	// Config-aware relief coherence (the generator enforces this too, but
	// catching it here lets the retry loop fix it).
	if in.catalog.Biomes[doc.Map.Biome] == nil {
		return nil, fmt.Errorf("map.biome %q is not an available biome", doc.Map.Biome)
	}
	for i := range doc.Zones {
		z := &doc.Zones[i]
		if z.ReliefOverride != "" && !in.catalog.reliefKnown(doc.Map.Biome, z.ReliefOverride) {
			return nil, fmt.Errorf("zone %q: relief_override %q is not a relief of biome %q (valid: %s)",
				z.ID, z.ReliefOverride, doc.Map.Biome, strings.Join(in.catalog.Biomes[doc.Map.Biome], ", "))
		}
	}
	return doc, nil
}

// extractJSON pulls the JSON object out of a model response, tolerating stray
// prose or ```json fences even though the prompt forbids them.
func extractJSON(raw string) string {
	s := strings.TrimSpace(raw)
	// Strip a leading ```json / ``` fence and its closing counterpart.
	if strings.HasPrefix(s, "```") {
		if nl := strings.IndexByte(s, '\n'); nl != -1 {
			s = s[nl+1:]
		}
		if end := strings.LastIndex(s, "```"); end != -1 {
			s = s[:end]
		}
		s = strings.TrimSpace(s)
	}
	// Fall back to the outermost {...} span.
	start := strings.IndexByte(s, '{')
	end := strings.LastIndexByte(s, '}')
	if start == -1 || end == -1 || end < start {
		return ""
	}
	return s[start : end+1]
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
