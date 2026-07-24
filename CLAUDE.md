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
7. **TaleSpire export**: encode the slab to the base64 blob via `talescoder`.
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
- **Phase 3 — spatial resolution + stitching (stitching DONE).** Weighted-Voronoi
  placement (Phase 1) plus border transition logic — the riskiest part — now
  smooths **height and prop density** across a band of tiles at zone borders
  (`spatial.Transitions`, `generator.smoothHeights`, density blending). Validated
  on the two-zone/straight-border case (`testdata/transition.json`): the seam
  cliff drops from 5 to 3 blocks at half-width 3. **Remaining:** carving
  `connections` (paths between zones) and nested/contained zones (Voronoi can't
  nest) — deferred.
- **Phase 4 — UI + iteration loop.** 2D render, web UI, in-memory IR for
  conversational adjustments.
- **Phase 5 — export + polish.** `talescoder` integration end-to-end, real
  TaleSpire import tests.

## Current implementation (Phases 1–2)

```
cmd/generate/         CLI: IR JSON -> TaleSpire code (+ PNG preview)
cmd/describe/         CLI: NL description -> IR (-> optional code + preview)
internal/ir/          IR types, strict JSON parsing & validation
internal/spatial/     weighted-Voronoi zone mask + border transition bands
internal/generator/   zone-aware slab generation, stitching, deterministic mapper
internal/preview/     top-down 2D PNG renderer
internal/nl/          NL -> IR: Claude call, prompt, catalogue, validate+retry
configs/              biomes.json, props.json (copied from taleslab)
testdata/             castle.json, twozones.json, transition.json, descriptions.txt
```

Run it:

```
# Phase 1: IR -> map
go run ./cmd/generate -input testdata/castle.json -out out/castle.txt -preview out/castle.png

# Phase 2: description -> map (needs ANTHROPIC_API_KEY)
go run ./cmd/describe -description "une clairière au bord d'un étang" -code out/x.txt -preview out/x.png

go test ./...                                   # offline; live NL eval skips without a key
ANTHROPIC_API_KEY=... go test ./internal/nl/    # runs the ~10-description eval
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
- **Voronoi cannot nest zones.** A small zone whose anchor coincides with a
  larger zone's anchor is swallowed (the brief's "pond at the courtyard centre").
  Nested/contained zones need a different resolution step — a Phase 3 concern.
  `testdata/castle.json` offsets the pond so Phase 1 demonstrates *adjacent*
  zones (its explicit goal).
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
- **Single slab.** Phase 1 emits one slab for the whole map. taleslab slices at
  50 tiles; large maps may need slicing for the TaleSpire editor (revisit with
  TaleSpire's documented limits — brief section 8).

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

### Open risks (from the brief)

- Zone stitching (step 5) — DONE as intra-biome density/height smoothing. Next
  spatial work: connection carving and nested zones.
- IR reliability: validate the model's IR strictly server-side, retry on invalid.
- Claude API call count/cost per session (one initial + one per adjustment).
- TaleSpire's real max map size — verify before defaulting to large maps.

## Conventions

- Keep the pipeline packages thin and composable so an HTTP server (Phase 4) can
  drive the exact same code path as the CLI.
- Prefer reproducibility: thread an explicit seed, avoid Go-map iteration in any
  output path.
- Validate the IR at the boundary (`ir.Parse`) — fail loudly, never panic deep
  in generation.
