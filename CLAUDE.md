# CLAUDE.md — TaleSpire map generator from natural language

Guidance for Claude Code (and humans) working in this repository. Read this
first. It summarises the project brief and records the concrete decisions taken
so far.

## Vision

Describe a scene in plain French (e.g. *"une cour de château en ruines, avec une
mare au centre et des arbres morts autour, un chemin qui monte vers des vestiges
au nord"*) and get a **coherent** TaleSpire map, with a 2D visual round-trip
before the final export into the game.

The highest ambition level is the target: the model reasons about the **whole
map layout like a level designer**, not merely picking parameters inside one
existing biome.

## Core rule: one biome per map (coherence over collage)

A map has **one dominant biome**, fixed once for the whole map (e.g.
`temperate_forest`). This is a hard rule everywhere in the project — the goal is
coherent scenes, not collages of unrelated biomes touching each other.

**Zones never change the biome.** Within the single map biome, a zone only
modulates:
- **prop density** (more/fewer trees, stones, …) via `density_overrides`;
- **the relief used** (`water`, `base_ground`, `ground`, `mountain`, or a new
  relief such as `ruins`) via an optional `relief_override`;
- **explicit points of interest** placed deterministically (altar, columns, …).

If a description needs something the chosen biome does not have yet (e.g. ruins
in a temperate forest), the correct response is to **add a new relief entry to
that biome in `configs/biomes.json`** (with building blocks and props coherent
with the rest of the biome) — *not* to borrow another existing biome for that
zone. The `ruins` relief on `temperate_forest` is the worked example.

## What we reuse from taleslab, what we go beyond

Base inspiration: [johnfercher/taleslab](https://github.com/johnfercher/taleslab),
a Go procedural map generator for TaleSpire. We import it as a **Go module
dependency** (`github.com/johnfercher/taleslab`) rather than forking, and build
our own layer on top of its exported pieces.

Reused as-is:
- Biome/prop catalogues (`configs/biomes.json`, `configs/props.json`) and the
  repositories that load them (`taleslabrepositories`).
- Terrain/relief entities and the height-noise, mountain, river, canyon, slicer
  algorithms (`pkg/shared/grid`).
- The TaleSpire encoder (`talescoder`).

What taleslab does **not** do, and is the real subject of this project:
- No natural-language layer — everything is hand-filled Go structs.
- No **modulation within a biome**. A `MapGeneration` applies a single relief
  system uniformly (height noise decides `ground`/`mountain`/`water`); it cannot
  express "this region is paved courtyard, that region is ruins, that one is a
  pond" as deliberate, placed zones inside one coherent biome.
- No **precise placement** of individual elements ("an altar at the centre of
  the ruins"). All prop placement is weighted-random.

## Pipeline (8 steps)

1. **NL → IR**: a Claude API call turns the free description into an intermediate
   JSON (see step 2). The model picks one biome and reasons about
   zones/relations/POIs, not tiles.
2. **IR (level-design intermediate representation)**: engine-independent JSON
   pivot. One **biome at the map level**; zones with approximate anchor, relative
   size, elevation, optional `relief_override`, density overrides, and explicit
   points of interest; plus connections.
3. **Deterministic spatial resolution (IR → zone mask)**: a classic geometric
   algorithm (weighted Voronoi now, relaxation later) turns approximate anchors
   + relative sizes into an exact per-tile zone mask. The LLM never computes a
   60×60 grid.
4. **Per-zone content generation**: for each tile, generate ground + props using
   the **map's single biome**, the tile's resolved relief (`relief_override` or
   the height-derived relief) and the zone's density overrides, reusing
   taleslab's catalogues. Explicit POIs are placed deterministically on top.
5. **Zone stitching / transitions**: smooth prop **density and height** across a
   few tiles at zone borders. Because every zone shares one biome, this is a
   simple intra-biome smoothing — *not* a fusion of two generation systems.
   Implemented in Phase 3 (`spatial.Transitions` + `generator.smoothHeights` and
   density blending).
6. **2D preview**: top-down image (colour per biome, height shading, POI markers)
   generated before any TaleSpire export. Enables the fast iteration loop.
7. **TaleSpire export**: encode the slab to the base64 blob via `talescoder`;
   slice into ≤30 kB slabs for large maps (Phase 5).
8. **Conversational iteration**: each follow-up ("move the pond further south")
   is a targeted edit of the in-memory IR, re-running only affected zones — not
   a full regeneration.

## Stack

- **Go** for the whole engine (reuses taleslab's algorithms; avoids reimplementing
  them). Single Go service, HTTP API later, Anthropic Go SDK for Claude calls.
- Frontend stays simple (HTML/JS) — the complexity is in generation.

## Phases

- **Phase 1 — foundation (DONE).** Import taleslab, write per-zone generation
  within one biome (relief/density modulation bounded to each zone's perimeter),
  validate two distinct zones of the same biome side by side (no careful
  transition yet).
- **Phase 2 — IR + NL (DONE, this is where we are).** IR schema finalised; a
  Claude API call (`internal/nl`) turns a French description into valid IR with
  strict schema + biome/relief validation and retry (invalid output is fed back
  to the model). `cmd/describe` runs description → IR → map end to end. Evaluated
  on ~10 varied descriptions via a live test gated on `ANTHROPIC_API_KEY`.
- **Phase 3 — spatial resolution + stitching (DONE).** Weighted-Voronoi placement
  plus: (1) border **stitching** — smooths height and prop density across a band
  at zone borders (`spatial.Transitions`, `generator.smoothHeights`); the seam
  cliff on `testdata/transition.json` drops from 5 to 3 blocks at half-width 3.
  (2) **Nested zones** — a zone swallowed by a larger one (the pond at the
  courtyard centre) is re-stamped as a disc (`spatial.stampNestedZones`,
  `testdata/nested.json`). (3) **Connection carving** — `connections` become
  bare, leveled corridors that ramp in height between zones (`spatial.BuildPaths`,
  `generator.carvePaths`).
- **Phase 4 — UI + iteration loop (DONE, this is where we are).** A Go HTTP
  server (`internal/server`, `cmd/server`) serves a single-page web UI: describe
  a scene, see the 2D preview, adjust it conversationally, copy the TaleSpire
  code. The IR is kept **in memory per session**, so a follow-up ("move the pond
  south") is `nl.Adjust` — a targeted edit of the current IR — not a fresh
  generation. The server drives the exact same pipeline as the CLIs.
- **Phase 5 — export + polish (DONE, this is where we are).** `talescoder`
  integration is end-to-end (all outputs round-trip through the real decoder in
  tests). TaleSpire's real limit is **~30 kB per slab** (a bigger slab pastes but
  fails on save / board switch — [FAQ](https://talespire.com/faq)), so the
  generator can **slice** a map into a grid of local-origin slabs
  (`SetSliceSize`, `-slice`), and warns when a single slab exceeds the limit.
  Remaining is manual: importing a generated code into an actual TaleSpire client
  (cannot be automated here).

## Current implementation (Phases 1–2)

```
cmd/generate/         CLI: IR JSON -> TaleSpire code (+ PNG preview)
cmd/describe/         CLI: NL description -> IR (-> optional code + preview)
cmd/server/           HTTP server for the web UI
internal/ir/          IR types, strict JSON parsing & validation
internal/spatial/     zone mask (Voronoi + nesting), transitions, path carving
internal/generator/   zone-aware slab generation, stitching, paths, mapper
internal/preview/     top-down 2D PNG renderer
internal/nl/          NL -> IR: Claude call, prompt, catalogue, validate+retry, Adjust
internal/server/      HTTP API + embedded single-page UI, in-memory sessions
internal/chimera/     correct decoder/encoder for community "Chimera" slabs
internal/buildings/   on-disk library of reusable buildings (manifest + slabs)
cmd/slabdecode/       vet a slab code; -levels lists its horizontal levels
cmd/buildings/        add/list/show/remove buildings in the library
cmd/composemap/       generate a map and place a library building on it
configs/              biomes.json, props.json (copied from taleslab)
testdata/             castle.json, twozones.json, transition.json, nested.json, descriptions.txt
```

Run it:

```
# Phase 1: IR -> map
go run ./cmd/generate -input testdata/castle.json -out out/castle.txt -preview out/castle.png

# Phase 2: description -> map (needs ANTHROPIC_API_KEY)
go run ./cmd/describe -description "une clairière au bord d'un étang" -code out/x.txt -preview out/x.png

go test ./...                                   # offline; live NL eval skips without a key
ANTHROPIC_API_KEY=... go test ./internal/nl/    # runs the ~10-description eval

# Phase 4: web UI (describe, preview, adjust, export) at http://localhost:8080
ANTHROPIC_API_KEY=... go run ./cmd/server
```

### Key design decisions & notes

- **taleslab as a dependency, not a fork.** We reimplement only the slice-fill
  orchestration (`internal/generator`) to be zone-aware, reusing taleslab's
  repositories, entities, grid functions and encoder. The seam is a per-tile
  **relief** lookup from the mask (within the map's single biome) instead of
  taleslab's uniform height-derived relief.
- **`relief_override` precedence.** A zone's `relief_override` wins for the
  *material* (which relief/blocks/props a tile uses); elevation still controls
  *height*. A deep depression becomes water only when the zone does not override
  the relief (see `shapeTerrain`). `relief_override` is validated against the
  map biome's reliefs at generation time — an unknown relief fails loudly.
- **Deterministic output.** taleslab's `TaleSpireSlabFromSlab` groups assets via
  Go map iteration (random order → non-reproducible base64). We use our own
  `taleSpireSlabFromSlab` (first-seen order) so a given seed yields identical
  output — important for caching and the iteration loop.
- **Nested zones via disc stamping (Phase 3).** Pure Voronoi swallows a small
  zone centred on a larger one. After the Voronoi pass, `stampNestedZones` finds
  any zone whose area is far below its normalized fair share (the swallow signal)
  and re-stamps it as a disc sized from that share, largest first. So a pond can
  sit at the exact centre of a courtyard (`testdata/nested.json`, `castle.json`).
- **New reliefs, not borrowed biomes.** Needs unmet by the biome (ruins, paving,
  …) are added as reliefs to that biome in `configs/biomes.json`. `ruins` was
  added to `temperate_forest` (broken-stone floor blocks + wall/skull props).
- **POI vocabulary is bridged.** The asset catalogue is fixed and small, so
  descriptive POI names ("broken_pillar", "altar") are mapped to real prop ids
  via `internal/generator/aliases.go`. Unknown props become warnings, never
  silent drops.
- **NL layer is provider-abstracted and validated (Phase 2).** `internal/nl`
  splits the model call (`Completer` interface; `AnthropicCompleter` is the only
  implementation) from the validate-and-retry loop, so the loop is unit-tested
  offline with a fake completer. The model gets the one-biome rule, the JSON
  schema, and a **catalogue** (each biome's real relief keys + the POI
  vocabulary) in a cache-marked system prompt. Output is parsed with `ir.Parse`
  and checked for biome/relief coherence; on any failure the exact error is fed
  back and the model retries (default 2 extra attempts). No structured-output
  schema — the IR's union/​map fields don't fit JSON-schema constraints, and
  validate+retry is what the brief asks for. Default model `claude-opus-4-8`
  with adaptive thinking.
- **Slicing for TaleSpire's ~30 kB limit (Phase 5).** A slab over ~30 kB pastes
  but fails on save / board switch. `Generate` always produces the whole-map
  `Code` (and warns if it's oversized); with `SetSliceSize(n)` it also returns a
  grid of slabs of ≤ n tiles per side (`Result.Slices`), each rebased to its own
  local origin so they paste adjacent. Coordinates deferred as `placement`s make
  this a pure re-bucketing — single-slab output is byte-identical whether or not
  slicing is enabled.

### Phase 4 web UI & iteration notes

- **Same pipeline, driven over HTTP.** `internal/server` wires the existing
  generator/spatial/preview/nl packages; no generation logic lives in the server.
  `/api/describe` (NL → IR), `/api/adjust` (edit the session IR), `/api/generate`
  (accept an edited IR directly, no LLM), `/api/preview` (PNG). The single-page
  UI is embedded via `go:embed` — one binary, no external assets.
- **In-memory sessions = the iteration loop.** Each session holds the current
  IR. An adjustment ("move the pond south") calls `nl.Adjust`, which hands the
  model the current IR + the instruction and asks for the full updated IR,
  validated by the same validate+retry loop. Regeneration is cheap (ms), so the
  brief's "re-run only affected zones" is unnecessary for compute; the value of
  keeping the IR is that edits are *relative to the current map*, not a fresh
  description. The `/api/generate` path also lets the UI apply hand-edited IR.
- **Keyless prompt is interactive (v1.03).** `nl.BuildPrompt` (the "Générer le
  prompt" button) opens with a `# Before anything else: ask 3 clarifying questions`
  section (density/mood, focal point, circulation) and a relaxed output contract:
  the assistant asks the three questions and waits before emitting JSON (or skips
  if told to). This is added only in the interactive prompt — `systemPrompt(false)`
  used by the automated `/api/describe` path stays single-shot JSON, since its
  reply must satisfy the validate/retry loop. `nl.PromptVersion` carries the
  wording revision and is printed at the top of the generated prompt.
- **Runs without a key.** With no Anthropic credential the server still serves
  the UI and the `/api/generate` (paste/edit IR) path; `/api/describe` and
  `/api/adjust` return 503 with a clear message.
- **Desktop app packaging.** `cmd/server` is the double-click app: it embeds the
  UI (`go:embed`) and the asset catalogues (root `embed.go` → written to a temp
  dir at startup, so no working-directory dependency), binds `127.0.0.1` (with an
  ephemeral-port fallback), auto-opens the browser, and exposes `/api/quit`
  (wired to a graceful `http.Server.Shutdown` via `Options.OnQuit`) so the UI's
  Quit button can stop it — there is no terminal to Ctrl+C when double-clicked.
  `.env` is loaded from cwd, next to the executable, and `~/.talespire/.env` (a
  GUI app inherits no shell env). `scripts/build-{mac,windows}.sh` produce a
  `.app` bundle and a no-console `.exe`. The generation logic is untouched — this
  is packaging only.
- **Branding.** The macOS app is named "Le Cartographe, a TaleSpire Map
  Generator" (menu-bar name "Le Cartographe"), with the compass/mountains logo
  as its icon. The `.icns` is committed at `assets/icon.icns` and generated from
  `assets/logo.png` by `scripts/make-icns.py` (Pillow, no Xcode Command Line
  Tools needed); `build-mac.sh` copies it into the bundle and sets
  `CFBundleIconFile` (still builds without it). The same logo lives in the web UI
  — the emblem in the header mark and the full badge as the landing hero —
  served from `internal/server/static/` via the `/static/` route.
- **macOS build is host-arch by default.** `build-mac.sh` detects the host
  architecture (`uname -m`) and builds only that, so it needs nothing but the Go
  toolchain (no `lipo`/Command Line Tools). `--universal` still fuses an
  Intel+Apple-Silicon binary when `lipo` is available.

### Phase 3 stitching notes

- **Transition band via bounded BFS.** `Mask.Transitions(halfWidth)` seeds every
  seam tile (a tile adjacent to a different zone) at distance 1 and grows inward,
  staying within each tile's own zone, up to the half-width. Each in-band tile
  records the foreign zone across its nearest seam and a blend factor (0.5 at the
  seam, fading to 0 at the band edge) — so a seam tile blending 0.5 toward its
  neighbour meets the neighbour blending 0.5 back, converging to the average.
- **Height** is smoothed by averaging in-band tiles over a few passes (fresh
  buffer per pass for order-independence). Water tiles keep their height (a pond
  stays a pond) but still pull neighbours down, so a hill slopes to the water's
  edge. **Density** is blended: near a border the effective category weight lerps
  toward the neighbouring zone's weight. The **material relief stays sharp** — a
  tile is either ground or mountain; only height and density are smoothed, which
  is exactly the intra-biome model (no material fusion).
- **Off by default? No — on by default** (half-width 3), disable with
  `SetTransitionHalfWidth(0)` or `-transition 0`. Single-zone maps have no seams,
  so stitching is a no-op there (Phase 1/2 outputs are unchanged).

### Phase 3 connection & nesting notes

- **Connections carved as ramped corridors.** `spatial.BuildPaths` rasterizes
  each connection as a thick segment (its `width`, default 3) between the two
  zones' anchors, recording per tile the parameter t in [0,1] along the path.
  `generator.carvePaths` then levels each path tile to a linear height ramp
  between the two anchors' heights (endpoints snapshotted first) and sets its
  relief to bare `base_ground`; `placeProps` skips path tiles so the corridor
  stays clear. A path from a low courtyard to high ruins climbs.
- **Order of operations.** shapeTerrain → transitions + height smoothing →
  carve paths → place ground/props/POIs. Paths override the smoothed heights so
  the corridor is crisp; props are skipped on it.

### Open risks (from the brief)

- Spatial resolution (steps 3 & 5) — DONE: stitching, nested zones, connection
  carving. Possible future work: Voronoi relaxation for rounder cells, and paths
  that route around obstacles rather than straight lines.
- IR reliability: validate the model's IR strictly server-side, retry on invalid
  — DONE (`internal/nl`).
- Claude API call count/cost per session (one initial + one per adjustment) —
  still worth watching; the system prompt is cache-marked to reduce cost.
- TaleSpire's max slab size — RESOLVED: ~30 kB per slab, slicing implemented.
  The one remaining unautomatable step is importing a code into a real client.

### Community slab import (exploration, NOT wired in)

- **Goal.** Import real player-made buildings (Tales Tavern / TalesBazaar,
  "Chimera" format) and drop them into generated maps, chosen from the prompt.
  The realistic path is a hand-curated, decoded-once library, not live scraping
  (both sites need a human click to copy a slab code).
- **Blocker fixed: the Y axis.** `talescoder` v1.0.5 (latest) decodes a real
  slab's horizontal-depth (Y) axis wrongly. TaleSpire packs each placement as a
  64-bit little-endian blob with 18-bit fields (X@0, Z@18, Y@36, rot@54, rot in
  15° steps); talescoder reads byte-aligned 16-bit fields, so Y is 4 bits off and
  its high bits leak into "rotation". It is only accidentally right for
  grid-snapped tiles (rawY a multiple of 100), which is why a building's floor
  decoded fine but its offset/rotated pieces ran to impossible Y (0..1009).
  Confirmed against LuPro/SlabelFish (the reference Chimera implementation).
- **`internal/chimera`** is a standalone, tested decoder with the correct bit
  layout (round-trip + bug-reproduction + full gzip/parse tests). Nothing in the
  generator imports it. `cmd/slabdecode` vets a pasted slab code (prints axis
  ranges).
- **VALIDATED on a real community slab** ("Smiling Goat Inn", 318 distinct
  assets / 5449 placements): X 0–29.93, **Y 0–37.00**, Z 0–17.39 tiles — a
  coherent ~30×37 footprint 17 tiles tall, where talescoder gave Y up to 1009.
  Two independent confirmations of the bit layout fell out of the same data:
  rotations decode as **exactly 24 distinct steps of 15°** (0–345, none off the
  step), and X/Y sit on a clean 100-unit lattice.
- **Scale: 100 units per tile horizontally, 50 per step vertically** (a vertical
  step is half a tile). Both were settled by round-tripping the generator's own
  output — a map at height 1 encodes to `rawZ=50`, and a 6-tile map to
  `rawX/rawY` of 0,100,…,500 — and are consistent with the community slab, whose
  floor slabs sit 400 units (8 steps) apart. `Raw{X,Y,Z}` stay exposed so any
  rescale is lossless.
- **`internal/buildings` is the library.** A slab is decoded, measured and filed
  once under an id (`buildings add`), then reused by name. The manifest records
  provenance (source / author / licence) and the measurements that matter for
  placement: footprint, height, and the **ground level** to seat on terrain.
  The library directory (`./buildings`, or `$TALESPIRE_BUILDINGS`) is
  **gitignored** — it holds other people's builds, each under its own licence.
- **`cmd/composemap` places a building on a generated map** (experimental, and
  deliberately outside `internal/generator`: it composes the generator and
  chimera from the outside, so the generation pipeline stays untouched). It
  takes a library id or a file path, seats the building, clears what it
  occupies, and can slice the result.
- **Seating rule (learned the hard way).** Align the building's **ground level**
  with the terrain surface — *not* its lowest piece. Builds often carry a cellar
  or footings meant to end up buried; anchoring the bottom shoves the whole thing
  into the air (the Smiling Goat Inn has 8.5 steps below its ground floor). The
  ground level defaults to the slab's busiest level, which is almost always the
  ground floor; check it with `slabdecode -levels` and override with
  `buildings add -ground` or `composemap -building-ground`. Since TaleSpire has
  no negative Z, the whole scene is raised when the buried part needs the room,
  and the map's own ground is dropped on tiles where the build digs in.
- **Sizing reality.** A whole map plus a whole building does not fit TaleSpire's
  ~30 kB slab limit — the inn alone is 39 kB — so `composemap -slice N` is the
  normal path, not a fallback.

## Conventions

- Keep the pipeline packages thin and composable so an HTTP server (Phase 4) can
  drive the exact same code path as the CLI.
- Prefer reproducibility: thread an explicit seed, avoid Go-map iteration in any
  output path.
- Validate the IR at the boundary (`ir.Parse`) — fail loudly, never panic deep
  in generation.
