// Le Cartographe, browser build: routes the page's /api/* requests to the Go
// engine compiled to WebAssembly (cmd/web), so the unchanged desktop UI runs
// entirely in the visitor's browser. Loaded before the UI's own script, so its
// very first fetch('/api/config') is already routed here.
(() => {
  const realFetch = window.fetch.bind(window);

  // Start loading the engine immediately; requests wait for it.
  const ready = new Promise((resolve, reject) => {
    window.__cartographeReady = resolve;
    window.__cartographeFailed = (msg) => reject(new Error(msg));
    const go = new Go();
    const load = WebAssembly.instantiateStreaming
      ? WebAssembly.instantiateStreaming(realFetch('/app.wasm'), go.importObject)
          // A host serving .wasm with the wrong MIME type breaks streaming only.
          .catch(() => realFetch('/app.wasm').then((r) => r.arrayBuffer())
            .then((buf) => WebAssembly.instantiate(buf, go.importObject)))
      : realFetch('/app.wasm').then((r) => r.arrayBuffer())
          .then((buf) => WebAssembly.instantiate(buf, go.importObject));
    load.then((result) => go.run(result.instance))
      .catch((err) => reject(new Error("le moteur de génération n'a pas pu se charger (" + err.message + ')')));
  });

  let lastPreview = null;

  async function goFetch(method, path, body) {
    await ready;
    return window.cartographeFetch(method, path, body);
  }

  window.fetch = async (input, init = {}) => {
    const url = new URL(typeof input === 'string' ? input : input.url, location.href);
    if (url.origin !== location.origin || !url.pathname.startsWith('/api/')) {
      return realFetch(input, init);
    }
    const method = (init.method || (typeof input === 'string' ? 'GET' : input.method) || 'GET').toUpperCase();
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
