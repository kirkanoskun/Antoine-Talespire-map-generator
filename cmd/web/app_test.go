package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"

	talespire "github.com/kirkanoskun/antoine-talespire-map-generator"
	"github.com/kirkanoskun/antoine-talespire-map-generator/internal/generator"
	"github.com/kirkanoskun/antoine-talespire-map-generator/internal/ir"
	"github.com/kirkanoskun/antoine-talespire-map-generator/internal/nl"
	"github.com/kirkanoskun/antoine-talespire-map-generator/internal/spatial"
)

func handler(t *testing.T) http.Handler {
	t.Helper()
	h, err := newHandler()
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func decode(t *testing.T, r response, into any) {
	t.Helper()
	if err := json.Unmarshal(r.body, into); err != nil {
		t.Fatalf("status %d, body %q: %v", r.status, r.body, err)
	}
}

// The public site must advertise the keyless flow only: no AI on the owner's
// key, no Quit button, and no building import (a public site must not accept
// uploads, and the browser has no disk to keep them on).
func TestConfigIsPublicKeylessMode(t *testing.T) {
	r := call(handler(t), "GET", "/api/config", "")
	var cfg map[string]bool
	decode(t, r, &cfg)
	if !cfg["prompt_enabled"] || cfg["nl_enabled"] || cfg["quit_enabled"] || cfg["prefabs_enabled"] {
		t.Errorf("config = %v, want prompt only", cfg)
	}
}

func TestPrivateFeaturesAreUnavailable(t *testing.T) {
	h := handler(t)
	if r := call(h, "POST", "/api/describe", `{"description":"une forêt"}`); r.status != http.StatusServiceUnavailable {
		t.Errorf("/api/describe = %d, want 503", r.status)
	}
	if r := call(h, "POST", "/api/quit", ""); r.status != http.StatusNotFound {
		t.Errorf("/api/quit = %d, want 404", r.status)
	}
	if r := call(h, "POST", "/api/prefabs/import", `{"id":"x","name":"x","code":"x"}`); r.status != http.StatusServiceUnavailable {
		t.Errorf("/api/prefabs/import = %d, want 503", r.status)
	}
}

func TestPromptFlow(t *testing.T) {
	r := call(handler(t), "POST", "/api/prompt", `{"description":"une clairière au bord d'un étang","width":40,"length":40}`)
	var out struct{ Prompt string }
	decode(t, r, &out)
	if r.status != http.StatusOK || !strings.Contains(out.Prompt, "clairière") {
		t.Fatalf("status %d, prompt %.80q", r.status, out.Prompt)
	}
}

// The full UI round trip: paste an IR, get a code and a preview image.
func TestGenerateAndPreview(t *testing.T) {
	h := handler(t)
	doc, err := os.ReadFile("../../testdata/castle.json")
	if err != nil {
		t.Fatal(err)
	}
	r := call(h, "POST", "/api/generate", `{"ir":`+string(doc)+`}`)
	var out struct {
		Code       string `json:"code"`
		PreviewURL string `json:"preview_url"`
	}
	decode(t, r, &out)
	if r.status != http.StatusOK || out.Code == "" || !strings.HasPrefix(out.PreviewURL, "/api/preview?") {
		t.Fatalf("status %d, code %d chars, preview %q", r.status, len(out.Code), out.PreviewURL)
	}

	img := call(h, "GET", out.PreviewURL, "")
	if img.status != http.StatusOK || img.contentType != "image/png" || !bytes.HasPrefix(img.body, []byte("\x89PNG")) {
		t.Fatalf("preview: status %d, type %q, %d bytes", img.status, img.contentType, len(img.body))
	}
}

// Visitors paste the AI assistant's whole reply, prose and ```json fence
// included; the site must find the map in it and explain the colours it drew.
func TestPastedReplyIsAccepted(t *testing.T) {
	doc, err := os.ReadFile("../../testdata/castle.json")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]string{"text": "Voici la carte :\n```json\n" + string(doc) + "\n```\nBonne partie !"})
	r := call(handler(t), "POST", "/api/generate", string(body))
	var out struct {
		Code   string `json:"code"`
		Legend []struct{ Key, Color string }
	}
	decode(t, r, &out)
	if r.status != http.StatusOK || out.Code == "" || len(out.Legend) == 0 {
		t.Fatalf("status %d, code %d chars, legend %v", r.status, len(out.Code), out.Legend)
	}
}

// While the engine loads, web/cartographe.js assembles the prompt itself from
// the head the build writes (cmd/web/prompthead) plus these exact scene lines.
// This holds the engine to that contract; the browser journey checks the
// JavaScript side against the engine.
func TestPromptIsHeadPlusSceneLines(t *testing.T) {
	catalog, err := nl.NewCatalog(talespire.BiomesJSON(), generator.POIAliasNames())
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ body, scene string }{
		{`{"description":"  une clairière  ","width":40,"length":30,"biome":""}`,
			"Map: width=40, length=30\n\nDescription:\nune clairière"},
		{`{"description":"un désert","width":0,"length":0,"biome":"desert"}`,
			"Map: width=50, length=50\nUse this biome for the map (do not choose another): desert\n\nDescription:\nun désert"},
	} {
		r := call(handler(t), "POST", "/api/prompt", c.body)
		var out struct{ Prompt string }
		decode(t, r, &out)
		if want := catalog.PromptHead() + c.scene; out.Prompt != want {
			t.Errorf("%s: the engine's prompt is not the head plus the scene lines the website writes", c.body)
		}
	}
}

// The site and the desktop app must produce the same TaleSpire code for the
// same IR: one is the other compiled for the browser, with catalogues read from
// memory instead of disk.
func TestWebMatchesDesktopOutput(t *testing.T) {
	raw, err := os.ReadFile("../../testdata/castle.json")
	if err != nil {
		t.Fatal(err)
	}
	doc, err := ir.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	desktop, err := generator.New("../../configs/biomes.json", "../../configs/props.json")
	if err != nil {
		t.Fatal(err)
	}
	want, err := desktop.Generate(doc, spatial.Resolve(doc), 1)
	if err != nil {
		t.Fatal(err)
	}

	r := call(handler(t), "POST", "/api/generate", `{"ir":`+string(raw)+`}`)
	var out struct{ Code string }
	decode(t, r, &out)
	if out.Code != want.Code {
		t.Errorf("web code (%d chars) differs from desktop code (%d chars)", len(out.Code), len(want.Code))
	}
}

func TestErrorsReachTheUI(t *testing.T) {
	r := call(handler(t), "POST", "/api/generate", `{"ir":{"map":{"width":10}}}`)
	var out struct{ Error string }
	decode(t, r, &out)
	if r.status != http.StatusBadRequest || out.Error == "" {
		t.Errorf("status %d, error %q — the UI shows data.error to the user", r.status, out.Error)
	}
}

// A panicking handler must cost one request, not the whole engine.
func TestPanicBecomesAnError(t *testing.T) {
	boom := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("boom") })
	r := call(boom, "GET", "/api/x", "")
	var out struct{ Error string }
	decode(t, r, &out)
	if r.status != http.StatusInternalServerError || !strings.Contains(out.Error, "boom") {
		t.Errorf("status %d, error %q", r.status, out.Error)
	}
}
