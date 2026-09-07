// Package ir defines the intermediate representation (IR) that sits between
// natural-language understanding and the deterministic generation engine.
//
// The IR is deliberately engine-independent: it captures a level designer's
// *intent* (zones, relative sizes, spatial anchors, points of interest) rather
// than a tile-by-tile grid. A separate spatial resolver (package spatial) turns
// these approximate intentions into an exact per-tile zone mask, and the
// generator (package generator) fills that mask with TaleSpire assets.
//
// See CLAUDE.md section 5.2 for the design rationale.
package ir

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// Map describes the overall canvas. A map has a single dominant biome, fixed
// for the whole map; zones only modulate within it (see Zone).
type Map struct {
	Width  int    `json:"width"`
	Length int    `json:"length"`
	Name   string `json:"name"`
	Biome  string `json:"biome"`
}

// Anchor is an approximate position for a zone. X/Y are tile coordinates on the
// map grid. DirectionFrom is an optional relative hint (e.g. "courtyard: north")
// kept for future relative resolution; Phase 1 relies on X/Y.
type Anchor struct {
	X             int    `json:"x"`
	Y             int    `json:"y"`
	DirectionFrom string `json:"direction_from,omitempty"`
}

// Elevation is a qualitative height intent for a zone.
type Elevation string

const (
	ElevationFlat       Elevation = "flat"
	ElevationHill       Elevation = "hill"
	ElevationDepression Elevation = "depression"
	ElevationMountain   Elevation = "mountain"
)

// Position is a point-of-interest placement. It is either an explicit tile
// coordinate ({"x":.., "y":..}) or the string keyword "scattered".
type Position struct {
	Scattered bool
	X         int
	Y         int
}

// UnmarshalJSON accepts either "scattered" or {"x":..,"y":..}.
func (p *Position) UnmarshalJSON(data []byte) error {
	trimmed := strings.TrimSpace(string(data))
	if strings.HasPrefix(trimmed, "\"") {
		var s string
		if err := json.Unmarshal(data, &s); err != nil {
			return err
		}
		if s != "scattered" {
			return fmt.Errorf("unknown position keyword %q (expected \"scattered\")", s)
		}
		*p = Position{Scattered: true}
		return nil
	}
	var obj struct {
		X *int `json:"x"`
		Y *int `json:"y"`
	}
	dec := json.NewDecoder(strings.NewReader(trimmed))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&obj); err != nil {
		return err
	}
	if obj.X == nil || obj.Y == nil {
		return fmt.Errorf("explicit position must provide both x and y")
	}
	*p = Position{X: *obj.X, Y: *obj.Y}
	return nil
}

// MarshalJSON mirrors UnmarshalJSON so a resolved IR round-trips.
func (p Position) MarshalJSON() ([]byte, error) {
	if p.Scattered {
		return json.Marshal("scattered")
	}
	return json.Marshal(map[string]int{"x": p.X, "y": p.Y})
}

// PointOfInterest is an explicit, deterministically-placed prop (as opposed to
// the weighted-random prop scatter that fills the rest of a zone).
type PointOfInterest struct {
	Prop     string   `json:"prop"`
	Position Position `json:"position"`
	Count    int      `json:"count"`
}

// Zone is a single semantic region of the map. Zones never change the biome
// (that is fixed at the map level). Within the map's biome, a zone modulates:
// the prop density (DensityOverrides), the relief used (ReliefOverride, which
// must name a relief of the map's biome), and the explicit points of interest.
type Zone struct {
	ID               string             `json:"id"`
	Anchor           Anchor             `json:"anchor"`
	RelativeSize     float64            `json:"relative_size"`
	Elevation        Elevation          `json:"elevation"`
	ReliefOverride   string             `json:"relief_override,omitempty"`
	DensityOverrides map[string]float64 `json:"density_overrides,omitempty"`
	PointsOfInterest []PointOfInterest  `json:"points_of_interest,omitempty"`
	Description      string             `json:"description,omitempty"`
}

// Connection is a link between two zones (e.g. a path). Kept in the schema for
// Phase 3 (routing) — Phase 1 parses but does not yet carve connections.
type Connection struct {
	From  string `json:"from"`
	To    string `json:"to"`
	Type  string `json:"type"`
	Width int    `json:"width"`
}

// IR is the full intermediate representation of a map.
type IR struct {
	Map         Map          `json:"map"`
	Zones       []Zone       `json:"zones"`
	Connections []Connection `json:"connections,omitempty"`
}

// Parse decodes and validates an IR document.
func Parse(data []byte) (*IR, error) {
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()
	var doc IR
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("decoding IR: %w", err)
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		return nil, fmt.Errorf("decoding IR: expected a single JSON document")
	}
	if err := doc.Validate(); err != nil {
		return nil, err
	}
	return &doc, nil
}
