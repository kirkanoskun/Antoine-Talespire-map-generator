---
target: Cartographe site
total_score: 22
p0_count: 0
p1_count: 3
timestamp: 2026-10-26T09-10-32Z
slug: internal-server-static-index-html
---
# Critique: Le Cartographe (cartographe.antoinedebaets.com, main @ 9aebe36)

## Design Health Score: 22/40 (Acceptable)

| # | Heuristic | Score | Key issue |
|---|---|---|---|
| 1 | Visibility of system status | 3 | Good step indicator and busy labels; empty-description click gives no message; ~6 s WASM load with no indicator |
| 2 | Match system / real world | 2 | English "TaleSpire IR prompt" shown to French GMs; English validator errors; "slab", "board" unexplained |
| 3 | User control and freedom | 2 | Steps not clickable, browser Back leaves the site, refresh loses everything, "Recommencer" skips back to step 1 |
| 4 | Consistency and standards | 2 | vous/tu mix; 4 steps in header vs 3 in how-to; tab state desyncs after Recommencer |
| 5 | Error prevention | 2 | Pasting the AI's full reply (prose + ```json fence) is rejected; desktop path already strips fences |
| 6 | Recognition rather than recall | 3 | Chips help; step 4 has no legend for zone colours |
| 7 | Flexibility and efficiency | 2 | No Ctrl+Enter, no .json drop, no code download, no history |
| 8 | Aesthetic and minimalist design | 3 | Calm, restrained; hero badge pushes the CTA below the fold; base64 wall equals the map in weight |
| 9 | Error recovery | 1 | Raw English JSON.parse messages, no next step (input preserved) |
| 10 | Help and documentation | 2 | Nothing covers the two real failure moments (AI asks questions; paste rejected) |

## Anti-patterns verdict
LLM: not generic AI SaaS, but a capable template; fails the category-reflex check at both altitudes (fantasy tool -> dark brown + amber + Cinzel). Identity lives in the logo and font, not the working surface.
Detector (index.html, mentions-legales.html, plus rendered DOM of all 4 steps): 2 rules, same 8 hits per state.
- dark-glow x5: button.primary box-shadow rgba(224,145,63,.22) on a dark page (also flagged by the LLM review).
- low-contrast x3 (+1 on mentions légales): --muted-2 #7d6d5c on #1b1511 = 3.6:1 (step labels, eyebrow, footer).
Detector missed: round-trip fragility, missing legend/alt, em dashes, emoji, alert(), border contrast 1.29:1. No false positives.

## Priority issues
1. [P1] Paste-back rejects real AI replies (fences/prose); English parser errors. Fix: extract first { .. last }, share Go extractJSON, French actionable errors, role=alert. -> harden, clarify
2. [P1] Step 2 shows the 6k-char English prompt, says "il vous renverra un JSON" while the AI first asks 3 questions; Copier is a ghost while "J'ai le JSON" is primary. Fix: big "Copier le prompt" primary, prompt behind disclosure, lead announces the 3 questions, drop the tip box. -> distill, clarify
3. [P1] Result screen undersells the payoff: 400 px map, no legend/labels/alt, base64 wall at equal weight, building caveat always shown, tiny Copier, "tu". Fix: map first and large, legend from IR zones and POIs, one primary "Copier le code TaleSpire", code behind disclosure, conditional caveat. -> layout, bolder
4. [P2] WCAG AA gaps: --muted-2 3.6:1, input borders 1.29:1, unlabelled textareas, focus lost on step change, silent errors, colour-only zones. -> audit, harden
5. [P2] Mobile: 32 px horizontal scroll, 143 px sticky header with clipped steps; tab desync bug; no persistence across app switches. -> adapt, harden

## Persona red flags
- Margaux (French GM, never seen JSON): English prompt, unexpected AI questions, fenced paste rejected in English: bounces at step 3.
- Jordan (first-timer): "J'ai déjà un JSON" tab on first screen, jargon labels, "copier [0,1]".
- Sam (screen reader): unlabelled fields, focus dropped to body, silent errors, preview without alt.
- Casey (mobile): sideways scroll, oversized header, two app switches, no persistence.
