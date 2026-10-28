const { test } = require('node:test');
const assert = require('node:assert/strict');

test('connection states preserve compatible older hosts and reject known incompatible schemas', async () => {
  const { connectionState } = await import('../extension/lib/connection.js');
  const offline = connectionState(
    { connected: false, error: 'cannot open existing archive' },
    '1.3.0',
  );
  assert.equal(offline.kind, 'unavailable');
  assert.equal(offline.usable, false);
  assert.match(offline.error, /cannot open/);
  for (const [version, kind] of [
    ['1.3.0', 'ready'],
    ['1.2.0', 'update'],
    ['1.4.0', 'ready'],
    ['unknown', 'ready'],
  ]) {
    const state = connectionState(
      { connected: true, version, schema_version: 1, db_path: '/archive.sqlite3' },
      '1.3.0',
    );
    assert.equal(state.kind, kind);
    assert.equal(state.usable, true);
    assert.equal(state.dbPath, '/archive.sqlite3');
  }
  const incompatible = connectionState(
    { connected: true, version: '2.0.0', schema_version: 2 },
    '1.3.0',
  );
  assert.equal(incompatible.kind, 'incompatible');
  assert.equal(incompatible.usable, false);
});

test('platform instructions keep downloads on the published releases page', async () => {
  const { normalizePlatform, platformInstructions, DOWNLOADS_URL } =
    await import('../extension/lib/connection.js');
  assert.equal(normalizePlatform('mac'), 'mac');
  assert.equal(normalizePlatform('win'), 'win');
  assert.equal(normalizePlatform('linux'), 'linux');
  assert.equal(normalizePlatform('android'), 'unknown');
  assert.equal(normalizePlatform(undefined), 'unknown');
  assert.equal(DOWNLOADS_URL, 'https://github.com/akarazhev/pagevault/releases');
  assert.match(platformInstructions('mac').detail, /\.pkg/);
  assert.equal(platformInstructions('mac').command, '');
  assert.match(platformInstructions('win').command, /install-windows/);
  assert.match(platformInstructions('linux').command, /bash install.sh chrome/);
});
