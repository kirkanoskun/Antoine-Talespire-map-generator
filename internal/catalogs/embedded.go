package catalogs

import (
	"sync"

	talespire "github.com/kirkanoskun/antoine-talespire-map-generator"
)

// The embedded catalogue and themes are immutable, so the generator and the
// prompt share one parsed copy (parsing ~1 MB once matters in the browser).
var (
	embeddedOnce   sync.Once
	embeddedAssets *AssetCatalog
	embeddedThemes []Theme
	embeddedErr    error
)

// Embedded returns the full catalogue and the scene themes built into the
// binary (configs/assets.json and configs/themes.json).
func Embedded() (*AssetCatalog, []Theme, error) {
	embeddedOnce.Do(func() {
		embeddedAssets, embeddedErr = ParseAssets(talespire.AssetsJSON())
		if embeddedErr == nil {
			embeddedThemes, embeddedErr = ParseThemes(talespire.ThemesJSON(), embeddedAssets)
		}
	})
	return embeddedAssets, embeddedThemes, embeddedErr
}
