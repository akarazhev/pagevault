const { test, before, after } = require('node:test');
const assert = require('node:assert/strict');
const { launchBrowser, serveProject } = require('./helpers.cjs');
let browser, server;
before(async () => {
  server = await serveProject();
  browser = await launchBrowser();
});
after(async () => {
  await browser?.close();
  await server?.close();
});

async function fixture(options = {}) {
  const page = await browser.newPage();
  page.setDefaultTimeout(3000);
  await page.addInitScript((options) => {
    window.fixture = {
      host: { connected: false, error: 'Native host unavailable' },
      reject: false,
      pending: false,
      calls: 0,
      resolvers: [],
      ...options,
    };
    window.releaseChecks = () => {
      for (const resolve of fixture.resolvers.splice(0)) resolve();
    };
    window.chrome = {
      runtime: {
        id: 'ijkioljfdaaplhaldbdpabckkfcieiee',
        getManifest: () => ({
          version: '1.4.0',
          ...(options.firefox
            ? { browser_specific_settings: { gecko: { id: 'pagevault@pagevault.local' } } }
            : {}),
        }),
        getPlatformInfo: async () => {
          if (fixture.platformFailure) throw Error('platform lookup failed');
          return { os: 'mac' };
        },
        sendMessage: async (message) => {
          if (message.type === 'queue.list') return { ok: true, data: fixture.rejected || [] };
          if (message.type === 'queue.retry' || message.type === 'queue.discard') {
            fixture.action = message;
            fixture.rejected = [];
            return { ok: true, data: { changed: true } };
          }
          if (message.type !== 'status') throw Error('Unexpected message ' + message.type);
          fixture.calls++;
          if (fixture.pending) await new Promise((resolve) => fixture.resolvers.push(resolve));
          if (fixture.reject) throw Error('Controlled status failure');
          return {
            ok: true,
            data: {
              host: fixture.host,
              pendingCount: 3,
              queueBytes: 256,
              enabledOrigins: [],
              settings: {
                saveHtml: true,
                captureIntervalSeconds: 30,
                debounceMs: 1200,
                maxBlocks: 5000,
                maxTextChars: 1000000,
                maxHtmlChars: 2000000,
              },
            },
          };
        },
      },
    };
    if (options.firefox) {
      window.browser = window.chrome;
      window.browser.runtime.id = 'pagevault@pagevault.local';
      delete window.chrome;
    }
  }, options);
  await page.goto(server.url + '/extension/ui/options.html');
  await page.waitForFunction(() => !document.querySelector('#settings-fields').disabled);
  return page;
}

test('unavailable helper shows install flow, actual extension ID, download page and macOS steps', async () => {
  const page = await fixture();
  try {
    assert.equal(await page.locator('#open-setup').isVisible(), true);
    assert.equal(await page.locator('#host-version').innerText(), 'Unknown');
    assert.match(await page.locator('#host-error').innerText(), /Native host unavailable/);
    assert.equal(
      await page.locator('#extension-id').innerText(),
      'ijkioljfdaaplhaldbdpabckkfcieiee',
    );
    assert.equal(
      await page.locator('#download-link').getAttribute('href'),
      'https://github.com/akarazhev/pagevault/releases',
    );
    assert.equal(await page.locator('#download-link').getAttribute('target'), '_blank');
    assert.match(await page.locator('#download-link').getAttribute('rel'), /noopener/);
    await page.waitForFunction(() => document.querySelector('#download-platform').value === 'mac');
    assert.match(await page.locator('#platform-detail').innerText(), /\.pkg/);
    assert.equal(await page.locator('#platform-command').isVisible(), false);
    assert.match(await page.locator('#queue-status').innerText(), /3 captures/);
  } finally {
    await page.close();
  }
});

test('Firefox settings use promise APIs and show Firefox helper installation commands', async () => {
  const page = await fixture({ firefox: true });
  try {
    assert.equal(await page.locator('#extension-id').innerText(), 'pagevault@pagevault.local');
    await page.locator('#download-platform').selectOption('linux');
    assert.equal(
      await page.locator('#platform-command code').innerText(),
      'bash install.sh firefox',
    );
    await page.locator('#download-platform').selectOption('win');
    assert.equal(
      await page.locator('#platform-command code').innerText(),
      '.\\install-windows.cmd firefox',
    );
    await page.locator('#check-connection').click();
    await page.waitForFunction(() => !document.querySelector('#check-connection').disabled);
    assert.match(await page.locator('#host-error').innerText(), /Native host unavailable/);
  } finally {
    await page.close();
  }
});

test('a shared connection check disables both buttons, recovers and preserves unsaved settings', async () => {
  const page = await fixture();
  try {
    await page.locator('#captureIntervalSeconds').fill('77');
    await page.evaluate(() => {
      fixture.pending = true;
      document.querySelector('#check-connection').click();
      document.querySelector('#setup-check-connection').click();
    });
    assert.equal(await page.locator('#check-connection').isDisabled(), true);
    assert.equal(await page.locator('#setup-check-connection').isDisabled(), true);
    assert.equal(await page.evaluate(() => fixture.calls), 2);
    await page.evaluate(() => {
      fixture.host = {
        connected: true,
        version: '1.2.0',
        schema_version: 1,
        db_path: '/tmp/сохранённый архив.sqlite3',
      };
      fixture.pending = false;
      releaseChecks();
    });
    await page.waitForFunction(() => !document.querySelector('#setup-check-connection').disabled);
    assert.match(await page.locator('#host-status').innerText(), /SQLite connected/);
    assert.equal(await page.locator('#host-version').innerText(), '1.2.0');
    assert.equal(await page.locator('#db-path').innerText(), '/tmp/сохранённый архив.sqlite3');
    assert.equal(await page.locator('#captureIntervalSeconds').inputValue(), '77');
    assert.match(await page.locator('#connection-guidance').innerText(), /updat/i);
    await page.evaluate(() => {
      fixture.reject = true;
    });
    await page.locator('#setup-check-connection').click();
    await page.waitForFunction(() => !document.querySelector('#setup-check-connection').disabled);
    assert.match(await page.locator('#host-error').innerText(), /Controlled status failure/);
    assert.doesNotMatch(await page.locator('#db-path').innerText(), /сохранённый/);
    await page.evaluate(() => {
      fixture.reject = false;
      fixture.host = {
        connected: true,
        version: '1.4.0',
        schema_version: 1,
        db_path: '/restored.sqlite3',
      };
    });
    await page.locator('#setup-check-connection').click();
    await page.waitForFunction(
      () => document.querySelector('#db-path').textContent === '/restored.sqlite3',
    );
    assert.equal(await page.locator('#open-setup').isVisible(), false);
  } finally {
    await page.close();
  }
});

test('unsupported schema and untrusted helper values render safely', async () => {
  const text = '<img src=x onerror="window.pwned=true">';
  const page = await fixture({
    host: { connected: true, version: text, schema_version: 2, db_path: text, error: text },
  });
  try {
    assert.match(await page.locator('#host-status').innerText(), /incompatible/i);
    assert.equal(
      await page.locator('#host-dot').evaluate((n) => n.classList.contains('online')),
      false,
    );
    assert.equal(await page.locator('#open-setup').isVisible(), true);
    assert.equal(await page.locator('#host-version').innerText(), text);
    assert.equal(await page.locator('#db-path').innerText(), text);
    assert.equal(await page.locator('img').count(), 0);
    assert.equal(await page.evaluate(() => !!window.pwned), false);
  } finally {
    await page.close();
  }
});

test('platform failure leaves a manual selector and Windows/Linux instructions', async () => {
  const page = await fixture({ platformFailure: true });
  try {
    assert.equal(await page.locator('#download-platform').inputValue(), 'unknown');
    await page.locator('#download-platform').selectOption('win');
    assert.match(await page.locator('#platform-command').innerText(), /install-windows/);
    await page.locator('#download-platform').selectOption('linux');
    assert.match(await page.locator('#platform-command').innerText(), /bash install.sh chrome/);
    await page.locator('#download-platform').selectOption('mac');
    assert.equal(await page.locator('#platform-command').isVisible(), false);
  } finally {
    await page.close();
  }
});

test('rejected capture recovery displays its error and retries the displayed revision', async () => {
  const page = await fixture({
    rejected: [
      {
        id: 'synthetic',
        revision: 3,
        title: 'Test capture',
        url: 'https://example.org/unique-page',
        createdAt: Date.UTC(2026, 0, 2, 12),
        error: 'Invalid capture',
        bytes: 100,
      },
    ],
  });
  try {
    await page.getByText('Invalid capture').waitFor();
    assert.match(
      await page.locator('#rejected-list').innerText(),
      /https:\/\/example\.org\/unique-page/,
    );
    assert.match(await page.locator('#rejected-list').innerText(), /2026/);
    await page.getByRole('button', { name: 'Retry', exact: true }).click();
    assert.deepEqual(await page.evaluate(() => fixture.action), {
      type: 'queue.retry',
      id: 'synthetic',
      revision: 3,
    });
    await page.getByText('No rejected captures.').waitFor();
  } finally {
    await page.close();
  }
});
test('rejected capture controls fit within a mobile viewport', async () => {
  const page = await fixture({
    rejected: [
      {
        id: 'synthetic',
        revision: 1,
        title: 'Example page',
        error: 'Capture rejected: invalid URL',
      },
    ],
  });
  try {
    await page.setViewportSize({ width: 390, height: 844 });
    await page.getByText('Capture rejected: invalid URL').waitFor();
    const bounds = await page.locator('#rejected-list button').evaluateAll((buttons) =>
      buttons.map((button) => ({
        left: button.getBoundingClientRect().left,
        right: button.getBoundingClientRect().right,
      })),
    );
    assert.equal(bounds.length, 3);
    assert.ok(bounds.every((b) => b.left >= 0 && b.right <= 390));
  } finally {
    await page.close();
  }
});
