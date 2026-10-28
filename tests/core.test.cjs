const test = require('node:test');
const assert = require('node:assert/strict');
const path = require('node:path');
const { pathToFileURL } = require('node:url');

async function mod(name) {
  try {
    return await import(pathToFileURL(path.resolve(__dirname, '../extension/lib/', name)));
  } catch (error) {
    assert.fail(`Required production module ${name} unavailable: ${error.message}`);
  }
}

test('settings reject invalid types and bound expensive capture options', async () => {
  const { normalizeSettings } = await mod('config.js');
  const result = normalizeSettings({
    saveHtml: 'false',
    captureIntervalSeconds: 0,
    debounceMs: -10,
    maxBlocks: 1e9,
    maxHtmlChars: NaN,
  });
  assert.equal(result.saveHtml, true);
  assert.equal(result.captureIntervalSeconds, 10);
  assert.equal(result.debounceMs, 500);
  assert.equal(result.maxBlocks, 20000);
  assert.equal(result.maxHtmlChars, 2000000);
  assert.equal(normalizeSettings({ saveHtml: false }).saveHtml, false);
});

test('permission host pattern keeps scheme and host while tracking enabled port separately', async () => {
  const { pageOrigin, permissionPattern } = await mod('policy.js');
  assert.equal(pageOrigin('https://example.org:8443/a#b'), 'https://example.org:8443');
  assert.equal(permissionPattern('https://example.org:8443'), 'https://example.org/*');
  assert.equal(pageOrigin('chrome://extensions'), null);
  assert.equal(pageOrigin('javascript:alert(1)'), null);
  assert.throws(() => permissionPattern('file:///tmp/data'));
});

test('page content cannot invoke privileged archive commands', async () => {
  const { isExtensionPage } = await mod('policy.js');
  const origin = 'chrome-extension://abcdefghijklmnopabcdefghijklmnop/';
  assert.equal(isExtensionPage({ url: origin + 'ui/archive.html' }, origin), true);
  assert.equal(isExtensionPage({ url: 'https://evil.test/' + origin }, origin), false);
  assert.equal(isExtensionPage({ url: 'https://example.org', tab: { id: 3 } }, origin), false);
});

test('capture authorization is bound to actual sender origin, main frame and enabled site', async () => {
  const { validateCaptureSource } = await mod('policy.js');
  const capture = { schema_version: 1, url: 'https://example.org/new', blocks: [] };
  const sender = { tab: { id: 7 }, frameId: 0, url: 'https://example.org/old' };
  const context = { enabledOrigins: ['https://example.org'], oneShots: {}, now: 1000 };
  assert.equal(validateCaptureSource(capture, sender, context), 'https://example.org');
  assert.throws(() =>
    validateCaptureSource({ ...capture, url: 'https://other.org/' }, sender, context),
  );
  assert.throws(() => validateCaptureSource(capture, { ...sender, frameId: 2 }, context));
  assert.throws(() => validateCaptureSource(capture, sender, { ...context, enabledOrigins: [] }));
});

test('manual capture grant expires and cannot authorize another origin', async () => {
  const { validateCaptureSource } = await mod('policy.js');
  const cap = { schema_version: 1, url: 'https://example.org/', blocks: [] };
  const sender = { tab: { id: 7 }, frameId: 0, url: cap.url };
  const context = {
    enabledOrigins: [],
    oneShots: { 7: { origin: 'https://example.org', expires: 2000 } },
    now: 1000,
  };
  assert.equal(validateCaptureSource(cap, sender, context), 'https://example.org');
  assert.throws(() => validateCaptureSource(cap, sender, { ...context, now: 3000 }));
  assert.throws(() =>
    validateCaptureSource(cap, sender, {
      ...context,
      oneShots: { 7: { origin: 'https://other.org', expires: 2000 } },
    }),
  );
});

test('semantic digest ignores capture timestamps but distinguishes changed content and SPA URL', async () => {
  const { captureDigest } = await mod('canonical.js');
  const cap = {
    url: 'https://example.org/#one',
    title: 'Документ',
    blocks: [{ type: 'paragraph', text: 'Привет' }],
    captured_at: '2026-10-27T10:00:00Z',
  };
  const first = await captureDigest(cap);
  assert.equal(
    await captureDigest({
      ...cap,
      captured_at: '2026-10-27T11:00:00Z',
      reason: 'periodic',
      html: 'random DOM',
    }),
    first,
  );
  assert.notEqual(await captureDigest({ ...cap, url: 'https://example.org/#two' }), first);
  assert.notEqual(
    await captureDigest({ ...cap, blocks: [{ type: 'paragraph', text: 'Изменение' }] }),
    first,
  );
});

test('delivery keeps a snapshot when host rejects and removes it only after commit acknowledgement', async () => {
  const { deliverBatch } = await mod('delivery.js');
  const pending = [{ id: 'a', revision: 1, capture: { url: 'https://example.org', blocks: [] } }];
  let failure = true;
  const queue = {
    peek: async () => pending.slice(0, 1),
    remove: async (id, rev) => {
      const at = pending.findIndex((x) => x.id === id && x.revision === rev);
      if (at >= 0) pending.splice(at, 1);
    },
    markError: async () => {},
  };
  const send = async () => {
    if (failure) throw new Error('Native host missing');
    return { capture_id: 1 };
  };
  const failed = await deliverBatch(queue, send, { maxItems: 10 });
  assert.equal(pending.length, 1);
  assert.equal(failed.sent, 0);
  assert.match(failed.error, /Native host missing/);
  failure = false;
  const succeeded = await deliverBatch(queue, send, { maxItems: 10 });
  assert.equal(pending.length, 0);
  assert.equal(succeeded.sent, 1);
});

test('capture rejects a URL exceeding the native codepoint limit', async () => {
  const { validateCaptureSource } = await mod('policy.js');
  const url = 'https://example.org/' + 'a'.repeat(65536);
  assert.throws(
    () =>
      validateCaptureSource(
        { schema_version: 1, url, blocks: [] },
        { tab: { id: 1 }, url, frameId: 0 },
        { enabledOrigins: ['https://example.org'] },
      ),
    /URL/,
  );
});
