// Command generate is the Phase 1 entry point: it reads a level-design IR
// (JSON), resolves it into a zone mask, generates a TaleSpire slab, and writes
// both the base64 code and a 2D top-down preview PNG.
//
// It is deliberately a thin wrapper around the internal packages so the same
// pipeline can later be driven by an HTTP server (Phase 4).
//
// Usage:
//
//	generate -input testdata/castle.json -out out/castle.txt -preview out/castle.png
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	talespire "github.com/kirkanoskun/antoine-talespire-map-generator"
	"github.com/kirkanoskun/antoine-talespire-map-generator/internal/generator"
	"github.com/kirkanoskun/antoine-talespire-map-generator/internal/ir"
	"github.com/kirkanoskun/antoine-talespire-map-generator/internal/prefab"
	"github.com/kirkanoskun/antoine-talespire-map-generator/internal/preview"
	"github.com/kirkanoskun/antoine-talespire-map-generator/internal/spatial"
)

func main() {
	input := flag.String("input", "", "path to the IR JSON file (required)")
	out := flag.String("out", "", "path to write the TaleSpire base64 code (default: stdout)")
	previewPath := flag.String("preview", "", "path to write a PNG preview (optional)")
	biomes := flag.String("biomes", "configs/biomes.json", "path to biomes.json")
	props := flag.String("props", "configs/props.json", "path to props.json")
	scale := flag.Int("scale", 8, "preview pixels per tile")
	seed := flag.Int64("seed", 1, "random seed for reproducible generation")
	transition := flag.Int("transition", 3, "zone-border stitching half-width in tiles (0 = hard borders)")
	slice := flag.Int("slice", 0, "slice the map into slabs of at most N tiles per side (0 = single slab)")
	prefabsPath := flag.String("prefabs", "", "optional external prefab catalogue instead of the embedded catalogue")
	flag.Parse()
	builtin := talespire.PrefabCatalog()
	if *prefabsPath != "" {
		builtin = nil
	}
	prefabs, err := prefab.Open(*prefabsPath, builtin)
	if err != nil {
		log.Fatalf("loading prefabs: %v", err)
	}

	if *input == "" {
		flag.Usage()
		log.Fatal("missing required -input")
	}

	data, err := os.ReadFile(*input)
	if err != nil {
		log.Fatalf("reading input: %v", err)
	}
	doc, err := ir.Parse(data)
	if err != nil {
		log.Fatalf("invalid IR: %v", err)
	}

	mask := spatial.Resolve(doc)

	gen, err := generator.New(*biomes, *props)
	if err != nil {
		log.Fatalf("initialising generator: %v", err)
	}
	gen.SetPrefabs(prefabs)
	gen.SetTransitionHalfWidth(*transition)
	gen.SetSliceSize(*slice)
	res, err := gen.Generate(doc, mask, *seed)
	if err != nil {
		log.Fatalf("generating: %v", err)
	}

	for _, wmsg := range res.Warnings {
		log.Printf("warning: %s", wmsg)
	}

	if *previewPath != "" {
		if err := os.MkdirAll(filepath.Dir(*previewPath), 0o755); err != nil {
			log.Fatalf("creating preview dir: %v", err)
		}
		if err := preview.SavePNG(*previewPath, doc, mask, res.Height, preview.Options{Scale: *scale}); err != nil {
			log.Fatalf("writing preview: %v", err)
		}
		log.Printf("preview written to %s", *previewPath)
	}

	if *out == "" {
		fmt.Println(res.Code)
	} else {
		if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
			log.Fatalf("creating out dir: %v", err)
		}
		if err := os.WriteFile(*out, []byte(res.Code), 0o644); err != nil {
			log.Fatalf("writing code: %v", err)
		}
		log.Printf("TaleSpire code written to %s (%d bytes)", *out, len(res.Code))
		// When sliced, write each slab next to -out as <base>.<x>_<y>.<ext>.
		if res.Slices != nil {
			ext := filepath.Ext(*out)
			base := strings.TrimSuffix(*out, ext)
			for sx := range res.Slices {
				for sy := range res.Slices[sx] {
					p := fmt.Sprintf("%s.%d_%d%s", base, sx, sy, ext)
					if err := os.WriteFile(p, []byte(res.Slices[sx][sy]), 0o644); err != nil {
						log.Fatalf("writing slice: %v", err)
					}
				}
			}
			log.Printf("wrote %dx%d slices next to %s", len(res.Slices), len(res.Slices[0]), *out)
		}
	}

	log.Printf("generated %q: %d zones, %d assets, %d tiles",
		doc.Map.Name, len(doc.Zones), res.AssetCount, doc.Map.Width*doc.Map.Length)
}
