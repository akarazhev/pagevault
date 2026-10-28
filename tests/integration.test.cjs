const { test, before, after } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const os = require('node:os');
const { execFileSync } = require('node:child_process');
const { playwright, browserOptions, serveProject } = require('./helpers.cjs');
const root = path.resolve(__dirname, '..');
// Chrome on Windows uses the real HKCU registration, not this temporary profile.
// Keep native integration isolated; other OSes still run the portable Go/DOM tests.
const nativeTest = (name, fn) =>
  test(
    name,
    {
      skip:
        process.platform !== 'linux'
          ? 'Native integration uses an isolated Linux Chromium profile'
          : false,
    },
    fn,
  );
let server, temp, profile, context, extensionId, archive, site, nativeManifest;

async function openBrowser() {
  const options = browserOptions();
  context = await playwright().chromium.launchPersistentContext(profile, {
    ...options,
    channel: 'chromium',
    ignoreDefaultArgs: ['--disable-extensions'],
    args: [
      ...options.args,
      `--disable-extensions-except=${path.join(temp, 'extension')}`,
      `--load-extension=${path.join(temp, 'extension')}`,
    ],
  });
  if (!context.serviceWorkers().length)
    await context.waitForEvent('serviceworker', { timeout: 15000 }).catch(() => {});
  assert.ok(context.serviceWorkers().length, 'MV3 background worker must load');
  archive = await context.newPage();
  await archive.goto(`chrome-extension://${extensionId}/ui/archive.html`);
}
async function message(type, args = {}) {
  const reply = await archive.evaluate((msg) => chrome.runtime.sendMessage(msg), { type, ...args });
  assert.equal(reply?.ok, true, reply?.error || `No reply to ${type}`);
  return reply.data;
}
async function until(read, predicate, description, timeout = 15000) {
  const stop = Date.now() + timeout;
  let last;
  while (Date.now() < stop) {
    last = await read();
    if (predicate(last)) return last;
    await new Promise((resolve) => setTimeout(resolve, 150));
  }
  assert.fail(`${description}: ${JSON.stringify(last)}`);
}
before(async () => {
  if (process.platform !== 'linux') return;
  assert.ok(
    fs.existsSync(path.join(root, 'extension/background.js')),
    'production background implementation must exist',
  );
  temp = fs.mkdtempSync(path.join(os.tmpdir(), 'pagevault-integration-'));
  profile = path.join(temp, 'profile');
  fs.mkdirSync(profile);
  server = await serveProject();
  const copy = path.join(temp, 'extension');
  fs.cpSync(path.join(root, 'extension'), copy, { recursive: true });
  const manifest = JSON.parse(fs.readFileSync(path.join(copy, 'manifest.json'), 'utf8'));
  // This fixture grants only the controlled local test site, equivalent to the user granting site access.
  manifest.host_permissions = ['http://127.0.0.1/*', 'http://insecure.pagevault.test/*'];
  fs.writeFileSync(path.join(copy, 'manifest.json'), JSON.stringify(manifest));
  extensionId = fs.readFileSync(path.join(root, 'docs/extension-id.txt'), 'utf8').trim();
  const binary =
    process.env.PAGEVAULT_HOST_BINARY ||
    path.join(temp, process.platform === 'win32' ? 'pagevault-host.exe' : 'pagevault-host');
  if (!process.env.PAGEVAULT_HOST_BINARY)
    execFileSync(process.env.PAGEVAULT_GO || 'go', ['build', '-o', binary, './cmd/pagevault'], {
      cwd: root,
      stdio: 'pipe',
    });
  const install = JSON.parse(
    execFileSync(
      binary,
      [
        'install',
        '--browser',
        'chrome',
        '--extension-manifest',
        path.join(copy, 'manifest.json'),
        '--manifest-dir',
        path.join(profile, 'NativeMessagingHosts'),
        '--runtime-dir',
        path.join(temp, 'host'),
        '--db',
        path.join(temp, 'archive.sqlite3'),
      ],
      { cwd: root, encoding: 'utf8' },
    ),
  );
  nativeManifest =
    install.manifest_path || path.join(profile, 'NativeMessagingHosts', 'com.pagevault.host.json');
  await openBrowser();
});
after(async () => {
  await context?.close();
  await server?.close();
  if (temp) fs.rmSync(temp, { recursive: true, force: true });
});

async function disableNativeHost() {
  const host = await message('native', { request: { type: 'ping' } });
  assert.ok(Number.isSafeInteger(host.process_id) && host.process_id > 0);
  fs.renameSync(nativeManifest, nativeManifest + '.disabled');
  process.kill(host.process_id, 'SIGKILL');
  await until(
    () => message('status'),
    (x) => !x.host.connected,
    'Native port observes the real process failure',
  );
  return host;
}

nativeTest(
  'real extension records DOM changes into SQLite and keeps removed forum blocks',
  async () => {
    const health = await message('status');
    assert.equal(health.host.connected, true, health.host.error);
    site = await context.newPage();
    await site.goto(server.url + '/tests/fixtures/integration-forum.html');
    await message('site.enable', { origin: server.url });
    const listing = await until(
      () => message('native', { request: { type: 'pages.list' } }),
      (x) => x.total === 1,
      'Initial page reaches SQLite',
    );
    const pageId = listing.items[0].id;
    await site.evaluate(() => {
      const p = document.createElement('p');
      p.id = 'new-reply';
      p.textContent = 'Новый ответ с кириллицей и emoji 🦊';
      document.querySelector('#thread').append(p);
    });
    await until(
      () => message('native', { request: { type: 'pages.list', query: 'Новый ответ' } }),
      (x) => x.total === 1,
      'Mutation reaches SQLite',
    );
    await site.evaluate(() => document.querySelector('#first').remove());
    await message('capture.now', {
      tabId: await site
        .evaluate(() => 0)
        .then(async () => {
          const tabs = await archive.evaluate(() => chrome.tabs.query({}));
          return tabs.find((x) => x.url?.includes('integration-forum')).id;
        }),
    });
    const data = await until(
      () => message('native', { request: { type: 'pages.get', page_id: pageId } }),
      (x) => x.total >= 3,
      'Changed page creates version',
    );
    assert.ok(data.page.block_count > 0);
    const whole = await message('native', { request: { type: 'pages.export', page_id: pageId } });
    const payload = JSON.parse(whole.json_chunk);
    const text = JSON.stringify(payload.blocks);
    assert.match(text, /Первое сообщение/);
    assert.match(text, /Новый ответ/);
    assert.equal(payload.archive_kind, 'observed_blocks');
  },
);

nativeTest(
  'mixed browser requests retain one Go host process and one SQLite connection',
  async () => {
    const initial = await message('native', { request: { type: 'ping' } });
    assert.equal(initial.db_connections, 1);
    assert.match(initial.go_version, /^go/);
    assert.equal(typeof initial.host_id, 'string');
    assert.ok(initial.host_id.length > 0);
    for (let i = 0; i < 12; i++) {
      const [ping, stats, pages, domains] = await Promise.all(
        ['ping', 'stats', 'pages.list', 'pages.domains'].map((type) =>
          message('native', { request: { type } }),
        ),
      );
      assert.equal(ping.host_id, initial.host_id);
      assert.equal(ping.process_id, initial.process_id);
      assert.equal(ping.db_connections, 1);
      assert.ok(stats.pages >= 1);
      assert.ok(pages.total >= 1);
      assert.ok(domains.items.length >= 1);
    }
  },
);

nativeTest('SPA navigation creates a separate page without full reload', async () => {
  await site.evaluate(() => {
    history.pushState({}, '', location.pathname + '?tab=second');
    document.title = 'Вторая вкладка';
    document
      .querySelector('#thread')
      .append(
        Object.assign(document.createElement('p'), { textContent: 'Содержимое второй вкладки' }),
      );
  });
  const list = await until(
    () => message('native', { request: { type: 'pages.list', query: 'tab=second' } }),
    (x) => x.total === 1,
    'SPA address is captured',
  );
  assert.match(list.items[0].url, /tab=second/);
});

nativeTest(
  'ordinary HTTP pages are captured even when crypto.randomUUID is unavailable',
  async () => {
    await context.route('http://insecure.pagevault.test/**', (route) =>
      route.fulfill({
        contentType: 'text/html',
        body: '<!doctype html><title>HTTP forum</title><h1>Обычный HTTP форум</h1><p>Содержимое без HTTPS</p>',
      }),
    );
    const plain = await context.newPage();
    await plain.goto('http://insecure.pagevault.test/thread');
    assert.equal(await plain.evaluate(() => isSecureContext), false);
    await message('site.enable', { origin: 'http://insecure.pagevault.test' });
    const list = await until(
      () =>
        message('native', { request: { type: 'pages.list', query: 'insecure.pagevault.test' } }),
      (x) => x.total === 1,
      'HTTP page capture',
      5000,
    );
    assert.equal(list.items[0].title, 'HTTP forum');
    await plain.close();
  },
);

nativeTest(
  'native host failure retains queue across browser restart and replay commits it',
  async () => {
    const original = await disableNativeHost();
    await site.evaluate(() =>
      document.querySelector('#thread').append(
        Object.assign(document.createElement('p'), {
          textContent: 'Снимок во время отключения помощника',
        }),
      ),
    );
    await until(
      () => message('status'),
      (x) => x.pendingCount > 0 && !x.host.connected,
      'Offline snapshot remains queued',
    );
    await context.close();
    context = null;
    await openBrowser();
    const retained = await message('status');
    assert.ok(retained.pendingCount > 0);
    fs.renameSync(nativeManifest + '.disabled', nativeManifest);
    await message('queue.flush');
    const restored = await until(
      () => message('status'),
      (x) => x.pendingCount === 0 && x.host.connected,
      'Queued snapshot is delivered after restart',
    );
    assert.equal(restored.lastError, null);
    const list = await message('native', {
      request: { type: 'pages.list', query: 'отключения помощника' },
    });
    assert.equal(list.total, 1);
    const replacement = await message('native', { request: { type: 'ping' } });
    assert.notEqual(replacement.host_id, original.host_id);
    assert.equal(replacement.db_connections, 1);
  },
);

async function seedQueue(captures) {
  return archive.evaluate(async (captures) => {
    const q = await import(chrome.runtime.getURL('lib/outbox.js'));
    for (const capture of captures) await q.enqueue(capture);
    return q.getStats();
  }, captures);
}
function queuedCapture(url, text) {
  return {
    schema_version: 1,
    url,
    title: 'Delete regression',
    captured_at: new Date().toISOString(),
    blocks: [{ type: 'paragraph', text }],
  };
}

nativeTest(
  'confirmed deletion purges offline copies, retains unrelated URLs and preserves queue on native failure',
  async () => {
    const url = 'https://delete.pagevault.test/offline';
    const other = 'https://delete.pagevault.test/unrelated';
    await seedQueue([queuedCapture(url, 'Archived original')]);
    await message('queue.flush');
    const {
      items: [page],
    } = await message('native', { request: { type: 'pages.list', query: url } });
    await seedQueue([
      queuedCapture(url, 'Offline copy one'),
      queuedCapture(url, 'Offline copy two'),
      queuedCapture(other, 'Keep unrelated copy'),
    ]);
    const before = await message('status');
    assert.equal(before.pendingCount, 3);
    await disableNativeHost();
    try {
      const failure = await archive.evaluate(
        (page_id) =>
          chrome.runtime.sendMessage({
            type: 'native',
            request: { type: 'pages.delete', page_id },
          }),
        page.id,
      );
      assert.equal(failure.ok, false);
      const after = await message('status');
      assert.equal(after.pendingCount, before.pendingCount);
      assert.equal(after.queueBytes, before.queueBytes);
    } finally {
      fs.renameSync(nativeManifest + '.disabled', nativeManifest);
    }
    const deleted = await message('native', {
      request: { type: 'pages.delete', page_id: page.id },
    });
    assert.equal(deleted.deleted, true);
    assert.equal(deleted.pendingRemoved, 2);
    assert.equal((await message('status')).pendingCount, 1);
    assert.equal(await archive.evaluate(() => chrome.action.getBadgeText({})), '1');
    await message('queue.flush');
    assert.equal(
      (await message('native', { request: { type: 'pages.list', query: url } })).total,
      0,
    );
    assert.equal(
      (await message('native', { request: { type: 'pages.list', query: other } })).total,
      1,
    );
  },
);

nativeTest(
  'explicit deletion waits for in-flight delivery before deleting and prevents old replay resurrection',
  async () => {
    const url = server.url + '/delete-inflight';
    // Required fixture host permissions cannot be removed; disable recording state directly.
    await archive.evaluate(() => chrome.storage.local.set({ enabledOrigins: [] }));
    const source = await context.newPage();
    await source.goto(server.url + '/blank');
    const tabId = await archive.evaluate(
      async (url) => (await chrome.tabs.query({})).find((tab) => tab.url === url).id,
      server.url + '/blank',
    );
    await seedQueue([queuedCapture(url, 'Initial copy')]);
    await message('queue.flush');
    const {
      items: [page],
    } = await message('native', { request: { type: 'pages.list', query: url } });
    const worker = context.serviceWorkers()[0];
    const previousHost = await message('native', { request: { type: 'ping' } });
    await worker.evaluate(() => {
      const original = chrome.runtime.connectNative.bind(chrome.runtime);
      const restorations = [];
      globalThis.deliveryHeld = false;
      globalThis.deleteNativeStarted = false;
      chrome.runtime.connectNative = (host) => {
        const port = original(host);
        const kinds = new Map();
        const listeners = new Map();
        const post = port.postMessage.bind(port),
          add = port.onMessage.addListener.bind(port.onMessage),
          remove = port.onMessage.removeListener.bind(port.onMessage);
        port.postMessage = (request) => {
          kinds.set(request.id, request.type);
          if (request.type === 'pages.delete') globalThis.deleteNativeStarted = true;
          return post(request);
        };
        port.onMessage.addListener = (callback) => {
          const wrapped = (response) => {
            const kind = kinds.get(response.id);
            kinds.delete(response.id);
            if (kind === 'capture.save' && !globalThis.deliveryHeld) {
              globalThis.deliveryHeld = true;
              globalThis.releaseDelivery = () => callback(response);
            } else callback(response);
          };
          listeners.set(callback, wrapped);
          add(wrapped);
        };
        port.onMessage.removeListener = (callback) => {
          remove(listeners.get(callback) || callback);
          listeners.delete(callback);
        };
        restorations.push(() => {
          for (const [callback, wrapped] of listeners) {
            remove(wrapped);
            add(callback);
          }
          port.postMessage = post;
          port.onMessage.addListener = add;
          port.onMessage.removeListener = remove;
        });
        return port;
      };
      globalThis.restoreNative = () => {
        chrome.runtime.connectNative = original;
        for (const restore of restorations) restore();
      };
    });
    process.kill(previousHost.process_id, 'SIGKILL');
    await until(
      () => message('status'),
      (x) => x.host.connected && x.host.host_id !== previousHost.host_id,
      'Replacement persistent port is observable',
    );
    await seedQueue([queuedCapture(url, 'Committed but acknowledgement held')]);
    const flushing = message('queue.flush');
    await until(
      () => worker.evaluate(() => globalThis.deliveryHeld),
      Boolean,
      'Real SQLite commit acknowledgement held',
    );
    // Native I/O must not delay write-ahead acceptance of a new content submission.
    const accepted = await Promise.race([
      archive.evaluate(
        async ({ tabId, capture, origin }) => {
          await chrome.storage.session.set({
            oneShots: { [tabId]: { origin, expires: Date.now() + 60000 } },
          });
          const [{ result }] = await chrome.scripting.executeScript({
            target: { tabId },
            func: (capture) => chrome.runtime.sendMessage({ type: 'capture.submit', capture }),
            args: [capture],
          });
          return result;
        },
        {
          tabId,
          capture: queuedCapture(url, 'Accepted during slow native I/O'),
          origin: server.url,
        },
      ),
      new Promise((_, reject) =>
        setTimeout(() => reject(new Error('Capture acceptance waited for native I/O')), 2000),
      ),
    ]).catch(async (error) => {
      await worker.evaluate(() => globalThis.releaseDelivery());
      throw error;
    });
    assert.equal(accepted.ok, true, accepted.error);
    assert.equal(accepted.data.queued, true);
    await source.close();
    // Start deletion while the committed write still belongs to an in-flight batch.
    const deleting = message('native', { request: { type: 'pages.delete', page_id: page.id } });
    await archive.evaluate(() => new Promise((resolve) => setTimeout(resolve, 150)));
    const deletionStartedEarly = await worker.evaluate(() => globalThis.deleteNativeStarted);
    await worker.evaluate(() => globalThis.releaseDelivery());
    try {
      await flushing;
      await deleting;
      assert.equal(
        deletionStartedEarly,
        false,
        'SQLite delete must wait for the active delivery batch',
      );
      await message('queue.flush');
      assert.equal(
        (await message('native', { request: { type: 'pages.list', query: url } })).total,
        0,
      );
    } finally {
      await worker.evaluate(() => globalThis.restoreNative());
    }
  },
);

nativeTest('archive UI downloads the Go-backed SQLite page as valid JSON', async () => {
  const errors = [];
  archive.on('pageerror', (error) => errors.push(error.message));
  await archive.reload();
  await archive.locator('#search').fill('integration-forum');
  await archive.locator('#search-form button').click();
  await until(
    () => archive.locator('.page-item').count(),
    (count) => count === 2,
    'UI search returns the two captured forum URLs',
  );
  await archive.locator('.page-item').first().click();
  await archive.waitForFunction(
    () => !document.querySelector('#export').disabled && !document.querySelector('#detail').hidden,
  );
  const [download] = await Promise.all([
    archive.waitForEvent('download'),
    archive.locator('#export').click(),
  ]);
  const saved = JSON.parse(fs.readFileSync(await download.path(), 'utf8'));
  assert.equal(saved.schema_version, 1);
  assert.equal(saved.archive_kind, 'observed_blocks');
  assert.match(saved.page.url, /integration-forum/);
  assert.ok(saved.blocks.length > 0);
  const expected = await message('native', {
    request: { type: 'pages.export', page_id: saved.page.id },
  });
  assert.deepEqual(saved, JSON.parse(expected.json_chunk));
  // The newest entry is the SPA URL first seen after the original post was removed.
  assert.match(JSON.stringify(saved.blocks), /Новый ответ с кириллицей и emoji 🦊/);
  await archive.locator('#view-versions').click();
  await archive.waitForFunction(
    () =>
      !document.querySelector('#version-bar').hidden &&
      document.querySelector('#version-select').options.length > 0,
  );
  assert.deepEqual(errors, []);
});

nativeTest('content scripts cannot list, export, retry, or discard queued captures', async () => {
  site = await context.newPage();
  await site.goto(server.url + '/tests/fixtures/integration-forum.html');
  const tabId = await archive.evaluate(
    async (url) => (await chrome.tabs.query({})).find((tab) => tab.url === url).id,
    site.url(),
  );
  const results = await archive.evaluate(async (tabId) => {
    const [{ result }] = await chrome.scripting.executeScript({
      target: { tabId },
      func: async () => {
        return Promise.all(
          ['queue.list', 'queue.export', 'queue.retry', 'queue.discard'].map((type) =>
            chrome.runtime.sendMessage({ type, id: 'synthetic', revision: 1 }),
          ),
        );
      },
    });
    return result;
  }, tabId);
  assert.equal(results.length, 4);
  assert.ok(results.every((result) => result?.ok === false));
});
