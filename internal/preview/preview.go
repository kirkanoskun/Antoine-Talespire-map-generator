// Package preview renders a top-down 2D image of a resolved map, before any
// export to TaleSpire. This is the fast visual feedback loop from the brief
// (section 5.6). A map has one biome, so tiles are colour-coded by their
// resolved relief (ground, water, mountain, ruins, ...); terrain height is
// shaded, zone borders are outlined, and points of interest are marked.
package preview

import (
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
	"base_ground": {170, 160, 110, 255},
	"ground":      {70, 130, 65, 255},
	"mountain":    {130, 125, 120, 255},
	"ruins":       {150, 140, 125, 255},
}

var (
	boundaryColor  = color.RGBA{25, 25, 25, 255}
	poiColor       = color.RGBA{240, 60, 200, 255}
	anchorColor    = color.RGBA{20, 20, 20, 255}
	fallbackRelief = color.RGBA{150, 150, 150, 255}
)

// Options configures rendering.
type Options struct {
	Scale int // pixels per tile (default 8)
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

	for x := 0; x < w; x++ {
		for y := 0; y < l; y++ {
			base := colorFor(field, x, y)
			if field.BuildingAt(x, y) {
				base = color.RGBA{195, 142, 65, 255}
			}
			if !field.IsWaterAt(x, y) {
				base = shade(base, field.HeightAt(x, y), minH, maxH)
			}
			if mask.IsBoundary(x, y) {
				base = blend(base, boundaryColor, 0.45)
			}
			fill(img, x, y, scale, base)
		}
	}

	// Mark zone anchors and explicit points of interest.
	for zi := range doc.Zones {
		z := &doc.Zones[zi]
		marker(img, z.Anchor.X, z.Anchor.Y, scale, anchorColor)
		for _, poi := range z.PointsOfInterest {
			if !poi.Position.Scattered {
				marker(img, poi.Position.X, poi.Position.Y, scale, poiColor)
			}
		}
	}
	return img
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
