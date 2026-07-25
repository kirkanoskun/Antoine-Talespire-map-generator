package spatial

import (
	"math"

	"github.com/kirkanoskun/antoine-talespire-map-generator/internal/ir"
)

// Paths is a per-tile overlay of the carved connections between zones (the IR's
// `connections`). It marks which tiles belong to a path, and for each such tile
// its parameter t in [0,1] along the connection (0 at the "from" anchor, 1 at
// the "to" anchor) and which connection it belongs to. The generator turns this
// into a leveled, bare corridor whose height ramps between the two endpoints —
// "a path that climbs toward the ruins in the north".
type Paths struct {
	Width  int
	Length int
	on     [][]bool
	t      [][]float64
	conn   [][]int
}

// BuildPaths rasterizes every connection in the document into a path overlay.
// Each connection is a thick segment (its `width`, default 3 tiles) between the
// two zones' anchors. Overlapping paths: the later connection wins the tile.
func BuildPaths(doc *ir.IR) *Paths {
	w, l := doc.Map.Width, doc.Map.Length
	p := &Paths{Width: w, Length: l}
	p.on = make([][]bool, w)
	p.t = make([][]float64, w)
	p.conn = make([][]int, w)
	for x := 0; x < w; x++ {
		p.on[x] = make([]bool, l)
		p.t[x] = make([]float64, l)
		p.conn[x] = make([]int, l)
		for y := 0; y < l; y++ {
			p.conn[x][y] = -1
		}
	}

	anchor := make(map[string][2]int, len(doc.Zones))
	for i := range doc.Zones {
		anchor[doc.Zones[i].ID] = [2]int{doc.Zones[i].Anchor.X, doc.Zones[i].Anchor.Y}
	}

	for ci := range doc.Connections {
		c := &doc.Connections[ci]
		from, okf := anchor[c.From]
		to, okt := anchor[c.To]
		if !okf || !okt {
			continue
		}
		width := c.Width
		if width <= 0 {
			width = 3
		}
		p.carveSegment(ci, from, to, float64(width)/2.0)
	}
	return p
}

// carveSegment marks tiles within `radius` of the segment from→to.
func (p *Paths) carveSegment(conn int, from, to [2]int, radius float64) {
	ax, ay := float64(from[0]), float64(from[1])
	bx, by := float64(to[0]), float64(to[1])
	dx, dy := bx-ax, by-ay
	len2 := dx*dx + dy*dy

	r := int(math.Ceil(radius))
	minX := clampInt(min(from[0], to[0])-r, 0, p.Width-1)
	maxX := clampInt(max(from[0], to[0])+r, 0, p.Width-1)
	minY := clampInt(min(from[1], to[1])-r, 0, p.Length-1)
	maxY := clampInt(max(from[1], to[1])+r, 0, p.Length-1)

	for x := minX; x <= maxX; x++ {
		for y := minY; y <= maxY; y++ {
			px, py := float64(x), float64(y)
			var t float64
			if len2 > 0 {
				t = ((px-ax)*dx + (py-ay)*dy) / len2
				if t < 0 {
					t = 0
				} else if t > 1 {
					t = 1
				}
			}
			cx, cy := ax+t*dx, ay+t*dy
			ddx, ddy := px-cx, py-cy
			if ddx*ddx+ddy*ddy <= radius*radius {
				p.on[x][y] = true
				p.t[x][y] = t
				p.conn[x][y] = conn
			}
		}
	}
}

// On reports whether (x,y) is on a carved path.
func (p *Paths) On(x, y int) bool { return p.on[x][y] }

// Param returns the tile's position along its connection, in [0,1].
func (p *Paths) Param(x, y int) float64 { return p.t[x][y] }

// Conn returns the connection index owning the tile, or -1.
func (p *Paths) Conn(x, y int) int { return p.conn[x][y] }

// Any reports whether any path tile exists.
func (p *Paths) Any() bool {
	for x := 0; x < p.Width; x++ {
		for y := 0; y < p.Length; y++ {
			if p.on[x][y] {
				return true
			}
		}
	}
	return false
}
