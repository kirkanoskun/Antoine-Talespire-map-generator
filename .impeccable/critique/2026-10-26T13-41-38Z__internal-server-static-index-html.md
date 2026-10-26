---
target: Cartographe site (PR 8)
total_score: 26
p0_count: 0
p1_count: 3
timestamp: 2026-10-26T13-41-38Z
slug: internal-server-static-index-html
---
# Critique 3: Le Cartographe, PR #8 (auto-slicing, truthful preview, retouch)

## Design Health Score: 26/40 (Acceptable), previous run 28/40

A different, deeper reviewer. Scores per heuristic: status 3, real world 3, control 2,
consistency 3, prevention 2, recognition 3, flexibility 2, minimalism 3, recovery 3, help 2.

Detector: one advisory (decorative grid gradient) on the masonry legend swatch: a false
positive, it is the key of the map's ruins texture.

## Fixed right after this scoring (in PR #8)
- Retouch hand-off emptied the saved reply: a reload before pasting lost the map. The map's
  reply is now kept apart from the draft (mapReply), and restored on reload.
- "Nouvelle carte" had no way back: inline "Carte effacée. La rétablir".
- A retouch replaced the map with no revert: "Revenir à la carte précédente".
- The map frame collapsed before the image loaded: aspect-ratio reserved.
- Width/length accepted 900: clamped to 8..200 with a note.
- The "AI is asking" heuristic now needs at least two question marks.

## Remaining priority issues
1. [P1] The preview draws no vegetation or props: dense woods and a clearing look the same,
   and "ajoute des arbres morts" changes nothing visible. -> shape, polish
2. [P1] Sliced export ends on bare numbered buttons: needs a pasting checklist (ticks kept,
   next piece highlighted, concrete placement guidance). -> onboard, clarify
3. [P2] The landing shows no example of the result. -> onboard
4. [P2] Water/ground/mountain differ by hue only; map alt text has no summary. -> colorize, harden
5. [P2] Preparing the text prompt waits for the 14 MB engine on a first visit. -> optimize
6. [P3] Category reflex (brown + amber + Cinzel) still guessable.
