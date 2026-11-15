// The engine's fallbacks (web/cartographe.js), with a stand-in engine so its
// failures can be staged: a download that fails then works, one that never
// works, an engine that stops mid-visit, a browser without WebAssembly.
const { chromium } = require('playwright');

// A stand-in for wasm_exec.js: "running" the engine announces it as ready and
// answers every request; window.__engine lets the test stop it.
const fakeRuntime = `
window.__engine = { starts: 0, stop: null, hang: false };
window.Go = class {
  constructor() { this.importObject = {}; }
  run() {
    const n = ++window.__engine.starts;
    return new Promise((exit) => {
      let alive = true;
      window.__engine.stop = () => { alive = false; exit(); };
      window.cartographeFetch = (method, path) => {
        if (!alive) throw new Error('Go program has already exited');
        if (window.__engine.hang) return new Promise(() => {});
        const body = new TextEncoder().encode(JSON.stringify({ engine: n, path }));
        return Promise.resolve({ status: 200, contentType: 'application/json', body });
      };
      if (!window.__engine.failStart) window.__cartographeReady(); else exit();
    });
  }
};`;
const emptyWasm = Buffer.from([0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00]);

async function open(browser, base, { failures = 0, init = '' } = {}) {
  const p = await (await browser.newContext()).newPage();
  let requests = 0;
  await p.route('**/wasm_exec.js', (r) => r.fulfill({ contentType: 'text/javascript', body: fakeRuntime + init }));
  await p.route('**/app.wasm', (r) => (++requests <= failures
    ? r.abort('connectionfailed')
    : r.fulfill({ contentType: 'application/wasm', body: emptyWasm })));
  await p.goto(base);
  return { p, requests: () => requests };
}
const state = (p) => p.evaluate(() => window.cartographeEngine.state);
const ask = (p) => p.evaluate(() => Promise.race([
  fetch('/api/config').then((r) => r.json()).then((d) => 'engine ' + d.engine, (e) => 'error: ' + e.message),
  new Promise((r) => setTimeout(() => r('timeout'), 4000)),
]));

module.exports = async function engine({ base, check }) {
  const browser = await chromium.launch();

  // A download that fails twice is retried, and the third try works.
  let { p, requests } = await open(browser, base, { failures: 2 });
  await p.waitForFunction(() => window.cartographeEngine.state === 'ready', null, { timeout: 15000 }).catch(() => {});
  check('engine: a failed download is retried until it works', (await state(p)) === 'ready' && requests() === 3, `${await state(p)}, ${requests()} requests`);
  check('engine: no failure shown after a successful retry', await p.locator('#engineRetry').isHidden());
  await p.context().close();

  // A download that never works gives up after three tries, says so, keeps the text.
  ({ p, requests } = await open(browser, base, { failures: 99 }));
  await p.fill('#desc', 'Une crique battue par les vagues.');
  await p.waitForFunction(() => window.cartographeEngine.state === 'failed', null, { timeout: 15000 }).catch(() => {});
  const text = await p.textContent('#engineText');
  check('engine: gives up after three tries', (await state(p)) === 'failed' && requests() === 3, `${await state(p)}, ${requests()} requests`);
  check('engine: failure explained with a retry button', /n’a pas pu se charger/.test(text) && await p.locator('#engineRetry').isVisible(), text);
  check('engine: a request fails instead of waiting for ever', (await ask(p)).startsWith('error:'), await ask(p));
  await p.unroute('**/app.wasm');
  await p.route('**/app.wasm', (r) => r.fulfill({ contentType: 'application/wasm', body: emptyWasm }));
  await p.click('#engineRetry');
  await p.waitForFunction(() => window.cartographeEngine && window.cartographeEngine.state === 'ready', null, { timeout: 15000 }).catch(() => {});
  check('engine: "Réessayer" reloads and keeps what was typed', (await state(p)) === 'ready' && (await p.inputValue('#desc')) === 'Une crique battue par les vagues.');
  await p.context().close();

  // An engine that stops mid-visit is started again; requests keep working.
  ({ p } = await open(browser, base));
  await p.waitForFunction(() => window.cartographeEngine.state === 'ready');
  check('engine: answers once ready', (await ask(p)) === 'engine 1', await ask(p));
  await p.evaluate(() => window.__engine.stop());
  check('engine: restarted after it stops', (await ask(p)) === 'engine 2', await ask(p));
  // A request in flight when the engine dies is asked again of the new one.
  const inFlight = await p.evaluate(async () => {
    window.__engine.hang = true;
    const pending = fetch('/api/config').then((r) => r.json()).then((d) => 'engine ' + d.engine, (e) => 'error: ' + e.message);
    await new Promise((r) => setTimeout(r, 100));
    window.__engine.hang = false;
    window.__engine.stop();
    return Promise.race([pending, new Promise((r) => setTimeout(() => r('timeout'), 4000))]);
  });
  check('engine: a request in flight survives the restart', inFlight === 'engine 3', inFlight);
  // Restarts are bounded: an engine that keeps dying is reported, not looped on.
  await p.evaluate(() => window.__engine.stop());
  await p.waitForTimeout(300);
  const after = await ask(p);
  check('engine: restarts are bounded', (await state(p)) === 'failed' && after.startsWith('error:') && (await p.evaluate(() => window.__engine.starts)) === 3,
    `${await state(p)}, ${after}, ${await p.evaluate(() => window.__engine.starts)} starts`);
  await p.context().close();

  // An engine that stops while starting is a failure, not a restart loop.
  ({ p } = await open(browser, base, { init: 'window.__engine.failStart = true;' }));
  await p.waitForFunction(() => window.cartographeEngine.state === 'failed', null, { timeout: 5000 }).catch(() => {});
  check('engine: a start-up failure does not loop', (await state(p)) === 'failed' && (await p.evaluate(() => window.__engine.starts)) === 1);
  await p.context().close();

  // A browser without WebAssembly is told so, with no pointless retry button.
  ({ p } = await open(browser, base, { init: 'delete window.WebAssembly; Object.defineProperty(window, "WebAssembly", { value: undefined });' }));
  await p.waitForFunction(() => window.cartographeEngine.state === 'failed', null, { timeout: 5000 }).catch(() => {});
  const unsupported = await p.textContent('#engineText');
  check('engine: an unsupported browser is told so', /Ce navigateur ne peut pas/.test(unsupported) && await p.locator('#engineRetry').isHidden(), unsupported);
  await p.context().close();

  await browser.close();
};
