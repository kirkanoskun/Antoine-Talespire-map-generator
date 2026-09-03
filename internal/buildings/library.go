// Package buildings is a small on-disk library of reusable TaleSpire buildings.
//
// A building is a community "Chimera" slab (from Tales Tavern, TalesBazaar, or
// your own export) that has been decoded once, measured, and filed under an id.
// From then on a map can ask for it by name instead of juggling base64 blobs.
//
// Layout on disk:
//
//	<dir>/manifest.json   the index: ids, provenance, measurements
//	<dir>/<id>.slab       the slab code, one per building
//
// Provenance matters here: community builds carry per-creator licences (several
// are CC BY-NC), so every entry records where it came from and under what terms.
// The library directory is kept out of git by default for exactly that reason —
// it holds other people's work, not ours.
package buildings

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/kirkanoskun/antoine-talespire-map-generator/internal/chimera"
)

// ManifestName is the index file inside a library directory.
const ManifestName = "manifest.json"

// Entry describes one building: what it is, where it came from, and the
// measurements taken from its slab when it was added.
type Entry struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	File        string   `json:"file"` // slab code, relative to the library dir
	Tags        []string `json:"tags,omitempty"`
	Description string   `json:"description,omitempty"`

	// Provenance. Community builds are other people's work — record it.
	Source  string `json:"source,omitempty"`
	Author  string `json:"author,omitempty"`
	License string `json:"license,omitempty"`

	// Measured from the slab at add time.
	GroundZ     uint32  `json:"ground_z"` // rawZ of the level to seat on the terrain
	WidthTiles  int     `json:"width_tiles"`
	LengthTiles int     `json:"length_tiles"`
	HeightSteps float64 `json:"height_steps"`
	BuriedSteps float64 `json:"buried_steps"` // depth below GroundZ (cellar, footings)
	Assets      int     `json:"assets"`
	Placements  int     `json:"placements"`
	CodeChars   int     `json:"code_chars"`
	AddedAt     string  `json:"added_at"`
}

// Library is a building library rooted at a directory.
type Library struct {
	Dir     string
	Entries []Entry
}

// Open loads the library at dir, creating an empty one if it does not exist.
func Open(dir string) (*Library, error) {
	l := &Library{Dir: dir}
	raw, err := os.ReadFile(filepath.Join(dir, ManifestName))
	if os.IsNotExist(err) {
		return l, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading manifest: %w", err)
	}
	if err := json.Unmarshal(raw, &l.Entries); err != nil {
		return nil, fmt.Errorf("parsing manifest: %w", err)
	}
	return l, nil
}

// Get returns the entry with the given id.
func (l *Library) Get(id string) (*Entry, error) {
	for i := range l.Entries {
		if l.Entries[i].ID == id {
			return &l.Entries[i], nil
		}
	}
	return nil, fmt.Errorf("no building %q in %s (try `buildings list`)", id, l.Dir)
}

// Code returns the raw slab code for an entry.
func (l *Library) Code(e *Entry) (string, error) {
	raw, err := os.ReadFile(filepath.Join(l.Dir, e.File))
	if err != nil {
		return "", fmt.Errorf("reading slab for %q: %w", e.ID, err)
	}
	return strings.TrimSpace(string(raw)), nil
}

// Slab decodes an entry's slab.
func (l *Library) Slab(e *Entry) (*chimera.Slab, error) {
	code, err := l.Code(e)
	if err != nil {
		return nil, err
	}
	return chimera.Decode(code)
}

// AddOptions describes a building being filed.
type AddOptions struct {
	ID          string
	Name        string
	Code        string // the base64 slab code
	Tags        []string
	Description string
	Source      string
	Author      string
	License     string

	// GroundZ is the level to seat on the terrain. Leave nil to auto-detect the
	// busiest level, which for a building is almost always its ground floor.
	GroundZ *uint32

	// Replace allows overwriting an existing id.
	Replace bool
}

var idPattern = regexp.MustCompile(`^[a-z0-9]+(?:[-_][a-z0-9]+)*$`)

// accentFolds maps the accented letters common in French (and neighbours) to
// their plain forms, so "L'Auberge du Chêne" slugs to "l-auberge-du-chene"
// rather than losing the letter to a dash.
var accentFolds = map[rune]rune{
	'à': 'a', 'á': 'a', 'â': 'a', 'ä': 'a', 'ã': 'a', 'å': 'a',
	'ç': 'c',
	'è': 'e', 'é': 'e', 'ê': 'e', 'ë': 'e',
	'ì': 'i', 'í': 'i', 'î': 'i', 'ï': 'i',
	'ñ': 'n',
	'ò': 'o', 'ó': 'o', 'ô': 'o', 'ö': 'o', 'õ': 'o',
	'ù': 'u', 'ú': 'u', 'û': 'u', 'ü': 'u',
	'ý': 'y', 'ÿ': 'y',
}

// Slugify turns a name into a usable id.
func Slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	lastDash := true // avoid a leading dash
	for _, r := range s {
		if folded, ok := accentFolds[r]; ok {
			r = folded
		}
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		case r == 'œ':
			b.WriteString("oe")
			lastDash = false
		case r == 'æ':
			b.WriteString("ae")
			lastDash = false
		default:
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

// Add decodes and measures the slab, writes it into the library, and records
// the entry. The slab is validated here so a broken or truncated code is
// rejected at the door rather than halfway through building a map.
func (l *Library) Add(opts AddOptions) (*Entry, error) {
	if opts.ID == "" {
		opts.ID = Slugify(opts.Name)
	}
	if opts.ID == "" {
		return nil, fmt.Errorf("a building needs an id or a name to derive one from")
	}
	if !idPattern.MatchString(opts.ID) {
		return nil, fmt.Errorf("id %q must be lowercase letters, digits, '-' or '_'", opts.ID)
	}
	if _, err := l.Get(opts.ID); err == nil && !opts.Replace {
		return nil, fmt.Errorf("building %q already exists (pass -replace to overwrite)", opts.ID)
	}

	slab, err := chimera.Decode(opts.Code)
	if err != nil {
		return nil, fmt.Errorf("decoding slab: %w", err)
	}
	b := slab.Bounds()
	if b.Placements == 0 {
		return nil, fmt.Errorf("slab is empty")
	}

	ground := slab.BusiestLevel()
	if opts.GroundZ != nil {
		ground = *opts.GroundZ
	}
	if ground < b.MinZ || ground > b.MaxZ {
		return nil, fmt.Errorf("ground level %d is outside the slab's vertical range %d..%d", ground, b.MinZ, b.MaxZ)
	}

	name := opts.Name
	if name == "" {
		name = opts.ID
	}
	entry := Entry{
		ID:          opts.ID,
		Name:        name,
		File:        opts.ID + ".slab",
		Tags:        normaliseTags(opts.Tags),
		Description: opts.Description,
		Source:      opts.Source,
		Author:      opts.Author,
		License:     opts.License,
		GroundZ:     ground,
		WidthTiles:  b.WidthTiles(),
		LengthTiles: b.LengthTiles(),
		HeightSteps: b.HeightSteps(),
		BuriedSteps: float64(ground-b.MinZ) / 50.0,
		Assets:      len(slab.Assets),
		Placements:  b.Placements,
		CodeChars:   len(strings.TrimSpace(opts.Code)),
		AddedAt:     time.Now().UTC().Format(time.RFC3339),
	}

	if err := os.MkdirAll(l.Dir, 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(l.Dir, entry.File), []byte(strings.TrimSpace(opts.Code)+"\n"), 0o644); err != nil {
		return nil, fmt.Errorf("writing slab: %w", err)
	}

	replaced := false
	for i := range l.Entries {
		if l.Entries[i].ID == entry.ID {
			l.Entries[i] = entry
			replaced = true
			break
		}
	}
	if !replaced {
		l.Entries = append(l.Entries, entry)
	}
	if err := l.Save(); err != nil {
		return nil, err
	}
	return &entry, nil
}

// Remove deletes an entry and its slab file.
func (l *Library) Remove(id string) error {
	for i := range l.Entries {
		if l.Entries[i].ID != id {
			continue
		}
		_ = os.Remove(filepath.Join(l.Dir, l.Entries[i].File))
		l.Entries = append(l.Entries[:i], l.Entries[i+1:]...)
		return l.Save()
	}
	return fmt.Errorf("no building %q", id)
}

// Find returns entries matching a query against id, name, tags and description.
// An empty query returns everything.
func (l *Library) Find(query string) []Entry {
	q := strings.ToLower(strings.TrimSpace(query))
	var out []Entry
	for _, e := range l.Entries {
		if q == "" || strings.Contains(strings.ToLower(e.ID+" "+e.Name+" "+e.Description+" "+strings.Join(e.Tags, " ")), q) {
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Save writes the manifest, sorted by id so diffs stay readable.
func (l *Library) Save() error {
	if err := os.MkdirAll(l.Dir, 0o755); err != nil {
		return err
	}
	sort.Slice(l.Entries, func(i, j int) bool { return l.Entries[i].ID < l.Entries[j].ID })
	raw, err := json.MarshalIndent(l.Entries, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(l.Dir, ManifestName), append(raw, '\n'), 0o644)
}

func normaliseTags(tags []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, t := range tags {
		t = strings.ToLower(strings.TrimSpace(t))
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}
