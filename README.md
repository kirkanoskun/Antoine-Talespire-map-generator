# Le Cartographe — a TaleSpire map generator

Describe a scene in plain French and get a coherent [TaleSpire](https://talespire.com/)
map: a 2D preview first, then the code to paste into the game.

**Try it: <https://cartographe.antoinedebaets.com>** — it runs entirely in your
browser, with the AI assistant of your choice (ChatGPT, Claude, Gemini…): the
site prepares a prompt, you paste the assistant's answer back, and the map is
drawn. No account, no key, nothing collected.

Built on top of [johnfercher/taleslab](https://github.com/johnfercher/taleslab)
(imported as a Go module), this project adds what taleslab lacks: **semantic
zones that modulate relief and density within a single, coherent biome**,
**precise placement of points of interest** from TaleSpire's full catalogue, a
natural-language layer and a web UI. See [CLAUDE.md](./CLAUDE.md) for the design
and the decisions taken so far.

A map has **one dominant biome**, fixed for the whole map. Zones never change the
biome — they only modulate, within it, the prop density, the relief used (via
`relief_override`), and the explicit points of interest. Needs the biome does not
cover yet (e.g. ruins, paving) are added as a new relief to that biome in
`configs/biomes.json`, never by borrowing a different biome.

## Validation status

The automated suite is green: `go vet`, `go test -race ./...`, the engine
compiled to WebAssembly, and about 180 checks in a real browser (`e2e/`), all
run before every deployment. What only a person can check:

- [x] **In-game import, single slab.** Two generated scenes (a graveyard in a
  swamp, a paved village square) were pasted into TaleSpire: terrain, catalogue
  objects on their tiles and resting on the ground, lily pads on the water.
- [ ] **North is north.** Those two imports showed the map mirrored
  north-south compared with the preview. Fixed since (positions are flipped
  when encoding); to be confirmed in game.
- [ ] **In-game import, sliced.** A large map pasted piece by piece is still to
  be confirmed in game.
- [ ] **Real Claude call.** The path that calls Claude directly with an API key
  (`/api/describe`, `/api/adjust`, `cmd/describe`, desktop app only) has never
  run against the live model, only a fake one. The website does not use it.

## How it works

```
description  ->  prompt for your AI assistant  ->  its reply, pasted back
reply        ->  lenient reader (repairs benign slips)  ->  IR (JSON)
IR (JSON)    ->  weighted-Voronoi zone mask ->  border stitching  ->  slab  ->  base64 code
                                            \->  2D top-down PNG preview
retouch      ->  a request carrying the current IR, for the assistant  ->  new reply
```

Adjacent zones are **stitched** (height and prop density smoothed across a band
at each border; the material relief stays crisp), a small zone sharing a larger
zone's anchor is **nested** as a disc (a pond at the courtyard centre), and
**connections** are carved into bare corridors that ramp between zones. Points
of interest can name any of TaleSpire's ~2,900 tiles and props; the prompt
offers curated **scene themes** (graveyard, village square, swamp, abandoned
village). Large maps are **sliced** automatically into slabs under TaleSpire's
~30 kB limit.

The reader is forgiving on purpose: an assistant's reply with a decimal
coordinate, an extra field, a French keyword or an unknown terrain is repaired
and drawn, and the page lists what was corrected (`ir.ParseLenient`).

## Desktop app (no terminal)

`cmd/server` is a self-contained desktop app: it embeds the UI and the asset
catalogues, starts a local server, opens the web UI in your default browser, and
runs until you click **Quit**. Build a double-clickable app:

```bash
# macOS — produces "dist/TaleSpire Map Generator.app" (run on a Mac)
./scripts/build-mac.sh

# Windows — produces "dist/TaleSpire Map Generator.exe" (no console window;
# cross-compiles from any OS)
./scripts/build-windows.sh
```

The app is **unsigned**, so on first launch: macOS → right-click the `.app` →
*Open* (once); Windows → *More info* → *Run anyway*.

For the natural-language features, put your Anthropic key where the app can find
it (it inherits no shell environment when double-clicked):

- macOS: `mkdir -p ~/.talespire && echo 'ANTHROPIC_API_KEY=sk-ant-...' > ~/.talespire/.env`
- Windows: a `.env` next to the `.exe`, or `%USERPROFILE%\.talespire\.env`

Without a key, the app still runs and the "apply edited IR" path works offline.

## Website

The same app also ships as a fully static website: the generation engine is
compiled to WebAssembly and runs in the visitor's browser, so there is no server
to host. `./scripts/build-web.sh` builds it into `dist/web/`; the **Website**
GitHub workflow publishes it to Cloudflare Workers (static assets, free plan). It runs in keyless mode (the
AI features would otherwise spend the owner's key on every visitor). Setup
guide, in French: [`docs/site-web.md`](docs/site-web.md).

## Quick start (from source)

```bash
# Web UI: describe, copy the prompt, paste the reply, preview, export. Works
# without a key; with ANTHROPIC_API_KEY it can also call Claude directly. The key can be
# exported, or dropped in a .env file (auto-loaded — see .env.example); an
# exported variable wins over .env.
go run ./cmd/server        # opens http://127.0.0.1:8080 in your browser

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
| `-slice`   | `0`                  | slice into ≤N-tile slabs (0 = single)    |

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
cmd/describe/         CLI: description -> IR -> map (needs an Anthropic key)
cmd/server/           the desktop app: HTTP server + web UI
cmd/web/              the same app compiled to WebAssembly, for the website
cmd/import-index/     TaleSpire's pack indexes -> configs/assets.json
cmd/import-slab/      a community building's code -> configs/prefabs.json
web/                  the website's bridge between the page and the engine
e2e/                  browser checks on the built website
internal/ir/          IR types, strict and lenient parsing, validation
internal/spatial/     zone mask (Voronoi + nesting), transitions, path carving
internal/generator/   zone-aware slab generation, stitching, paths, objects
internal/preview/     top-down 2D PNG renderer
internal/nl/          prompt, catalogue, Claude call, validate + retry
internal/server/      HTTP API + embedded single-page UI
internal/catalogs/    catalogues parsed from memory; full catalogue and themes
internal/slab/        TaleSpire's slab binary format
internal/prefab/      imported buildings and their licences
configs/              biomes, props, the full catalogue, scene themes
docs/                 website setup and rollback, specs, reviews
testdata/             example IR documents, reference scenes, descriptions
```

## License

This project's own code is under the [MIT License](./LICENSE). What it builds
on keeps its own terms:

- [taleslab](https://github.com/johnfercher/taleslab) and
  [talescoder](https://github.com/johnfercher/talescoder), MIT, used as Go
  modules; `configs/biomes.json` and `configs/props.json` started as copies of
  taleslab's.
- `configs/taleweaver/` and `configs/assets.json` are TaleSpire's own asset
  indexes (names, identifiers, sizes; no artwork), © Bouncyrock Entertainment,
  included to make the generator usable and removed on request.
- The fonts in `internal/server/static/fonts/` are under the SIL Open Font
  License (texts alongside).

### Bâtiments importés (Slabs v2)

Collez un code dans **Bâtiments importés**, ou utilisez
`go run ./cmd/import-slab -input maison.txt -id maison_pierre -name 'Maison en pierre'`.
Le catalogue est inclus dans les prompts et l'IR peut placer ces bâtiments avec
`buildings`. Les imports locaux persistent entre les lancements ; ceux ajoutés à
`configs/prefabs.json` sont embarqués à la prochaine compilation.
Voir [le guide d'import et le parcours depuis une conversation](docs/slab-importer.md).
