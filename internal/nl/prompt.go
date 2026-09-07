package nl

import (
	"fmt"
	"sort"
	"strings"
)

// PromptVersion identifies the wording of the generated IR prompt. Bump it when
// the instruction text changes so users can tell which revision they pasted.
const PromptVersion = "1.03"

// clarifyingQuestions is the interactive preamble used only by the keyless
// "generate a prompt" flow (BuildPrompt), where the user has a real back-and-forth
// with their own AI. The automated API path (Interpret) never includes it — it is
// a single-shot call that must return JSON immediately for the validate loop.
const clarifyingQuestions = `
# Before anything else: ask 3 clarifying questions
The user's initial description is often short. Before producing any JSON, ask exactly three short clarifying questions, in the same language as the description (usually French), to refine the scene. Cover these three angles, adapted to what the description leaves open:
1. Density and mood: dense and wild, or open and calm? Any particular atmosphere (bright, eerie, mysterious)?
2. Focal point: is there one element that should clearly stand out (an altar, a campfire, a specific ruin), and roughly where?
3. Circulation: should the zones be linked by a visible path, or is a more organic layout without a marked path preferred?
Ask them together, briefly, in a single message. Wait for the user's answer before continuing. If the user says to skip the questions or just go ahead, proceed straight to the JSON using your best judgment on these three points.
`

// systemPrompt builds the instruction prompt from the catalog. It is stable for
// a given catalog (no timestamps, deterministic ordering) so it can be prompt-
// cached across the retry turns and across descriptions. When interactive is
// true it prepends the clarifying-questions preamble and relaxes the output
// contract accordingly (used by the keyless BuildPrompt flow only).
func (c *Catalog) systemPrompt(interactive bool) string {
	var b strings.Builder

	b.WriteString(`You are a level designer for TaleSpire maps. You translate a scene description (usually in French) into a single JSON document called the IR (intermediate representation). You never draw tiles; you reason about zones, relief, density and points of interest.
`)

	if interactive {
		b.WriteString(clarifyingQuestions)
	}

	b.WriteString(`
# Absolute rule: one biome per map
A map has ONE dominant biome, chosen once for the whole map, for coherence. Zones NEVER change the biome. Within the single biome a zone only modulates:
- prop density (density_overrides),
- the relief used (relief_override, which must be a relief of the map's biome),
- explicit points of interest (points_of_interest).
Pick the single biome that best fits the whole scene. If the scene needs a material the biome does not have, use the closest relief the biome DOES have (you cannot invent reliefs).

# Building and community slab limitations
The current engine places only catalogue props; it cannot import complete buildings or external Slabs. Never put a URL, base64 Slab code or invented building identifier in a prop field. For a requested building, reserve a flat area with low prop density and describe the intended building in the zone description; this reserves space only and does not construct the building. Use only the listed POI vocabulary for actual placements.

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
- Nested zones are supported: to put a small feature at the CENTRE of a larger zone (a pond in the middle of a courtyard), give the small zone the SAME anchor as the larger one. It is carved out as a disc inside its container.
- connections are carved into walkable paths that clear props and ramp in height between the two zones' anchors — use one for "a path that climbs to the ruins".

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

	if interactive {
		b.WriteString(`
# Output contract
Once the user has answered the three clarifying questions (or has told you to skip them / to go ahead), return ONLY the JSON document — no markdown, no code fences, no commentary before or after. It must parse as a single JSON object and satisfy every constraint above. Until then, ask the three clarifying questions and wait — do not output any JSON yet.`)
	} else {
		b.WriteString(`
# Output contract
Return ONLY the JSON document. No markdown, no code fences, no commentary before or after. It must parse as a single JSON object and satisfy every constraint above.`)
	}

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

// BuildPrompt returns a single self-contained prompt the user can paste into any
// AI assistant (ChatGPT, Claude, …) to get back a valid IR JSON — the "bring
// your own AI" flow, which needs no API key. It combines the schema, the
// one-biome rules and the catalogue (the same system prompt used for direct API
// calls) with the scene description and map parameters.
func (c *Catalog) BuildPrompt(description string, opts Options) string {
	if opts.Width <= 0 {
		opts.Width = 50
	}
	if opts.Length <= 0 {
		opts.Length = 50
	}
	var b strings.Builder
	fmt.Fprintf(&b, "TaleSpire IR prompt — version %s\n\n", PromptVersion)
	b.WriteString(c.systemPrompt(true))
	b.WriteString("\n\n=====\n\n")
	b.WriteString(userPrompt(description, opts.Width, opts.Length, opts.Name, opts.ForcedBiome))
	return b.String()
}

// adjustPrompt frames a conversational edit: the current IR plus the change to
// apply. The model must return the whole updated IR (same rules), changing only
// what the instruction asks for.
func adjustPrompt(currentIR, message string) string {
	var b strings.Builder
	b.WriteString("Here is the current map IR:\n")
	b.WriteString(currentIR)
	b.WriteString("\n\nApply this adjustment, changing only what it asks for and keeping everything else intact:\n")
	b.WriteString(strings.TrimSpace(message))
	b.WriteString("\n\nReturn the full updated IR as JSON only.")
	return b.String()
}
