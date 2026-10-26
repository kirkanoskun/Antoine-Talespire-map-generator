// Package preview renders a top-down 2D image of a resolved map, before any
// export to TaleSpire. This is the fast visual feedback loop from the brief
// (section 5.6). A map has one biome, so tiles are colour-coded by their
// resolved relief (ground, water, mountain, ruins, ...); terrain height is
// shaded, zone borders are outlined, and points of interest are marked.
package preview

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"

	"github.com/kirkanoskun/antoine-talespire-map-generator/internal/generator"
	"github.com/kirkanoskun/antoine-talespire-map-generator/internal/ir"
	"github.com/kirkanoskun/antoine-talespire-map-generator/internal/spatial"
)

// reliefPalette maps a relief key to a representative top-down colour. Reliefs
// not listed fall back to fallbackRelief.
var reliefPalette = map[string]color.RGBA{
	"water":       {60, 110, 180, 255},
	"base_ground": {214, 186, 120, 255}, // paths: a sand clearly lighter than stone
	"ground":      {70, 130, 65, 255},
	"mountain":    {125, 120, 118, 255},
	"ruins":       {150, 140, 125, 255},
}

// texturedReliefs are told apart by pattern as well as colour, so the map
// never relies on hue alone (ruins and mountain are close in value once
// height-shaded, water and ground in luminance): the legend reports it.
var texturedReliefs = map[string]string{"ruins": "masonry", "water": "waves", "mountain": "hatch"}

var (
	boundaryColor  = color.RGBA{25, 25, 25, 255}
	poiColor       = color.RGBA{246, 238, 222, 255} // a light pin with a dark ring reads on any terrain
	poiRingColor   = color.RGBA{27, 21, 17, 255}
	anchorColor    = color.RGBA{20, 20, 20, 255}
	buildingColor  = color.RGBA{195, 142, 65, 255}
	fallbackRelief = color.RGBA{150, 150, 150, 255}
	canopyInk      = color.RGBA{14, 22, 10, 255}    // trees: the ground beneath, much darker
	stoneColor     = color.RGBA{204, 198, 188, 255} // rocks: light, with a dark shadow pixel
	white          = color.RGBA{255, 255, 255, 255}
)

// reliefOrder fixes the legend order of the known reliefs (the palette is a
// map, and output must not depend on map iteration).
var reliefOrder = []string{"water", "ground", "base_ground", "mountain", "ruins"}

// LegendEntry is one swatch of the preview's key: what a colour stands for.
// Key is a relief key, "other" (a relief outside the palette), "building",
// "tree", "stone", "misc", "boundary" or "poi"; the UI translates it. Color is
// the unshaded "#rrggbb"; Pattern names a texture drawn over it ("masonry",
// "waves", "hatch") or a symbol ("tree", "stone", "dot", "line", "pin").
// Count is how many are drawn, for the scatter and points of interest.
type LegendEntry struct {
	Key     string `json:"key"`
	Color   string `json:"color"`
	Pattern string `json:"pattern,omitempty"`
	Count   int    `json:"count,omitempty"`
}

// Legend lists, in a fixed order, the colours that actually appear in the
// preview of this map, so the UI can explain the image without guessing.
func Legend(doc *ir.IR, field *generator.HeightField) []LegendEntry {
	present := map[string]bool{}
	var props [4]int // per generator.PropKind
	for x := 0; x < field.Width; x++ {
		for y := 0; y < field.Length; y++ {
			props[field.PropAt(x, y)]++
			switch {
			case field.BuildingAt(x, y):
				present["building"] = true
			default:
				if _, ok := reliefPalette[field.ReliefAt(x, y)]; ok {
					present[field.ReliefAt(x, y)] = true
				} else {
					present["other"] = true
				}
			}
		}
	}
	var out []LegendEntry
	for _, key := range reliefOrder {
		if present[key] {
			out = append(out, LegendEntry{key, hex(reliefPalette[key]), texturedReliefs[key], 0})
		}
	}
	if present["other"] {
		out = append(out, LegendEntry{"other", hex(fallbackRelief), "", 0})
	}
	if present["building"] {
		out = append(out, LegendEntry{"building", hex(buildingColor), "", 0})
	}
	ground := reliefPalette["ground"]
	if n := props[generator.PropTree]; n > 0 {
		out = append(out, LegendEntry{"tree", hex(canopy(ground)), "tree", n})
	}
	if n := props[generator.PropStone]; n > 0 {
		out = append(out, LegendEntry{"stone", hex(stoneColor), "stone", n})
	}
	if n := props[generator.PropMisc]; n > 0 {
		out = append(out, LegendEntry{"misc", hex(detail(ground)), "dot", n})
	}
	if len(doc.Zones) > 1 {
		out = append(out, LegendEntry{"boundary", hex(blend(ground, boundaryColor, 0.45)), "line", 0})
	}
	if n := len(field.POIs()); n > 0 {
		out = append(out, LegendEntry{"poi", hex(poiColor), "pin", n})
	}
	return out
}

func canopy(ground color.RGBA) color.RGBA { return blend(ground, canopyInk, 0.55) }
func detail(ground color.RGBA) color.RGBA { return blend(ground, canopyInk, 0.35) }

func hex(c color.RGBA) string { return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B) }

// Options configures rendering.
type Options struct {
	Scale int // pixels per tile (default 8)
	// MarkAnchors draws each zone's anchor as a dark square: handy when the
	// PNG stands alone (CLIs). The web UI numbers its zones itself instead.
	MarkAnchors bool
}

// Render draws the map to an RGBA image.
func Render(doc *ir.IR, mask *spatial.Mask, field *generator.HeightField, opts Options) *image.RGBA {
	scale := opts.Scale
	if scale <= 0 {
		scale = 8
	}
	w, l := doc.Map.Width, doc.Map.Length
	img := image.NewRGBA(image.Rect(0, 0, w*scale, l*scale))

	minH, maxH := heightRange(field)

	bases := make([][]color.RGBA, w)
	for x := 0; x < w; x++ {
		bases[x] = make([]color.RGBA, l)
		for y := 0; y < l; y++ {
			base := colorFor(field, x, y)
			if field.BuildingAt(x, y) {
				base = buildingColor
			}
			if !field.IsWaterAt(x, y) {
				base = shade(base, field.HeightAt(x, y), minH, maxH)
			}
			if mask.IsBoundary(x, y) {
				base = blend(base, boundaryColor, 0.45)
			}
			fill(img, x, y, scale, base)
			bases[x][y] = base
			if !field.BuildingAt(x, y) && scale >= 4 {
				switch texturedReliefs[field.ReliefAt(x, y)] {
				case "masonry":
					masonry(img, x, y, scale, blend(base, boundaryColor, 0.3))
				case "waves":
					waves(img, x, y, scale, blend(base, white, 0.22))
				case "hatch":
					hatch(img, x, y, scale, blend(base, boundaryColor, 0.18))
				}
			}
		}
	}

	// The scatter the slab actually carries, over the finished ground: tree
	// canopies overhang their tile (trees are never side by side), which
	// painting the next tile would otherwise clip.
	for x := 0; x < w; x++ {
		for y := 0; y < l; y++ {
			switch field.PropAt(x, y) {
			case generator.PropTree:
				disc(img, x, y, scale, float64(scale)*0.62, canopy(bases[x][y]))
			case generator.PropStone:
				stone(img, x, y, scale)
			case generator.PropMisc:
				disc(img, x, y, scale, float64(scale)*0.2, detail(bases[x][y]))
			}
		}
	}

	if opts.MarkAnchors {
		for _, z := range doc.Zones {
			marker(img, z.Anchor.X, z.Anchor.Y, scale, anchorColor)
		}
	}
	// Every point of interest actually placed, scattered ones included.
	for _, p := range field.POIs() {
		pin(img, p[0], p[1], scale)
	}
	return img
}

// masonry draws a stone-joint pattern over a tile: its top and left edges,
// plus a half-offset vertical joint, so ruins read as laid stone.
func masonry(img *image.RGBA, tx, ty, scale int, joint color.RGBA) {
	x0, y0 := tx*scale, ty*scale
	for i := 0; i < scale; i++ {
		img.SetRGBA(x0+i, y0, joint)
		img.SetRGBA(x0, y0+i, joint)
		img.SetRGBA(x0+i, y0+scale/2, joint)
	}
	for i := scale / 2; i < scale; i++ {
		img.SetRGBA(x0+scale/2, y0+i, joint)
	}
}

// waves draws two short light dashes across a water tile, offset row to row.
func waves(img *image.RGBA, tx, ty, scale int, c color.RGBA) {
	x0, y0 := tx*scale, ty*scale
	off := (ty % 2) * scale / 2
	for i := 0; i < scale/2; i++ {
		img.SetRGBA(x0+(off+i)%scale, y0+scale/3, c)
		img.SetRGBA(x0+(off+scale/2+i)%scale, y0+2*scale/3+1, c)
	}
}

// hatch draws diagonal strokes across a tile, continuous from tile to tile,
// the cartographer's mark for rock.
func hatch(img *image.RGBA, tx, ty, scale int, c color.RGBA) {
	x0, y0 := tx*scale, ty*scale
	for i := 0; i < scale; i++ {
		for j := 0; j < scale; j++ {
			if (x0+i+y0+j)%4 == 0 {
				img.SetRGBA(x0+i, y0+j, c)
			}
		}
	}
}

// disc fills a circle of radius r centred on a tile.
func disc(img *image.RGBA, tx, ty, scale int, r float64, c color.RGBA) {
	cx, cy := float64(tx*scale)+float64(scale)/2, float64(ty*scale)+float64(scale)/2
	for x := int(cx - r); x <= int(cx+r); x++ {
		for y := int(cy - r); y <= int(cy+r); y++ {
			if dx, dy := float64(x)+.5-cx, float64(y)+.5-cy; dx*dx+dy*dy <= r*r && image.Pt(x, y).In(img.Bounds()) {
				img.SetRGBA(x, y, c)
			}
		}
	}
}

// stone draws a small light rock with a shadow on its lower right.
func stone(img *image.RGBA, tx, ty, scale int) {
	side := max(1, scale*3/8)
	x0, y0 := tx*scale+(scale-side)/2, ty*scale+(scale-side)/2
	for i := 0; i < side; i++ {
		for j := 0; j < side; j++ {
			img.SetRGBA(x0+i, y0+j, stoneColor)
		}
	}
	for i := 0; i <= side; i++ {
		img.SetRGBA(x0+i, y0+side, poiRingColor)
		img.SetRGBA(x0+side, y0+i, poiRingColor)
	}
}

// pin draws a point of interest as a light disc with a dark ring, centred on
// its tile, so it stands out by value and shape rather than by hue.
func pin(img *image.RGBA, tx, ty, scale int) {
	cx, cy := float64(tx*scale)+float64(scale)/2, float64(ty*scale)+float64(scale)/2
	r := float64(scale)/2 + 1.5
	b := img.Bounds()
	for x := int(cx - r - 1); x <= int(cx+r+1); x++ {
		for y := int(cy - r - 1); y <= int(cy+r+1); y++ {
			if x < b.Min.X || y < b.Min.Y || x >= b.Max.X || y >= b.Max.Y {
				continue
			}
			dx, dy := float64(x)+.5-cx, float64(y)+.5-cy
			switch d := dx*dx + dy*dy; {
			case d <= (r-1.6)*(r-1.6):
				img.SetRGBA(x, y, poiColor)
			case d <= r*r:
				img.SetRGBA(x, y, poiRingColor)
			}
		}
	}
}

// WritePNG renders and encodes to w.
func WritePNG(out io.Writer, doc *ir.IR, mask *spatial.Mask, field *generator.HeightField, opts Options) error {
	return png.Encode(out, Render(doc, mask, field, opts))
}

// SavePNG renders and writes to a file path.
func SavePNG(path string, doc *ir.IR, mask *spatial.Mask, field *generator.HeightField, opts Options) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return WritePNG(f, doc, mask, field, opts)
}

func colorFor(field *generator.HeightField, x, y int) color.RGBA {
	if c, ok := reliefPalette[field.ReliefAt(x, y)]; ok {
		return c
	}
	return fallbackRelief
}

func heightRange(f *generator.HeightField) (int, int) {
	minH, maxH := 1<<30, -(1 << 30)
	for x := 0; x < f.Width; x++ {
		for y := 0; y < f.Length; y++ {
			h := f.HeightAt(x, y)
			if h < minH {
				minH = h
			}
			if h > maxH {
				maxH = h
			}
		}
	}
	if minH > maxH {
		minH, maxH = 0, 0
	}
	return minH, maxH
}

// shade lightens/darkens a base colour by relative height (higher = lighter).
func shade(c color.RGBA, h, minH, maxH int) color.RGBA {
	if maxH == minH {
		return c
	}
	t := float64(h-minH) / float64(maxH-minH) // 0..1
	factor := 0.65 + 0.5*t                    // 0.65..1.15
	return color.RGBA{
		R: clamp(float64(c.R) * factor),
		G: clamp(float64(c.G) * factor),
		B: clamp(float64(c.B) * factor),
		A: 255,
	}
}

func blend(a, b color.RGBA, t float64) color.RGBA {
	return color.RGBA{
		R: clamp(float64(a.R)*(1-t) + float64(b.R)*t),
		G: clamp(float64(a.G)*(1-t) + float64(b.G)*t),
		B: clamp(float64(a.B)*(1-t) + float64(b.B)*t),
		A: 255,
	}
}

func clamp(v float64) uint8 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v)
}

func fill(img *image.RGBA, tx, ty, scale int, c color.RGBA) {
	for px := 0; px < scale; px++ {
		for py := 0; py < scale; py++ {
			img.SetRGBA(tx*scale+px, ty*scale+py, c)
		}
	}
}

// marker draws a small filled square centred on a tile.
func marker(img *image.RGBA, tx, ty, scale int, c color.RGBA) {
	cx := tx*scale + scale/2
	cy := ty*scale + scale/2
	r := scale/2 + 1
	for px := -r; px <= r; px++ {
		for py := -r; py <= r; py++ {
			x, y := cx+px, cy+py
			if x >= 0 && x < img.Bounds().Dx() && y >= 0 && y < img.Bounds().Dy() {
				img.SetRGBA(x, y, c)
			}
		}
	}
}
