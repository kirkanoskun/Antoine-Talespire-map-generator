package ir

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"regexp"
	"strconv"
	"strings"
)

// Repair is one correction ParseLenient made to a map before accepting it.
// The UI turns the code into a sentence, so the visitor knows what was changed
// without having to send anything back to their AI.
type Repair struct {
	Code  string `json:"code"`
	Zone  string `json:"zone,omitempty"`  // the zone concerned, if any
	Field string `json:"field,omitempty"` // the field, prop or key concerned
	From  string `json:"from,omitempty"`  // what the AI wrote
	To    string `json:"to,omitempty"`    // what is used instead
}

// Repair codes.
const (
	RepairSyntax          = "syntax_cleaned"     // comments or trailing commas removed
	RepairUnknownField    = "unknown_field"      // a field the format does not have, dropped
	RepairRounded         = "number_rounded"     // a decimal where a whole number is needed
	RepairMapSize         = "map_size"           // missing size defaulted, or oversize scaled down
	RepairBiome           = "biome"              // biome spelled differently
	RepairElevation       = "elevation"          // elevation spelled differently, or unknown (flat)
	RepairZoneID          = "zone_id"            // missing or duplicate id
	RepairZoneSize        = "zone_size"          // relative_size outside (0, 1]
	RepairAnchor          = "anchor"             // anchor missing or outside the map
	RepairDensity         = "density"            // density key or value adjusted
	RepairPosition        = "position"           // a point of interest's position fixed
	RepairCount           = "count"              // scattered without a count
	RepairPOIDropped      = "poi_dropped"        // a point of interest with no usable prop
	RepairProp            = "prop"               // a prop name spelled differently
	RepairRelief          = "relief"             // relief_override replaced by a relief of the biome
	RepairReliefDropped   = "relief_dropped"     // unknown relief_override: the biome's own terrain is used
	RepairConnection      = "connection_dropped" // a path between zones that do not exist
	RepairPathWidth       = "path_width"         // path width out of range
	RepairBuilding        = "building"           // a building's position or rotation adjusted
	RepairBuildingDropped = "building_dropped"   // unknown or unusable building
)

// Catalogue is what ParseLenient needs to know about the engine to fall back
// gracefully on things only the catalogues can judge. Any field may be nil.
type Catalogue struct {
	Reliefs     func(biome string) []string // the relief keys of a biome
	KnownProp   func(name string) bool      // whether a point-of-interest name can be placed
	KnownPrefab func(id string) bool        // whether a building id exists
}

// ParseLenient is Parse for a map written by an AI assistant: where the strict
// reader would reject a benign slip (a decimal coordinate, an extra field, an
// anchor one tile outside the map, "Flat" for "flat"…), it repairs the document
// and reports what it changed. A valid document comes back unchanged, exactly
// as Parse returns it. What cannot be repaired without guessing the scene (no
// zones, an unknown biome) is still an error.
func ParseLenient(data []byte, cat *Catalogue) (*IR, []Repair, error) {
	r := &repairer{cat: cat, sx: 1, sy: 1}
	doc, strictErr := Parse(data)
	if strictErr != nil {
		root, err := r.decode(data)
		if err != nil {
			return nil, nil, strictErr // not JSON even after cleaning: the first error says it best
		}
		r.document(root)
		fixed, err := json.Marshal(root)
		if err != nil {
			return nil, nil, strictErr
		}
		if doc, err = Parse(fixed); err != nil {
			return nil, nil, err
		}
	}
	r.catalogue(doc)
	return doc, r.repairs, nil
}

type repairer struct {
	cat     *Catalogue
	repairs []Repair
	seen    map[Repair]bool
	w, l    int     // the map's size, once known
	sx, sy  float64 // scale applied to coordinates when an oversize map is shrunk
}

func (r *repairer) note(rep Repair) {
	if r.seen == nil {
		r.seen = map[Repair]bool{}
	}
	if !r.seen[rep] {
		r.seen[rep] = true
		r.repairs = append(r.repairs, rep)
	}
}

// decode reads one JSON object, keeping numbers as written. If the text is not
// valid JSON it retries without comments and trailing commas, which assistants
// sometimes add.
func (r *repairer) decode(data []byte) (map[string]any, error) {
	read := func(b []byte) (map[string]any, error) {
		dec := json.NewDecoder(bytes.NewReader(b))
		dec.UseNumber()
		var root map[string]any
		if err := dec.Decode(&root); err != nil {
			return nil, err
		}
		if err := dec.Decode(new(any)); err != io.EOF {
			return nil, fmt.Errorf("expected a single JSON document")
		}
		if root == nil {
			return nil, fmt.Errorf("expected a JSON object")
		}
		return root, nil
	}
	root, err := read(data)
	if err == nil {
		return root, nil
	}
	cleaned := cleanSyntax(data)
	if bytes.Equal(cleaned, data) {
		return nil, err
	}
	if root, err = read(cleaned); err != nil {
		return nil, err
	}
	r.note(Repair{Code: RepairSyntax})
	return root, nil
}

// cleanSyntax removes // and /* */ comments and trailing commas, outside
// strings.
func cleanSyntax(data []byte) []byte {
	out := make([]byte, 0, len(data))
	inString, escaped := false, false
	for i := 0; i < len(data); i++ {
		c := data[i]
		if inString {
			out = append(out, c)
			switch {
			case escaped:
				escaped = false
			case c == '\\':
				escaped = true
			case c == '"':
				inString = false
			}
			continue
		}
		switch {
		case c == '"':
			inString = true
			out = append(out, c)
		case c == '/' && i+1 < len(data) && data[i+1] == '/':
			for i < len(data) && data[i] != '\n' {
				i++
			}
			out = append(out, '\n')
		case c == '/' && i+1 < len(data) && data[i+1] == '*':
			end := bytes.Index(data[i+2:], []byte("*/"))
			if end < 0 {
				i = len(data)
			} else {
				i += end + 3
			}
		case c == '}' || c == ']':
			// Drop a comma left dangling before this closing bracket.
			j := len(out) - 1
			for j >= 0 && (out[j] == ' ' || out[j] == '\n' || out[j] == '\t' || out[j] == '\r') {
				j--
			}
			if j >= 0 && out[j] == ',' {
				out = append(out[:j], out[j+1:]...)
			}
			out = append(out, c)
		default:
			out = append(out, c)
		}
	}
	return out
}

// document repairs the decoded tree in place.
func (r *repairer) document(root map[string]any) {
	r.keep(root, "", "", "map", "zones", "connections", "buildings")

	m, _ := root["map"].(map[string]any)
	if m == nil {
		m = map[string]any{}
		root["map"] = m
	}
	r.mapHeader(m)

	zones, _ := root["zones"].([]any)
	ids := map[string]bool{}
	var kept []any
	for i, z := range zones {
		zone, ok := z.(map[string]any)
		if !ok {
			continue
		}
		r.zone(zone, i, ids)
		kept = append(kept, zone)
	}
	if kept != nil {
		root["zones"] = kept
	}

	if conns, ok := root["connections"].([]any); ok {
		root["connections"] = r.connections(conns, ids)
	} else {
		delete(root, "connections")
	}
	if buildings, ok := root["buildings"].([]any); ok {
		root["buildings"] = r.buildings(buildings)
	} else {
		delete(root, "buildings")
	}
}

// keep drops every key of obj that is not allowed.
func (r *repairer) keep(obj map[string]any, zone, prefix string, allowed ...string) {
	for key := range obj {
		ok := false
		for _, a := range allowed {
			if key == a {
				ok = true
				break
			}
		}
		if !ok {
			delete(obj, key)
			r.note(Repair{Code: RepairUnknownField, Zone: zone, Field: prefix + key})
		}
	}
}

func (r *repairer) mapHeader(m map[string]any) {
	r.keep(m, "", "map.", "width", "length", "name", "biome")
	if name, ok := m["name"]; ok {
		if _, isString := name.(string); !isString {
			delete(m, "name")
		}
	}
	size := func(key string) (int, float64) {
		v, ok := r.integer(m, key, "", "map."+key)
		switch {
		case !ok || v <= 0:
			r.note(Repair{Code: RepairMapSize, Field: key, From: show(m[key]), To: "50"})
			v = 50
		case v > MaxDimension:
			r.note(Repair{Code: RepairMapSize, Field: key, From: strconv.Itoa(v), To: strconv.Itoa(MaxDimension)})
			m[key] = MaxDimension
			return MaxDimension, float64(MaxDimension) / float64(v)
		}
		m[key] = v
		return v, 1
	}
	r.w, r.sx = size("width")
	r.l, r.sy = size("length")

	if raw, ok := m["biome"].(string); ok {
		if biome, found := biomeNames[fold(raw)]; found && biome != raw {
			m["biome"] = biome
			r.note(Repair{Code: RepairBiome, From: raw, To: biome})
		}
	}
}

func (r *repairer) zone(z map[string]any, index int, ids map[string]bool) {
	// The id first, so the other repairs can name the zone.
	id, _ := z["id"].(string)
	id = strings.TrimSpace(id)
	if id == "" {
		if n, ok := z["id"].(json.Number); ok {
			id = "zone_" + n.String()
		} else {
			id = fmt.Sprintf("zone_%d", index+1)
		}
		r.note(Repair{Code: RepairZoneID, To: id})
	}
	if ids[id] {
		was := id
		for n := 2; ids[id]; n++ {
			id = fmt.Sprintf("%s_%d", was, n)
		}
		r.note(Repair{Code: RepairZoneID, From: was, To: id})
	}
	ids[id] = true
	z["id"] = id

	// A "name" is what the format calls "description".
	if name, ok := z["name"].(string); ok {
		if _, has := z["description"]; !has {
			z["description"] = name
		}
		delete(z, "name")
	}
	r.keep(z, id, "", "id", "anchor", "relative_size", "elevation", "relief_override",
		"density_overrides", "points_of_interest", "description")
	if d, ok := z["description"]; ok {
		if _, isString := d.(string); !isString {
			delete(z, "description")
		}
	}

	ax, ay := r.anchor(z, id)

	size, ok := number(z["relative_size"])
	fixed := size
	switch {
	case !ok || size <= 0:
		fixed = 0.3
	case size >= 5 && size <= 100:
		fixed = size / 100 // a percentage
	case size > 1:
		fixed = 1
	}
	if fixed != size || !ok {
		r.note(Repair{Code: RepairZoneSize, Zone: id, From: show(z["relative_size"]), To: trimFloat(fixed)})
	}
	z["relative_size"] = fixed

	switch e := z["elevation"].(type) {
	case string:
		if e == "" {
			break
		}
		elevation, found := elevationNames[fold(e)]
		if !found {
			elevation = string(ElevationFlat)
		}
		if elevation != e {
			z["elevation"] = elevation
			r.note(Repair{Code: RepairElevation, Zone: id, From: e, To: elevation})
		}
	case nil:
	default:
		delete(z, "elevation")
	}

	switch relief := z["relief_override"].(type) {
	case string:
		if strings.TrimSpace(relief) == "" {
			delete(z, "relief_override")
		}
	default:
		delete(z, "relief_override") // null or not a name: the biome's own terrain
	}

	if d, ok := z["density_overrides"].(map[string]any); ok {
		r.densities(d, id)
	} else {
		delete(z, "density_overrides")
	}

	if pois, ok := z["points_of_interest"].([]any); ok {
		var kept []any
		for _, p := range pois {
			if poi, ok := p.(map[string]any); ok && r.poi(poi, id, ax, ay) {
				kept = append(kept, poi)
			}
		}
		z["points_of_interest"] = kept
	} else {
		delete(z, "points_of_interest")
	}
}

// anchor repairs a zone's anchor and returns it.
func (r *repairer) anchor(z map[string]any, id string) (int, int) {
	a, _ := z["anchor"].(map[string]any)
	if pair, ok := z["anchor"].([]any); ok && len(pair) >= 2 {
		a = map[string]any{"x": pair[0], "y": pair[1]}
	}
	if a == nil {
		a = map[string]any{"x": r.w / 2, "y": r.l / 2}
		r.note(Repair{Code: RepairAnchor, Zone: id, To: fmt.Sprintf("(%d, %d)", r.w/2, r.l/2)})
	}
	z["anchor"] = a
	r.keep(a, id, "anchor.", "x", "y", "direction_from")
	if d, ok := a["direction_from"]; ok {
		if _, isString := d.(string); !isString {
			delete(a, "direction_from")
		}
	}
	x, y, moved := r.point(a, id, "anchor")
	if moved != "" {
		r.note(Repair{Code: RepairAnchor, Zone: id, From: moved, To: fmt.Sprintf("(%d, %d)", x, y)})
	}
	return x, y
}

// point repairs the x and y of obj: whole numbers, scaled if the map was shrunk,
// inside the map. It returns the point and, if it had to be moved into the map,
// where it was.
func (r *repairer) point(obj map[string]any, zone, field string) (x, y int, moved string) {
	x, okx := r.integer(obj, "x", zone, field+".x")
	y, oky := r.integer(obj, "y", zone, field+".y")
	if !okx {
		x = r.w / 2
	}
	if !oky {
		y = r.l / 2
	}
	x, y = int(float64(x)*r.sx), int(float64(y)*r.sy)
	cx, cy := min(max(x, 0), r.w-1), min(max(y, 0), r.l-1)
	if cx != x || cy != y || !okx || !oky {
		moved = fmt.Sprintf("(%s, %s)", show(obj["x"]), show(obj["y"]))
	}
	obj["x"], obj["y"] = cx, cy
	return cx, cy, moved
}

func (r *repairer) densities(d map[string]any, zone string) {
	for key, raw := range d {
		name, known := densityNames[fold(key)]
		v, isNumber := number(raw)
		if !known || !isNumber {
			delete(d, key)
			r.note(Repair{Code: RepairDensity, Zone: zone, Field: key, From: show(raw)})
			continue
		}
		fixed := v
		switch {
		case v < 0:
			fixed = 0
		case v >= 5 && v <= 100:
			fixed = v / 100 // a percentage
		case v > 1:
			fixed = 1
		}
		if name != key || fixed != v {
			r.note(Repair{Code: RepairDensity, Zone: zone, Field: key, From: show(raw), To: name + " " + trimFloat(fixed)})
		}
		delete(d, key)
		d[name] = fixed
	}
}

// poi repairs one point of interest; it reports false if nothing usable is left.
func (r *repairer) poi(p map[string]any, zone string, ax, ay int) bool {
	prop, _ := p["prop"].(string)
	prop = strings.TrimSpace(prop)
	if prop == "" {
		// Assistants sometimes call it "name" or "type".
		for _, alt := range []string{"name", "type", "object", "id"} {
			if s, ok := p[alt].(string); ok && strings.TrimSpace(s) != "" {
				prop = strings.TrimSpace(s)
				break
			}
		}
	}
	if prop == "" {
		r.note(Repair{Code: RepairPOIDropped, Zone: zone})
		return false
	}
	p["prop"] = prop
	delete(p, "name")
	delete(p, "type")
	delete(p, "object")
	delete(p, "id")
	r.keep(p, zone, prop+".", "prop", "position", "count")

	scattered := false
	atAnchor := func(from string) {
		p["position"] = map[string]any{"x": ax, "y": ay}
		r.note(Repair{Code: RepairPosition, Zone: zone, Field: prop, From: from, To: fmt.Sprintf("(%d, %d)", ax, ay)})
	}
	switch pos := p["position"].(type) {
	case string:
		switch positionWords[fold(pos)] {
		case "scattered":
			scattered = true
			if pos != "scattered" {
				p["position"] = "scattered"
			}
		case "anchor":
			atAnchor(pos)
		default:
			scattered = true
			p["position"] = "scattered"
			r.note(Repair{Code: RepairPosition, Zone: zone, Field: prop, From: pos, To: "scattered"})
		}
	case []any:
		if len(pos) < 2 {
			atAnchor(show(pos))
			break
		}
		obj := map[string]any{"x": pos[0], "y": pos[1]}
		p["position"] = obj
		if _, _, moved := r.point(obj, zone, prop); moved != "" {
			r.note(Repair{Code: RepairPosition, Zone: zone, Field: prop, From: moved, To: fmt.Sprintf("(%v, %v)", obj["x"], obj["y"])})
		}
	case map[string]any:
		r.keep(pos, zone, prop+".position.", "x", "y")
		if x, y, moved := r.point(pos, zone, prop); moved != "" {
			r.note(Repair{Code: RepairPosition, Zone: zone, Field: prop, From: moved, To: fmt.Sprintf("(%d, %d)", x, y)})
		}
	default:
		atAnchor("")
	}

	if _, has := p["count"]; has || scattered {
		count, ok := r.integer(p, "count", zone, prop+".count")
		if scattered && (!ok || count <= 0) {
			count = 1
			r.note(Repair{Code: RepairCount, Zone: zone, Field: prop})
		}
		p["count"] = max(count, 0)
	}
	return true
}

// maxPathWidth keeps a path a path: wider, it would bare the whole map.
const maxPathWidth = 9

func (r *repairer) connections(conns []any, ids map[string]bool) []any {
	var kept []any
	for _, c := range conns {
		conn, ok := c.(map[string]any)
		if !ok {
			continue
		}
		r.keep(conn, "", "connection.", "from", "to", "type", "width")
		from, _ := conn["from"].(string)
		to, _ := conn["to"].(string)
		if !ids[from] || !ids[to] || from == to {
			r.note(Repair{Code: RepairConnection, From: from, To: to})
			continue
		}
		if t, ok := conn["type"]; ok {
			if _, isString := t.(string); !isString {
				delete(conn, "type")
			}
		}
		if _, has := conn["width"]; has {
			width, ok := r.integer(conn, "width", "", "connection.width")
			fixed := min(max(width, 0), maxPathWidth)
			if !ok {
				fixed = 0
			}
			if fixed != width {
				r.note(Repair{Code: RepairPathWidth, From: show(conn["width"]), To: strconv.Itoa(fixed)})
			}
			conn["width"] = fixed
		}
		kept = append(kept, conn)
	}
	return kept
}

func (r *repairer) buildings(list []any) []any {
	var kept []any
	for _, b := range list {
		building, ok := b.(map[string]any)
		if !ok {
			continue
		}
		prefab, _ := building["prefab"].(string)
		pos, hasPos := building["position"].(map[string]any)
		if pair, ok := building["position"].([]any); ok && len(pair) >= 2 {
			pos, hasPos = map[string]any{"x": pair[0], "y": pair[1]}, true
		}
		if strings.TrimSpace(prefab) == "" || !hasPos || len(kept) == 100 {
			r.note(Repair{Code: RepairBuildingDropped, From: prefab})
			continue
		}
		r.keep(building, "", "building.", "prefab", "position", "rotation")
		building["position"] = pos
		r.keep(pos, "", "building.position.", "x", "y", "direction_from")
		delete(pos, "direction_from")
		if x, y, moved := r.point(pos, "", "building"); moved != "" {
			r.note(Repair{Code: RepairBuilding, Field: prefab, From: moved, To: fmt.Sprintf("(%d, %d)", x, y)})
		}
		if _, has := building["rotation"]; has {
			rot, _ := r.integer(building, "rotation", "", "building.rotation")
			snapped := ((int(math.Round(float64(rot)/90))*90)%360 + 360) % 360
			if snapped != rot {
				r.note(Repair{Code: RepairBuilding, Field: prefab, From: show(building["rotation"]) + "°", To: strconv.Itoa(snapped) + "°"})
			}
			building["rotation"] = snapped
		}
		kept = append(kept, building)
	}
	return kept
}

// catalogue applies the fallbacks only the engine's catalogues can decide, on
// the parsed document: an unknown relief falls back to the biome's own terrain,
// an unknown building is left out, a prop name is matched loosely.
func (r *repairer) catalogue(doc *IR) {
	// A path wider than this would bare the whole map; the strict reader does
	// not bound it, so it is checked here for every document.
	for i := range doc.Connections {
		c := &doc.Connections[i]
		if fixed := min(max(c.Width, 0), maxPathWidth); fixed != c.Width {
			r.note(Repair{Code: RepairPathWidth, From: strconv.Itoa(c.Width), To: strconv.Itoa(fixed)})
			c.Width = fixed
		}
	}
	if r.cat == nil {
		return
	}
	if r.cat.Reliefs != nil {
		reliefs := r.cat.Reliefs(doc.Map.Biome)
		has := func(key string) bool {
			for _, k := range reliefs {
				if k == key {
					return true
				}
			}
			return false
		}
		for i := range doc.Zones {
			z := &doc.Zones[i]
			if z.ReliefOverride == "" || len(reliefs) == 0 || has(z.ReliefOverride) {
				continue
			}
			folded := fold(z.ReliefOverride)
			replacement := ""
			if has(folded) {
				replacement = folded
			} else if alt := reliefNames[folded]; alt != "" && has(alt) {
				replacement = alt
			}
			if replacement != "" {
				r.note(Repair{Code: RepairRelief, Zone: z.ID, From: z.ReliefOverride, To: replacement})
			} else {
				r.note(Repair{Code: RepairReliefDropped, Zone: z.ID, From: z.ReliefOverride})
			}
			z.ReliefOverride = replacement
		}
	}
	if r.cat.KnownProp != nil {
		for i := range doc.Zones {
			z := &doc.Zones[i]
			for j := range z.PointsOfInterest {
				p := &z.PointsOfInterest[j]
				if r.cat.KnownProp(p.Prop) {
					continue
				}
				// "Fountain 01" for fountain_01. A name that still matches nothing
				// is left alone: the generator reports it as unknown.
				if loose := fold(p.Prop); loose != p.Prop && r.cat.KnownProp(loose) {
					r.note(Repair{Code: RepairProp, Zone: z.ID, From: p.Prop, To: loose})
					p.Prop = loose
				}
			}
		}
	}
	if r.cat.KnownPrefab != nil {
		kept := doc.Buildings[:0]
		for _, b := range doc.Buildings {
			if r.cat.KnownPrefab(b.Prefab) {
				kept = append(kept, b)
			} else {
				r.note(Repair{Code: RepairBuildingDropped, From: b.Prefab})
			}
		}
		doc.Buildings = kept
		if len(kept) == 0 {
			doc.Buildings = nil
		}
	}
}

// integer reads obj[key] as a whole number, rounding a decimal (and noting it)
// and accepting a number written as a string.
func (r *repairer) integer(obj map[string]any, key, zone, field string) (int, bool) {
	raw, has := obj[key]
	if !has {
		return 0, false
	}
	v, ok := number(raw)
	if !ok || math.IsNaN(v) {
		delete(obj, key)
		return 0, false
	}
	v = math.Max(-1e9, math.Min(1e9, v)) // absurd values: the caller clamps them
	n := int(math.Round(v))
	if float64(n) != v {
		r.note(Repair{Code: RepairRounded, Zone: zone, Field: field, From: show(raw), To: strconv.Itoa(n)})
	}
	obj[key] = n
	return n, true
}

// number reads a JSON number, or a string holding one.
func number(v any) (float64, bool) {
	switch n := v.(type) {
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	case string:
		f, err := strconv.ParseFloat(strings.TrimSuffix(strings.TrimSpace(strings.ReplaceAll(n, ",", ".")), "%"), 64)
		return f, err == nil
	case int:
		return float64(n), true
	case float64:
		return n, true
	}
	return 0, false
}

// show renders a decoded value the way the AI wrote it, for a Repair.
func show(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case json.Number:
		return t.String()
	}
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	if len(b) > 40 {
		return string(b[:40]) + "…"
	}
	return string(b)
}

func trimFloat(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }

var (
	foldAccents = strings.NewReplacer("é", "e", "è", "e", "ê", "e", "ë", "e", "à", "a", "â", "a", "ä", "a",
		"î", "i", "ï", "i", "ô", "o", "ö", "o", "û", "u", "ù", "u", "ü", "u", "ç", "c", "œ", "oe")
	foldSeparators = regexp.MustCompile(`[^a-z0-9]+`)
)

// fold reduces a word to a comparable key: lower case, no accents, underscores
// for anything else ("Forêt tempérée" -> "foret_temperee").
func fold(s string) string {
	s = foldAccents.Replace(strings.ToLower(strings.TrimSpace(s)))
	return strings.Trim(foldSeparators.ReplaceAllString(s, "_"), "_")
}

func names(groups map[string][]string) map[string]string {
	out := map[string]string{}
	for canonical, words := range groups {
		out[canonical] = canonical
		for _, w := range words {
			out[w] = canonical
		}
	}
	return out
}

// The words assistants use for the format's keywords, in French and English.
var (
	biomeNames = names(map[string][]string{
		"temperate_forest":   {"foret_temperee", "foret", "forest", "temperate", "woodland", "bois"},
		"subtropical_forest": {"foret_subtropicale", "foret_tropicale", "subtropical", "tropical", "tropical_forest", "jungle"},
		"dead_forest":        {"foret_morte", "dead", "haunted_forest", "foret_hantee"},
		"swamp":              {"marais", "marecage", "marsh", "bog"},
		"lava":               {"lave", "volcan", "volcano", "volcanic", "volcanique"},
		"desert":             {},
		"tundra":             {"toundra", "snow", "neige", "arctic", "arctique"},
		"beach":              {"plage", "coast", "cote", "littoral", "seaside"},
	})
	elevationNames = names(map[string][]string{
		"flat":       {"plat", "plate", "plain", "plaine", "plateau", "level", "none", "normal", "ground"},
		"hill":       {"hills", "colline", "collines", "butte", "mound", "raised", "elevated", "high"},
		"mountain":   {"mountains", "montagne", "montagnes", "peak", "cliff", "falaise"},
		"depression": {"valley", "vallee", "basin", "bassin", "pit", "hollow", "creux", "low", "sunken", "pond", "lake"},
	})
	densityNames = names(map[string][]string{
		"vegetation": {"vegetations", "trees", "tree", "arbres", "plants", "plantes", "forest"},
		"stones":     {"stone", "rocks", "rock", "rochers", "pierres"},
		"misc":       {"details", "detail", "bushes", "flowers", "divers", "decor", "props"},
	})
	positionWords = names(map[string][]string{
		"scattered": {"scatter", "random", "aleatoire", "disperse", "dispersed", "eparpille", "around"},
		"anchor":    {"center", "centre", "middle", "milieu", "central"},
	})
	reliefNames = names(map[string][]string{
		"paving":      {"cobblestone", "cobblestones", "cobble", "paved", "pavement", "paves", "pave", "flagstones", "stone_floor", "courtyard"},
		"swamp_floor": {"mud", "marsh", "bog", "swamp", "boue", "marais"},
		"water":       {"lake", "pond", "river", "eau", "mare", "etang", "sea"},
		"mountain":    {"rock", "rocks", "rocky", "cliff", "stone", "roche"},
		"base_ground": {"path", "dirt", "bare", "road", "chemin", "terre"},
		"ground":      {"grass", "meadow", "herbe", "prairie", "forest"},
		"ruins":       {"ruin", "rubble", "ruines", "ruine"},
	})
)
