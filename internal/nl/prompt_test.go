package nl

import (
	"strings"
	"testing"
)

// The keyless BuildPrompt (the "Générer le prompt" button) must ask the three
// clarifying questions before any JSON, and carry the prompt version.
func TestBuildPromptAsksClarifyingQuestionsAndVersion(t *testing.T) {
	p := testCatalog().BuildPrompt("une clairière", Options{Width: 40, Length: 40})

	for _, want := range []string{
		"version " + PromptVersion,
		"ask 3 clarifying questions",
		"Density and mood",
		"Focal point",
		"Circulation",
		"Wait for the user's answer",
		"skip them / to go ahead", // relaxed output contract
	} {
		if !strings.Contains(p, want) {
			t.Errorf("BuildPrompt missing %q", want)
		}
	}
	if PromptVersion != "1.03" {
		t.Errorf("PromptVersion = %q, want 1.03", PromptVersion)
	}
}

// The automated API path (Interpret) must NOT ask clarifying questions: it is a
// single-shot call whose reply has to be JSON for the validate/retry loop.
func TestAPISystemPromptHasNoClarifyingQuestions(t *testing.T) {
	api := testCatalog().systemPrompt(false)
	if strings.Contains(api, "clarifying questions") {
		t.Error("non-interactive systemPrompt should not ask clarifying questions")
	}
	if !strings.Contains(api, "Return ONLY the JSON document") {
		t.Error("non-interactive systemPrompt should keep the immediate output contract")
	}
}
