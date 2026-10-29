const { test } = require('node:test');
const assert = require('node:assert/strict');

test('shared modules select Firefox promise APIs when both namespaces exist', async () => {
  globalThis.browser = { runtime: { id: 'firefox' } };
  globalThis.chrome = { runtime: { id: 'chromium' } };
  try {
    const { extensionAPI } = await import('../extension/lib/browser.js?firefox');
    assert.equal(extensionAPI, globalThis.browser);
  } finally {
    delete globalThis.browser;
    delete globalThis.chrome;
  }
});

test('shared modules retain Chromium support without a browser namespace', async () => {
  globalThis.chrome = { runtime: { id: 'chromium' } };
  try {
    const { extensionAPI } = await import('../extension/lib/browser.js?chromium');
    assert.equal(extensionAPI, globalThis.chrome);
  } finally {
    delete globalThis.chrome;
  }
});
