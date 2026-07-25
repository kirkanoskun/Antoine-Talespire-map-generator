package spatial

// Transitions describes, for every tile near a zone border, how far it is from
// the seam and which neighbouring zone lies across it. It is the input to the
// Phase 3 stitching step (brief 5.5): because every zone shares the map's single
// biome, stitching is a simple intra-biome smoothing of height and prop density
// across a few tiles at the border — not a fusion of two generation systems.
//
// The field is computed by a bounded multi-source BFS: every tile adjacent to a
// different zone is a seam tile (distance 1), and the distance grows inward,
// staying within each tile's own zone, up to a half-width. Each in-band tile
// records the foreign zone across its nearest seam.
type Transitions struct {
	Width  int
	Length int
	half   int
	dist   [][]int // 0 = outside the band; 1..half = tiles to the nearest seam
	other  [][]int // nearest foreign zone index; -1 outside the band
}

// Transitions builds the transition field for the mask with the given band
// half-width (in tiles). A half-width of 0 or 1 both yield a one-tile band.
func (m *Mask) Transitions(halfWidth int) *Transitions {
	if halfWidth < 1 {
		halfWidth = 1
	}
	w, l := m.Width, m.Length
	t := &Transitions{Width: w, Length: l, half: halfWidth}
	t.dist = make([][]int, w)
	t.other = make([][]int, w)
	for x := 0; x < w; x++ {
		t.dist[x] = make([]int, l)
		t.other[x] = make([]int, l)
		for y := 0; y < l; y++ {
			t.other[x][y] = -1
		}
	}

	type cell struct{ x, y int }
	queue := make([]cell, 0, w) // FIFO; deterministic scan-order seeding

	// Seed: every tile adjacent to a different zone (distance 1), labelled with
	// the foreign zone across that seam.
	for x := 0; x < w; x++ {
		for y := 0; y < l; y++ {
			if foreign := m.foreignNeighbour(x, y); foreign >= 0 {
				t.dist[x][y] = 1
				t.other[x][y] = foreign
				queue = append(queue, cell{x, y})
			}
		}
	}

	// Expand inward, staying within each tile's own zone, up to half-width.
	for len(queue) > 0 {
		c := queue[0]
		queue = queue[1:]
		d := t.dist[c.x][c.y]
		if d >= halfWidth {
			continue
		}
		z := m.Cells[c.x][c.y]
		for _, n := range fourNeighbours(c.x, c.y, w, l) {
			if m.Cells[n.x][n.y] != z {
				continue // don't cross into another zone
			}
			if t.dist[n.x][n.y] == 0 { // unset
				t.dist[n.x][n.y] = d + 1
				t.other[n.x][n.y] = t.other[c.x][c.y]
				queue = append(queue, cell{n.x, n.y})
			}
		}
	}
	return t
}

// At returns the foreign zone across the nearest seam and a blend factor in
// [0, 0.5] for tile (x,y): 0.5 exactly at the seam, fading to 0 at the band
// edge, and (-1, 0) outside any transition band. A seam tile blending 0.5 toward
// its neighbour meets the neighbour's seam tile blending 0.5 back, so both sides
// converge to the average at the border.
func (t *Transitions) At(x, y int) (otherZone int, blend float64) {
	d := t.dist[x][y]
	if d == 0 {
		return -1, 0
	}
	f := 0.5 * float64(t.half-d+1) / float64(t.half)
	if f < 0 {
		f = 0
	}
	return t.other[x][y], f
}

// InBand reports whether (x,y) is within a transition band.
func (t *Transitions) InBand(x, y int) bool { return t.dist[x][y] != 0 }

// Dist returns the tile distance to the nearest seam (0 outside the band).
func (t *Transitions) Dist(x, y int) int { return t.dist[x][y] }

// foreignNeighbour returns the smallest zone index among (x,y)'s 4-neighbours
// that differs from (x,y)'s own zone, or -1 if all neighbours share the zone.
// Choosing the smallest index keeps the result deterministic.
func (m *Mask) foreignNeighbour(x, y int) int {
	z := m.Cells[x][y]
	best := -1
	for _, n := range fourNeighbours(x, y, m.Width, m.Length) {
		if nz := m.Cells[n.x][n.y]; nz != z {
			if best == -1 || nz < best {
				best = nz
			}
		}
	}
	return best
}

type point struct{ x, y int }

func fourNeighbours(x, y, w, l int) []point {
	out := make([]point, 0, 4)
	if x > 0 {
		out = append(out, point{x - 1, y})
	}
	if x < w-1 {
		out = append(out, point{x + 1, y})
	}
	if y > 0 {
		out = append(out, point{x, y - 1})
	}
	if y < l-1 {
		out = append(out, point{x, y + 1})
	}
	return out
}
