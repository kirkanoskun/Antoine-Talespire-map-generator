// Command web is the browser build of Le Cartographe, for the public website.
//
// It is the desktop app's HTTP API (internal/server) compiled to WebAssembly
// and called in-process: a small script in the page (web/cartographe.js)
// routes the unchanged UI's /api/* requests here instead of to the network. So
// the site needs no server at all — it is static files — and generates exactly
// the maps the desktop app does, from the same code.
//
// The site is public, so it runs in keyless mode: the "generate a prompt" flow
// and the paste-a-JSON flow work; the natural-language endpoints, which would
// spend the owner's Anthropic key on every visitor, are left unwired, and so is
// Quit, since there is no local process to stop.
//
// Build it with scripts/build-web.sh.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	talespire "github.com/kirkanoskun/antoine-talespire-map-generator"
	"github.com/kirkanoskun/antoine-talespire-map-generator/internal/generator"
	"github.com/kirkanoskun/antoine-talespire-map-generator/internal/nl"
	"github.com/kirkanoskun/antoine-talespire-map-generator/internal/server"
)

// newHandler builds the API with the desktop app's defaults (cmd/server's
// flag defaults), minus the AI backend and Quit.
func newHandler() (http.Handler, error) {
	gen, err := generator.NewFromJSON(talespire.BiomesJSON(), talespire.PropsJSON())
	if err != nil {
		return nil, fmt.Errorf("initialising generator: %w", err)
	}
	catalog, err := nl.NewCatalog(talespire.BiomesJSON(), generator.POIAliasNames())
	if err != nil {
		return nil, fmt.Errorf("loading catalog: %w", err)
	}
	srv := server.New(gen, server.Options{
		Catalog:      catalog,
		PreviewScale: 8,
		MaxRetries:   2,
		Seed:         1,
	})
	return srv.Handler(), nil
}

// response is one API reply, ready to hand back to JavaScript.
type response struct {
	status      int
	contentType string
	body        []byte
}

// call runs one request through the handler in-process.
func call(h http.Handler, method, target, body string) (res response) {
	// net/http recovers a panicking handler per request; a direct ServeHTTP call
	// does not, and in the browser a panic would kill the whole engine for the
	// rest of the visit. Answer this one request with an error instead.
	defer func() {
		if p := recover(); p != nil {
			res = errorResponse(http.StatusInternalServerError, fmt.Sprintf("internal error: %v", p))
		}
	}()

	req, err := http.NewRequest(method, target, strings.NewReader(body))
	if err != nil {
		return errorResponse(http.StatusBadRequest, err.Error())
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := &recorder{header: http.Header{}}
	h.ServeHTTP(rec, req)
	if rec.status == 0 {
		rec.status = http.StatusOK
	}
	return response{status: rec.status, contentType: rec.header.Get("Content-Type"), body: rec.body.Bytes()}
}

// errorResponse matches the server's own error shape, {"error": "..."}, which
// is what the UI reads.
func errorResponse(status int, msg string) response {
	body, _ := json.Marshal(map[string]string{"error": msg})
	return response{status: status, contentType: "application/json", body: body}
}

// recorder is a minimal http.ResponseWriter that buffers the reply.
type recorder struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func (r *recorder) Header() http.Header { return r.header }

func (r *recorder) WriteHeader(code int) {
	if r.status == 0 {
		r.status = code
	}
}

func (r *recorder) Write(b []byte) (int, error) {
	r.WriteHeader(http.StatusOK)
	return r.body.Write(b)
}
