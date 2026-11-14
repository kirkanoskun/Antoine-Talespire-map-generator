// Full-catalogue objects on the result screen: legend, French names, and the
// correction message when the AI names an object that does not exist.
const { chromium } = require('playwright');
const fs = require('fs');
const path = require('path');

module.exports = async function catalogue({ base, out, check }) {
  const b = await chromium.launch();
  for (const [label, viewport] of [['desk', { width: 1440, height: 900 }], ['mob', { width: 390, height: 844 }]]) {
    const ctx = await b.newContext({ viewport, permissions: ['clipboard-read', 'clipboard-write'] });
    const p = await ctx.newPage();
    const errors = []; p.on('pageerror', (e) => errors.push(e.message));
    await p.goto(base, { waitUntil: 'networkidle' });
    const doc = JSON.parse(fs.readFileSync(path.join(__dirname, '..', 'testdata', 'scene_place_village.json'), 'utf8'));
    doc.zones[0].points_of_interest.push({ prop: 'marble_fountain', position: { x: 30, y: 30 } });
    await p.click('#pasteDirect');
    await p.fill('#jsonIn', 'Voici :\n```json\n' + JSON.stringify(doc, null, 2) + '\n```');
    await p.click('#drawMap');
    await p.waitForSelector('#s4:not([hidden]) #mapCell img', { timeout: 60000 });
    const legend = await p.textContent('#legend');
    check(`${label}: legend names paving and footprints`, legend.includes('Pavés') && legend.includes('Place occupée par un objet'), legend.replace(/\s+/g, ' '));
    const zonesText = await p.textContent('#zones');
    check(`${label}: zone list names objects in French`, zonesText.includes('fontaine') && zonesText.includes('étal de fruits') && !zonesText.includes('fountain 01'), zonesText.replace(/\s+/g, ' '));
    check(`${label}: unknown prop fix shown`, await p.locator('#unknownFix').isVisible());
    const fix = await p.textContent('#unknownFixText');
    check(`${label}: fix names the made-up object`, fix.includes('« marble_fountain »'), fix);
    await p.click('#copyUnknownFix');
    const clip = await p.evaluate(() => navigator.clipboard.readText().catch(() => ''));
    check(`${label}: fix message copied`, clip === fix);
    const warn = await p.textContent('#mapWarnings');
    check(`${label}: warning in French`, warn.includes('marble fountain') && warn.includes('n’existe pas'), warn);
    await p.locator('#warningsWrap').scrollIntoViewIfNeeded();
    await p.screenshot({ path: `${out}/lot1-${label}.png`, fullPage: true });
    await p.click('#unknownNext');
    check(`${label}: "J'ai la nouvelle réponse" leads to an empty paste step`, await p.locator('#s3').isVisible() && (await p.inputValue('#jsonIn')) === '');
    check(`${label}: no page errors`, errors.length === 0, errors.join('; '));
    await ctx.close();
  }
  await b.close();
};
