// Package spatial converts the approximate, intention-level IR (anchors +
// relative sizes) into an exact, tile-by-tile zone mask.
//
// This is the deliberate design seam described in the brief (section 5.3): the
// language model reasons in relative terms, and a classic geometric algorithm
// does the precise grid work. Phase 1 uses a multiplicatively-weighted Voronoi
// diagram — each tile belongs to the zone whose anchor is "closest" once
// distance is divided by a size-derived weight, so larger zones claim more
// territory.
//
// Pure Voronoi cannot nest a small zone inside a larger one at the same centre
// (the brief's "pond at the courtyard centre"): the container swallows it. So
// after the Voronoi pass, Resolve detects contained zones — a zone whose own
// anchor is owned by a larger zone — and stamps each as a disc, carving it back
// out of its container. Connection carving lives in path.go.
package spatial

import (
	"math"
	"sort"

	"github.com/kirkanoskun/antoine-talespire-map-generator/internal/ir"
)

// Mask is a per-tile assignment of grid cells to zones.
type Mask struct {
	Width  int
	Length int
	// Cells[x][y] is the index into the zones slice that owns tile (x,y).
	Cells [][]int
	zones []ir.Zone
}

// Resolve builds a zone mask from an IR document using weighted Voronoi.
func Resolve(doc *ir.IR) *Mask {
	w, l := doc.Map.Width, doc.Map.Length
	m := &Mask{
		Width:  w,
		Length: l,
		Cells:  make([][]int, w),
		zones:  doc.Zones,
	}

	// weight ~ sqrt(relative_size): a zone's captured radius scales with the
	// square root of its area target, which keeps areas roughly proportional
	// to relative_size.
	weights := make([]float64, len(doc.Zones))
	for i, z := range doc.Zones {
		weights[i] = math.Sqrt(z.RelativeSize)
	}

	for x := 0; x < w; x++ {
		m.Cells[x] = make([]int, l)
		for y := 0; y < l; y++ {
			best := 0
			bestScore := math.MaxFloat64
			for i, z := range doc.Zones {
				dx := float64(x - z.Anchor.X)
				dy := float64(y - z.Anchor.Y)
				dist := math.Sqrt(dx*dx + dy*dy)
				score := dist / weights[i]
				if score < bestScore {
					bestScore = score
					best = i
				}
			}
			m.Cells[x][y] = best
		}
	}

	m.stampNestedZones(doc.Zones)
	return m
}

// nestSwallowedRatio: a zone whose Voronoi area falls below this fraction of its
// normalized fair share was swallowed by a container and must be re-stamped.
const nestSwallowedRatio = 0.25

// stampNestedZones carves contained zones back out of their container. A zone is
// "contained" when the Voronoi pass gave it far less area than its fair share —
// the tell-tale sign a bigger zone centred nearby (the brief's pond at the
// courtyard centre) swallowed it. Each such zone is stamped as a disc centred on
// its anchor, sized from its fair share of the map. Larger discs are stamped
// first so a zone nested inside a nested zone still lands on top.
func (m *Mask) stampNestedZones(zones []ir.Zone) {
	w, l := m.Width, m.Length

	var sizeSum float64
	for i := range zones {
		sizeSum += zones[i].RelativeSize
	}
	if sizeSum == 0 {
		return
	}
	counts := m.TileCounts()
	area := float64(w * l)

	type disc struct {
		idx    int
		radius float64
	}
	var discs []disc
	for i := range zones {
		fairShare := zones[i].RelativeSize / sizeSum * area
		if float64(counts[i]) >= nestSwallowedRatio*fairShare {
			continue // got its fair share; not contained
		}
		r := math.Sqrt(fairShare / math.Pi)
		if maxR := 0.45 * math.Min(float64(w), float64(l)); r > maxR {
			r = maxR
		}
		if r < 1 {
			r = 1
		}
		discs = append(discs, disc{idx: i, radius: r})
	}

	sort.SliceStable(discs, func(a, b int) bool { return discs[a].radius > discs[b].radius })

	for _, d := range discs {
		ax := zones[d.idx].Anchor.X
		ay := zones[d.idx].Anchor.Y
		r := int(math.Ceil(d.radius))
		r2 := d.radius * d.radius
		for x := ax - r; x <= ax+r; x++ {
			if x < 0 || x >= w {
				continue
			}
			for y := ay - r; y <= ay+r; y++ {
				if y < 0 || y >= l {
					continue
				}
				dx := float64(x - ax)
				dy := float64(y - ay)
				if dx*dx+dy*dy <= r2 {
					m.Cells[x][y] = d.idx
				}
			}
		}
	}
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// ZoneAt returns the zone that owns tile (x,y).
func (m *Mask) ZoneAt(x, y int) *ir.Zone {
	return &m.zones[m.Cells[x][y]]
}

// ZoneIndexAt returns the zone index that owns tile (x,y).
func (m *Mask) ZoneIndexAt(x, y int) int {
	return m.Cells[x][y]
}

// Zones returns the ordered zones backing this mask.
func (m *Mask) Zones() []ir.Zone {
	return m.zones
}

// TileCounts returns, per zone index, how many tiles it owns. Useful for
// diagnostics and tests (e.g. asserting a larger relative_size wins more area).
func (m *Mask) TileCounts() []int {
	counts := make([]int, len(m.zones))
	for x := 0; x < m.Width; x++ {
		for y := 0; y < m.Length; y++ {
			counts[m.Cells[x][y]]++
		}
	}
	return counts
}

// IsBoundary reports whether tile (x,y) sits on a border between two different
// zones (any 4-neighbour belongs to a different zone). Zone transitions
// (brief 5.5) will operate on these tiles; the preview also highlights them.
func (m *Mask) IsBoundary(x, y int) bool {
	z := m.Cells[x][y]
	if x > 0 && m.Cells[x-1][y] != z {
		return true
	}
	if x < m.Width-1 && m.Cells[x+1][y] != z {
		return true
	}
	if y > 0 && m.Cells[x][y-1] != z {
		return true
	}
	if y < m.Length-1 && m.Cells[x][y+1] != z {
		return true
	}
	return false
}
