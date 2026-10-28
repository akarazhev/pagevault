const { test, before, after } = require('node:test');
const assert = require('node:assert/strict');
const { playwright, browserOptions, serveProject } = require('./helpers.cjs');
let server, browser;
before(async () => {
  server = await serveProject();
  browser = await playwright().chromium.launch(browserOptions());
});
after(async () => {
  await browser?.close();
  await server?.close();
});

test('outbox persists across page reload and preserves newer revision after old acknowledgement', async () => {
  const context = await browser.newContext();
  const page = await context.newPage();
  await page.goto(server.url + '/blank');
  const original = await page.evaluate(async () => {
    const q = await import('/extension/lib/outbox.js');
    const capture = {
      schema_version: 1,
      url: 'https://example.org/',
      title: 'Сохранение',
      blocks: [{ type: 'paragraph', text: 'Тест' }],
      captured_at: '2026-10-27T10:00:00Z',
    };
    await q.enqueue(capture);
    return (await q.peek())[0];
  });
  await page.reload();
  const result = await page.evaluate(async (first) => {
    const q = await import('/extension/lib/outbox.js');
    const before = await q.getStats();
    await q.enqueue({ ...first.capture, captured_at: '2026-10-27T11:00:00Z' });
    await q.remove(first.id, first.revision);
    const afterOld = await q.getStats();
    const [newer] = await q.peek();
    await q.remove(newer.id, newer.revision);
    return { before, afterOld, afterNew: await q.getStats(), newTime: newer.capture.captured_at };
  }, original);
  assert.equal(result.before.pendingCount, 1);
  assert.equal(result.afterOld.pendingCount, 1);
  assert.equal(result.afterNew.pendingCount, 0);
  assert.equal(result.newTime, '2026-10-27T11:00:00Z');
  await context.close();
});

test('offline snapshots with changed content remain ordered and survive failed delivery', async () => {
  const context = await browser.newContext();
  const page = await context.newPage();
  await page.goto(server.url + '/blank');
  const result = await page.evaluate(async () => {
    const q = await import('/extension/lib/outbox.js');
    const { deliverBatch } = await import('/extension/lib/delivery.js');
    const originalNow = Date.now;
    Date.now = () => 1777777777777;
    try {
      for (const text of ['Первое сообщение', 'Новый ответ'])
        await q.enqueue({
          url: 'https://forum.example/thread',
          blocks: [{ type: 'paragraph', text }],
        });
    } finally {
      Date.now = originalNow;
    }
    const fail = await deliverBatch(q, async () => {
      throw new Error('offline');
    });
    return {
      fail,
      stats: await q.getStats(),
      items: (await q.peek()).map((x) => x.capture.blocks[0].text),
    };
  });
  assert.equal(result.stats.pendingCount, 2);
  assert.equal(result.fail.sent, 0);
  assert.deepEqual(result.items, ['Первое сообщение', 'Новый ответ']);
  assert.ok(result.stats.queueBytes > 0);
  await context.close();
});

test('URL purge atomically removes every accepted matching revision and retains unrelated queue bytes', async () => {
  const context = await browser.newContext();
  const page = await context.newPage();
  await page.goto(server.url + '/blank');
  const result = await page.evaluate(async () => {
    const q = await import('/extension/lib/outbox.js');
    const url = 'https://example.org/page?q=1#state';
    await q.enqueue({ url, blocks: [{ text: 'old' }] });
    await q.enqueue({ url, blocks: [{ text: 'changed' }] });
    await q.enqueue({ url: url + '-other', blocks: [{ text: 'keep' }] });
    const removed = await q.removeUrl(url);
    const [remaining] = await q.peek();
    return {
      removed,
      stats: await q.getStats(),
      url: remaining.capture.url,
      remainingBytes: remaining.bytes,
    };
  });
  assert.equal(result.removed, 2);
  assert.equal(result.stats.pendingCount, 1);
  assert.equal(result.stats.queueBytes, result.remainingBytes);
  assert.equal(result.url, 'https://example.org/page?q=1#state-other');
  await context.close();
});

test('outbox ordering survives revisions, acknowledgement, URL purge, reload and clock rollback', async () => {
  const context = await browser.newContext();
  const page = await context.newPage();
  await page.goto(server.url + '/blank');
  const before = await page.evaluate(async () => {
    const q = await import('/extension/lib/outbox.js');
    const originalNow = Date.now;
    Date.now = () => 1000;
    try {
      const a = { url: 'https://example.org/a', blocks: [{ text: 'first' }] };
      await q.enqueue(a);
      await q.enqueue({ url: 'https://example.org/b', blocks: [{ text: 'second' }] });
      await q.enqueue({ ...a, captured_at: 'later revision' });
      const entries = await q.peek();
      await q.remove(entries[0].id, entries[0].revision);
      await q.removeUrl('https://example.org/b');
      return { entries, stats: await q.getStats() };
    } finally {
      Date.now = originalNow;
    }
  });
  assert.deepEqual(
    before.entries.map((x) => x.capture.url),
    ['https://example.org/a', 'https://example.org/b'],
  );
  assert.equal(before.entries[0].revision, 2);
  assert.equal(before.entries[0].createdAt, 1000);
  assert.equal(before.entries[1].createdAt, 1001);
  assert.equal(before.stats.pendingCount, 0);
  await page.reload();
  const after = await page.evaluate(async () => {
    const q = await import('/extension/lib/outbox.js');
    const originalNow = Date.now;
    Date.now = () => 1;
    try {
      await q.enqueue({ url: 'https://example.org/c', blocks: [] });
      return (await q.peek())[0];
    } finally {
      Date.now = originalNow;
    }
  });
  assert.equal(after.createdAt, 1002);
  await context.close();
});

test('permanent rejection survives reload, skips delivery, and revision operations preserve accounting', async () => {
  const context = await browser.newContext();
  const page = await context.newPage();
  await page.goto(server.url + '/blank');
  const first = await page.evaluate(async () => {
    const q = await import('/extension/lib/outbox.js');
    await q.enqueue({ url: 'https://example.org/bad', blocks: [] });
    await q.enqueue({ url: 'https://example.org/good', blocks: [] });
    const [bad] = await q.peek();
    await q.markError(bad.id, bad.revision, 'invalid', { permanent: true });
    return bad;
  });
  await page.reload();
  const result = await page.evaluate(async (bad) => {
    const q = await import('/extension/lib/outbox.js');
    const { deliverBatch } = await import('/extension/lib/delivery.js');
    const delivered = await deliverBatch(q, async () => ({ capture_id: 1 }));
    const rejected = await q.listRejected();
    await q.retry(bad.id, bad.revision + 1);
    const afterStale = await q.listRejected();
    await q.retry(bad.id, bad.revision);
    const afterRetry = await q.listRejected();
    await q.remove(bad.id, bad.revision);
    return { delivered, rejected, afterStale, afterRetry, stats: await q.getStats() };
  }, first);
  assert.equal(result.delivered.sent, 1);
  assert.equal(result.rejected.length, 1);
  assert.equal(result.afterStale.length, 1);
  assert.equal(result.afterRetry.length, 0);
  assert.equal(result.stats.pendingCount, 0);
  assert.equal(result.stats.queueBytes, 0);
  await context.close();
});

test('permanent host error skips the rejected entry and stale errors cannot reject replacements', async () => {
  const context = await browser.newContext();
  const page = await context.newPage();
  await page.goto(server.url + '/blank');
  const result = await page.evaluate(async () => {
    const q = await import('/extension/lib/outbox.js');
    const { deliverBatch } = await import('/extension/lib/delivery.js');
    const bad = { url: 'https://example.org/bad', blocks: [] };
    await q.enqueue(bad);
    await q.enqueue({ url: 'https://example.org/good', blocks: [] });
    const [first] = await q.peek();
    const delivered = await deliverBatch(q, async (capture) => {
      if (capture.url === bad.url) {
        const e = new Error('invalid');
        e.code = 'invalid_request';
        throw e;
      }
      return { capture_id: 1 };
    });
    await q.enqueue({ ...bad, captured_at: 'new' });
    await q.markError(first.id, first.revision, 'late', { permanent: true });
    return { delivered, items: await q.peek(), rejected: await q.listRejected() };
  });
  assert.equal(result.delivered.sent, 1);
  assert.equal(result.delivered.rejected, 1);
  assert.equal(result.items.length, 1);
  assert.equal(result.items[0].revision, 2);
  assert.equal(result.rejected.length, 0);
  await context.close();
});
test('discard applies only to the rejected revision and updates totals atomically', async () => {
  const context = await browser.newContext();
  const page = await context.newPage();
  await page.goto(server.url + '/blank');
  const result = await page.evaluate(async () => {
    const q = await import('/extension/lib/outbox.js');
    await q.enqueue({ url: 'https://example.org/x', blocks: [] });
    const [item] = await q.peek();
    await q.markError(item.id, item.revision, 'bad', { permanent: true });
    const rejected = await q.getStats();
    await q.discard(item.id, item.revision + 1);
    const stale = await q.getStats();
    await q.retry(item.id, item.revision);
    await q.discard(item.id, item.revision);
    const retried = await q.getStats();
    await q.markError(item.id, item.revision, 'bad', { permanent: true });
    await q.discard(item.id, item.revision);
    return { rejected, stale, retried, empty: await q.getStats() };
  });
  assert.equal(result.rejected.rejectedCount, 1);
  assert.deepEqual(result.stale, result.rejected);
  assert.equal(result.retried.rejectedCount, 0);
  assert.equal(result.retried.pendingCount, 1);
  assert.deepEqual(result.empty, { pendingCount: 0, queueBytes: 0, rejectedCount: 0 });
  await context.close();
});
test('legacy v1 totals survive marking and replacing a rejected capture', async () => {
  const context = await browser.newContext();
  const page = await context.newPage();
  await page.goto(server.url + '/blank');
  const result = await page.evaluate(async () => {
    const q = await import('/extension/lib/outbox.js');
    const capture = { url: 'https://example.org/legacy', blocks: [] };
    await q.enqueue(capture);
    const [first] = await q.peek();
    const db = await new Promise((resolve, reject) => {
      const r = indexedDB.open('pagevault-outbox-v1');
      r.onsuccess = () => resolve(r.result);
      r.onerror = () => reject(r.error);
    });
    await new Promise((resolve, reject) => {
      const tx = db.transaction('meta', 'readwrite');
      tx.objectStore('meta').put({ pendingCount: 1, queueBytes: first.bytes }, 'totals');
      tx.oncomplete = resolve;
      tx.onerror = () => reject(tx.error);
    });
    db.close();
    await q.markError(first.id, first.revision, 'bad', { permanent: true });
    const marked = await q.getStats();
    await q.enqueue({ ...capture, captured_at: 'new' });
    const replaced = await q.getStats();
    return { marked, replaced, items: await q.peek() };
  });
  assert.equal(result.marked.rejectedCount, 1);
  assert.equal(result.replaced.rejectedCount, 0);
  assert.equal(result.replaced.pendingCount, 1);
  assert.equal(result.replaced.queueBytes, result.items[0].bytes);
  await context.close();
});
test('rejected summaries omit capture bodies and export only the selected revision', async () => {
  const context = await browser.newContext();
  const page = await context.newPage();
  await page.goto(server.url + '/blank');
  const result = await page.evaluate(async () => {
    const q = await import('/extension/lib/outbox.js');
    const cap = {
      url: 'https://example.org/x',
      title: 'title',
      blocks: [{ text: 'PRIVATE BODY' }],
    };
    await q.enqueue(cap);
    const [first] = await q.peek();
    await q.markError(first.id, first.revision, 'bad', { permanent: true });
    const summaries = await q.listRejected();
    const exported = await q.getRevision(first.id, first.revision);
    await q.enqueue({ ...cap, captured_at: 'replacement' });
    const stale = await q.getRevision(first.id, first.revision);
    return { summaries, exported, stale };
  });
  assert.doesNotMatch(JSON.stringify(result.summaries), /PRIVATE BODY/);
  assert.equal(result.exported.blocks[0].text, 'PRIVATE BODY');
  assert.equal(result.stale, null);
  await context.close();
});
test('background only grants queue management to extension pages', async () => {
  const context = await browser.newContext();
  const page = await context.newPage();
  await page.goto(server.url + '/blank');
  const result = await page.evaluate(async () => {
    const event = () => ({
      addListener(fn) {
        this.listener = fn;
      },
    });
    const messageEvent = event();
    const data = {};
    const storage = {
      get: async (keys) =>
        Object.fromEntries((Array.isArray(keys) ? keys : [keys]).map((key) => [key, data[key]])),
      set: async (values) => Object.assign(data, values),
    };
    globalThis.chrome = {
      runtime: {
        getURL: () => 'chrome-extension://fixture/',
        onMessage: messageEvent,
        onInstalled: event(),
        onStartup: event(),
        connectNative() {
          throw Error('offline');
        },
      },
      storage: { local: storage, session: storage },
      action: { setBadgeText: async () => {}, setBadgeBackgroundColor: async () => {} },
      alarms: { get: async () => ({}), create: async () => {}, onAlarm: event() },
      permissions: { onRemoved: event(), contains: async () => false },
      scripting: {
        getRegisteredContentScripts: async () => [],
        unregisterContentScripts: async () => {},
        registerContentScripts: async () => {},
        updateContentScripts: async () => {},
      },
      tabs: { query: async () => [] },
    };
    await import('/extension/background.js');
    const call = (type, sender) =>
      new Promise((resolve) =>
        messageEvent.listener({ type, id: 'missing', revision: 1 }, sender, resolve),
      );
    const content = { url: 'https://example.org/', tab: { id: 1 }, frameId: 0 };
    const denied = await Promise.all(
      ['queue.list', 'queue.export', 'queue.retry', 'queue.discard'].map((type) =>
        call(type, content),
      ),
    );
    const allowed = await call('queue.list', { url: 'chrome-extension://fixture/ui/options.html' });
    return { denied, allowed };
  });
  assert.ok(result.denied.every((reply) => reply.ok === false));
  assert.equal(result.allowed.ok, true);
  assert.deepEqual(result.allowed.data, []);
  await context.close();
});
