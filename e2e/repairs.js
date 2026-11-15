// A sloppy AI reply is drawn anyway, and the page says what it corrected.
const { chromium } = require('playwright');

const sloppy = 'Voici la carte !\n```json\n' + `{
  "map": {"width": 40, "length": 40, "name": "Cour", "biome": "Forêt tempérée"},
  "zones": [
    {"id": "cour", "name": "Cour pavée", "anchor": {"x": 20.4, "y": 40}, "relative_size": 60,
     "elevation": "Plat", "relief_override": "cobblestone", "mood": "calme",
     "points_of_interest": [{"prop": "Fountain 01", "position": "centre"}]},
    {"id": "lac", "anchor": {"x": 5, "y": 5}, "relative_size": 0.2, "elevation": "valley", "relief_override": "lava_lake"},
  ],
  "connections": [{"from": "cour", "to": "nulle_part"}],
  "buildings": [{"prefab": "taverne", "position": {"x": 3, "y": 3}}]
}` + '\n```\nBonne partie {à vous} !';

module.exports = async function repairs({ base, out, check }) {
  const b = await chromium.launch();
  const p = await (await b.newContext({ viewport: { width: 1440, height: 900 } })).newPage();
  const errors = []; p.on('pageerror', (e) => errors.push(e.message));
  await p.goto(base, { waitUntil: 'networkidle' });
  await p.click('#pasteDirect');
  await p.fill('#jsonIn', sloppy);
  await p.click('#drawMap');
  await p.waitForSelector('#s4:not([hidden]) #mapCell img', { timeout: 60000 }).catch(() => {});
  check('repairs: a sloppy reply is drawn', await p.locator('#s4').isVisible() && !(await p.locator('#jsonErr').isVisible()),
    await p.locator('#jsonErr').isVisible() ? await p.textContent('#jsonErr') : '');
  const summary = await p.textContent('#repairsSummary');
  check('repairs: the page says it corrected the reply', /corrigé \d+ détails/.test(summary), summary);
  // Something the GM asked for is missing (a path, a building): the list is open.
  check('repairs: details open when something is missing', await p.locator('#repairsWrap').evaluate((d) => d.open));
  const list = await p.textContent('#repairsList');
  for (const [what, re] of [
    ['decimal rounded', /arrondi : 20\.4 devient 20/],
    ['paving recognised', /Sol « cobblestone » compris comme « pavés »/],
    ['unknown terrain falls back', /Sol « lava_lake » inconnu/],
    ['unknown building left out', /Bâtiment « taverne » inconnu/],
    ['dangling path ignored', /Chemin ignoré/],
    ['object name matched', /Objet « Fountain 01 » compris comme « fontaine »/],
  ]) check(`repairs: ${what}`, re.test(list), list.replace(/\s+/g, ' ').slice(0, 400));
  check('repairs: the French biome is understood, without a line saying so', (await p.textContent('#mapMeta')).includes('Forêt tempérée') && !/Biome «/.test(list));
  check('repairs: no English or code in the list', !/unknown|relief_|_dropped|undefined/.test(list), list);
  const legend = await p.textContent('#legend');
  check('repairs: the repaired map has its paving and fountain', legend.includes('Pavés') && legend.includes('Place occupée par un objet'), legend.replace(/\s+/g, ' '));
  await p.locator('#repairsWrap').scrollIntoViewIfNeeded();
  await p.screenshot({ path: `${out}/repairs.png`, fullPage: true });

  // A clean reply shows no such notice.
  await p.click('#restart');
  await p.click('#pasteDirect');
  await p.fill('#jsonIn', '{"map":{"width":30,"length":30,"biome":"desert"},"zones":[{"id":"a","anchor":{"x":15,"y":15},"relative_size":1,"elevation":"flat"}]}');
  await p.click('#drawMap');
  await p.waitForFunction(() => document.getElementById('mapTitle').textContent === 'Votre carte', null, { timeout: 60000 }).catch(() => {});
  check('repairs: nothing reported for a clean reply', await p.locator('#repairsWrap').isHidden());
  check('repairs: no page errors', errors.length === 0, errors.join(' | '));
  await b.close();
};
