import {$, el, send, notice, errorText, hostConnection, date, number, bytes} from './common.js';
import {DOWNLOADS_URL, normalizePlatform, platformInstructions} from '../lib/connection.js';
const message = $('#message');
const keys = ['captureIntervalSeconds','debounceMs','maxBlocks','maxTextChars','maxHtmlChars'];
let settingsLoaded = false;
let dirty = false;
let checking = false;
let platformChosen = false;
function renderSites(origins) {
  const list = $('#sites-list'); list.replaceChildren();
  $('#sites-count').textContent = number(origins.length);
  if (!origins.length) { list.append(el('p', 'empty-state', 'Пока нет разрешённых сайтов. Откройте нужный сайт и нажмите «Разрешить запись» в меню расширения.')); return; }
  for (const origin of origins) {
    const row = el('div', 'site-row');
    const name = el('div'); name.append(el('strong', 'break-text', origin), el('span', 'help teal-text', 'Автоматическая запись включена'));
    const button = el('button', 'button small', 'Отключить');
    button.type = 'button';
    button.setAttribute('aria-label', `Отключить запись для ${origin}`);
    button.addEventListener('click', async () => {
      button.disabled = true;
      try { await send('site.disable', {origin}); await refresh(); notice(message, `Запись для ${origin} отключена.`, 'success'); }
      catch (error) { notice(message, errorText(error), 'error'); button.disabled = false; }
    });
    row.append(name, button); list.append(row);
  }
}
function renderConnection(status) {
  const connection = hostConnection(status);
  $('#host-dot').classList.toggle('online', connection.usable);
  $('#host-status').textContent = connection.label;
  $('#host-version').textContent = connection.version;
  $('#connection-guidance').textContent = connection.detail;
  $('#setup-result').textContent = connection.usable ? 'Подключение установлено. ' + connection.detail : connection.detail;
  $('#open-setup').hidden = connection.kind === 'ready';
  $('#open-setup').textContent = connection.kind === 'update' ? 'Обновить помощника' : 'Подключить помощника';
  $('#host-error').textContent = connection.error || status.lastError || '';
  $('#host-error').hidden = !$('#host-error').textContent;
  $('#db-path').textContent = connection.dbPath || 'Путь появится после подключения помощника';
  return connection;
}
function render(status) {
  renderConnection(status);
  $('#last-saved').textContent = date(status.lastSavedAt);
  $('#queue-status').textContent = `${number(status.pendingCount)} снимков · ${bytes(status.queueBytes)}`;
  renderSites(status.enabledOrigins || []);
  if (!settingsLoaded || !dirty) {
    $('#saveHtml').checked = !!status.settings.saveHtml;
    for (const key of keys) $(`#${key}`).value = status.settings[key];
    settingsLoaded = true;
  }
  $('#settings-fields').disabled = false;
}
async function refresh() {
  try { const status = await send('status'); render(status); return status; }
  catch (error) {
    renderConnection({host:{connected:false,error:errorText(error)}});
    notice(message, errorText(error), 'error');
    return null;
  }
}
$('#settings-form').addEventListener('input', () => { dirty = true; $('#save-state').textContent = 'Есть несохранённые изменения'; });
$('#settings-form').addEventListener('submit', async event => {
  event.preventDefault();
  if (!event.currentTarget.reportValidity()) return;
  const settings = {saveHtml:$('#saveHtml').checked};
  for (const key of keys) settings[key] = Number($(`#${key}`).value);
  $('#save-settings').disabled = true; notice(message);
  try {
    const saved = await send('settings.set', {settings});
    const value = saved.settings || saved;
    for (const key of keys) if (value[key] !== undefined) $(`#${key}`).value = value[key];
    if (value.saveHtml !== undefined) $('#saveHtml').checked = value.saveHtml;
    dirty = false; $('#save-state').textContent = 'Настройки сохранены';
    notice(message, 'Настройки применены. Новые снимки будут использовать эти параметры.', 'success');
  } catch (error) { notice(message, errorText(error), 'error'); }
  finally { $('#save-settings').disabled = false; }
});
async function checkConnection(showNotice = true) {
  if (checking) return;
  checking = true;
  $('#check-connection').disabled = $('#setup-check-connection').disabled = true;
  $('#setup-result').textContent = 'Проверяем подключение…';
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
  const info = platformInstructions($('#download-platform').value);
  $('#platform-detail').textContent = info.detail;
  $('#platform-install').textContent = info.install;
  $('#platform-command').hidden = !info.command;
  $('#platform-command code').textContent = info.command;
}
$('#download-platform').addEventListener('change', () => { platformChosen = true; renderPlatform(); });
$('#download-link').href = DOWNLOADS_URL;
renderPlatform();
async function detectPlatform() {
  let platform = 'unknown';
  try { platform = normalizePlatform((await globalThis.chrome?.runtime?.getPlatformInfo?.())?.os); } catch { /* Keep the manual selector available. */ }
  if (!platformChosen) { $('#download-platform').value = platform; renderPlatform(); }
}
$('#retry-queue').addEventListener('click', async () => {
  $('#retry-queue').disabled = true;
  try { await send('queue.flush'); await refresh(); notice(message, 'Попытка доставки завершена. Текущее состояние указано в очереди.', 'success'); }
  catch (error) { notice(message, errorText(error), 'error'); }
  finally { $('#retry-queue').disabled = false; }
});
$('#extension-id').textContent = globalThis.chrome?.runtime?.id || 'Откройте настройки установленного расширения';
checkConnection(false);
detectPlatform();
