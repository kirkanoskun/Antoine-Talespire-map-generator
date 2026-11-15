// The visitor's whole journey through the built website (dist/web), on a
// desktop and on a phone: describe, copy the prompt, paste the AI's reply, read
// the map, retouch it, paste a sliced map piece by piece. Run by e2e/run.js.
const { chromium } = require('playwright');
const fs = require('fs');
const path = require('path');

const castle = fs.readFileSync(path.join(__dirname, '..', 'testdata', 'castle.json'), 'utf8');
const fenced = `Parfait, voici votre carte :\n\n\`\`\`json\n${castle}\n\`\`\`\n\nBonne partie !`;
let base, out, check;

async function pass(label, viewport) {
  const browser = await chromium.launch();
  const phone = viewport.width < 600;
  const ctx = await browser.newContext({ viewport, isMobile: phone, hasTouch: phone, permissions: ['clipboard-read', 'clipboard-write'] });
  const p = await ctx.newPage();
  const errors = [];
  p.on('pageerror', (e) => errors.push(e.message));
  p.on('console', (m) => { if (m.type() === 'error') errors.push(m.text()); });
  const shot = (n) => p.screenshot({ path: `${out}/${label}-${n}.png`, fullPage: true });
  const overflow = () => p.evaluate(() => document.documentElement.scrollWidth - innerWidth);
  const visibleStep = () => p.evaluate(() => [1, 2, 3, 4].find((i) => !document.getElementById('s' + i).hidden));

  await p.goto(base, { waitUntil: 'networkidle' });
  // Before any full-page screenshot: taking one drops Playwright's touch emulation.
  const fieldSize = await p.$eval('#desc', (e) => getComputedStyle(e).fontSize);
  check(`${label}: description field ${phone ? 'at 16px (no iOS zoom)' : 'at body size'}`, fieldSize === (phone ? '16px' : '15px'), fieldSize);
  if (phone) {
    await p.tap('#examples .chip >> nth=2');
    const border = await p.locator('#examples .chip').nth(2).evaluate((e) => getComputedStyle(e).borderColor);
    check(`${label}: a tapped chip does not stay lit`, border !== 'rgb(224, 145, 63)', border);
    await p.fill('#desc', '');
  }
  await shot('1-landing');
  check(`${label}: no horizontal scroll on step 1`, (await overflow()) <= 0, `overflow ${await overflow()}`);
  const ctaBox = await p.locator('#genPrompt').boundingBox();
  check(`${label}: primary CTA above the fold`, ctaBox && ctaBox.y + ctaBox.height <= viewport.height, `bottom ${ctaBox && Math.round(ctaBox.y + ctaBox.height)}`);

  // A button keeps its width while its label flashes.
  check(`${label}: a flashed label never shrinks its button`, await p.evaluate(() => {
    const b = document.getElementById('genPrompt'), w = b.offsetWidth;
    flash(b, 'OK', b.textContent); const ok = b.offsetWidth >= w; resetFlash(b, 'Préparer le prompt →'); return ok && !b.style.minWidth;
  }));
  // An example of the result sits beside the form (below it on a phone).
  const sample = await p.locator('.sample img').boundingBox();
  const form = await p.locator('#desc').boundingBox();
  check(`${label}: sample map shown`, !!sample && sample.width > 200
    && (viewport.width > 860 ? sample.x > form.x + form.width : sample.y > form.y));
  // On a phone the result comes before the options, not under them.
  const chips = await p.locator('#examples').boundingBox();
  check(`${label}: sample before the examples on a phone`, viewport.width > 860 || sample.y < chips.y, `sample ${Math.round(sample.y)}, chips ${Math.round(chips.y)}`);
  check(`${label}: three examples only`, (await p.locator('#examples .chip').count()) === 3);
  // The sample is the first example: its caption and that chip say the same.
  const chip1 = (await p.textContent('#examples .chip >> nth=0')).trim();
  check(`${label}: sample caption quotes the first example`, (await p.textContent('.sample figcaption')).includes(chip1));
  await p.click('#examples .chip >> nth=0');
  check(`${label}: first example is the sample's description`, (await p.inputValue('#desc')).includes('vestiges au nord'));
  await p.fill('#desc', '');

  // Empty description gives an inline message, not silence.
  await p.click('#genPrompt');
  check(`${label}: empty description explained`, await p.locator('#descErr').isVisible());

  // An example never silently eats a typed description.
  await p.fill('#desc', 'Ma propre scène : un port de pêche au crépuscule.');
  await p.click('#examples .chip >> nth=1');
  await p.click('#undoExample');
  check(`${label}: typed text restorable after an example`, (await p.inputValue('#desc')).startsWith('Ma propre scène'));

  // Example -> prompt.
  await p.click('#examples .chip >> nth=0');
  await p.click('#genPrompt');
  await p.waitForSelector('#s2:not([hidden])');
  await p.waitForFunction(() => { const c = getComputedStyle(document.getElementById('s2')); return c.opacity === '1' && c.transform === 'none'; }, null, { timeout: 2000 }).catch(() => {});
  check(`${label}: the new step fades in fully`, await p.$eval('#s2', (e) => getComputedStyle(e).opacity === '1' && getComputedStyle(e).transform === 'none'));
  check(`${label}: focus moves to step heading`, await p.evaluate(() => document.activeElement.id === 't2'));
  check(`${label}: prompt folded away`, !(await p.locator('#promptText').isVisible()));
  check(`${label}: copy is the primary action`, (await p.getAttribute('#copyPrompt', 'class')).includes('primary'));
  await shot('2-prompt');
  await p.click('#copyPrompt');
  const clip = await p.evaluate(() => navigator.clipboard.readText().catch(() => ''));
  check(`${label}: prompt reaches the clipboard`, clip.includes('TaleSpire'), `${clip.length} chars`);
  check(`${label}: next step promoted after copy`, (await p.getAttribute('#goTo3', 'class')).includes('primary'));
  check(`${label}: no horizontal scroll on step 2`, (await overflow()) <= 0, `overflow ${await overflow()}`);

  // Browser Back returns to step 1, Forward to step 2.
  await p.goBack(); await p.waitForTimeout(200);
  check(`${label}: browser Back goes to step 1`, (await visibleStep()) === 1);
  await p.goForward(); await p.waitForTimeout(200);
  check(`${label}: browser Forward returns to step 2`, (await visibleStep()) === 2);

  // Step 3 errors, each in French with the right guidance.
  await p.click('#goTo3');
  for (const [name, text, expect] of [
    ['questions only', 'Avant de générer la carte, trois questions : 1) ...', 'Aucune carte trouvée'],
    ['truncated', 'Voici : ```json\n{"map": {"width": 50} "zones": []}', 'incomplète'],
    ['invalid map', '{"map":{"width":50,"length":0,"name":"x","biome":"desert"},"zones":[]}', 'ne sait pas construire'],
  ]) {
    await p.fill('#jsonIn', text);
    await p.click('#drawMap');
    await p.waitForSelector('#jsonErr:not([hidden])');
    const title = await p.textContent('#errTitle');
    check(`${label}: error "${name}" explained in French`, title.includes(expect), title);
    if (name === 'invalid map') {
      check(`${label}: correction message offered`, await p.locator('#errFix').isVisible());
      await shot('3-error');
    }
  }

  // The AI asking its questions is a calm waypoint, not a red error.
  await p.fill('#jsonIn', 'Avant de dessiner la carte : quelle ambiance voulez-vous ? Quel est le point fort ?');
  await p.click('#drawMap');
  await p.waitForSelector('#jsonErr:not([hidden])');
  check(`${label}: AI questions shown as a waypoint`, (await p.textContent('#errTitle')).includes('attend vos réponses')
    && (await p.getAttribute('#jsonErr', 'class')).includes('info'));
  check(`${label}: focus kept after a rejected paste`, await p.evaluate(() => document.activeElement.id === 'drawMap'));
  await p.fill('#jsonIn', 'Voici : ```json\n{"map": {"width": 50} "zones": []}');
  await p.click('#drawMap');
  await p.waitForSelector('#errFix:not([hidden])');
  check(`${label}: no jargon in the truncation message`, !/IR|EOF|decoding/.test(await p.textContent('#errFixText')));

  // An oversized map warns at the copy button.
  const big = JSON.parse(castle); big.map.width = 160; big.map.length = 160;
  await p.fill('#jsonIn', JSON.stringify(big));
  await p.click('#drawMap');
  await p.waitForSelector('#s4:not([hidden]) #mapCell img', { timeout: 60000 });
  const pieces = await p.locator('#slices button').count();
  check(`${label}: oversized map arrives in pieces`, pieces > 1 && !(await p.locator('#copyCode').isVisible())
    && !(await p.locator('#oversize').isVisible()), `${pieces} pieces`);
  check(`${label}: pieces numbered on the map`, (await p.locator('#mapCell .piece-label').count()) === pieces);
  // The pieces are laid out like the cut, right under the map, in its column.
  const layout = await p.evaluate(() => {
    const map = document.getElementById('mapCell').getBoundingClientRect();
    const tiles = [...document.querySelectorAll('#slices li')].map((li) => li.getBoundingClientRect());
    const firstRow = tiles.filter((r) => Math.abs(r.top - tiles[0].top) < 2).length;
    const labels = document.querySelectorAll('#mapCell .piece-label');
    const mapCols = new Set([...labels].map((t) => t.style.left)).size;
    return { below: tiles[0].top > map.bottom && tiles[0].top - map.bottom < 80, aligned: Math.abs(tiles[0].left - map.left) < 2
      && Math.abs(tiles[tiles.length - 1].right - map.right) < 2, firstRow, mapCols };
  });
  check(`${label}: piece grid under the map, aligned with it`, layout.below && layout.aligned, JSON.stringify(layout));
  check(`${label}: piece grid mirrors the cut`, layout.firstRow === layout.mapCols, `${layout.firstRow} per row, map has ${layout.mapCols} columns`);
  // The pasting checklist: copying a piece marks it copied; only the
  // visitor's tick says it is pasted and moves "next" on.
  check(`${label}: first piece is the next one`, (await p.getAttribute('#slices li:nth-child(1) button', 'class')) === 'primary'
    && (await p.getAttribute('#mapCell .piece-label[data-piece="0"]', 'class')).includes('next'));
  await p.click('#slices li:nth-child(1) button');
  const piece1 = await p.evaluate(() => navigator.clipboard.readText().catch(() => ''));
  check(`${label}: piece 1 copied`, piece1.startsWith('H4sI') && piece1.length < 30000, `${piece1.length} chars`);
  check(`${label}: copying does not tick "Collé"`, !(await p.isChecked('#piece0'))
    && (await p.getAttribute('#slices li:nth-child(1)', 'class')).includes('copied')
    && (await p.textContent('#piecesProgress')).includes('Aucun morceau collé'));
  await p.check('#piece0');
  check(`${label}: ticking moves on to the next piece`, (await p.getAttribute('#slices li:nth-child(2) button', 'class')) === 'primary'
    && (await p.textContent('#mapCell .piece-label[data-piece="0"]')).startsWith('✓'));
  check(`${label}: progress counted`, (await p.textContent('#piecesProgress')).includes(`1 morceau collé sur ${pieces}`));
  await p.reload({ waitUntil: 'networkidle' });
  await p.waitForSelector('#s4:not([hidden]) #slices li', { timeout: 60000 });
  check(`${label}: ticks survive a reload`, await p.isChecked('#piece0'));
  for (let i = 1; i < pieces; i++) await p.check(`#piece${i}`);
  check(`${label}: completion message`, (await p.textContent('#piecesProgress')).includes('la carte est complète'));
  await shot('4b-oversize');
  await p.click('#editReply');

  // A normal fenced reply with prose around it is accepted.
  await p.fill('#jsonIn', fenced);
  await p.click('#drawMap');
  await p.waitForSelector('#s4:not([hidden]) #mapCell img', { timeout: 60000 });
  await p.waitForFunction(() => document.querySelector('#mapCell img').complete);
  await shot('4-map');
  check(`${label}: fenced AI reply accepted`, (await visibleStep()) === 4);
  check(`${label}: legend rendered`, (await p.locator('#legend li').count()) >= 4, `${await p.locator('#legend li').count()} entries`);
  check(`${label}: zones numbered`, (await p.locator('#mapCell .zone-badge').count()) === 3);
  check(`${label}: zone-number legend shown as a badge`, (await p.locator('#legend .legend-badge').count()) === 1);
  check(`${label}: points of interest in the legend as a pin`, (await p.locator('#legend .swatch.pin').count()) === 1);
  const legendText = await p.textContent('#legend');
  check(`${label}: legend counts the trees and rocks drawn`, /Arbres\s*[\d\u202f ]+/.test(legendText) && /Rochers\s*\d/.test(legendText), legendText.replace(/\s+/g, ' '));
  check(`${label}: alt text summarises the map`, /zones, terrain .*arbres/.test(await p.getAttribute('#mapCell img', 'alt')));
  const b = await p.$$eval('#mapCell .zone-badge', (els) => els.map((e) => e.getBoundingClientRect()).map((r) => [r.x, r.y]));
  check(`${label}: courtyard and pond numbered apart`, Math.hypot(b[0][0] - b[1][0], b[0][1] - b[1][1]) > 40, JSON.stringify(b));

  // One primary action on the result: copying the code. Retouching becomes
  // primary only once a request is typed.
  check(`${label}: one primary action on the map`, (await p.locator('#s4 button.primary:visible').count()) === 1
    && (await p.getAttribute('#retouchBtn', 'class')).includes('ghost'));
  // Retouch: the request for the AI carries the current map and the change.
  await p.fill('#retouchText', 'déplace la mare plus au sud');
  check(`${label}: typing a retouch makes it the primary action`, (await p.getAttribute('#retouchBtn', 'class')).includes('primary'));
  await p.click('#retouchBtn');
  const req = await p.evaluate(() => navigator.clipboard.readText().catch(() => ''));
  check(`${label}: retouch request copied with the map`, req.includes('déplace la mare plus au sud') && req.includes('"zones"'));
  check(`${label}: retouch offers the way back`, await p.locator('#retouchNext').isVisible());
  await p.click('#retouchNext');
  check(`${label}: retouch leads to an empty paste step`, (await visibleStep()) === 3 && (await p.inputValue('#jsonIn')) === '');
  // The way back from step 3 is the map on screen, not the old prompt.
  check(`${label}: step 3 leads back to the map`, (await p.textContent('#backFrom3')) === '← Revenir à la carte'
    && (viewport.width > 860 || await p.locator('#stepsMap').isVisible()));
  await p.click('#backFrom3');
  check(`${label}: "Revenir à la carte" shows the map`, (await visibleStep()) === 4);
  await p.click('#retouchText');
  await p.fill('#retouchText', 'déplace la mare plus au sud');
  await p.click('#retouchBtn');
  await p.click('#retouchNext');
  // A reload before pasting the new reply must not lose the map.
  await p.reload({ waitUntil: 'networkidle' });
  await p.waitForFunction(() => !document.querySelector('.step[data-step="4"]').disabled, null, { timeout: 60000 }).catch(() => {});
  check(`${label}: map survives a reload during a retouch`, !(await p.locator('.step[data-step="4"]').isDisabled()));
  await p.fill('#jsonIn', fenced);
  await p.click('#drawMap');
  await p.waitForSelector('#s4:not([hidden]) #mapCell img', { timeout: 60000 });
  check(`${label}: no oversize warning on a normal map`, !(await p.locator('#oversize').isVisible()));
  check(`${label}: previous map can be brought back`, await p.locator('#revertMap').isVisible());
  check(`${label}: map frame keeps its shape`, await p.$eval('#mapCell', (e) => e.style.aspectRatio.replace(/\s/g, '') === '50/50'));
  check(`${label}: map has alt text`, ((await p.getAttribute('#mapCell img', 'alt')) || '').includes('Cour'));
  check(`${label}: building note hidden without buildings`, !(await p.locator('#buildingNote').isVisible()));
  check(`${label}: code folded away`, !(await p.locator('#codeBox').isVisible()));
  check(`${label}: no horizontal scroll on step 4`, (await overflow()) <= 0, `overflow ${await overflow()}`);
  await p.click('#copyCode');
  const code = await p.evaluate(() => navigator.clipboard.readText().catch(() => ''));
  check(`${label}: TaleSpire code copied`, code.startsWith('H4sI'), `${code.length} chars`);
  // A quick second click must not leave "Code copié ✓" stuck on the button.
  await p.click('#copyCode'); await p.waitForTimeout(2000);
  check(`${label}: copy button returns to its label`, (await p.textContent('#copyCode')) === 'Copier le code TaleSpire');

  // A reload keeps the flow (state in sessionStorage) and redraws the map.
  await p.reload({ waitUntil: 'networkidle' });
  await p.waitForSelector('#s4:not([hidden]) #mapCell img', { timeout: 60000 });
  check(`${label}: reload keeps the map`, (await visibleStep()) === 4);

  // "Nouvelle carte" can be undone.
  await p.click('#restart');
  await p.click('#undoRestart');
  check(`${label}: "Nouvelle carte" can be undone`, (await visibleStep()) === 4);

  // Sizes stay within 8 to 200.
  await p.click('#restart');
  await p.click('details.adv:not(#prefabsPanel) summary');
  await p.fill('#w', '900'); await p.press('#w', 'Tab');
  check(`${label}: width clamped to 200`, (await p.inputValue('#w')) === '200' && await p.locator('#sizeNote').isVisible());
  await p.fill('#w', '50'); await p.press('#w', 'Tab');

  // Keyboard: Tab from the top reaches a visibly focused control.
  await p.evaluate(() => window.scrollTo(0, 0));
  await p.click('#t1');
  await p.keyboard.press('Tab');
  const outline = await p.evaluate(() => getComputedStyle(document.activeElement).outlineStyle);
  check(`${label}: keyboard focus is visible`, outline !== 'none', outline);

  check(`${label}: no page errors`, errors.length === 0, errors.join(' | '));
  await browser.close();
}

async function slowLoad() {
  const browser = await chromium.launch();
  const p = await (await browser.newContext()).newPage();
  const cdp = await p.context().newCDPSession(p);
  await cdp.send('Network.enable');
  await cdp.send('Network.emulateNetworkConditions', { offline: false, latency: 100, downloadThroughput: 4e6 / 8, uploadThroughput: 1e6 / 8 });
  await p.goto(base);
  await p.waitForSelector('#engineStatus:not([hidden])', { timeout: 20000 }).catch(() => {});
  const text = await p.textContent('#engineText');
  check('slow: engine loading shown with progress', /Mo/.test(text) && !(await p.locator('#engineStatus').isHidden()), text);
  await p.screenshot({ path: `${out}/slow-loading.png` });
  await p.waitForSelector('#engineStatus', { state: 'hidden', timeout: 120000 });
  check('slow: loading line hidden once ready', await p.locator('#engineStatus').isHidden());
  await browser.close();
}

// The prompt must not wait for the engine: hold app.wasm back, prepare a
// prompt, then check the engine (once loaded) produces exactly the same text.
async function promptBeforeEngine() {
  const browser = await chromium.launch();
  const p = await (await browser.newContext()).newPage();
  let release;
  const held = new Promise((r) => { release = r; });
  await p.route('**/app.wasm', async (route) => { await held; await route.continue(); });
  await p.goto(base);
  await p.fill('#desc', '  Une clairière brumeuse, un menhir au centre.  ');
  await p.click('details.adv:not(#prefabsPanel) summary');
  await p.fill('#w', '64'); await p.selectOption('#biome', 'swamp');
  const t0 = Date.now();
  await p.click('#genPrompt');
  await p.waitForSelector('#s2:not([hidden])', { timeout: 5000 }).catch(() => {});
  const early = await p.evaluate(() => ({ state: window.cartographeEngine.state, prompt: document.getElementById('promptText').textContent }));
  check('prompt ready before the engine', early.state !== 'ready' && early.prompt.includes('menhir'), `${Date.now() - t0} ms, engine ${early.state}`);
  release();
  await p.waitForFunction(() => window.cartographeEngine.state === 'ready', null, { timeout: 60000 });
  const fromEngine = await p.evaluate(async () => (await (await fetch('/api/prompt', { method: 'POST', headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ description: '  Une clairière brumeuse, un menhir au centre.  ', width: 64, length: 50, biome: 'swamp' }) })).json()).prompt);
  check('early prompt identical to the engine\'s', fromEngine === early.prompt, `${early.prompt.length} vs ${fromEngine.length} chars`);
  await browser.close();
}

module.exports = async function journey(ctx) {
  ({ base, out, check } = ctx);
  await promptBeforeEngine();
  if (process.env.SLOW) await slowLoad();
  await pass('desk', { width: 1440, height: 900 });
  await pass('mob', { width: 390, height: 844 });
};
