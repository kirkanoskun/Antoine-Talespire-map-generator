---
target: Cartographe site (redesign)
total_score: 28
p0_count: 0
p1_count: 3
timestamp: 2026-10-26T09-54-29Z
slug: internal-server-static-index-html
---
# Critique 2: Le Cartographe after the redesign (branch claude/talespire-map-generator-m11ewg)

## Design Health Score: 28/40 (Good, low end), up from 22/40

| # | Heuristic | Score | Key issue |
|---|---|---|---|
| 1 | Visibility of system status | 3 | 14 MB WASM engine loads silently (68 s at 1.6 Mbps) |
| 2 | Match system / real world | 3 | Happy path fluent; error path leaked "decoding IR" (fixed after scoring) |
| 3 | User control and freedom | 3 | "Nouvelle carte" wipes without undo; example chip overwrote typed text (fixed after scoring) |
| 4 | Consistency and standards | 3 | Three different "copied" feedbacks |
| 5 | Error prevention | 2 | Sizes up to 200 offered; >~75x75 exceeds the 30 kB limit and the web build never slices |
| 6 | Recognition rather than recall | 3 | Step 3 no longer reminds that the AI asks questions first |
| 7 | Flexibility and efficiency | 2 | No plain-language adjust loop |
| 8 | Aesthetic and minimalist design | 3 | 4-step process explained three times on landing |
| 9 | Error recovery | 3 | AI questions shown as red error (fixed after scoring) |
| 10 | Help and documentation | 3 | Nothing for a failed TaleSpire paste |

Detector: 0 findings on all 4 rendered steps and the legal page (was 8 per page).

## Fixed right after this scoring
Questions-as-waypoint panel; plain correction messages; focus kept after a rejected paste; zone-centre legend as a badge; copies announced; example chips undoable; oversize warning at the copy button + size hint.

## Remaining priority issues
1. [P1] Oversized maps: web build has no slicing; enable SliceSize (web + desktop) so big maps export in pieces. -> harden
2. [P1] No plain-language adjust loop at step 4 ("Retoucher dans votre IA"). -> onboard, clarify
3. [P1] Preview misleads: scattered POIs not drawn, parent-zone badge sits in the nested zone, POI magenta relies on hue, ruins/mountain/path values too close. -> colorize, polish
4. [P2] Silent WASM cold load; serve compressed, show progress. -> optimize
5. [P3] Category reflex: brown + amber + Cinzel is guessable; survey identity only appears at step 4. -> bolder/delight
