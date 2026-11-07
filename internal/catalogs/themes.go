package catalogs

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Theme is a curated selection of catalogue objects for one kind of scene
// (configs/themes.json). The model sees themes, never the whole catalogue.
type Theme struct {
	ID      string
	Label   string // French, as the GM would name the scene
	Objects []ThemeObject
}

// ThemeObject is one object of a theme and the role it plays there.
type ThemeObject struct {
	Asset *Asset
	Role  string
}

// Label is the short French name of a theme object: its role up to the first
// comma ("fontaine" for "fontaine, pièce centrale").
func (o ThemeObject) Label() string {
	if i := strings.IndexByte(o.Role, ','); i >= 0 {
		return strings.TrimSpace(o.Role[:i])
	}
	return o.Role
}

// ParseThemes reads configs/themes.json, resolving each object by UUID (names
// repeat in the index, UUIDs do not). An unknown UUID is an error: the game
// update that removed it must be noticed.
func ParseThemes(data []byte, assets *AssetCatalog) ([]Theme, error) {
	var raw []struct {
		ID      string `json:"id"`
		Label   string `json:"label"`
		Objects []struct {
			UUID string `json:"uuid"`
			Role string `json:"role"`
		} `json:"objects"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parsing themes: %w", err)
	}
	themes := make([]Theme, 0, len(raw))
	for _, t := range raw {
		theme := Theme{ID: t.ID, Label: t.Label}
		for _, o := range t.Objects {
			a, ok := assets.ByUUID(o.UUID)
			if !ok {
				return nil, fmt.Errorf("theme %q: asset %s is not in the catalogue", t.ID, o.UUID)
			}
			theme.Objects = append(theme.Objects, ThemeObject{Asset: a, Role: o.Role})
		}
		themes = append(themes, theme)
	}
	return themes, nil
}
