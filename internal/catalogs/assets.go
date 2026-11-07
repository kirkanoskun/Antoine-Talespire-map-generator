package catalogs

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
)

// Asset is one entry of the full TaleSpire catalogue (configs/assets.json),
// imported from the official packs' index.json files by cmd/import-index.
//
// Sizes and centres are in tiles, in the index's axis order turned to the
// map's: [x, z, y] where x is east, z is south (the map's y) and y is up.
type Asset struct {
	ID        string     `json:"id"`   // base64 of 2 zero bytes + the UUID in .NET byte order (props.json format)
	UUID      string     `json:"uuid"` // the index's own id
	Slug      string     `json:"slug"` // unique, what an IR names in a point of interest
	Name      string     `json:"name"`
	Kind      string     `json:"kind"` // "tile" or "prop"
	Pack      string     `json:"pack"`
	Folder    string     `json:"folder"`
	Group     string     `json:"group"`
	Tags      []string   `json:"tags"`
	Size      [3]float64 `json:"size"`      // [x, z, y] in tiles (2 x m_Extent)
	Center    [3]float64 `json:"center"`    // [x, z, y] of the bounds, relative to the asset's origin
	Footprint [2]int     `json:"footprint"` // [w, d] tiles reserved on the map
}

// footprintTolerance absorbs the few centimetres by which meshes overhang
// their nominal tile: a 2.03-tile fountain still takes 2x2 tiles.
const footprintTolerance = 0.15

// LayoutID returns the 16-byte asset id a Slab layout carries: the UUID in
// .NET byte order, i.e. ID without its two leading zero bytes.
func (a *Asset) LayoutID() ([16]byte, error) {
	var out [16]byte
	raw, err := base64.StdEncoding.DecodeString(a.ID)
	if err != nil || len(raw) != 18 {
		return out, fmt.Errorf("asset %q: malformed id %q", a.Slug, a.ID)
	}
	copy(out[:], raw[2:])
	return out, nil
}

// AssetCatalog is the parsed configs/assets.json, indexed by slug and UUID.
type AssetCatalog struct {
	list   []Asset
	bySlug map[string]*Asset
	byUUID map[string]*Asset
	byID   map[string]*Asset
}

// ParseAssets reads configs/assets.json.
func ParseAssets(data []byte) (*AssetCatalog, error) {
	var list []Asset
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, fmt.Errorf("parsing assets: %w", err)
	}
	c := &AssetCatalog{list: list, bySlug: map[string]*Asset{}, byUUID: map[string]*Asset{}, byID: map[string]*Asset{}}
	for i := range c.list {
		a := &c.list[i]
		if a.Slug == "" {
			return nil, fmt.Errorf("parsing assets: entry %s has no slug", a.UUID)
		}
		if _, dup := c.bySlug[a.Slug]; dup {
			return nil, fmt.Errorf("parsing assets: duplicate slug %q", a.Slug)
		}
		c.bySlug[a.Slug] = a
		c.byUUID[a.UUID] = a
		c.byID[a.ID] = a
	}
	return c, nil
}

// BySlug returns the asset an IR names.
func (c *AssetCatalog) BySlug(slug string) (*Asset, bool) { a, ok := c.bySlug[slug]; return a, ok }

// ByUUID returns the asset with the index's UUID.
func (c *AssetCatalog) ByUUID(uuid string) (*Asset, bool) { a, ok := c.byUUID[uuid]; return a, ok }

// ByID returns the asset with a props.json-style base64 id.
func (c *AssetCatalog) ByID(id string) (*Asset, bool) { a, ok := c.byID[id]; return a, ok }

// Len is the number of assets.
func (c *AssetCatalog) Len() int { return len(c.list) }

// indexFile is the part of a TaleWeaver pack index.json the catalogue keeps.
// Assets (sub-meshes) and Icon are the game's business and are not read.
type indexFile struct {
	Name  string       `json:"Name"`
	Tiles []indexEntry `json:"Tiles"`
	Props []indexEntry `json:"Props"`
}

type indexEntry struct {
	ID           string   `json:"Id"`
	Name         string   `json:"Name"`
	IsDeprecated bool     `json:"IsDeprecated"`
	GroupTag     string   `json:"GroupTag"`
	Tags         []string `json:"Tags"`
	Folder       string   `json:"Folder"`
	Bounds       struct {
		Center vec3 `json:"m_Center"`
		Extent vec3 `json:"m_Extent"`
	} `json:"ColliderBoundsBound"`
}

type vec3 struct{ X, Y, Z float64 }

// ImportIndexes builds the catalogue from TaleWeaver index.json files: every
// non-deprecated tile and prop, with a unique slug, sorted by slug. The same
// inputs always give the same catalogue.
func ImportIndexes(indexes ...[]byte) ([]Asset, error) {
	var assets []Asset
	seen := map[string]bool{}
	for _, data := range indexes {
		var idx indexFile
		if err := json.Unmarshal(data, &idx); err != nil {
			return nil, fmt.Errorf("parsing index: %w", err)
		}
		for _, group := range []struct {
			kind    string
			entries []indexEntry
		}{{"tile", idx.Tiles}, {"prop", idx.Props}} {
			for _, e := range group.entries {
				if e.IsDeprecated {
					continue
				}
				if seen[e.ID] {
					return nil, fmt.Errorf("asset %s appears twice in the indexes", e.ID)
				}
				seen[e.ID] = true
				a, err := newAsset(e, group.kind, idx.Name)
				if err != nil {
					return nil, err
				}
				assets = append(assets, a)
			}
		}
	}
	if err := assignSlugs(assets); err != nil {
		return nil, err
	}
	sort.Slice(assets, func(i, j int) bool { return assets[i].Slug < assets[j].Slug })
	return assets, nil
}

func newAsset(e indexEntry, kind, pack string) (Asset, error) {
	id, err := dotNetID(e.ID)
	if err != nil {
		return Asset{}, err
	}
	tags := e.Tags
	if tags == nil {
		tags = []string{}
	}
	c, x := e.Bounds.Center, e.Bounds.Extent
	size := [3]float64{round3(2 * x.X), round3(2 * x.Z), round3(2 * x.Y)}
	return Asset{
		ID: id, UUID: strings.ToLower(e.ID), Name: e.Name, Kind: kind, Pack: pack,
		Folder: e.Folder, Group: e.GroupTag, Tags: tags,
		Size:      size,
		Center:    [3]float64{round3(c.X), round3(c.Z), round3(c.Y)},
		Footprint: [2]int{footprint(size[0]), footprint(size[1])},
	}, nil
}

// footprint is the number of tiles an object spans along one axis.
func footprint(size float64) int {
	return max(1, int(math.Ceil(size-footprintTolerance)))
}

// dotNetID encodes a UUID the way props.json and Slabs do: two zero bytes, then
// the UUID with its first three fields little-endian (Guid.ToByteArray).
func dotNetID(uuid string) (string, error) {
	b, err := hex.DecodeString(strings.ReplaceAll(uuid, "-", ""))
	if err != nil || len(b) != 16 {
		return "", fmt.Errorf("malformed asset uuid %q", uuid)
	}
	le := []byte{b[3], b[2], b[1], b[0], b[5], b[4], b[7], b[6]}
	le = append(le, b[8:]...)
	return base64.StdEncoding.EncodeToString(append([]byte{0, 0}, le...)), nil
}

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

// Slugify lowercases a name and turns every run of other characters into one
// underscore.
func Slugify(name string) string {
	return strings.Trim(nonSlug.ReplaceAllString(strings.ToLower(name), "_"), "_")
}

// assignSlugs gives every asset a unique slug: its name; for names shared by
// several assets, the name plus the folder; and if that still collides (variants
// in one folder), plus the first characters of the UUID. A nameless asset is
// named after its UUID.
func assignSlugs(assets []Asset) error {
	base := make([]string, len(assets))
	count := map[string]int{}
	for i, a := range assets {
		base[i] = Slugify(a.Name)
		if base[i] == "" {
			base[i] = "asset_" + a.UUID[:8]
		}
		count[base[i]]++
	}
	second := make([]string, len(assets))
	count2 := map[string]int{}
	for i, a := range assets {
		second[i] = base[i]
		if count[base[i]] > 1 {
			if folder := Slugify(a.Folder); folder != "" {
				second[i] += "_" + folder
			}
		}
		count2[second[i]]++
	}
	taken := map[string]bool{}
	for i := range assets {
		s := second[i]
		if count2[s] > 1 {
			s += "_" + assets[i].UUID[:4]
		}
		if taken[s] {
			return fmt.Errorf("slug %q is not unique even with its UUID prefix", s)
		}
		taken[s] = true
		assets[i].Slug = s
	}
	return nil
}

// EncodeAssets writes the catalogue as JSON, one asset per line, so a game
// update shows as a readable diff.
func EncodeAssets(assets []Asset) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteString("[\n")
	for i := range assets {
		line, err := json.Marshal(&assets[i])
		if err != nil {
			return nil, err
		}
		buf.Write(line)
		if i < len(assets)-1 {
			buf.WriteByte(',')
		}
		buf.WriteByte('\n')
	}
	buf.WriteString("]\n")
	return buf.Bytes(), nil
}

// round3 rounds to a thousandth of a tile, so the file is stable and small.
func round3(v float64) float64 {
	r := math.Round(v*1000) / 1000
	if r == 0 {
		return 0 // no "-0" in the file
	}
	return r
}
