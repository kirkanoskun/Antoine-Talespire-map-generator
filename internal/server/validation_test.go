package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kirkanoskun/antoine-talespire-map-generator/internal/generator"
	"github.com/kirkanoskun/antoine-talespire-map-generator/internal/ir"
)

func TestDecodeBodyStrictAndBounded(t *testing.T) {
	for _, tc := range []struct {
		name   string
		body   string
		status int
	}{
		{"valid", `{"name":"map"} `, 200},
		{"trailing object", `{"name":"map"} {}`, 400},
		{"trailing junk", `{"name":"map"} invalid`, 400},
		{"unknown field", `{"name":"map","typo":1}`, 400},
		{"oversized value", `{"name":"` + strings.Repeat("x", 1<<20) + `"}`, 413},
		{"oversized whitespace", `{"name":"map"}` + strings.Repeat(" ", 1<<20), 413},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var body struct {
				Name string `json:"name"`
			}
			r := httptest.NewRequest(http.MethodPost, "/api/generate", strings.NewReader(tc.body))
			w := httptest.NewRecorder()
			ok := decodeBody(w, r, &body)
			if ok != (tc.status == 200) || w.Code != tc.status {
				t.Fatalf("ok=%v status=%d body=%s", ok, w.Code, w.Body.String())
			}
		})
	}
}

func TestFinishPreservesPublishedSession(t *testing.T) {
	gen, err := generator.New("../../configs/biomes.json", "../../configs/props.json")
	if err != nil {
		t.Fatal(err)
	}
	s := New(gen, Options{})
	doc, err := ir.Parse([]byte(`{"map":{"width":4,"length":4,"biome":"temperate_forest"},"zones":[{"id":"a","anchor":{"x":2,"y":2},"relative_size":1}]}`))
	if err != nil {
		t.Fatal(err)
	}
	first := httptest.NewRecorder()
	s.finish(first, "test", doc, 0)
	if first.Code != 200 {
		t.Fatal(first.Body.String())
	}
	snapshot := s.sessions["test"]
	second := httptest.NewRecorder()
	s.finish(second, "test", doc, 0)
	if second.Code != 200 {
		t.Fatal(second.Body.String())
	}
	if snapshot == s.sessions["test"] || snapshot.version != 1 || s.sessions["test"].version != 2 {
		t.Fatal("updating a session must publish a new snapshot without mutating readers' snapshot")
	}
}

func TestRejectsCrossOriginQuit(t *testing.T) {
	// No generator is needed: rejected requests must not reach any handler.
	s := &Server{}
	h := s.Handler()
	for _, origin := range []string{"https://evil.example", "null", "http://localhost:9999"} {
		r := httptest.NewRequest(http.MethodPost, "http://localhost:8080/api/quit", nil)
		r.Header.Set("Origin", origin)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != http.StatusForbidden {
			t.Errorf("origin %q: got %d", origin, w.Code)
		}
	}
	for _, origin := range []string{"", "http://localhost:8080"} {
		r := httptest.NewRequest(http.MethodPost, "http://localhost:8080/api/quit", nil)
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != http.StatusNotFound {
			t.Errorf("legitimate request should reach quit handler, got %d", w.Code)
		}
	}
	r := httptest.NewRequest(http.MethodPost, "http://localhost:8080/api/quit", nil)
	r.Header.Set("Sec-Fetch-Site", "cross-site")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusForbidden {
		t.Errorf("cross-site request: got %d", w.Code)
	}
}
