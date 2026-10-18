package catalogs

import (
	"os"
	"reflect"
	"testing"

	"github.com/johnfercher/taleslab/pkg/taleslab/taleslabrepositories"
)

// The in-memory parsers must yield exactly what taleslab's file loaders yield,
// or the website and the desktop app would generate different maps.
func TestParsersMatchTaleslabLoaders(t *testing.T) {
	const biomesPath, propsPath = "../../configs/biomes.json", "../../configs/props.json"

	wantBiomes, err := taleslabrepositories.NewBiomeRepository(biomesPath)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(biomesPath)
	if err != nil {
		t.Fatal(err)
	}
	gotBiomes, err := ParseBiomes(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(gotBiomes.GetBiomes(), wantBiomes.GetBiomes()) {
		t.Error("ParseBiomes differs from taleslab's biome loader")
	}

	wantProps, err := taleslabrepositories.NewPropRepository(propsPath)
	if err != nil {
		t.Fatal(err)
	}
	raw, err = os.ReadFile(propsPath)
	if err != nil {
		t.Fatal(err)
	}
	gotProps, err := ParseProps(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(gotProps.GetProps(), wantProps.GetProps()) {
		t.Error("ParseProps differs from taleslab's prop loader")
	}
}

func TestParsersRejectMalformedJSON(t *testing.T) {
	if _, err := ParseBiomes([]byte(`{`)); err == nil {
		t.Error("ParseBiomes accepted malformed JSON")
	}
	if _, err := ParseProps([]byte(`{`)); err == nil {
		t.Error("ParseProps accepted malformed JSON")
	}
}
