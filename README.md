# Antoine-Talespire-map-generator

Generate coherent [TaleSpire](https://talespire.com/) maps from a natural-language
description, with a 2D visual round-trip before exporting into the game.

Built on top of [johnfercher/taleslab](https://github.com/johnfercher/taleslab)
(imported as a Go module), this project adds what taleslab lacks: **distinct
semantic zones on one map**, **precise placement of points of interest**, and (in
later phases) a natural-language layer and web UI. See [CLAUDE.md](./CLAUDE.md)
for the full design and roadmap.

## Status: Phase 1 — zone-aware generation

The engine turns a level-design **intermediate representation (IR)** into a
TaleSpire slab, generating each zone with its own biome/densities and stitching
them into a single map:

```
IR (JSON)  ->  weighted-Voronoi zone mask  ->  per-tile zone-aware slab  ->  base64 code
                                            \->  2D top-down PNG preview
```

Natural language (Phase 2), border transitions (Phase 3) and the web UI
(Phase 4) are not implemented yet.

## Quick start

```bash
# Generate a TaleSpire code + a preview PNG from the example IR
go run ./cmd/generate \
  -input testdata/castle.json \
  -out out/castle.txt \
  -preview out/castle.png

# Paste the contents of out/castle.txt into TaleSpire.

go test ./...
```

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

## The IR format

A zone-based, engine-independent description of a map. Minimal example
(`testdata/twozones.json`):

```json
{
  "map": { "width": 40, "length": 40, "name": "Two zones, one border" },
  "zones": [
    { "id": "west_forest", "biome": "temperate_forest",
      "anchor": { "x": 10, "y": 20 }, "relative_size": 0.5, "elevation": "flat" },
    { "id": "east_desert", "biome": "desert",
      "anchor": { "x": 30, "y": 20 }, "relative_size": 0.5, "elevation": "flat" }
  ]
}
```

- **biome**: one of `temperate_forest`, `subtropical_forest`, `dead_forest`,
  `swamp`, `desert`, `tundra`, `lava`, `beach`.
- **anchor**: approximate tile position (the Voronoi resolver turns anchors +
  `relative_size` into exact per-tile zones).
- **relative_size**: `(0,1]`; larger zones claim more area.
- **elevation**: `flat`, `hill`, `mountain`, or `depression` (deep depressions
  become water).
- **density_overrides**: optional per-category weights (`vegetation`, `stones`,
  `misc`).
- **points_of_interest**: explicit props placed deterministically, either at an
  `{x,y}` or `"scattered"` with a `count`.

See `testdata/castle.json` for the full brief example (a ruined castle courtyard
with a pond and northern ruins).

## Project layout

```
cmd/generate/         CLI entry point
internal/ir/          IR types, strict parsing & validation
internal/spatial/     weighted-Voronoi zone mask
internal/generator/   zone-aware slab generation + deterministic encoding
internal/preview/     top-down 2D PNG renderer
configs/              biome & prop catalogues (from taleslab)
testdata/             example IR documents
```

## License

Inherits the terms of the upstream taleslab project for reused assets/config.
