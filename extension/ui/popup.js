import { $, send, notice, errorText, hostSummary, hostConnection, date, bytes } from './common.js';
let current = null;
let busy = false;
const message = $('#message');
function render(status) {
  current = status;
  const tab = status.activeTab;
  let hostname = 'Page unavailable';
  if (tab?.supported) {
    try {
      hostname = new URL(tab.url).hostname;
    } catch {
      // Keep the fallback when the active tab URL cannot be parsed.
    }
  }
  $('#site-name').textContent = hostname;
  $('#site-url').textContent = tab?.supported ? tab.origin : 'Open an HTTP or HTTPS page.';
  $('#site-state').textContent = tab?.enabled
    ? 'Automatic recording on'
    : 'Automatic recording off';
  $('#site-state').classList.toggle('teal-text', !!tab?.enabled);
  $('#toggle-site').textContent = tab?.enabled ? 'Disable' : 'Enable recording';
  $('#toggle-site').disabled = busy || !tab?.supported;
  $('#capture-now').disabled = busy || !tab?.supported;
  const connection = hostConnection(status);
  $('#host-dot').classList.toggle('online', connection.usable);
  $('#host-status').textContent = hostSummary(status);
  $('#host-error').textContent = status.host?.error || status.lastError || '';
  $('#host-error').hidden = !$('#host-error').textContent;
  $('#setup-link').hidden = connection.kind === 'ready';
  $('#last-saved').textContent = status.lastSavedAt
    ? `Last saved: ${date(status.lastSavedAt)}`
    : 'Saved pages stay on your computer.';
  $('#queue-count').textContent = status.pendingCount || 0;
  $('#queue-size').textContent = status.pendingCount
    ? `${bytes(status.queueBytes)} · ${status.rejectedCount ? `${status.rejectedCount} rejected; see settings` : 'waiting to be saved to SQLite'}`
    : 'All captures delivered';
  $('#retry-queue').disabled = busy;
}
async function refresh() {
  try {
    render(await send('status'));
  } catch (error) {
    notice(message, errorText(error), 'error');
  }
}
async function action(fn, success) {
  busy = true;
  if (current) render(current);
  notice(message);
  try {
    await fn();
    notice(message, success, 'success');
  } catch (error) {
    notice(message, errorText(error), 'error');
  } finally {
    busy = false;
    await refresh();
  }
}
$('#toggle-site').addEventListener('click', () => {
  const tab = current?.activeTab;
  if (!tab?.supported || busy) return;
  if (tab.enabled) {
    action(
      () => send('site.disable', { origin: tab.origin }),
      'Automatic recording disabled for this site.',
    );
    return;
  }
  // Keep permission request in the original user gesture; do not await status first.
  const site = new URL(tab.origin);
  const permission = chrome.permissions.request({
    origins: [`${site.protocol}//${site.hostname}/*`],
  });
  action(async () => {
    if (!(await permission))
      throw new Error('Permission was not granted. Recording was not enabled.');
    await send('site.enable', { origin: tab.origin });
  }, 'Recording enabled. Page content will be saved automatically.');
});
$('#capture-now').addEventListener('click', () =>
  action(
    () => send('capture.now', { tabId: current?.activeTab?.id }),
    'Capture queued. It will appear in the archive after being saved to SQLite.',
  ),
);
$('#retry-queue').addEventListener('click', () =>
  action(() => send('queue.flush'), 'Delivery retry completed. Check the queue status.'),
);
refresh();
setInterval(() => {
  if (!busy) refresh();
}, 5000);
