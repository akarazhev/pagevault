const test = require('node:test');
const assert = require('node:assert/strict');
const path = require('node:path');
const { pathToFileURL } = require('node:url');

async function factory() {
  try { return (await import(pathToFileURL(path.resolve(__dirname, '../extension/lib/native-client.js')))).createNativeClient; }
  catch (error) { assert.fail(`Required native transport module unavailable: ${error.message}`); }
}

function event() {
  const listeners = [];
  return { addListener(listener) { listeners.push(listener); }, emit(value) { for (const listener of listeners) listener(value); } };
}
function harness(options = {}) {
  const ports = [];
  const timers = new Map();
  let nextId = 0;
  let nextTimer = 0;
  const runtime = {
    lastError: undefined,
    connectNative(hostName) {
      if (runtime.connectError) throw runtime.connectError;
      const port = {
        hostName, sent: [], disconnectCalls: 0,
        onMessage: event(), onDisconnect: event(),
        postMessage(message) { if (port.sendError) throw port.sendError; port.sent.push(message); },
        disconnect() { port.disconnectCalls++; port.onDisconnect.emit(); },
        reply(index, data = {}, ok = true) { port.onMessage.emit({ id: port.sent[index].id, ok, ...(ok ? { data } : { error: { message: data } }) }); },
        close(message) {
          runtime.lastError = message ? { message } : undefined;
          try { port.onDisconnect.emit(); } finally { runtime.lastError = undefined; }
        },
      };
      ports.push(port);
      return port;
    },
  };
  return {
    ports, timers, runtime,
    options: {
      randomUUID: () => `request-${++nextId}`,
      setTimeout(callback, delay) { const timer = ++nextTimer; timers.set(timer, { callback, delay }); return timer; },
      clearTimeout(timer) { timers.delete(timer); },
      ...options,
    },
    expireReads() { for (const [id, timer] of [...timers]) { timers.delete(id); timer.callback(); } },
  };
}
async function setup(options) {
  const createNativeClient = await factory();
  const h = harness(options);
  return { ...h, client: createNativeClient(h.runtime, 'com.pagevault.host', h.options) };
}
const turn = () => new Promise(resolve => setImmediate(resolve));

test('native client lazily reuses one port for mixed sequential and concurrent requests', async () => {
  const h = await setup();
  assert.equal(h.ports.length, 0);
  const ping = h.client.request({ type: 'ping', id: 'untrusted-caller-id' });
  assert.equal(h.ports.length, 1);
  const port = h.ports[0];
  assert.equal(port.hostName, 'com.pagevault.host');
  assert.equal(port.sent[0].id, 'request-1');
  port.reply(0, { version: '1.1.0' });
  assert.deepEqual(await ping, { version: '1.1.0' });
  const save = h.client.request({ type: 'capture.save', capture: { url: 'https://example.org/' } });
  const stats = h.client.request({ type: 'stats' });
  assert.equal(h.ports.length, 1);
  assert.equal(port.sent[1].capture.url, 'https://example.org/');
  port.reply(1, { capture_id: 7 });
  port.reply(2, { pages: 1 });
  assert.deepEqual(await save, { capture_id: 7 });
  assert.deepEqual(await stats, { pages: 1 });
  assert.equal(h.timers.size, 0);
});

test('native replies correlate by ID when host replies in reverse order', async () => {
  const h = await setup();
  const first = h.client.request({ type: 'pages.list' });
  const second = h.client.request({ type: 'pages.get', page_id: 4 });
  h.ports[0].reply(1, { title: 'second' });
  h.ports[0].onMessage.emit({ id: 'unknown', ok: true, data: 'ignore' });
  h.ports[0].reply(0, { pages: ['first'] });
  assert.deepEqual(await first, { pages: ['first'] });
  assert.deepEqual(await second, { title: 'second' });
});

test('native host failure and malformed correlated reply reject only their requests', async () => {
  const h = await setup();
  const failure = h.client.request({ type: 'pages.get' });
  const failed = assert.rejects(failure, /Нет такой страницы/);
  const malformed = h.client.request({ type: 'stats' });
  const invalid = assert.rejects(malformed, /Некорректный ответ локального помощника/);
  const mutation = h.client.request({ type: 'capture.save', capture: {} });
  h.ports[0].reply(0, 'Нет такой страницы', false);
  h.ports[0].onMessage.emit({ id: h.ports[0].sent[1].id, data: {} });
  h.ports[0].reply(2, { capture_id: 2 });
  await Promise.all([failed, invalid]);
  assert.deepEqual(await mutation, { capture_id: 2 });
  assert.equal(h.ports.length, 1);
  assert.equal(h.timers.size, 0);
});

test('actual disconnect rejects all pending and reconnects only when next request arrives', async () => {
  const h = await setup();
  const read = h.client.request({ type: 'stats' });
  const save = h.client.request({ type: 'capture.save', capture: {} });
  const failures = [assert.rejects(read, /Помощник SQLite недоступен.*host exited/), assert.rejects(save, /host exited/)];
  h.ports[0].close('host exited');
  await Promise.all(failures);
  assert.equal(h.timers.size, 0);
  await turn();
  assert.equal(h.ports.length, 1);
  const retry = h.client.request({ type: 'ping' });
  assert.equal(h.ports.length, 2);
  h.ports[1].reply(0, { reconnected: true });
  assert.deepEqual(await retry, { reconnected: true });
});

test('stale generation messages and disconnects cannot settle a replacement request', async () => {
  const h = await setup({ randomUUID: () => 'same-id-across-generations' });
  const old = h.client.request({ type: 'ping' });
  const rejected = assert.rejects(old, /Помощник SQLite недоступен/);
  h.ports[0].close();
  await rejected;
  let settled = false;
  const current = h.client.request({ type: 'pages.delete', page_id: 2 });
  current.then(() => { settled = true; }, () => { settled = true; });
  h.ports[0].reply(0, { stale: true });
  h.ports[0].close('late old disconnect');
  await turn();
  assert.equal(settled, false);
  assert.equal(h.ports.length, 2);
  h.ports[1].reply(0, { deleted: true });
  assert.deepEqual(await current, { deleted: true });
  const extra = h.client.request({ type: 'stats' });
  h.ports[1].reply(1, { pages: 2 });
  assert.deepEqual(await extra, { pages: 2 });
  assert.equal(h.ports.length, 2);
});

test('read timeout expires only that read while save and deletion await actual replies', async () => {
  const h = await setup();
  const read = h.client.request({ type: 'pages.list' });
  const expired = assert.rejects(read, /не ответил за 20 секунд/);
  let saves = 0;
  const save = h.client.request({ type: 'capture.save', capture: {} }).then(data => { saves++; return data; });
  const deletion = h.client.request({ type: 'pages.delete', page_id: 5 }).then(data => { saves++; return data; });
  assert.equal(h.timers.size, 1);
  assert.equal([...h.timers.values()][0].delay, 20000);
  h.expireReads();
  await expired;
  await turn();
  assert.equal(saves, 0);
  assert.equal(h.ports[0].disconnectCalls, 0);
  const after = h.client.request({ type: 'stats' });
  assert.equal(h.ports.length, 1);
  h.ports[0].reply(0, { late: true });
  h.ports[0].reply(3, { pages: 1 });
  assert.deepEqual(await after, { pages: 1 });
  h.ports[0].reply(1, { capture_id: 5 });
  h.ports[0].reply(2, { deleted: true });
  assert.deepEqual(await save, { capture_id: 5 });
  assert.deepEqual(await deletion, { deleted: true });
  assert.equal(saves, 2);
  assert.equal(h.timers.size, 0);
});

test('postMessage exception cleans only its request and keeps in-flight mutation alive', async () => {
  const h = await setup();
  const save = h.client.request({ type: 'capture.save', capture: {} });
  const port = h.ports[0];
  port.sendError = new Error('cannot serialize message');
  await assert.rejects(h.client.request({ type: 'stats' }), /Помощник SQLite недоступен.*cannot serialize message/);
  assert.equal(h.timers.size, 0);
  assert.equal(port.disconnectCalls, 0);
  delete port.sendError;
  const next = h.client.request({ type: 'ping' });
  assert.equal(h.ports.length, 1);
  port.reply(0, { capture_id: 9 });
  port.reply(1, { connected: true });
  assert.deepEqual(await save, { capture_id: 9 });
  assert.deepEqual(await next, { connected: true });
});

test('connectNative synchronous failure rejects clearly and permits a later lazy retry', async () => {
  const h = await setup();
  h.runtime.connectError = new Error('host not installed');
  await assert.rejects(h.client.request({ type: 'ping' }), /Помощник SQLite недоступен.*host not installed/);
  assert.equal(h.timers.size, 0);
  delete h.runtime.connectError;
  const retry = h.client.request({ type: 'ping' });
  h.ports[0].reply(0, { ready: true });
  assert.deepEqual(await retry, { ready: true });
});
