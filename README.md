# Antoine-Talespire-map-generator

Generate coherent [TaleSpire](https://talespire.com/) maps from a natural-language
description, with a 2D visual round-trip before exporting into the game.

Built on top of [johnfercher/taleslab](https://github.com/johnfercher/taleslab)
(imported as a Go module), this project adds what taleslab lacks: **semantic
zones that modulate relief and density within a single, coherent biome**,
**precise placement of points of interest**, and (in later phases) a
natural-language layer and web UI. See [CLAUDE.md](./CLAUDE.md) for the full
design and roadmap.

A map has **one dominant biome**, fixed for the whole map. Zones never change the
biome — they only modulate, within it, the prop density, the relief used (via
`relief_override`), and the explicit points of interest. Needs the biome does not
cover yet (e.g. ruins) are added as a new relief to that biome in
`configs/biomes.json`, never by borrowing a different biome.

## Status: Phases 1–2 — natural language to map

Describe a scene in French; the engine turns it into a level-design
**intermediate representation (IR)**, resolves the IR into a TaleSpire slab
(each zone in the map's single biome with its own relief/densities), and renders
a preview:

```
description  ->  Claude (validate + retry)  ->  IR (JSON)
IR (JSON)    ->  weighted-Voronoi zone mask ->  border stitching  ->  slab  ->  base64 code
                                            \->  2D top-down PNG preview
```

The spatial layer is complete: adjacent zones are **stitched** (height and prop
density smoothed across a band at each border; the material relief stays crisp),
a small zone sharing a larger zone's anchor is **nested** as a disc (a pond at
the courtyard centre), and **connections** are carved into bare corridors that
ramp between zones. The web UI and conversational iteration (Phase 4) are not
implemented yet.

## Quick start

```bash
# Phase 2 — from a French description (needs ANTHROPIC_API_KEY)
go run ./cmd/describe \
  -description "une cour de château en ruines, une mare au sud, des vestiges au nord" \
  -out out/castle.ir.json \
  -code out/castle.txt \
  -preview out/castle.png

# Phase 1 — from a hand-written IR
go run ./cmd/generate \
  -input testdata/castle.json \
  -out out/castle.txt \
  -preview out/castle.png

# Paste the contents of out/castle.txt into TaleSpire.

go test ./...                                 # offline; the live NL eval skips without a key
ANTHROPIC_API_KEY=... go test ./internal/nl/  # runs the ~10-description eval
```

### `describe` flags

| flag                 | default            | meaning                                  |
|----------------------|--------------------|------------------------------------------|
| `-description`       | *(required*)       | the scene, in natural language           |
| `-description-file`  | —                  | read the description from a file instead |
| `-width` / `-length` | `50`               | map size in tiles                        |
| `-biome`             | *(model chooses)*  | force the map biome                      |
| `-model`             | `claude-opus-4-8`  | Claude model id                          |
| `-max-retries`       | `2`                | retries on invalid IR                    |
| `-out`               | stdout             | write the IR JSON                        |
| `-code` / `-preview` | —                  | also generate the slab / PNG            |

The interpreter validates the model's IR strictly (schema + biome/relief
coherence) and, on any failure, feeds the exact error back and retries.

### CLI flags

| flag       | default              | meaning                                  |
|------------|----------------------|------------------------------------------|
| `-input`   | *(required)*         | IR JSON file                             |
| `-out`     | stdout               | where to write the base64 TaleSpire code |
| `-preview` | *(none)*             | write a top-down PNG preview             |
| `-biomes`  | `configs/biomes.json`| biome catalogue                          |
| `-props`   | `configs/props.json` | prop catalogue                           |
| `-scale`   | `8`                  | preview pixels per tile                  |
| `-seed`    | `1`                  | seed for reproducible generation         |
| `-transition` | `3`               | border stitching half-width (0 = hard)   |

## The IR format

A zone-based, engine-independent description of a map. Minimal example
(`testdata/twozones.json`) — one biome, two zones differing by relief and
density:

```json
{
  "map": { "width": 40, "length": 40, "name": "Two zones, one border",
           "biome": "temperate_forest" },
  "zones": [
    { "id": "dense_forest",
      "anchor": { "x": 10, "y": 20 }, "relative_size": 0.5, "elevation": "flat",
      "density_overrides": { "vegetation": 0.35 } },
    { "id": "rocky_clearing",
      "anchor": { "x": 30, "y": 20 }, "relative_size": 0.5, "elevation": "hill",
      "relief_override": "mountain",
      "density_overrides": { "vegetation": 0.02, "stones": 0.3 } }
  ]
}
```

Map-level:
- **biome**: the single biome for the whole map — one of `temperate_forest`,
  `subtropical_forest`, `dead_forest`, `swamp`, `desert`, `tundra`, `lava`,
  `beach`.

Per zone (a zone modulates the biome, it never replaces it):
- **anchor**: approximate tile position (the Voronoi resolver turns anchors +
  `relative_size` into exact per-tile zones).
- **relative_size**: `(0,1]`; larger zones claim more area.
- **elevation**: `flat`, `hill`, `mountain`, or `depression` (deep depressions
  become water). Controls **height**.
- **relief_override**: optional; forces the **material** relief for the zone
  (must be a relief of the map's biome, e.g. `water`, `mountain`, `ruins`). Wins
  over the height-derived relief; elevation still sets the height.
- **density_overrides**: optional per-category weights (`vegetation`, `stones`,
  `misc`).
- **points_of_interest**: explicit props placed deterministically, either at an
  `{x,y}` or `"scattered"` with a `count`.

Give a small zone the **same anchor** as a larger one to nest it (a pond at the
centre of a courtyard). Add a `connections` entry `{from, to, type, width}` to
carve a walkable, ramped path between two zones.

See `testdata/castle.json` for the full brief example (a ruined castle courtyard
with a pond and northern ruins).

## Project layout

```
cmd/generate/         CLI: IR -> map
cmd/describe/         CLI: description -> IR -> map
internal/ir/          IR types, strict parsing & validation
internal/spatial/     zone mask (Voronoi + nesting), transitions, path carving
internal/generator/   zone-aware slab generation, stitching, paths, encoding
internal/preview/     top-down 2D PNG renderer
internal/nl/          natural language -> IR (Claude call, validate + retry)
configs/              biome & prop catalogues (from taleslab)
testdata/             example IR documents + NL descriptions
```

## License

Inherits the terms of the upstream taleslab project for reused assets/config.
