// Command prompthead prints the scene-independent head of the keyless AI
// prompt (nl.Catalog.PromptHead) for the website build: web/cartographe.js
// completes it with the scene's own lines while the engine is still loading.
//
// Usage: go run ./cmd/web/prompthead > dist/web/prompt-head.txt
package main

import (
	"fmt"
	"log"
	"os"

	talespire "github.com/kirkanoskun/antoine-talespire-map-generator"
	"github.com/kirkanoskun/antoine-talespire-map-generator/internal/generator"
	"github.com/kirkanoskun/antoine-talespire-map-generator/internal/nl"
)

func main() {
	catalog, err := nl.NewCatalog(talespire.BiomesJSON(), generator.POIAliasNames())
	if err != nil {
		log.Fatalf("loading catalog: %v", err)
	}
	fmt.Fprint(os.Stdout, catalog.PromptHead())
}
