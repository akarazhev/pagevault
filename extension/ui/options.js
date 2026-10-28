import {$, el, send, notice, errorText, hostSummary, date, number, bytes} from './common.js';
const message = $('#message');
const keys = ['captureIntervalSeconds','debounceMs','maxBlocks','maxTextChars','maxHtmlChars'];
let settingsLoaded = false;
let dirty = false;
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
function render(status) {
  $('#host-dot').classList.toggle('online', !!status.host?.connected);
  $('#host-status').textContent = hostSummary(status);
  $('#host-error').textContent = status.host?.error || status.lastError || '';
  $('#host-error').hidden = !$('#host-error').textContent;
  $('#db-path').textContent = status.host?.db_path || 'Путь появится после подключения помощника';
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
  try { render(await send('status')); }
  catch (error) { notice(message, errorText(error), 'error'); }
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
$('#check-connection').addEventListener('click', async () => {
  $('#check-connection').disabled = true; notice(message);
  try { await refresh(); } finally { $('#check-connection').disabled = false; }
});
$('#retry-queue').addEventListener('click', async () => {
  $('#retry-queue').disabled = true;
  try { await send('queue.flush'); await refresh(); notice(message, 'Попытка доставки завершена. Текущее состояние указано в очереди.', 'success'); }
  catch (error) { notice(message, errorText(error), 'error'); }
  finally { $('#retry-queue').disabled = false; }
});
$('#extension-id').textContent = globalThis.chrome?.runtime?.id || 'Откройте настройки установленного расширения';
refresh();
