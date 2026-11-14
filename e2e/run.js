// Runs the browser checks against the built website.
//
//   scripts/build-web.sh && node e2e/run.js [suite...]
//
// Suites: engine, repairs, catalogue, journey (all by default). It serves
// dist/web itself, prints one line per check and exits non-zero if any failed.
// Screenshots go to e2e/shots/. Needs the "playwright" package and its Chromium
// (in CI: npm ci && npx playwright install --with-deps chromium, in e2e/).
const http = require('http');
const fs = require('fs');
const path = require('path');

const root = path.join(__dirname, '..', 'dist', 'web');
const out = path.join(__dirname, 'shots');
const types = { '.html': 'text/html; charset=utf-8', '.js': 'text/javascript', '.wasm': 'application/wasm',
  '.txt': 'text/plain; charset=utf-8', '.png': 'image/png', '.css': 'text/css', '.woff2': 'font/woff2', '.ico': 'image/x-icon' };

function serve() {
  const server = http.createServer((req, res) => {
    let rel = decodeURIComponent(new URL(req.url, 'http://x').pathname);
    if (rel.endsWith('/')) rel += 'index.html';
    let file = path.join(root, rel);
    if (!file.startsWith(root)) { res.writeHead(403).end(); return; }
    if (!fs.existsSync(file) && fs.existsSync(file + '.html')) file += '.html'; // like the host
    fs.readFile(file, (err, data) => {
      if (err) { res.writeHead(404).end('not found'); return; }
      res.writeHead(200, { 'Content-Type': types[path.extname(file)] || 'application/octet-stream' }).end(data);
    });
  });
  return new Promise((resolve) => server.listen(0, '127.0.0.1', () => resolve(server)));
}

(async () => {
  if (!fs.existsSync(path.join(root, 'app.wasm'))) {
    console.error('dist/web is not built: run scripts/build-web.sh first');
    process.exit(2);
  }
  fs.mkdirSync(out, { recursive: true });
  const all = ['engine', 'repairs', 'catalogue', 'journey'];
  const wanted = process.argv.slice(2).length ? process.argv.slice(2) : all;
  const server = await serve();
  const base = `http://127.0.0.1:${server.address().port}`;
  let passed = 0;
  const failed = [];
  const check = (name, ok, extra = '') => {
    console.log(`${ok ? 'PASS' : 'FAIL'} ${name}${!ok && extra ? ' (' + extra + ')' : ''}`);
    if (ok) passed++; else failed.push(name);
  };
  for (const suite of wanted) {
    try {
      await require('./' + suite + '.js')({ base, out, check });
    } catch (e) {
      check(`${suite}: ran to the end`, false, e.message.split('\n')[0]);
    }
  }
  server.close();
  console.log(`\n${passed} passed, ${failed.length} failed`);
  if (failed.length) { console.log(failed.map((f) => '  - ' + f).join('\n')); process.exit(1); }
  process.exit(0);
})();
