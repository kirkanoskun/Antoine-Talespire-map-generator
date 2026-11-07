// Command import-index turns TaleSpire's pack index.json files into the
// compact asset catalogue the generator embeds (configs/assets.json).
//
// Each official pack ships an index.json in TaleSpire's install folder, under
// Taleweaver/<pack-uuid>/. Re-run this after a game update:
//
//	go run ./cmd/import-index \
//	  -input configs/taleweaver/medieval-fantasy.index.json \
//	  -input configs/taleweaver/scifi.index.json \
//	  -out configs/assets.json
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/kirkanoskun/antoine-talespire-map-generator/internal/catalogs"
)

type inputs []string

func (i *inputs) String() string     { return strings.Join(*i, ",") }
func (i *inputs) Set(v string) error { *i = append(*i, v); return nil }

func main() {
	var in inputs
	flag.Var(&in, "input", "a pack index.json (repeat for each pack)")
	out := flag.String("out", "configs/assets.json", "catalogue to write")
	flag.Parse()
	if len(in) == 0 {
		log.Fatal("give at least one -input index.json")
	}

	var indexes [][]byte
	for _, path := range in {
		data, err := os.ReadFile(path)
		if err != nil {
			log.Fatal(err)
		}
		indexes = append(indexes, data)
	}
	assets, err := catalogs.ImportIndexes(indexes...)
	if err != nil {
		log.Fatal(err)
	}
	data, err := catalogs.EncodeAssets(assets)
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(*out, data, 0o644); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%s: %d assets, %.0f KiB\n", *out, len(assets), float64(len(data))/1024)
}
