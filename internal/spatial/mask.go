// Package spatial converts the approximate, intention-level IR (anchors +
// relative sizes) into an exact, tile-by-tile zone mask.
//
// This is the deliberate design seam described in the brief (section 5.3): the
// language model reasons in relative terms, and a classic geometric algorithm
// does the precise grid work. Phase 1 uses a multiplicatively-weighted Voronoi
// diagram — each tile belongs to the zone whose anchor is "closest" once
// distance is divided by a size-derived weight, so larger zones claim more
// territory. Relaxation / connection carving are left to Phase 3.
package spatial

import (
	"math"

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
	return m
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
