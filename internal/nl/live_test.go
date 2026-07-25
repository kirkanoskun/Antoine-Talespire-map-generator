package nl_test

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/johnfercher/talescoder/pkg/decoder"

	"github.com/kirkanoskun/antoine-talespire-map-generator/internal/generator"
	"github.com/kirkanoskun/antoine-talespire-map-generator/internal/nl"
	"github.com/kirkanoskun/antoine-talespire-map-generator/internal/spatial"
)

// repoPath resolves a path relative to the repository root regardless of cwd.
func repoPath(parts ...string) string {
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(file), "..", "..")
	return filepath.Join(append([]string{root}, parts...)...)
}

func readDescriptions(t *testing.T) []string {
	t.Helper()
	f, err := os.Open(repoPath("testdata", "descriptions.txt"))
	if err != nil {
		t.Fatalf("open descriptions: %v", err)
	}
	defer f.Close()
	var out []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, line)
	}
	return out
}

// TestInterpretLiveDescriptions runs the real Claude interpreter over the ~10
// sample descriptions and asserts each yields a valid IR that generates a
// decodable TaleSpire slab. It is skipped unless ANTHROPIC_API_KEY is set, so
// the offline `go test ./...` stays green.
func TestInterpretLiveDescriptions(t *testing.T) {
	if os.Getenv("ANTHROPIC_API_KEY") == "" {
		t.Skip("ANTHROPIC_API_KEY not set; skipping live NL eval")
	}

	biomesPath := repoPath("configs", "biomes.json")
	propsPath := repoPath("configs", "props.json")

	catalog, err := nl.LoadCatalog(biomesPath, generator.POIAliasNames())
	if err != nil {
		t.Fatalf("catalog: %v", err)
	}
	interp := nl.NewInterpreter(nl.NewAnthropicCompleter(nl.AnthropicConfig{}), catalog)

	gen, err := generator.New(biomesPath, propsPath)
	if err != nil {
		t.Fatalf("generator: %v", err)
	}
	dec := decoder.NewDecoder()

	for i, desc := range readDescriptions(t) {
		desc := desc
		t.Run(strings.SplitN(desc, ",", 2)[0], func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()

			res, err := interp.Interpret(ctx, desc, nl.Options{Width: 50, Length: 50, MaxRetries: 2})
			if err != nil {
				t.Fatalf("description %d interpret failed: %v", i, err)
			}
			// Every zone's relief_override (if any) must already be coherent —
			// the interpreter guarantees this — and generation must succeed and
			// decode.
			gres, err := gen.Generate(res.IR, spatial.Resolve(res.IR), 1)
			if err != nil {
				t.Fatalf("description %d generate failed: %v", i, err)
			}
			if _, err := dec.Decode(gres.Code); err != nil {
				t.Fatalf("description %d produced an undecodable slab: %v", i, err)
			}
			t.Logf("biome=%s zones=%d attempts=%d assets=%d",
				res.IR.Map.Biome, len(res.IR.Zones), len(res.Attempts), gres.AssetCount)
		})
	}
}
