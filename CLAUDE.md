# CLAUDE.md — TaleSpire map generator from natural language

Guidance for Claude Code (and humans) working in this repository. Read this
first. It summarises the project brief and records the concrete decisions taken
so far.

## Vision

Describe a scene in plain French (e.g. *"une cour de château en ruines, avec une
mare au centre et des arbres morts autour, un chemin qui monte vers des vestiges
au nord"*) and get a coherent TaleSpire map, with a 2D visual round-trip before
the final export into the game.

The highest ambition level is the target: the model reasons about the **whole
map layout like a level designer**, not merely picking parameters inside one
existing biome.

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
- No **distinct semantic zones** on one map. A `MapGeneration` applies a single
  biome to the whole map (at most a single left-to-right gradient between two
  biomes). It cannot express "a dead-forest zone touching a ruins zone touching
  a paved courtyard".
- No **precise placement** of individual elements ("an altar at the centre of
  the ruins"). All prop placement is weighted-random.

## Pipeline (8 steps)

1. **NL → IR**: a Claude API call turns the free description into an intermediate
   JSON (see step 2). The model reasons about zones/relations/POIs, not tiles.
2. **IR (level-design intermediate representation)**: engine-independent JSON
   pivot. Zones with biome, approximate anchor, relative size, elevation,
   density overrides, explicit points of interest; plus connections.
3. **Deterministic spatial resolution (IR → zone mask)**: a classic geometric
   algorithm (weighted Voronoi now, relaxation later) turns approximate anchors
   + relative sizes into an exact per-tile zone mask. The LLM never computes a
   60×60 grid.
4. **Per-zone content generation**: for each tile, generate ground + props using
   *that tile's* zone biome and densities, reusing taleslab's catalogues.
   Explicit POIs are placed deterministically on top.
5. **Zone stitching / transitions**: blend soil and prop densities across a few
   tiles at zone borders. **Biggest technical risk** — taleslab only does one
   biome per map. Not yet implemented (Phase 3).
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

- **Phase 1 — foundation (DONE, this is where we are).** Import taleslab, write
  per-zone generation bounded to a perimeter, validate two different-biome zones
  side by side (no careful transition yet).
- **Phase 2 — IR + NL.** Finalise the IR schema, build the Claude prompt/call
  that turns a description into valid IR, with strict schema validation + retry.
  Test on ~10 varied descriptions.
- **Phase 3 — spatial resolution + stitching.** Zone placement algorithm and the
  border transition logic (the riskiest part). Prototype early with the simple
  case: two zones, one straight border.
- **Phase 4 — UI + iteration loop.** 2D render, web UI, in-memory IR for
  conversational adjustments.
- **Phase 5 — export + polish.** `talescoder` integration end-to-end, real
  TaleSpire import tests.

## Current implementation (Phase 1)

```
cmd/generate/         CLI: IR JSON -> TaleSpire code (+ PNG preview)
internal/ir/          IR types, strict JSON parsing & validation
internal/spatial/     weighted-Voronoi zone mask resolver
internal/generator/   zone-aware slab generation + deterministic encoder mapper
internal/preview/     top-down 2D PNG renderer
configs/              biomes.json, props.json (copied from taleslab)
testdata/             castle.json (brief example), twozones.json (acceptance)
```

Run it:

```
go run ./cmd/generate -input testdata/castle.json -out out/castle.txt -preview out/castle.png
go test ./...
```

### Key design decisions & notes

- **taleslab as a dependency, not a fork.** We reimplement only the slice-fill
  orchestration (`internal/generator`) to be zone-aware, reusing taleslab's
  repositories, entities, grid functions and encoder. The seam is per-tile
  biome lookup from the mask instead of taleslab's single X-axis gradient.
- **Deterministic output.** taleslab's `TaleSpireSlabFromSlab` groups assets via
  Go map iteration (random order → non-reproducible base64). We use our own
  `taleSpireSlabFromSlab` (first-seen order) so a given seed yields identical
  output — important for caching and the iteration loop.
- **Voronoi cannot nest zones.** A small zone whose anchor coincides with a
  larger zone's anchor is swallowed (the brief's "pond at the courtyard centre").
  Nested/contained zones need a different resolution step — a Phase 3 concern.
  `testdata/castle.json` offsets the pond so Phase 1 demonstrates *adjacent*
  zones (its explicit goal).
- **POI vocabulary is bridged.** The asset catalogue is fixed and small, so
  descriptive POI names ("broken_pillar", "altar") are mapped to real prop ids
  via `internal/generator/aliases.go`. Unknown props become warnings, never
  silent drops.
- **Single slab.** Phase 1 emits one slab for the whole map. taleslab slices at
  50 tiles; large maps may need slicing for the TaleSpire editor (revisit with
  TaleSpire's documented limits — brief section 8).

### Open risks (from the brief)

- Zone stitching (step 5) is the main technical risk — prototype the simple
  two-zone/straight-border case first.
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
