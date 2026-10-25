package talespire

import (
	"github.com/kirkanoskun/antoine-talespire-map-generator/internal/prefab"
	"testing"
)

// Every code added from a conversation must pass this check before publication.
func TestEmbeddedPrefabCatalogue(t *testing.T) {
	if _, err := prefab.Open("", PrefabCatalog()); err != nil {
		t.Fatal(err)
	}
}
