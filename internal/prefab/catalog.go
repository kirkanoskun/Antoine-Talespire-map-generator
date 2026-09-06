// Package prefab validates and stores reusable community Slabs and metadata.
package prefab

import (
 "bytes"
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "fmt"
 "io"
 "os"
 "path/filepath"
 "regexp"
 "sort"
 "strings"
 "sync"

 "github.com/kirkanoskun/antoine-talespire-map-generator/internal/slab"
)

// Entry is the portable on-disk format. Code is never sent to the language model.
// Footprint overrides are in map tiles, including the one-tile safety margin.
type Entry struct {
 ID string `json:"id"`
 Name string `json:"name"`
 Code string `json:"code"`
 Author string `json:"author,omitempty"`
 SourceURL string `json:"source_url,omitempty"`
 Description string `json:"description,omitempty"`
 Width int `json:"width,omitempty"`
 Length int `json:"length,omitempty"`
}

type Info struct {
 ID string `json:"id"`
 Name string `json:"name"`
 Author string `json:"author,omitempty"`
 SourceURL string `json:"source_url,omitempty"`
 Description string `json:"description,omitempty"`
 Width int `json:"width"`
 Length int `json:"length"`
 AssetCount int `json:"asset_count"`
 SHA256 string `json:"sha256"`
}

type Building struct {
 Info Info
 Slab *slab.Slab
 SpanX, SpanZ int
}

var validID = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

func Validate(e Entry) (*Building, error) {
 if !validID.MatchString(e.ID) { return nil, fmt.Errorf("id must use lowercase letters, digits and underscores, starting with a letter (max 64 characters)") }
 if strings.TrimSpace(e.Name) == "" || len(e.Name) > 160 { return nil, fmt.Errorf("name is required (max 160 bytes)") }
 if len(e.Author) > 200 || len(e.SourceURL) > 2000 || len(e.Description) > 2000 { return nil, fmt.Errorf("Slab metadata is too long") }
 s, err := slab.Decode(e.Code)
 if err != nil { return nil, err }
 if s.Count() == 0 { return nil, fmt.Errorf("Slab contains no assets") }
 minX, minY, minZ := slab.MaxCoordinate, slab.MaxCoordinate, slab.MaxCoordinate
 maxX, maxZ := 0, 0
 for _, l := range s.Layouts { for _, p := range l.Instances {
  minX = min(minX, p.X); minY = min(minY, p.Y); minZ = min(minZ, p.Z)
  maxX = max(maxX, p.X); maxZ = max(maxZ, p.Z)
 } }
 for i := range s.Layouts { for j := range s.Layouts[i].Instances {
  p := &s.Layouts[i].Instances[j]; p.X -= minX; p.Y -= minY; p.Z -= minZ
 } }
 spanX, spanZ := maxX-minX, maxZ-minZ
 width, length := (spanX+99)/100+2, (spanZ+99)/100+2
 if e.Width != 0 {
  if e.Width < width { return nil, fmt.Errorf("width must be at least %d tiles", width) }; width = e.Width
 }
 if e.Length != 0 {
  if e.Length < length { return nil, fmt.Errorf("length must be at least %d tiles", length) }; length = e.Length
 }
 if width > 200 || length > 200 { return nil, fmt.Errorf("building footprint exceeds the 200-tile map limit") }
 normalized, err := slab.Encode(s)
 if err != nil { return nil, err }
 hash := sha256.Sum256([]byte(normalized))
 return &Building{Info:Info{ID:e.ID, Name:e.Name, Author:e.Author, SourceURL:e.SourceURL, Description:e.Description, Width:width, Length:length, AssetCount:s.Count(), SHA256:hex.EncodeToString(hash[:])}, Slab:s, SpanX:spanX, SpanZ:spanZ}, nil
}

// Store owns validated immutable buildings. All returned Buildings are copies.
// The local catalogue is separate from built-ins; a local ID cannot shadow one.
type Store struct {
 mu sync.RWMutex
 path string
 entries []Entry
 buildings map[string]*Building
}

func Open(path string, builtins []byte) (*Store, error) {
 s := &Store{path:path, buildings:map[string]*Building{}}
 base, err := parseEntries(builtins)
 if err != nil { return nil, fmt.Errorf("built-in prefabs: %w", err) }
 local := []Entry{}
 if path != "" {
  f, err := os.Open(path)
  if err == nil {
   data, rerr := io.ReadAll(io.LimitReader(f, 16<<20+1)); f.Close()
   if rerr != nil { return nil, rerr }
   if len(data) > 16<<20 { return nil, fmt.Errorf("prefab catalogue exceeds 16 MiB") }
   local, err = parseEntries(data)
   if err != nil { return nil, err }
  } else if !os.IsNotExist(err) { return nil, err }
 }
 for _, list := range [][]Entry{base, local} { for _, e := range list {
  b, err := Validate(e)
  if err != nil { return nil, fmt.Errorf("prefab %q: %w", e.ID, err) }
  if _, exists := s.buildings[e.ID]; exists { return nil, fmt.Errorf("duplicate prefab id %q", e.ID) }
  s.buildings[e.ID] = b
 } }
 s.entries = local
 return s, nil
}

func parseEntries(data []byte) ([]Entry, error) {
 if len(data) == 0 { return nil, nil }
 d := json.NewDecoder(bytes.NewReader(data)); d.DisallowUnknownFields()
 var entries []Entry
 if err := d.Decode(&entries); err != nil { return nil, err }
 if err := d.Decode(new(any)); err != io.EOF { return nil, fmt.Errorf("expected a single prefab catalogue") }
 return entries, nil
}

func (s *Store) Add(e Entry) (Info, error) {
 b, err := Validate(e)
 if err != nil { return Info{}, err }
 s.mu.Lock(); defer s.mu.Unlock()
 if _, exists := s.buildings[e.ID]; exists { return Info{}, fmt.Errorf("prefab id %q already exists; choose another id", e.ID) }
 for _, existing := range s.buildings {
  if existing.Info.SHA256 == b.Info.SHA256 { return Info{}, fmt.Errorf("this Slab is already imported as %q", existing.Info.ID) }
 }
 entries := append(append([]Entry(nil), s.entries...), e)
 sort.Slice(entries, func(i,j int) bool { return entries[i].ID < entries[j].ID })
 data, err := json.MarshalIndent(entries, "", "  ")
 if err != nil { return Info{}, err }
 if len(data) > 16<<20 { return Info{}, fmt.Errorf("prefab catalogue exceeds 16 MiB") }
 if s.path != "" {
  if err := saveAtomic(s.path, append(data, '\n')); err != nil { return Info{}, err }
 }
 s.entries = entries; s.buildings[e.ID] = b
 return b.Info, nil
}

func saveAtomic(path string, data []byte) error {
 if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil { return err }
 f, err := os.CreateTemp(filepath.Dir(path), ".prefabs-*")
 if err != nil { return err }
 defer os.Remove(f.Name())
 if _, err := f.Write(data); err != nil { f.Close(); return err }
 if err := f.Sync(); err != nil { f.Close(); return err }
 if err := f.Close(); err != nil { return err }
 return os.Rename(f.Name(), path)
}

func (s *Store) List() []Info {
 if s == nil { return []Info{} }
 s.mu.RLock(); defer s.mu.RUnlock()
 out := make([]Info, 0, len(s.buildings))
 for _, b := range s.buildings { out = append(out, b.Info) }
 sort.Slice(out, func(i,j int) bool { return out[i].ID < out[j].ID })
 return out
}

func (s *Store) Get(id string) (*Building, bool) {
 if s == nil { return nil, false }
 s.mu.RLock(); defer s.mu.RUnlock()
 b, ok := s.buildings[id]
 if !ok { return nil, false }
 copyB := *b
 copyB.Slab = &slab.Slab{Layouts:make([]slab.Layout, len(b.Slab.Layouts))}
 for i, l := range b.Slab.Layouts {
  copyB.Slab.Layouts[i] = l
  copyB.Slab.Layouts[i].Instances = append([]slab.Instance(nil), l.Instances...)
 }
 return &copyB, true
}
