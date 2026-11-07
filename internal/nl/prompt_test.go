package nl

import (
	"os"
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
	if PromptVersion != "1.05" {
		t.Errorf("PromptVersion = %q, want 1.05", PromptVersion)
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

// The scene themes reach the prompt with their slugs and footprints, and the
// whole catalogue does not.
func TestPromptListsSceneThemes(t *testing.T) {
	c, err := NewCatalog(mustRead(t, "../../configs/biomes.json"), []string{"altar"})
	if err != nil {
		t.Fatal(err)
	}
	p := c.PromptHead()
	for _, want := range []string{
		"# Scene themes",
		"- cimetiere (Cimetière): gravestone_01 [1x1] tombe",
		"fountain_01 [2x2] fontaine, pièce centrale",
		"- marais (Marais):",
		"- village_abandonne (Village abandonné):",
		"NORTH-WEST corner of its footprint",
		"fences and walls run east-west",
	} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt is missing %q", want)
		}
	}
	if strings.Contains(p, "aberration_decor01") {
		t.Error("the whole catalogue must not be in the prompt")
	}
	t.Logf("prompt head: %d bytes", len(p))
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
