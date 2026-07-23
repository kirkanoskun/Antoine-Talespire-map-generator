package nl

import (
	"fmt"
	"sort"
	"strings"
)

// systemPrompt builds the instruction prompt from the catalog. It is stable for
// a given catalog (no timestamps, deterministic ordering) so it can be prompt-
// cached across the retry turns and across descriptions.
func (c *Catalog) systemPrompt() string {
	var b strings.Builder

	b.WriteString(`You are a level designer for TaleSpire maps. You translate a scene description (usually in French) into a single JSON document called the IR (intermediate representation). You never draw tiles; you reason about zones, relief, density and points of interest.

# Absolute rule: one biome per map
A map has ONE dominant biome, chosen once for the whole map, for coherence. Zones NEVER change the biome. Within the single biome a zone only modulates:
- prop density (density_overrides),
- the relief used (relief_override, which must be a relief of the map's biome),
- explicit points of interest (points_of_interest).
Pick the single biome that best fits the whole scene. If the scene needs a material the biome does not have, use the closest relief the biome DOES have (you cannot invent reliefs).

# Coordinate system
The grid is x in [0, width) and y in [0, length). x grows to the east, y grows to the south (y=0 is north). Anchors are approximate; a solver turns anchors + relative_size into exact zones, so you do not need precision.

# JSON schema
{
  "map": { "width": int, "length": int, "name": string, "biome": string },
  "zones": [
    {
      "id": string,                       // unique, snake_case
      "anchor": { "x": int, "y": int },   // within map bounds
      "relative_size": number,            // in (0, 1]; larger zones claim more area
      "elevation": "flat"|"hill"|"mountain"|"depression",  // controls height
      "relief_override": string,          // optional; a relief of the map biome
      "density_overrides": { "vegetation": number, "stones": number, "misc": number }, // optional; each in [0,1]
      "points_of_interest": [             // optional
        { "prop": string, "position": {"x": int, "y": int} },
        { "prop": string, "position": "scattered", "count": int }
      ],
      "description": string               // optional, short
    }
  ],
  "connections": [ { "from": string, "to": string, "type": string, "width": int } ] // optional
}

Notes:
- elevation sets HEIGHT; relief_override sets the MATERIAL and wins over the height-derived relief. A deep "depression" becomes water only when there is no relief_override.
- density_overrides values are absolute weights (probability a tile gets that prop category), not multipliers. Typical values: sparse 0.02-0.1, normal 0.15-0.25, dense 0.3-0.5.
- Use points_of_interest for explicit, deliberately placed elements ("an altar at the centre", "four broken pillars around the pond").

`)

	// Biomes and their reliefs.
	b.WriteString("# Available biomes and their reliefs (relief_override must be one of the biome's reliefs)\n")
	for _, biome := range c.biomeNames() {
		reliefs := append([]string(nil), c.Biomes[biome]...)
		sort.Strings(reliefs)
		fmt.Fprintf(&b, "- %s: %s\n", biome, strings.Join(reliefs, ", "))
	}

	// POI vocabulary.
	b.WriteString("\n# Point-of-interest vocabulary (prefer these names; they resolve to real props)\n")
	b.WriteString(strings.Join(c.POINames, ", "))
	b.WriteString("\n")

	b.WriteString(`
# Output contract
Return ONLY the JSON document. No markdown, no code fences, no commentary before or after. It must parse as a single JSON object and satisfy every constraint above.`)

	return b.String()
}

// userPrompt frames a single description with the requested map parameters.
func userPrompt(description string, width, length int, name, forcedBiome string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Map: width=%d, length=%d", width, length)
	if name != "" {
		fmt.Fprintf(&b, ", name=%q", name)
	}
	if forcedBiome != "" {
		fmt.Fprintf(&b, "\nUse this biome for the map (do not choose another): %s", forcedBiome)
	}
	b.WriteString("\n\nDescription:\n")
	b.WriteString(strings.TrimSpace(description))
	return b.String()
}
