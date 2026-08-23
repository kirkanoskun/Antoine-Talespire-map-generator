// Command slabdecode decodes a TaleSpire (Chimera) slab code with the corrected
// Y axis and reports whether the coordinates look sane. Use it to vet a real
// community slab before considering it for a building library.
//
//	pbpaste | go run ./cmd/slabdecode              # macOS: paste a copied slab
//	go run ./cmd/slabdecode -in slab.txt           # or from a file
//	go run ./cmd/slabdecode -in slab.txt -json     # full decoded placements
//
// It is intentionally not connected to the map generator.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/kirkanoskun/antoine-talespire-map-generator/internal/chimera"
)

func main() {
	in := flag.String("in", "", "file containing the base64 slab code (default: stdin)")
	asJSON := flag.Bool("json", false, "print the full decoded slab as JSON")
	flag.Parse()

	var data []byte
	var err error
	if *in != "" {
		data, err = os.ReadFile(*in)
	} else {
		data, err = io.ReadAll(os.Stdin)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "read:", err)
		os.Exit(1)
	}
	code := strings.TrimSpace(string(data))
	if code == "" {
		fmt.Fprintln(os.Stderr, "no slab code provided (pass -in FILE or pipe it on stdin)")
		os.Exit(1)
	}

	slab, err := chimera.Decode(code)
	if err != nil {
		fmt.Fprintln(os.Stderr, "decode:", err)
		os.Exit(1)
	}

	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(slab)
		return
	}

	// Summary: ranges are the quick sanity check — a whole building should span
	// a few dozen tiles on each axis, never hundreds.
	var placements int
	minX, minY, minZ := 1e18, 1e18, 1e18
	maxX, maxY, maxZ := -1e18, -1e18, -1e18
	for _, a := range slab.Assets {
		for _, p := range a.Placements {
			placements++
			minX, maxX = min(minX, p.TileX), max(maxX, p.TileX)
			minY, maxY = min(minY, p.TileY), max(maxY, p.TileY)
			minZ, maxZ = min(minZ, p.Height), max(maxZ, p.Height)
		}
	}

	fmt.Printf("version:    %d\n", slab.Version)
	fmt.Printf("assets:     %d distinct\n", len(slab.Assets))
	fmt.Printf("placements: %d\n", placements)
	if placements > 0 {
		fmt.Printf("X range:    %.2f .. %.2f tiles\n", minX, maxX)
		fmt.Printf("Y range:    %.2f .. %.2f tiles   <- the axis this fixes\n", minY, maxY)
		fmt.Printf("Z range:    %.2f .. %.2f (height)\n", minZ, maxZ)
	}
	fmt.Printf("\nnote: %s\n", chimera.CalibrationNote)
}
