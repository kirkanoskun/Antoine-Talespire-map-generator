// Package talespire is the module root. It embeds the shared asset catalogues
// so the built binaries are self-contained and do not depend on the working
// directory — important for a double-click desktop app whose cwd is unknown.
package talespire

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
)

//go:embed configs/biomes.json configs/props.json configs/prefabs.json
var configsFS embed.FS

// WriteConfigs materializes the embedded biome/prop catalogues into a fresh
// temporary directory and returns their paths plus a cleanup function. The
// generator and NL catalogue loaders take file paths (taleslab reads configs
// from disk), so the app writes the embedded copies out once at startup.
func WriteConfigs() (biomesPath, propsPath string, cleanup func(), err error) {
	dir, err := os.MkdirTemp("", "talespire-configs-*")
	if err != nil {
		return "", "", nil, err
	}
	cleanup = func() { _ = os.RemoveAll(dir) }

	for _, name := range []string{"biomes.json", "props.json"} {
		data, rerr := configsFS.ReadFile("configs/" + name)
		if rerr != nil {
			cleanup()
			return "", "", nil, fmt.Errorf("reading embedded %s: %w", name, rerr)
		}
		out := filepath.Join(dir, name)
		if werr := os.WriteFile(out, data, 0o644); werr != nil {
			cleanup()
			return "", "", nil, fmt.Errorf("writing %s: %w", name, werr)
		}
	}
	return filepath.Join(dir, "biomes.json"), filepath.Join(dir, "props.json"), cleanup, nil
}

// PrefabCatalog returns a fresh copy of the built-in community prefab catalogue.
func PrefabCatalog() []byte {
	data, _ := configsFS.ReadFile("configs/prefabs.json")
	return data
}
