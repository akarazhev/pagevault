import { HOST_NAME, CAPTURE_FILES, normalizeSettings } from './lib/config.js';
import { pageOrigin, permissionPattern, isExtensionPage, validateCaptureSource } from './lib/policy.js';
import * as outbox from './lib/outbox.js';
import { deliverBatch } from './lib/delivery.js';
import { createNativeClient } from './lib/native-client.js';

const RETRY_ALARM = 'pagevault-delivery';
const UI_NATIVE_COMMANDS = new Set(['ping', 'stats', 'pages.list', 'pages.get', 'pages.export', 'pages.domains', 'pages.delete', 'captures.get']);
let ready;
let deliveryPromise;
let registrationPromise = Promise.resolve();
let stateWritePromise = Promise.resolve();
let nativeWritePromise = Promise.resolve();
let queueWritePromise = Promise.resolve();
const nativeClient = createNativeClient(chrome.runtime, HOST_NAME);

// Native mutations serialize, but durable capture acceptance never waits for native I/O.
function nativeWrite(operation) {
  const next = nativeWritePromise.catch(() => {}).then(operation);
  nativeWritePromise = next;
  return next;
}
function queueWrite(operation) {
  const next = queueWritePromise.catch(() => {}).then(operation);
  queueWritePromise = next;
  return next;
}

async function readState() {
  const state = await chrome.storage.local.get(['settings', 'enabledOrigins', 'lastError', 'lastSavedAt']);
  return { settings: normalizeSettings(state.settings), enabledOrigins: Array.isArray(state.enabledOrigins) ? state.enabledOrigins.filter(origin => pageOrigin(origin) === origin) : [], lastError: state.lastError || null, lastSavedAt: state.lastSavedAt || null };
}
function mutateState(mutation) {
  const next = stateWritePromise.catch(() => {}).then(async () => {
    const state = await readState();
    const changed = mutation(state) || state;
    await chrome.storage.local.set(changed);
    return changed;
  });
  stateWritePromise = next;
  return next;
}
function nativeRequest(request) {
  return nativeClient.request(request);
}
async function refreshBadge() {
  const [{ pendingCount }, state] = await Promise.all([outbox.getStats(), readState()]);
  await chrome.action.setBadgeText({ text: pendingCount ? (pendingCount > 99 ? '99+' : String(pendingCount)) : state.lastError ? '!' : '' });
  await chrome.action.setBadgeBackgroundColor({ color: state.lastError ? '#bb5a25' : '#087f78' });
}
async function flushQueue() {
  if (deliveryPromise) return deliveryPromise;
  deliveryPromise = nativeWrite(async () => {
    const result = await deliverBatch(outbox, capture => nativeRequest({ type: 'capture.save', capture }), { maxItems: 50 });
    const patch = { lastError: result.error || null };
    if (result.sent) patch.lastSavedAt = new Date().toISOString();
    await chrome.storage.local.set(patch);
    await refreshBadge();
    return { ...result, ...(await outbox.getStats()) };
  }).finally(() => { deliveryPromise = null; });
  return deliveryPromise;
}
async function ensureAlarm() {
  if (!(await chrome.alarms.get(RETRY_ALARM))) await chrome.alarms.create(RETRY_ALARM, { periodInMinutes: 1 });
}
async function registrationId(pattern) {
  const hash = await crypto.subtle.digest('SHA-256', new TextEncoder().encode(pattern));
  return 'pagevault-' + Array.from(new Uint8Array(hash).slice(0, 12), b => b.toString(16).padStart(2, '0')).join('');
}
function reconcileRegistrations() {
  const work = registrationPromise.catch(() => {}).then(async () => {
    const state = await readState();
    const patterns = [...new Set(state.enabledOrigins.map(permissionPattern))];
    const desired = [];
    for (const pattern of patterns) {
      if (await chrome.permissions.contains({ origins: [pattern] })) desired.push({
        id: await registrationId(pattern), matches: [pattern], js: CAPTURE_FILES,
        runAt: 'document_idle', allFrames: false, persistAcrossSessions: true,
      });
    }
    const registered = await chrome.scripting.getRegisteredContentScripts();
    const remove = registered.filter(item => item.id.startsWith('pagevault-') && !desired.some(value => value.id === item.id)).map(item => item.id);
    if (remove.length) await chrome.scripting.unregisterContentScripts({ ids: remove });
    const add = desired.filter(item => !registered.some(value => value.id === item.id));
    const update = desired.filter(item => registered.some(value => value.id === item.id));
    if (add.length) await chrome.scripting.registerContentScripts(add);
    if (update.length) await chrome.scripting.updateContentScripts(update);
  });
  registrationPromise = work;
  return work;
}
async function ensureContent(tabId) {
  try { const pong = await chrome.tabs.sendMessage(tabId, { type: 'capture.ping' }, { frameId: 0 }); if (pong?.ok) return; } catch { /* First injection. */ }
  await chrome.scripting.executeScript({ target: { tabId, frameIds: [0] }, files: CAPTURE_FILES });
}
async function notifyOrigin(origin, message, inject = false) {
  const tabs = await chrome.tabs.query({});
  await Promise.allSettled(tabs.filter(tab => pageOrigin(tab.url) === origin).map(async tab => {
    if (inject) await ensureContent(tab.id);
    await chrome.tabs.sendMessage(tab.id, message, { frameId: 0 });
  }));
}
async function oneShots() {
  return (await chrome.storage.session.get('oneShots')).oneShots || {};
}
async function status() {
  const [state, queue, tabs] = await Promise.all([readState(), outbox.getStats(), chrome.tabs.query({ active: true, currentWindow: true })]);
  let host;
  try { host = { connected: true, ...(await nativeRequest({ type: 'ping' })) }; }
  catch (error) { host = { connected: false, error: error.message }; }
  const tab = tabs[0]; const origin = pageOrigin(tab?.url);
  return { ...state, ...queue, host, activeTab: tab ? { id: tab.id, url: tab.url || '', origin, enabled: state.enabledOrigins.includes(origin), supported: !!origin } : null };
}
async function route(message, sender) {
  await ready;
  if (!message || typeof message.type !== 'string') throw new Error('Некорректное сообщение.');
  if (message.type === 'capture.config') {
    if (!sender.tab || (sender.frameId ?? 0) !== 0) throw new Error('Недопустимый источник.');
    const state = await readState();
    return { enabled: state.enabledOrigins.includes(pageOrigin(sender.url)), settings: state.settings };
  }
  if (message.type === 'capture.submit') {
    const [state, grants] = await Promise.all([readState(), oneShots()]);
    validateCaptureSource(message.capture, sender, { enabledOrigins: state.enabledOrigins, oneShots: grants });
    let result;
    try { result = await outbox.enqueue(message.capture); }
    catch (error) { await chrome.storage.local.set({ lastError: error.message }); await refreshBadge(); throw error; }
    if (grants[sender.tab.id]) { delete grants[sender.tab.id]; await chrome.storage.session.set({ oneShots: grants }); }
    void flushQueue().catch(error => chrome.storage.local.set({ lastError: error.message }));
    return result;
  }
  if (!isExtensionPage(sender, chrome.runtime.getURL(''))) throw new Error('Команда доступна только интерфейсу расширения.');
  switch (message.type) {
    case 'status': return status();
    case 'native': {
      if (!UI_NATIVE_COMMANDS.has(message.request?.type)) throw new Error('Недопустимая команда архива.');
      if (message.request.type === 'pages.delete') return nativeWrite(async () => {
        const result = await nativeRequest(message.request);
        // Reserve the purge behind submissions already validating/enqueuing. New captures
        // accepted after this purge may create a fresh archive when recording stays enabled.
        const pendingRemoved = result.deleted && typeof result.url === 'string'
          ? await queueWrite(() => outbox.removeUrl(result.url)) : 0;
        await refreshBadge();
        return { ...result, pendingRemoved, ...(await outbox.getStats()) };
      });
      return nativeRequest(message.request);
    }
    case 'queue.flush': return flushQueue();
    case 'site.enable': {
      const origin = pageOrigin(message.origin);
      if (!origin) throw new Error('Этот адрес не поддерживается.');
      if (!(await chrome.permissions.contains({ origins: [permissionPattern(origin)] }))) throw new Error('Сначала разрешите доступ к этому сайту.');
      const state = await mutateState(state => ({ ...state, enabledOrigins: [...new Set([...state.enabledOrigins, origin])] }));
      await reconcileRegistrations();
      await notifyOrigin(origin, { type: 'capture.configure', enabled: true, settings: state.settings }, true);
      return { enabledOrigins: state.enabledOrigins };
    }
    case 'site.disable': {
      const origin = pageOrigin(message.origin);
      if (!origin) throw new Error('Некорректный адрес сайта.');
      const state = await mutateState(state => ({ ...state, enabledOrigins: state.enabledOrigins.filter(value => value !== origin) }));
      await notifyOrigin(origin, { type: 'capture.stop' });
      await reconcileRegistrations();
      if (!state.enabledOrigins.some(value => permissionPattern(value) === permissionPattern(origin))) await chrome.permissions.remove({ origins: [permissionPattern(origin)] });
      return { enabledOrigins: state.enabledOrigins };
    }
    case 'capture.now': {
      const tab = Number.isInteger(message.tabId) ? await chrome.tabs.get(message.tabId) : (await chrome.tabs.query({ active: true, currentWindow: true }))[0];
      const origin = pageOrigin(tab?.url);
      if (!origin) throw new Error('Откройте обычную веб-страницу HTTP или HTTPS.');
      const state = await readState();
      if (!state.enabledOrigins.includes(origin)) {
        const grants = await oneShots();
        const now = Date.now();
        for (const [key, grant] of Object.entries(grants)) if (grant.expires <= now) delete grants[key];
        grants[tab.id] = { origin, expires: now + 60000 };
        await chrome.storage.session.set({ oneShots: grants });
      }
      await ensureContent(tab.id);
      const result = await chrome.tabs.sendMessage(tab.id, { type: 'capture.run', reason: 'manual', settings: state.settings }, { frameId: 0 });
      if (!result?.ok) throw new Error(result?.error || 'Не удалось получить снимок страницы.');
      return { queued: true };
    }
    case 'settings.set': {
      const state = await mutateState(state => ({ ...state, settings: normalizeSettings({ ...state.settings, ...message.settings }) }));
      await Promise.allSettled(state.enabledOrigins.map(origin => notifyOrigin(origin, { type: 'capture.configure', enabled: true, settings: state.settings })));
      return state.settings;
    }
    default: throw new Error('Неизвестная команда расширения.');
  }
}

chrome.runtime.onMessage.addListener((message, sender, respond) => {
  // Reserve acceptance at receipt, before asynchronous source checks/digest/IDB work.
  const work = message?.type === 'capture.submit'
    ? queueWrite(() => route(message, sender)) : route(message, sender);
  work.then(data => respond({ ok: true, data }), error => respond({ ok: false, error: error?.message || String(error) }));
  return true;
});
chrome.alarms.onAlarm.addListener(alarm => { if (alarm.name === RETRY_ALARM) void ready.then(flushQueue).catch(console.error); });
chrome.runtime.onInstalled.addListener(() => { void ready.then(reconcileRegistrations).catch(console.error); });
chrome.runtime.onStartup.addListener(() => { void ready.then(flushQueue).catch(console.error); });
chrome.permissions.onRemoved.addListener(() => {
  void ready.then(async () => {
    const state = await readState(); const kept = [];
    for (const origin of state.enabledOrigins) {
      if (await chrome.permissions.contains({ origins: [permissionPattern(origin)] })) kept.push(origin);
      else await notifyOrigin(origin, { type: 'capture.stop' });
    }
    await mutateState(current => ({ ...current, enabledOrigins: current.enabledOrigins.filter(value => kept.includes(value)) }));
    await reconcileRegistrations();
  }).catch(console.error);
});
ready = (async () => {
  await ensureAlarm();
  await reconcileRegistrations();
  await refreshBadge();
})();
void ready.then(flushQueue).catch(error => { console.error(error); void chrome.storage.local.set({ lastError: error.message }); });
