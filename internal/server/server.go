// Package server exposes the generation pipeline over HTTP and serves the
// single-page web UI (brief section 4). It keeps each map's IR in memory so
// conversational adjustments ("move the pond further south") edit the existing
// IR rather than regenerating from scratch (brief 5.8).
//
// The pipeline packages are thin and composable, so the server drives the exact
// same code path as the CLIs.
package server

import (
	"bytes"
	"context"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/kirkanoskun/antoine-talespire-map-generator/internal/generator"
	"github.com/kirkanoskun/antoine-talespire-map-generator/internal/ir"
	"github.com/kirkanoskun/antoine-talespire-map-generator/internal/nl"
	"github.com/kirkanoskun/antoine-talespire-map-generator/internal/preview"
	"github.com/kirkanoskun/antoine-talespire-map-generator/internal/spatial"
)

//go:embed static/*
var staticFS embed.FS

// Server holds the generation dependencies and the live sessions.
type Server struct {
	gen         *generator.Generator
	catalog     *nl.Catalog     // for the keyless prompt-generation flow
	interp      *nl.Interpreter // may be nil if no NL backend is configured
	previewOpts preview.Options
	maxRetries  int
	seed        int64
	onQuit      func()

	mu       sync.Mutex
	sessions map[string]*session
	seq      int64 // monotonic access counter for LRU eviction (guarded by mu)
}

// maxSessions bounds the in-memory session cache so a long-lived server (the
// desktop app runs for a whole play session) does not grow without bound. Each
// session holds a rendered PNG plus the IR; the least-recently-used one is
// evicted once the cap is exceeded.
const maxSessions = 64

type session struct {
	doc      *ir.IR
	code     string
	png      []byte
	warnings []string
	version  int
	lastSeq  int64 // last access order, for LRU eviction
}

// Options configures a Server.
type Options struct {
	// Catalog powers the keyless "generate a prompt" flow (the mockup's default).
	Catalog      *nl.Catalog
	Interpreter  *nl.Interpreter
	PreviewScale int
	MaxRetries   int
	Seed         int64
	SliceSize    int
	// OnQuit, if set, is invoked when the UI's Quit button hits /api/quit.
	// The desktop launcher uses it to shut the local server down cleanly (there
	// is no terminal to Ctrl+C when the app is double-clicked).
	OnQuit func()
}

// New builds a server around a generator and (optionally) an NL interpreter.
func New(gen *generator.Generator, opts Options) *Server {
	scale := opts.PreviewScale
	if scale <= 0 {
		scale = 8
	}
	gen.SetSliceSize(opts.SliceSize)
	return &Server{
		gen:         gen,
		catalog:     opts.Catalog,
		interp:      opts.Interpreter,
		previewOpts: preview.Options{Scale: scale},
		maxRetries:  opts.MaxRetries,
		seed:        opts.Seed,
		onQuit:      opts.OnQuit,
		sessions:    map[string]*session{},
	}
}

// Handler returns the HTTP handler for the whole app.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/static/", http.FileServer(http.FS(staticFS)))
	mux.HandleFunc("/api/prompt", s.handlePrompt)
	mux.HandleFunc("/api/describe", s.handleDescribe)
	mux.HandleFunc("/api/adjust", s.handleAdjust)
	mux.HandleFunc("/api/generate", s.handleGenerate)
	mux.HandleFunc("/api/preview", s.handlePreview)
	mux.HandleFunc("/api/quit", s.handleQuit)
	mux.HandleFunc("/api/config", s.handleConfig)
	mux.HandleFunc("/", s.handleIndex)
	return mux
}

// handleConfig exposes UI-relevant capabilities: whether an NL backend is
// available (describe/adjust) and whether the Quit button applies.
func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]bool{
		"prompt_enabled": s.catalog != nil,
		"nl_enabled":     s.interp != nil,
		"quit_enabled":   s.onQuit != nil,
	})
}

// QuitEnabled reports whether the Quit button should be shown (a shutdown
// callback is wired). The UI hides the button otherwise (e.g. `go run`).
func (s *Server) QuitEnabled() bool { return s.onQuit != nil }

// handleQuit lets the desktop UI shut the local server down cleanly.
func (s *Server) handleQuit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	if s.onQuit == nil {
		writeError(w, http.StatusNotFound, "quit is not enabled")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "shutting down"})
	go s.onQuit()
}

type genResponse struct {
	Session    string          `json:"session"`
	IR         json.RawMessage `json:"ir"`
	Code       string          `json:"code"`
	Slices     [][]string      `json:"slices,omitempty"`
	Warnings   []string        `json:"warnings"`
	Attempts   int             `json:"attempts"`
	PreviewURL string          `json:"preview_url"`
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	data, err := staticFS.ReadFile("static/index.html")
	if err != nil {
		http.Error(w, "index not found", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(data)
}

// handlePrompt builds the pasteable AI prompt for a scene description — the
// keyless flow (the mockup's default). No Anthropic credential is involved.
func (s *Server) handlePrompt(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Description string `json:"description"`
		Width       int    `json:"width"`
		Length      int    `json:"length"`
		Biome       string `json:"biome"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	if s.catalog == nil {
		writeError(w, http.StatusServiceUnavailable, "catalogue unavailable")
		return
	}
	if strings.TrimSpace(req.Description) == "" {
		writeError(w, http.StatusBadRequest, "description is empty")
		return
	}
	prompt := s.catalog.BuildPrompt(req.Description, nl.Options{
		Width: req.Width, Length: req.Length, ForcedBiome: req.Biome,
	})
	writeJSON(w, http.StatusOK, map[string]string{"prompt": prompt})
}

// handleDescribe: natural language -> IR -> map, in a new session.
func (s *Server) handleDescribe(w http.ResponseWriter, r *http.Request) {
	if s.interp == nil {
		writeError(w, http.StatusServiceUnavailable, "natural-language backend is not configured (set ANTHROPIC_API_KEY)")
		return
	}
	var req struct {
		Description string `json:"description"`
		Width       int    `json:"width"`
		Length      int    `json:"length"`
		Biome       string `json:"biome"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 120*time.Second)
	defer cancel()

	res, err := s.interp.Interpret(ctx, req.Description, nl.Options{
		Width: req.Width, Length: req.Length, ForcedBiome: req.Biome, MaxRetries: s.maxRetries,
	})
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	s.finish(w, "", res.IR, len(res.Attempts))
}

// handleAdjust: conversational edit of an existing session's IR.
func (s *Server) handleAdjust(w http.ResponseWriter, r *http.Request) {
	if s.interp == nil {
		writeError(w, http.StatusServiceUnavailable, "natural-language backend is not configured (set ANTHROPIC_API_KEY)")
		return
	}
	var req struct {
		Session string `json:"session"`
		Message string `json:"message"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	s.mu.Lock()
	sess := s.sessions[req.Session]
	var doc *ir.IR
	if sess != nil {
		doc = sess.doc
		s.touchLocked(sess)
	}
	s.mu.Unlock()
	if sess == nil {
		writeError(w, http.StatusNotFound, "unknown session")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 120*time.Second)
	defer cancel()

	res, err := s.interp.Adjust(ctx, doc, req.Message, nl.Options{MaxRetries: s.maxRetries})
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	s.finish(w, req.Session, res.IR, len(res.Attempts))
}

// handleGenerate: accept an IR directly (no LLM) — used for the "apply edited
// IR" path and for scripting. Creates or updates the session.
func (s *Server) handleGenerate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Session string          `json:"session"`
		IR      json.RawMessage `json:"ir"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	doc, err := ir.Parse(req.IR)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.finish(w, req.Session, doc, 0)
}

func (s *Server) handlePreview(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("session")
	s.mu.Lock()
	var png []byte
	if sess := s.sessions[id]; sess != nil {
		png = sess.png
		s.touchLocked(sess)
	}
	s.mu.Unlock()
	if png == nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-store")
	w.Write(png)
}

// finish generates the map for doc, stores it under the session (creating one if
// id is empty), and writes the JSON response.
func (s *Server) finish(w http.ResponseWriter, id string, doc *ir.IR, attempts int) {
	if doc == nil {
		writeError(w, http.StatusInternalServerError, "no IR produced")
		return
	}
	mask := spatial.Resolve(doc)
	res, err := s.gen.Generate(doc, mask, s.seed)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	var buf bytes.Buffer
	if err := preview.WritePNG(&buf, doc, mask, res.Height, s.previewOpts); err != nil {
		writeError(w, http.StatusInternalServerError, "rendering preview: "+err.Error())
		return
	}

	// Mint the session id before taking the lock so a crypto/rand failure is
	// surfaced as an error instead of a silent zero-value collision.
	if id == "" {
		nid, err := newID()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "generating session id: "+err.Error())
			return
		}
		id = nid
	}

	s.mu.Lock()
	sess := s.sessions[id]
	if sess == nil {
		sess = &session{}
		s.sessions[id] = sess
	}
	sess.doc = doc
	sess.code = res.Code
	sess.png = buf.Bytes()
	sess.warnings = res.Warnings
	sess.version++
	version := sess.version
	s.touchLocked(sess)
	s.evictLRULocked()
	s.mu.Unlock()

	irJSON, _ := json.Marshal(doc)
	writeJSON(w, http.StatusOK, genResponse{
		Session:    id,
		IR:         irJSON,
		Code:       res.Code,
		Slices:     res.Slices,
		Warnings:   res.Warnings,
		Attempts:   attempts,
		PreviewURL: fmt.Sprintf("/api/preview?session=%s&v=%d", id, version),
	})
}

// --- helpers ---

// touchLocked marks sess as most-recently-used. Callers must hold s.mu.
func (s *Server) touchLocked(sess *session) {
	s.seq++
	sess.lastSeq = s.seq
}

// evictLRULocked drops the least-recently-used session while the cache is over
// the cap. Callers must hold s.mu.
func (s *Server) evictLRULocked() {
	for len(s.sessions) > maxSessions {
		var oldestID string
		var oldest int64
		first := true
		for id, sess := range s.sessions {
			if first || sess.lastSeq < oldest {
				oldest, oldestID, first = sess.lastSeq, id, false
			}
		}
		delete(s.sessions, oldestID)
	}
}

func decodeBody(w http.ResponseWriter, r *http.Request, v any) bool {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST required")
		return false
	}
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func newID() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}
