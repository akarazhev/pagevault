(() => {
  if (globalThis.__pagevaultCapture) return;
  globalThis.__pagevaultCapture = true;
  // getRandomValues is available in content scripts on ordinary HTTP pages too.
  const sessionId = Array.from(crypto.getRandomValues(new Uint32Array(4)), value => value.toString(16).padStart(8, '0')).join('-');
  let enabled = false, settings = {}, dirty = false, capturing = null;
  let debounceTimer, deadlineTimer, intervalTimer, navigationTimer;
  let lastUrl = location.href;
  const observers = new Map();
  const frameListeners = new WeakSet();

  function call(message) {
    return new Promise((resolve, reject) => {
      chrome.runtime.sendMessage(message, response => {
        const error = chrome.runtime.lastError;
        if (error) { reject(new Error(error.message)); return; }
        if (!response?.ok) { reject(new Error(response?.error || 'No response from the extension.')); return; }
        resolve(response.data);
      });
    });
  }
  function clearScheduled() {
    clearTimeout(debounceTimer); clearTimeout(deadlineTimer);
    debounceTimer = deadlineTimer = undefined;
  }
  async function capture(reason = 'mutation', force = false) {
    if (!enabled && !force) return;
    if (capturing) { dirty = true; await capturing; if (force) return capture(reason, true); return; }
    clearScheduled(); dirty = false;
    capturing = (async () => {
      try {
        const snapshot = globalThis.PageVaultExtractor.extract(document, settings);
        snapshot.schema_version = 1;
        snapshot.url = location.href;
        snapshot.captured_at = new Date().toISOString();
        snapshot.reason = reason;
        snapshot.session_id = sessionId;
        return await call({ type: 'capture.submit', capture: snapshot });
      } catch (error) {
        dirty = true;
        // Queue or native errors are also visible in popup; keep content eligible for next retry.
        console.warn('PageVault:', error.message);
        if (force) throw error;
      }
    })();
    try { return await capturing; }
    finally { capturing = null; }
  }
  function schedule(reason = 'mutation') {
    if (!enabled) return;
    dirty = true;
    clearTimeout(debounceTimer);
    debounceTimer = setTimeout(() => void capture(reason), settings.debounceMs || 1200);
    // A busy live page still gets a snapshot at least every 10 seconds.
    if (!deadlineTimer) deadlineTimer = setTimeout(() => void capture(reason), 10000);
  }
  function observeRoot(root) {
    if (!root || observers.has(root)) return;
    const observer = new MutationObserver(() => { discoverRoots(); schedule('mutation'); });
    observer.observe(root, { childList: true, subtree: true, characterData: true, attributes: true, attributeFilter: ['href', 'src', 'alt', 'title', 'hidden', 'open', 'aria-expanded'] });
    observers.set(root, observer);
  }
  function rootIsLive(root) {
    // isConnected alone is insufficient for a superseded iframe document.
    const node = root.host || root;
    if (!node.isConnected) return false;
    const doc = node.ownerDocument;
    if (doc === document) return true;
    try {
      const frame = doc.defaultView?.frameElement;
      return !!frame && frame.contentDocument === doc && rootIsLive(frame);
    } catch { return false; }
  }
  function discoverRoots() {
    // Bound the supplementary search; the extractor independently reports its own content limits.
    let budget = 20000;
    const visited = new Set();
    function visit(root) {
      if (!root || visited.has(root) || budget <= 0) return;
      visited.add(root); observeRoot(root);
      const doc = root.ownerDocument || root;
      const walker = doc.createTreeWalker(root, NodeFilter.SHOW_ELEMENT);
      let element;
      while (budget-- > 0 && (element = walker.nextNode())) {
        if (element.shadowRoot) visit(element.shadowRoot);
        if (element.tagName === 'IFRAME') {
          try { if (element.contentDocument?.documentElement) visit(element.contentDocument.documentElement); } catch { /* Cross-origin frame. */ }
          if (!frameListeners.has(element)) { frameListeners.add(element); element.addEventListener('load', () => { if (enabled) { discoverRoots(); schedule('frame'); } }); }
        }
      }
    }
    visit(document.documentElement);
    for (const [root, observer] of observers) if (!rootIsLive(root)) { observer.disconnect(); observers.delete(root); }
  }
  function stop() {
    enabled = false; clearScheduled(); clearInterval(intervalTimer); clearInterval(navigationTimer);
    for (const observer of observers.values()) observer.disconnect(); observers.clear();
  }
  function configure(config) {
    const wasEnabled = enabled;
    stop(); settings = config.settings || settings; enabled = !!config.enabled;
    if (!enabled) return;
    lastUrl = location.href; discoverRoots();
    intervalTimer = setInterval(() => { discoverRoots(); void capture('periodic'); }, (settings.captureIntervalSeconds || 30) * 1000);
    navigationTimer = setInterval(() => {
      if (location.href !== lastUrl) { lastUrl = location.href; schedule('navigation'); }
    }, 1000);
    schedule(wasEnabled ? 'settings' : 'initial');
  }
  const initialized = call({ type: 'capture.config' }).then(configure).catch(error => console.warn('PageVault:', error.message));
  chrome.runtime.onMessage.addListener((message, sender, respond) => {
    if (sender.id !== chrome.runtime.id) return;
    if (!['capture.ping', 'capture.stop', 'capture.configure', 'capture.run'].includes(message?.type)) return;
    (async () => {
      await initialized;
      if (message.type === 'capture.ping') return { enabled };
      if (message.type === 'capture.stop') { stop(); return {}; }
      if (message.type === 'capture.configure') { configure(message); return {}; }
      if (message.settings) settings = message.settings;
      await capture(message.reason || 'manual', true);
      return { queued: true };
    })().then(data => respond({ ok: true, data }), error => respond({ ok: false, error: error.message }));
    return true;
  });
  addEventListener('popstate', () => schedule('navigation'));
  addEventListener('hashchange', () => schedule('navigation'));
  document.addEventListener('visibilitychange', () => { if (enabled && document.visibilityState === 'hidden' && dirty) void capture('visibility'); });
  addEventListener('pagehide', () => { if (enabled && dirty) void capture('pagehide'); });
})();
