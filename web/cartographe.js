// Le Cartographe, browser build: routes the page's /api/* requests to the Go
// engine compiled to WebAssembly (cmd/web), so the unchanged desktop UI runs
// entirely in the visitor's browser. Loaded before the UI's own script, so its
// very first fetch('/api/config') is already routed here.
(() => {
  const realFetch = window.fetch.bind(window);

  // The engine's uncompressed size, so the UI can show download progress even
  // when the host compresses it (Content-Length is then the compressed size).
  const WASM_BYTES = 0; // set by scripts/build-web.sh

  // The UI listens for cartographe:progress / :ready / :failed events; the
  // latest state is also kept on window for a listener that arrives late.
  const engine = window.cartographeEngine = { state: 'loading', loaded: 0, total: WASM_BYTES };
  const report = (state, extra = {}) => {
    Object.assign(engine, { state }, extra);
    window.dispatchEvent(new CustomEvent('cartographe:' + state, { detail: { ...engine } }));
  };

  // Counts the bytes of one branch of the response as they arrive.
  async function track(body) {
    const reader = body.getReader();
    for (;;) {
      const { done, value } = await reader.read();
      if (done) return;
      engine.loaded += value.byteLength;
      report('progress');
    }
  }

  async function instantiate(go) {
    const res = await realFetch('/app.wasm');
    if (!res.ok) throw new Error('HTTP ' + res.status);
    if (WebAssembly.instantiateStreaming && res.body && typeof res.body.tee === 'function') {
      const [compile, count] = res.body.tee();
      track(count).catch(() => {});
      try {
        return await WebAssembly.instantiateStreaming(
          new Response(compile, { headers: { 'Content-Type': 'application/wasm' } }), go.importObject);
      } catch {
        // Fall through: refetch and compile from a buffer below.
      }
    }
    const buf = await (await realFetch('/app.wasm')).arrayBuffer();
    engine.loaded = buf.byteLength; report('progress');
    return WebAssembly.instantiate(buf, go.importObject);
  }

  // Until the engine is ready, a prompt (pure text) is assembled here from the
  // head the build wrote out (nl.Catalog.PromptHead) and the same scene lines
  // as nl.userPrompt, so describing a scene never waits for the engine's
  // download. Once it has loaded, the engine answers as for any request.
  const promptHead = realFetch('/prompt-head.txt')
    .then((r) => (r.ok ? r.text() : Promise.reject(new Error('HTTP ' + r.status))));
  promptHead.catch(() => {});
  let engineReady = false;
  async function quickPrompt(body) {
    const head = await promptHead;
    const req = JSON.parse(body || '{}');
    const description = String(req.description || '').trim();
    if (!description) return { status: 400, data: { error: 'description is empty' } };
    const width = req.width > 0 ? req.width : 50, length = req.length > 0 ? req.length : 50;
    let scene = `Map: width=${width}, length=${length}`;
    if (req.biome) scene += `\nUse this biome for the map (do not choose another): ${req.biome}`;
    scene += '\n\nDescription:\n' + description;
    return { status: 200, data: { prompt: head + scene } };
  }

  // Loading can fail for reasons that pass (a dropped connection, a deploy
  // in progress): try again a couple of times before giving up.
  const RETRY_DELAYS_MS = [1500, 4000];
  // An engine that stops mid-visit (out of memory, say) is started again, a
  // bounded number of times so a crash on start-up cannot loop.
  const MAX_RESTARTS = 2;
  let restarts = 0;
  // Rejected when the running engine stops, so a request it was serving does
  // not wait for ever.
  let stoppedSignal = null, signalStop = () => {};
  const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

  // boot loads and starts the engine; the promise settles when the engine has
  // announced itself (__cartographeReady) or definitely could not start.
  function boot() {
    return new Promise((resolve, reject) => {
      let settled = false, started = false;
      const fail = (reason, msg) => {
        if (settled) return;
        settled = true; engineReady = false;
        report('failed', { reason });
        reject(new Error("le moteur de génération n'a pas pu se charger (" + msg + ')'));
      };
      window.__cartographeReady = () => {
        stoppedSignal = new Promise((_, rej) => { signalStop = rej; });
        stoppedSignal.catch(() => {});
        settled = started = true; engineReady = true; report('ready'); resolve();
      };
      window.__cartographeFailed = (msg) => fail('error', msg);

      (async () => {
        if (typeof WebAssembly !== 'object' || typeof Go !== 'function') {
          // Nothing a retry would change: the browser cannot run the engine,
          // or its runtime file did not load.
          return typeof WebAssembly !== 'object'
            ? fail('unsupported', 'WebAssembly indisponible')
            : fail('error', 'wasm_exec.js absent');
        }
        let lastError;
        for (let attempt = 0; attempt <= RETRY_DELAYS_MS.length; attempt++) {
          if (attempt > 0) {
            await sleep(RETRY_DELAYS_MS[attempt - 1]);
            engine.loaded = 0;
            report('progress', { attempt: attempt + 1 });
          }
          try {
            const go = new Go();
            const result = await instantiate(go);
            // run() only returns when the engine stops, which it should not.
            Promise.resolve(go.run(result.instance)).catch(() => {}).then(stopped);
            return;
          } catch (err) {
            lastError = err;
          }
        }
        fail('error', lastError ? lastError.message : 'erreur inconnue');
      })();

      function stopped() {
        if (!started) return fail('error', "le moteur s'est arrêté au démarrage");
        // It was running: start a fresh one. The browser has the file cached.
        engineReady = false;
        const stop = signalStop;
        if (restarts >= MAX_RESTARTS) {
          report('failed', { reason: 'error' });
        } else {
          restarts++;
          ready = boot();
          ready.catch(() => {});
        }
        stop(new Error("le moteur de génération s'est arrêté"));
      }
    });
  }

  // Start loading the engine immediately; requests wait for it.
  let ready = boot();
  ready.catch(() => {});

  let lastPreview = null;

  async function goFetch(method, path, body) {
    await ready;
    const running = ready;
    try {
      return await Promise.race([window.cartographeFetch(method, path, body), stoppedSignal]);
    } catch (err) {
      // If the engine died under this request and a new one was started, wait
      // for it and ask once more; any other failure is reported as it is.
      if (ready === running) throw err;
      await ready;
      return Promise.race([window.cartographeFetch(method, path, body), stoppedSignal]);
    }
  }

  window.fetch = async (input, init = {}) => {
    const url = new URL(typeof input === 'string' ? input : input.url, location.href);
    if (url.origin !== location.origin || !url.pathname.startsWith('/api/')) {
      return realFetch(input, init);
    }
    const method = (init.method || (typeof input === 'string' ? 'GET' : input.method) || 'GET').toUpperCase();
    if (!engineReady && method === 'POST' && url.pathname === '/api/prompt') {
      try {
        const q = await quickPrompt(typeof init.body === 'string' ? init.body : '');
        return new Response(JSON.stringify(q.data), { status: q.status, headers: { 'Content-Type': 'application/json' } });
      } catch {
        // No head file (an old build, say): wait for the engine instead.
      }
    }
    const res = await goFetch(method, url.pathname + url.search, typeof init.body === 'string' ? init.body : '');

    let body = res.body;
    if (res.contentType.startsWith('application/json')) {
      // The UI shows the preview with <img src=preview_url>, which never goes
      // through fetch: render it here and hand the UI a blob URL instead.
      const data = JSON.parse(new TextDecoder().decode(res.body));
      if (data && typeof data.preview_url === 'string' && data.preview_url.startsWith('/api/')) {
        const png = await goFetch('GET', data.preview_url, '');
        if (png.status === 200) {
          if (lastPreview) URL.revokeObjectURL(lastPreview);
          lastPreview = URL.createObjectURL(new Blob([png.body], { type: png.contentType }));
          data.preview_url = lastPreview;
        }
      }
      body = JSON.stringify(data);
    }
    return new Response(body, { status: res.status, headers: { 'Content-Type': res.contentType } });
  };
})();
