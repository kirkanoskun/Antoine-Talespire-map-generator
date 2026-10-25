// Command describe is the Phase 2 entry point: it turns a natural-language scene
// description into a validated IR document via a Claude API call, and can chain
// straight into generation (TaleSpire code + preview).
//
// It reuses the exact Phase 1 pipeline for generation, so description → map is
// one command:
//
//	describe -description "une clairière au bord d'un étang" -code out/x.txt -preview out/x.png
//
// Requires ANTHROPIC_API_KEY (or a configured Anthropic credential) for the
// interpretation step.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/joho/godotenv"

	talespire "github.com/kirkanoskun/antoine-talespire-map-generator"
	"github.com/kirkanoskun/antoine-talespire-map-generator/internal/generator"
	"github.com/kirkanoskun/antoine-talespire-map-generator/internal/nl"
	"github.com/kirkanoskun/antoine-talespire-map-generator/internal/prefab"
	"github.com/kirkanoskun/antoine-talespire-map-generator/internal/preview"
	"github.com/kirkanoskun/antoine-talespire-map-generator/internal/spatial"
)

func main() {
	// Load .env from the working directory if present; real env vars win.
	_ = godotenv.Load()

	description := flag.String("description", "", "scene description (natural language)")
	descFile := flag.String("description-file", "", "read the description from this file instead")
	width := flag.Int("width", 50, "map width in tiles")
	length := flag.Int("length", 50, "map length in tiles")
	name := flag.String("name", "", "optional map name hint")
	biome := flag.String("biome", "", "optional: force the map biome")
	model := flag.String("model", "", "optional Claude model id (default claude-opus-4-8)")
	maxRetries := flag.Int("max-retries", 2, "retries after the first attempt on invalid IR")
	irOut := flag.String("out", "", "write the IR JSON here (default: stdout)")
	codeOut := flag.String("code", "", "also generate and write the TaleSpire code here")
	previewPath := flag.String("preview", "", "also write a PNG preview here")
	biomes := flag.String("biomes", "configs/biomes.json", "path to biomes.json")
	props := flag.String("props", "configs/props.json", "path to props.json")
	scale := flag.Int("scale", 8, "preview pixels per tile")
	seed := flag.Int64("seed", 1, "generation seed")
	transition := flag.Int("transition", 3, "zone-border stitching half-width in tiles (0 = hard borders)")
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

	desc := *description
	if *descFile != "" {
		data, err := os.ReadFile(*descFile)
		if err != nil {
			log.Fatalf("reading description file: %v", err)
		}
		desc = string(data)
	}
	if desc == "" {
		flag.Usage()
		log.Fatal("provide -description or -description-file")
	}

	catalog, err := nl.LoadCatalog(*biomes, generator.POIAliasNames())
	if err != nil {
		log.Fatalf("loading catalog: %v", err)
	}

	catalog.Prefabs = prefabs
	interpreter := nl.NewInterpreter(
		nl.NewAnthropicCompleter(nl.AnthropicConfig{Model: *model}),
		catalog,
	)

	res, err := interpreter.Interpret(context.Background(), desc, nl.Options{
		Width:       *width,
		Length:      *length,
		Name:        *name,
		ForcedBiome: *biome,
		MaxRetries:  *maxRetries,
	})
	if err != nil {
		log.Fatalf("interpretation failed: %v", err)
	}
	if len(res.Attempts) > 1 {
		log.Printf("IR valid after %d attempts", len(res.Attempts))
	}

	irJSON, err := json.MarshalIndent(res.IR, "", "  ")
	if err != nil {
		log.Fatalf("marshalling IR: %v", err)
	}
	if *irOut == "" {
		fmt.Println(string(irJSON))
	} else {
		if err := writeFile(*irOut, irJSON); err != nil {
			log.Fatalf("writing IR: %v", err)
		}
		log.Printf("IR written to %s (biome %q, %d zones)", *irOut, res.IR.Map.Biome, len(res.IR.Zones))
	}

	if *codeOut == "" && *previewPath == "" {
		return
	}

	// Chain into the Phase 1 generation pipeline.
	mask := spatial.Resolve(res.IR)
	gen, err := generator.New(*biomes, *props)
	if err != nil {
		log.Fatalf("initialising generator: %v", err)
	}
	gen.SetPrefabs(prefabs)
	gen.SetTransitionHalfWidth(*transition)
	gres, err := gen.Generate(res.IR, mask, *seed)
	if err != nil {
		log.Fatalf("generating: %v", err)
	}
	for _, w := range gres.Warnings {
		log.Printf("warning: %s", w)
	}
	if *previewPath != "" {
		if err := preview.SavePNG(*previewPath, res.IR, mask, gres.Height, preview.Options{Scale: *scale}); err != nil {
			log.Fatalf("writing preview: %v", err)
		}
		log.Printf("preview written to %s", *previewPath)
	}
	if *codeOut != "" {
		if err := writeFile(*codeOut, []byte(gres.Code)); err != nil {
			log.Fatalf("writing code: %v", err)
		}
		log.Printf("TaleSpire code written to %s (%d assets)", *codeOut, gres.AssetCount)
	}
}

func writeFile(path string, data []byte) error {
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return os.WriteFile(path, data, 0o644)
}
