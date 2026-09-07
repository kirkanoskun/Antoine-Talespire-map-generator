package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/kirkanoskun/antoine-talespire-map-generator/internal/generator"
	"github.com/kirkanoskun/antoine-talespire-map-generator/internal/nl"
	"github.com/kirkanoskun/antoine-talespire-map-generator/internal/server"
)

func repoPath(parts ...string) string {
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(file), "..", "..")
	return filepath.Join(append([]string{root}, parts...)...)
}

// stubCompleter returns a canned response, satisfying nl.Completer offline.
type stubCompleter struct{ resp string }

func (s stubCompleter) Complete(_ context.Context, _ string, _ []nl.Message) (string, error) {
	return s.resp, nil
}

const cannedIR = `{"map":{"width":40,"length":40,"name":"t","biome":"temperate_forest"},
	"zones":[{"id":"a","anchor":{"x":20,"y":20},"relative_size":0.8,"elevation":"flat"}]}`

func testCatalog() *nl.Catalog {
	return &nl.Catalog{
		Biomes:   map[string][]string{"temperate_forest": {"base_ground", "ground", "mountain", "ruins", "water"}},
		POINames: []string{"altar"},
	}
}

func newServer(t *testing.T, interp *nl.Interpreter) http.Handler {
	t.Helper()
	gen, err := generator.New(repoPath("configs", "biomes.json"), repoPath("configs", "props.json"))
	if err != nil {
		t.Fatalf("generator: %v", err)
	}
	return server.New(gen, server.Options{Interpreter: interp}).Handler()
}

func post(t *testing.T, h http.Handler, path, body string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var out map[string]any
	json.Unmarshal(rec.Body.Bytes(), &out)
	return rec, out
}

func TestIndexServed(t *testing.T) {
	h := newServer(t, nil)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("index status %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Le Cartographe") {
		t.Error("index page missing title")
	}
}

func TestStaticLogoServed(t *testing.T) {
	h := newServer(t, nil)
	for _, name := range []string{"logo.png", "emblem.png"} {
		req := httptest.NewRequest(http.MethodGet, "/static/"+name, nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != 200 {
			t.Fatalf("/static/%s status %d", name, rec.Code)
		}
		if !bytes.HasPrefix(rec.Body.Bytes(), []byte("\x89PNG")) {
			t.Errorf("/static/%s is not a PNG", name)
		}
	}
}

func TestGenerateFromIRThenPreview(t *testing.T) {
	h := newServer(t, nil)
	rec, out := post(t, h, "/api/generate", `{"ir":`+cannedIR+`}`)
	if rec.Code != 200 {
		t.Fatalf("generate status %d: %s", rec.Code, rec.Body.String())
	}
	if out["session"] == "" || out["code"] == "" {
		t.Fatalf("missing session/code in response: %v", out)
	}
	previewURL, _ := out["preview_url"].(string)
	if previewURL == "" {
		t.Fatal("no preview_url")
	}

	// The preview must render as a PNG.
	req := httptest.NewRequest(http.MethodGet, previewURL, nil)
	prec := httptest.NewRecorder()
	h.ServeHTTP(prec, req)
	if prec.Code != 200 {
		t.Fatalf("preview status %d", prec.Code)
	}
	if ct := prec.Header().Get("Content-Type"); ct != "image/png" {
		t.Errorf("preview content-type %q", ct)
	}
	if !bytes.HasPrefix(prec.Body.Bytes(), []byte("\x89PNG")) {
		t.Error("preview body is not a PNG")
	}
}

func TestGenerateRejectsInvalidIR(t *testing.T) {
	h := newServer(t, nil)
	rec, _ := post(t, h, "/api/generate", `{"ir":{"map":{"width":0,"length":10,"name":"x","biome":"desert"},"zones":[]}}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid IR, got %d", rec.Code)
	}
}

func TestSessionLRUEviction(t *testing.T) {
	h := newServer(t, nil)
	// The first session is never touched again, so it stays least-recently-used.
	_, out := post(t, h, "/api/generate", `{"ir":`+cannedIR+`}`)
	first, _ := out["session"].(string)
	if first == "" {
		t.Fatal("no session from first generate")
	}
	// Create well past the cap (maxSessions = 64) so the first is evicted.
	for i := 0; i < 80; i++ {
		post(t, h, "/api/generate", `{"ir":`+cannedIR+`}`)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/preview?session="+first, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected evicted session preview to 404, got %d", rec.Code)
	}
}

// TestConcurrentGenerateAndPreview exercises finish() writing a session while
// handlePreview reads it, guarding the fix that reads png under the lock. Run
// with -race to catch a regression.
func TestConcurrentGenerateAndPreview(t *testing.T) {
	h := newServer(t, nil)
	_, out := post(t, h, "/api/generate", `{"ir":`+cannedIR+`}`)
	sess, _ := out["session"].(string)
	if sess == "" {
		t.Fatal("no session")
	}
	var wg sync.WaitGroup
	for i := 0; i < 25; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodPost, "/api/generate",
				strings.NewReader(`{"session":"`+sess+`","ir":`+cannedIR+`}`))
			h.ServeHTTP(httptest.NewRecorder(), req)
		}()
		go func() {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodGet, "/api/preview?session="+sess, nil)
			h.ServeHTTP(httptest.NewRecorder(), req)
		}()
	}
	wg.Wait()
}

func TestDescribeAndAdjust(t *testing.T) {
	interp := nl.NewInterpreter(stubCompleter{resp: cannedIR}, testCatalog())
	h := newServer(t, interp)

	rec, out := post(t, h, "/api/describe", `{"description":"une clairière","width":40,"length":40}`)
	if rec.Code != 200 {
		t.Fatalf("describe status %d: %s", rec.Code, rec.Body.String())
	}
	sess, _ := out["session"].(string)
	if sess == "" {
		t.Fatal("describe returned no session")
	}

	// Adjust reuses the same session and stub response.
	arec, aout := post(t, h, "/api/adjust", `{"session":"`+sess+`","message":"agrandis la clairière"}`)
	if arec.Code != 200 {
		t.Fatalf("adjust status %d: %s", arec.Code, arec.Body.String())
	}
	if aout["session"] != sess {
		t.Errorf("adjust should keep the same session, got %v", aout["session"])
	}
}

func TestDescribeWithoutBackend(t *testing.T) {
	h := newServer(t, nil)
	rec, _ := post(t, h, "/api/describe", `{"description":"x"}`)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 without NL backend, got %d", rec.Code)
	}
}

func TestAdjustUnknownSession(t *testing.T) {
	interp := nl.NewInterpreter(stubCompleter{resp: cannedIR}, testCatalog())
	h := newServer(t, interp)
	rec, _ := post(t, h, "/api/adjust", `{"session":"nope","message":"x"}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown session, got %d", rec.Code)
	}
}
