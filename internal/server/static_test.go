package server_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

// resourceRef matches what a page *loads* (stylesheets, scripts, images,
// frames), not the links a visitor may choose to follow.
var resourceRef = regexp.MustCompile(`<(?:link|script|img|iframe|source)\b[^>]*?\b(?:href|src)="([^"]+)"`)

var cssURL = regexp.MustCompile(`url\(([^)]+)\)`)

func get(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

// The public site promises (legal notice included) that no page asks a third
// party for anything — a Google Fonts link, say, hands every visitor's IP to
// Google. Every resource must be same-origin, and must exist.
func TestPagesLoadOnlyTheirOwnResources(t *testing.T) {
	h := newServer(t, nil)
	for _, page := range []string{"/", "/static/mentions-legales.html"} {
		rec := get(t, h, page)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status %d", page, rec.Code)
		}
		refs := resourceRef.FindAllStringSubmatch(rec.Body.String(), -1)
		if len(refs) == 0 {
			t.Fatalf("%s: no resources found — has the page structure changed?", page)
		}
		for _, m := range refs {
			ref := m[1]
			if !strings.HasPrefix(ref, "/") || strings.HasPrefix(ref, "//") {
				t.Errorf("%s loads %q from another origin", page, ref)
				continue
			}
			if code := get(t, h, ref).Code; code != http.StatusOK {
				t.Errorf("%s loads %q: status %d", page, ref, code)
			}
		}
	}
}

func TestSelfHostedFontsAreServed(t *testing.T) {
	h := newServer(t, nil)
	css := get(t, h, "/static/fonts/fonts.css").Body.String()
	urls := cssURL.FindAllStringSubmatch(css, -1)
	if len(urls) != 6 {
		t.Fatalf("fonts.css declares %d font files, want 6 (Cinzel 500/600/700, Work Sans 400/500/600)", len(urls))
	}
	for _, m := range urls {
		rec := get(t, h, m[1])
		if rec.Code != http.StatusOK || !bytes.HasPrefix(rec.Body.Bytes(), []byte("wOF2")) {
			t.Errorf("%s: status %d, not a WOFF2 font", m[1], rec.Code)
		}
	}
}
