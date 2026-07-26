// Command server runs the Phase 4 web app: a single page where you describe a
// scene, see a 2D preview, adjust it conversationally, and export the TaleSpire
// code. It drives the exact same pipeline as the CLIs.
//
//	go run ./cmd/server -addr :8080
//
// The natural-language endpoints need an Anthropic credential (ANTHROPIC_API_KEY);
// without one the server still runs and the "apply edited IR" path works offline.
// A `.env` file in the working directory is loaded automatically (existing
// environment variables take precedence over it).
package main

import (
	"flag"
	"log"
	"net/http"

	"github.com/joho/godotenv"

	"github.com/kirkanoskun/antoine-talespire-map-generator/internal/generator"
	"github.com/kirkanoskun/antoine-talespire-map-generator/internal/nl"
	"github.com/kirkanoskun/antoine-talespire-map-generator/internal/server"
)

func main() {
	// Load .env from the working directory if present (ignored when absent);
	// real environment variables win over it.
	_ = godotenv.Load()

	addr := flag.String("addr", ":8080", "listen address")
	biomes := flag.String("biomes", "configs/biomes.json", "path to biomes.json")
	props := flag.String("props", "configs/props.json", "path to props.json")
	model := flag.String("model", "", "Claude model id (default claude-opus-4-8)")
	maxRetries := flag.Int("max-retries", 2, "retries on invalid IR")
	scale := flag.Int("scale", 8, "preview pixels per tile")
	seed := flag.Int64("seed", 1, "generation seed")
	transition := flag.Int("transition", 3, "zone-border stitching half-width (0 = hard)")
	slice := flag.Int("slice", 0, "slice maps into slabs of at most N tiles per side (0 = single slab)")
	flag.Parse()

	gen, err := generator.New(*biomes, *props)
	if err != nil {
		log.Fatalf("initialising generator: %v", err)
	}
	gen.SetTransitionHalfWidth(*transition)

	catalog, err := nl.LoadCatalog(*biomes, generator.POIAliasNames())
	if err != nil {
		log.Fatalf("loading catalog: %v", err)
	}
	interp := nl.NewInterpreter(nl.NewAnthropicCompleter(nl.AnthropicConfig{Model: *model}), catalog)

	srv := server.New(gen, server.Options{
		Interpreter:  interp,
		PreviewScale: *scale,
		MaxRetries:   *maxRetries,
		Seed:         *seed,
		SliceSize:    *slice,
	})

	log.Printf("listening on %s", *addr)
	if err := http.ListenAndServe(*addr, srv.Handler()); err != nil {
		log.Fatal(err)
	}
}
