import {
  $,
  el,
  send,
  notice,
  errorText,
  hostConnection,
  date,
  number,
  count,
  bytes,
  downloadJSON,
} from './common.js';
import { DOWNLOADS_URL, normalizePlatform, platformInstructions } from '../lib/connection.js';
import { extensionAPI as chrome } from '../lib/browser.js';
const message = $('#message');
const keys = ['captureIntervalSeconds', 'debounceMs', 'maxBlocks', 'maxTextChars', 'maxHtmlChars'];
let settingsLoaded = false;
let dirty = false;
let checking = false;
let platformChosen = false;
function renderSites(origins) {
  const list = $('#sites-list');
  list.replaceChildren();
  $('#sites-count').textContent = number(origins.length);
  if (!origins.length) {
    list.append(
      el(
        'p',
        'empty-state',
        'No sites are enabled yet. Open a site and select Enable recording in the extension menu.',
      ),
    );
    return;
  }
  for (const origin of origins) {
    const row = el('div', 'site-row');
    const name = el('div');
    name.append(
      el('strong', 'break-text', origin),
      el('span', 'help teal-text', 'Automatic recording enabled'),
    );
    const button = el('button', 'button small', 'Disable');
    button.type = 'button';
    button.setAttribute('aria-label', `Disable recording for ${origin}`);
    button.addEventListener('click', async () => {
      button.disabled = true;
      try {
        await send('site.disable', { origin });
        await refresh();
        notice(message, `Recording for ${origin} disabled.`, 'success');
      } catch (error) {
        notice(message, errorText(error), 'error');
        button.disabled = false;
      }
    });
    row.append(name, button);
    list.append(row);
  }
}
function renderConnection(status) {
  const connection = hostConnection(status);
  $('#host-dot').classList.toggle('online', connection.usable);
  $('#host-status').textContent = connection.label;
  $('#host-version').textContent = connection.version;
  $('#connection-guidance').textContent = connection.detail;
  $('#setup-result').textContent = connection.usable
    ? 'Connection established. ' + connection.detail
    : connection.detail;
  $('#open-setup').hidden = connection.kind === 'ready';
  $('#open-setup').textContent = connection.kind === 'update' ? 'Update helper' : 'Connect helper';
  $('#host-error').textContent = connection.error || status.lastError || '';
  $('#host-error').hidden = !$('#host-error').textContent;
  $('#db-path').textContent =
    connection.dbPath || 'The path will appear after connecting the helper';
  return connection;
}
function render(status) {
  renderConnection(status);
  $('#last-saved').textContent = date(status.lastSavedAt);
  $('#queue-status').textContent =
    `${count(status.pendingCount, 'capture')} · ${bytes(status.queueBytes)}${status.rejectedCount ? ` · ${number(status.rejectedCount)} rejected` : ''}`;
  renderSites(status.enabledOrigins || []);
  if (!settingsLoaded || !dirty) {
    $('#saveHtml').checked = !!status.settings.saveHtml;
    for (const key of keys) $(`#${key}`).value = status.settings[key];
    settingsLoaded = true;
  }
  $('#settings-fields').disabled = false;
}
async function refresh() {
  try {
    const status = await send('status');
    render(status);
    await renderRejected();
    return status;
  } catch (error) {
    renderConnection({ host: { connected: false, error: errorText(error) } });
    notice(message, errorText(error), 'error');
    return null;
  }
}
$('#settings-form').addEventListener('input', () => {
  dirty = true;
  $('#save-state').textContent = 'Unsaved changes';
});
$('#settings-form').addEventListener('submit', async (event) => {
  event.preventDefault();
  if (!event.currentTarget.reportValidity()) return;
  const settings = { saveHtml: $('#saveHtml').checked };
  for (const key of keys) settings[key] = Number($(`#${key}`).value);
  $('#save-settings').disabled = true;
  notice(message);
  try {
    const saved = await send('settings.set', { settings });
    const value = saved.settings || saved;
    for (const key of keys) if (value[key] !== undefined) $(`#${key}`).value = value[key];
    if (value.saveHtml !== undefined) $('#saveHtml').checked = value.saveHtml;
    dirty = false;
    $('#save-state').textContent = 'Settings saved';
    notice(message, 'Settings applied. New captures will use these values.', 'success');
  } catch (error) {
    notice(message, errorText(error), 'error');
  } finally {
    $('#save-settings').disabled = false;
  }
});
async function checkConnection(showNotice = true) {
  if (checking) return;
  checking = true;
  $('#check-connection').disabled = $('#setup-check-connection').disabled = true;
  $('#setup-result').textContent = 'Checking connection…';
  notice(message);
  try {
    const status = await refresh();
    if (status && showNotice) {
      const connection = hostConnection(status);
      notice(message, connection.label, connection.usable ? 'success' : 'error');
    }
  } finally {
    checking = false;
    $('#check-connection').disabled = $('#setup-check-connection').disabled = false;
  }
}
$('#check-connection').addEventListener('click', () => checkConnection());
$('#setup-check-connection').addEventListener('click', () => checkConnection());
function renderPlatform() {
  const browser = chrome?.runtime?.getManifest?.()?.browser_specific_settings?.gecko
    ? 'firefox'
    : 'chromium';
  const info = platformInstructions($('#download-platform').value, browser);
  $('#platform-detail').textContent = info.detail;
  $('#platform-install').textContent = info.install;
  $('#platform-command').hidden = !info.command;
  $('#platform-command code').textContent = info.command;
}
$('#download-platform').addEventListener('change', () => {
  platformChosen = true;
  renderPlatform();
});
$('#download-link').href = DOWNLOADS_URL;
renderPlatform();
async function detectPlatform() {
  let platform = 'unknown';
  try {
    platform = normalizePlatform((await chrome?.runtime?.getPlatformInfo?.())?.os);
  } catch {
    /* Keep the manual selector available. */
  }
  if (!platformChosen) {
    $('#download-platform').value = platform;
    renderPlatform();
  }
}
$('#retry-queue').addEventListener('click', async () => {
  $('#retry-queue').disabled = true;
  try {
    await send('queue.flush');
    await refresh();
    notice(message, 'Delivery attempt complete. See the queue for its current status.', 'success');
  } catch (error) {
    notice(message, errorText(error), 'error');
  } finally {
    $('#retry-queue').disabled = false;
  }
});
$('#extension-id').textContent = chrome?.runtime?.id || 'Open the installed extension settings';
checkConnection(false);
detectPlatform();

async function renderRejected() {
  const list = $('#rejected-list');
  const items = await send('queue.list');
  list.replaceChildren();
  if (!items.length) {
    list.append(el('p', 'empty-state', 'No rejected captures.'));
    return;
  }
  for (const item of items) {
    const row = el('div', 'site-row rejected-row');
    const details = el('div');
    details.append(
      el('strong', 'break-text', String(item.title || 'Untitled capture').slice(0, 500)),
      el('p', 'help break-text', String(item.url || 'URL unavailable').slice(0, 2048)),
      el('p', 'help', `Queued: ${date(item.createdAt)}`),
      el('p', 'help break-text', item.error || 'The archive rejected this capture.'),
    );
    const actions = el('div', 'inline-actions');
    for (const [label, type] of [
      ['Export JSON', 'queue.export'],
      ['Retry', 'queue.retry'],
      ['Discard', 'queue.discard'],
    ]) {
      const button = el('button', 'button small', label);
      button.type = 'button';
      button.addEventListener('click', async () => {
        button.disabled = true;
        try {
          if (
            type === 'queue.discard' &&
            !confirm('Discard this rejected capture? This cannot be undone.')
          )
            return;
          const result = await send(type, { id: item.id, revision: item.revision });
          if (type === 'queue.export') downloadJSON(result, `pagevault-rejected-${item.id}.json`);
          else await refresh();
        } catch (error) {
          notice(message, errorText(error), 'error');
        } finally {
          button.disabled = false;
        }
      });
      actions.append(button);
    }
    row.append(details, actions);
    list.append(row);
  }
}
