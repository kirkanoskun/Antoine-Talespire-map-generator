// Command import-slab validates a pasted code and adds it to a portable catalogue.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/kirkanoskun/antoine-talespire-map-generator/internal/prefab"
	"github.com/kirkanoskun/antoine-talespire-map-generator/internal/slab"
)

func main() {
	input := flag.String("input", "-", "Slab text file, or - for stdin")
	catalog := flag.String("catalog", "configs/prefabs.json", "portable prefab catalogue to update")
	id := flag.String("id", "", "unique snake_case id (required)")
	name := flag.String("name", "", "building name (defaults to id)")
	author := flag.String("author", "", "creator (required when the licence requires attribution, e.g. any CC BY)")
	source := flag.String("source", "", "source URL, if known")
	license := flag.String("license", "", `licence terms (required): e.g. "CC BY-NC 4.0", or the creator's explicit permission`)
	ground := flag.Int("ground", -1, "ground level in Slab units above the lowest piece (50 = one terrain step); default: auto-detect the busiest level")
	description := flag.String("description", "", "short description for the model")
	width := flag.Int("width", 0, "optional larger reserved footprint in tiles")
	length := flag.Int("length", 0, "optional larger reserved footprint in tiles")
	flag.Parse()
	var r io.Reader = os.Stdin
	if *input != "-" {
		f, err := os.Open(*input)
		if err != nil {
			log.Fatal(err)
		}
		defer f.Close()
		r = f
	}
	code, err := io.ReadAll(io.LimitReader(r, slab.MaxInputBytes+1))
	if err != nil {
		log.Fatal(err)
	}
	if *name == "" {
		*name = *id
	}
	entry := prefab.Entry{ID: *id, Name: *name, Code: string(code), Author: *author, SourceURL: *source, License: *license, Description: *description, Width: *width, Length: *length}
	if *ground >= 0 {
		entry.GroundY = ground
	}
	// This tool writes the catalogue that ships in the repository and the app:
	// a building other people made is redistributed, so its terms must be known.
	if err := prefab.CheckProvenance(entry); err != nil {
		log.Fatal(err)
	}
	store, err := prefab.Open(*catalog, nil)
	if err != nil {
		log.Fatal(err)
	}
	info, err := store.Add(entry)
	if err != nil {
		log.Fatal(err)
	}
	result, _ := json.MarshalIndent(info, "", "  ")
	fmt.Println(string(result))
	fmt.Fprintf(os.Stderr, "ground level: %d units (%.1f terrain steps) above the lowest piece", info.GroundY, float64(info.GroundY)/50)
	if *ground < 0 {
		fmt.Fprint(os.Stderr, " — auto-detected; if this is not the ground floor, re-import with -ground")
	}
	fmt.Fprintln(os.Stderr)
}
