---
target: Cartographe live site
total_score: 30
p0_count: 0
p1_count: 1
timestamp: 2026-10-26T21-50-39Z
slug: internal-server-static-index-html
---
# Critique 4: Le Cartographe live site (main @ 70a1723)

## Design Health Score: 30/40 (Good)
Status 3, real world 3, control 3, consistency 3, prevention 3, recognition 3,
flexibility 2, minimalism 3, error recovery 4, help 3.

Detector: exit 0; two advisories (repeating stripes, grid gradient) on the legend swatches
that key the map's ripples, hatching and masonry: false positives.

## Priority issues
1. [P1] No way back to the map after a retouch (step 3 back button goes to the prompt;
   the compact mobile step bar has no buttons). -> harden, adapt
2. [P2] "Code copié ✓" can stick (double click within 1.6 s captures it as the idle label)
   and carries over to the next map. -> harden
3. [P2] "Copier" ticks "Collé dans TaleSpire": records something the app cannot know. -> clarify
4. [P2] Sliced checklist cramped in the 270 px sidebar and detached from the map; pieces use
   the zone badge style. -> layout
5. [P3] Two amber primaries on step 4; 8 choices on step 1; sample caption differs from chip 1;
   sample far down on mobile. -> distill

## Minor
"0 morceau collé" wording; "5 point d'intérêt" plural; revert link silently toggles; path
drawn as a bright yellow bar; engine progress only on step 1; zone names leak the AI's
"(zone imbriquee)"; category reflex (brown + amber + Cinzel) still predictable.
