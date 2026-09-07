package generator

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/johnfercher/talescoder/pkg/decoder"

	"github.com/kirkanoskun/antoine-talespire-map-generator/internal/ir"
	"github.com/kirkanoskun/antoine-talespire-map-generator/internal/spatial"
)

// goldenSeed pins the RNG so the scene is reproducible.
const goldenSeed = 42

func testdataPath(name string) string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "testdata", name)
}

// placementLines decodes a generated slab and returns one sorted line per placed
// asset: "<x> <y> <z> <prop> <assetId>". The assetId (base64 GUID) is the stable
// key; the prop name is a best-effort, deterministic label (first owner in
// props.json order — some GUIDs are shared between props).
func placementLines(t *testing.T, code string) string {
	t.Helper()

	var props propCatalog
	loadJSON(t, "props.json", &props)
	id2name := map[string]string{}
	for _, p := range props {
		for _, part := range p.Parts {
			if _, seen := id2name[part.ID]; !seen {
				id2name[part.ID] = p.ID
			}
		}
	}

	slab, err := decoder.NewDecoder().Decode(code)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	var lines []string
	for _, a := range slab.Assets {
		id := base64.StdEncoding.EncodeToString(a.Id)
		name := id2name[id]
		if name == "" {
			name = "?"
		}
		for _, l := range a.Layouts {
			lines = append(lines, sprintf5(l.Coordinates.X, l.Coordinates.Y, l.Coordinates.Z, name, id))
		}
	}
	sort.Strings(lines)
	return strings.Join(lines, "\n") + "\n"
}

func sprintf5(x, y, z uint16, name, id string) string {
	// Zero-pad coords so lexical sort matches numeric order per column.
	return pad(x) + " " + pad(y) + " " + pad(z) + " " + name + " " + id
}

func pad(v uint16) string {
	s := itoa(int(v))
	for len(s) < 4 {
		s = "0" + s
	}
	return s
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	var b []byte
	for v > 0 {
		b = append([]byte{byte('0' + v%10)}, b...)
		v /= 10
	}
	return string(b)
}

// TestGoldenPlacements pins the exact placements of a fixed scene at a fixed
// seed. Any unintended change to generation (prop pools, offsets, spatial
// resolution, ordering) makes the decoded placement list drift and fails here.
//
// To intentionally update the golden after a deliberate change:
//
//	UPDATE_GOLDEN=1 go test ./internal/generator/ -run TestGoldenPlacements
func TestGoldenPlacements(t *testing.T) {
	raw, err := os.ReadFile(testdataPath("golden_scene.json"))
	if err != nil {
		t.Fatalf("read scene: %v", err)
	}
	doc, err := ir.Parse(raw)
	if err != nil {
		t.Fatalf("parse scene: %v", err)
	}
	res, err := newTestGen(t).Generate(doc, spatial.Resolve(doc), goldenSeed)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	got := placementLines(t, res.Code)

	goldenFile := testdataPath("golden_scene_placements.txt")
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.WriteFile(goldenFile, []byte(got), 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		t.Logf("updated golden: %s (%d placements)", goldenFile, strings.Count(got, "\n"))
		return
	}

	want, err := os.ReadFile(goldenFile)
	if err != nil {
		t.Fatalf("read golden (run with UPDATE_GOLDEN=1 to create it): %v", err)
	}
	if got != string(want) {
		t.Errorf("placements drifted from golden.\n%s\nRe-run with UPDATE_GOLDEN=1 if this change is intentional.",
			firstDiff(string(want), got))
	}
}

// firstDiff returns a short human-readable summary of the first differing line.
func firstDiff(want, got string) string {
	w := strings.Split(want, "\n")
	g := strings.Split(got, "\n")
	n := len(w)
	if len(g) < n {
		n = len(g)
	}
	for i := 0; i < n; i++ {
		if w[i] != g[i] {
			return "first diff at line " + itoa(i+1) + ":\n  golden: " + w[i] + "\n  got:    " + g[i]
		}
	}
	return "line counts differ: golden=" + itoa(len(w)) + " got=" + itoa(len(g))
}
