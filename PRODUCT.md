# Product

## Register

product

## Users
TaleSpire game masters, French-speaking first (the whole UI is in French today), with
English-speaking players as a later audience. They are usually preparing a session at
their desk, sometimes at the last minute, with a scene already in their head ("a ruined
castle courtyard with a pond at the centre"). The job: turn that scene into a map they
can paste into TaleSpire, without learning a level editor. Many are not developers and
use a consumer AI assistant (ChatGPT, Claude, Gemini) as the bridge.

## Product Purpose
Le Cartographe turns a plain-language scene description into a coherent TaleSpire map:
describe → copy a prepared prompt into an AI assistant → paste back the JSON it returns
→ see a 2D top-down preview and copy the TaleSpire code. The public website runs
entirely in the browser with no AI key of its own; the desktop app can call Claude
directly. Success: a GM goes from idea to a map on their board in a few minutes, and
trusts the preview enough to paste it.

## Brand Personality
Cartographer's workshop: crafted, warm, quietly confident. Ink, compass and survey
sheet, the feel of a well-made tool on an old map-maker's bench. A tool with a soul,
never a costume. Voice: a helpful fellow GM, plain French, concrete, a little warmth,
no jargon.

## Anti-references
- Generic AI SaaS: purple/blue gradients, glassmorphism, template startup landing pages,
  "AI-powered" badges.
- Cheesy fantasy: fake aged parchment textures, illegible gothic or blackletter fonts,
  heavy skeuomorphism, dragon clip-art.
- Developer tool: raw JSON as the main surface, terminal/GitHub aesthetic, technical
  jargon (IR, slab, schema) shown to users who don't need it.

## Design Principles
1. **The map is the hero.** The preview and the moment it appears are the payoff; every
   step should lead there and the result screen should feel like a reward.
2. **Hide the plumbing.** JSON, prompts and slabs are necessary transport, not the
   experience; show them only when the user must act on them, in plain words.
3. **Guide like a fellow GM.** Every step says what to do next and why, in the table's
   language, with examples drawn from real scenes.
4. **Crafted, not costumed.** The cartographic identity lives in typography, colour and
   detail, never in fake textures or ornament that costs legibility.
5. **Respect the round trip.** Copying out to an AI and pasting back is the fragile
   part; make it obvious, forgiving and recoverable.

## Accessibility & Inclusion
WCAG 2.2 AA: text contrast ≥ 4.5:1 (3:1 for large text and UI components), full keyboard
operation with visible focus, `prefers-reduced-motion` respected, status changes
announced to screen readers. The 2D preview must not rely on colour alone to distinguish
zones. UI strings should be ready for an English translation later.
