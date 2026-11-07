---
target: Cartographe live site
total_score: 29
p0_count: 0
p1_count: 2
timestamp: 2026-11-07T21-47-32Z
slug: internal-server-static-index-html
---
# Critique 5: Le Cartographe live site (main @ 25a098a, after PR #14 and #15)

## Design Health Score: 29/40 (Good)

| # | Heuristic | Score | Key issue |
|---|-----------|-------|-----------|
| 1 | Visibility of system status | 3 | A retouched map redraws with no "what changed" note |
| 2 | Match system / real world | 3 | Go errors quoted in the fix message; "IR prompt" header; "Slab", "biome" |
| 3 | User control and freedom | 3 | Undo, revert, Back, reload-safe; one revert level only |
| 4 | Consistency and standards | 3 | Two amber primaries once a retouch is typed; chips on --border, not --control |
| 5 | Error prevention | 3 | No warning before a size that means 9 pastes |
| 6 | Recognition rather than recall | 3 | Sliced-map placement rule lives in the sidebar, away from the pieces |
| 7 | Flexibility and efficiency | 2 | No "copy next piece"; no link to open the AI; no PNG; no map link |
| 8 | Aesthetic and minimalist design | 3 | 10-row legend; how-to strip repeats the stepper |
| 9 | Error recovery | 3 | Fix-message copy is a ghost in the red box while "Afficher la carte" stays primary |
| 10 | Help and documentation | 3 | Nothing for "which AI works best" or after the paste |
| **Total** | | **29/40** | **Good** |

## Anti-patterns verdict
Not slop: no gradient text, glass, side stripes, hero metrics or card-grid landing; no em dashes.
Mild tells: the 4-column "Comment ça marche" strip; pill chips.
Detector: exit 0; two advisories (repeating stripes, grid gradient) on the legend swatches
that key the map's ripples, hatching and masonry: false positives, as in runs 1-4.

## Cognitive load
4 failures, almost all on the sliced step 4 (chunking, grouping, choices, working memory).
Options: step 1 = 7, step 2 = 4, step 3 = 3 (5 in a fixable error), step 4 single = 8,
step 4 at 200x200 = 25+.

## Priority issues
1. [P1] Sliced-map pasting: the grid starts below the fold (y 863 at 1440x900); instructions in
   the sidebar, controls under the map; on a phone the pieces come before their explanation;
   copy and tick are two actions per piece. Fix: one "Copier le morceau 3 sur 9" primary with a
   mini cut diagram, copying N+1 ticks N (undoable), full grid as overview. -> distill, layout
2. [P1] Phone: "Copier le code" below the fold (y 705 in 664); and TaleSpire is desktop-only, so
   the phone journey dead-ends with no hand-off. Fix: sticky export bar on narrow screens; a map
   link (JSON in the #hash, no server) to open on the computer. -> adapt, onboard
3. [P2] Jargon in recovery: "decoding IR: json: cannot unmarshal ... Go struct field ..." quoted
   to the GM; the prompt opens "TaleSpire IR prompt — version 1.04". Fix: French summary first,
   label the quote as the AI's text, rename the prompt header. -> clarify
4. [P2] Action hierarchy: during a fixable error the primary is still "Afficher la carte"; typing a
   retouch makes two amber buttons; "Copier le code TaleSpire" wraps to 2 lines and "Code copié ✓"
   to 1, so the sidebar jumps 24 px (width is held, height is not). -> polish
5. [P3] Retouch has no before/after: no "what changed" note; the revert link pushes the map down
   35 px and doesn't say it swaps. -> clarify, delight

## Persona red flags
- Jordan (first-timer): no link to open ChatGPT/Claude/Gemini on step 2; Go error quote; the
  landing sample image isn't clickable.
- Casey (mobile): export below the fold; no hand-off to desktop; small targets (Options avancées
  135x22, Afficher le code 123x22, footer links 15 px tall).
- Sam (keyboard/SR): mostly good; chips and piece cards use --border (1.36:1, fails 3:1 for UI
  boundaries); step 3 focuses the heading, not the paste field.
- Margaux (Friday-night GM): 9 copy/switch/paste/tick loops; no warning that 200x200 means 9
  pastes; the map can't be reopened next week.

## Minor
Step 3 buttons of different heights side by side; "Rétablir mon texte" underline covers the
status sentence; the empty-description error pushes the primary down; rocks and POI pins look
alike at 50x50 and POIs vanish among 2,252 rocks at 200x200; "Aucun morceau collé sur 9";
single-map export ends on a 1.6 s flash, only sliced maps get "Bonne partie !".
