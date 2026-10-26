package server_test

import (
	"bytes"
	"flag"
	"os"
	"testing"

	"github.com/kirkanoskun/antoine-talespire-map-generator/internal/generator"
	"github.com/kirkanoskun/antoine-talespire-map-generator/internal/ir"
	"github.com/kirkanoskun/antoine-talespire-map-generator/internal/preview"
	"github.com/kirkanoskun/antoine-talespire-map-generator/internal/spatial"
)

var updateSample = flag.Bool("update", false, "rewrite static/exemple-carte.png from testdata/castle.json")

// The landing page shows the castle example's real preview, so a visitor sees
// the result before starting. It must stay what the generator draws: after a
// deliberate preview change, regenerate it with
//
//	go test ./internal/server -run TestSampleMapIsCurrent -update
func TestSampleMapIsCurrent(t *testing.T) {
	raw, err := os.ReadFile(repoPath("testdata", "castle.json"))
	if err != nil {
		t.Fatal(err)
	}
	doc, err := ir.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	gen, err := generator.New(repoPath("configs", "biomes.json"), repoPath("configs", "props.json"))
	if err != nil {
		t.Fatal(err)
	}
	mask := spatial.Resolve(doc)
	res, err := gen.Generate(doc, mask, 1) // the desktop app and website seed
	if err != nil {
		t.Fatal(err)
	}
	var want bytes.Buffer
	if err := preview.WritePNG(&want, doc, mask, res.Height, preview.Options{Scale: 6}); err != nil {
		t.Fatal(err)
	}

	path := repoPath("internal", "server", "static", "exemple-carte.png")
	if *updateSample {
		if err := os.WriteFile(path, want.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	got, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(got, want.Bytes()) {
		t.Fatalf("static/exemple-carte.png is not what the generator draws today (%v); run: go test ./internal/server -run TestSampleMapIsCurrent -update", err)
	}
}
