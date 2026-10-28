import {$, send, notice, errorText, hostSummary, date, bytes} from './common.js';
let current = null;
let busy = false;
const message = $('#message');
function render(status) {
  current = status;
  const tab = status.activeTab;
  let hostname = 'Страница недоступна';
  if (tab?.supported) { try { hostname = new URL(tab.url).hostname; } catch {} }
  $('#site-name').textContent = hostname;
  $('#site-url').textContent = tab?.supported ? tab.origin : 'Откройте обычную страницу сайта (http или https).';
  $('#site-state').textContent = tab?.enabled ? 'Автозапись включена' : 'Автозапись выключена';
  $('#site-state').classList.toggle('teal-text', !!tab?.enabled);
  $('#toggle-site').textContent = tab?.enabled ? 'Отключить' : 'Разрешить запись';
  $('#toggle-site').disabled = busy || !tab?.supported;
  $('#capture-now').disabled = busy || !tab?.supported;
  $('#host-dot').classList.toggle('online', !!status.host?.connected);
  $('#host-status').textContent = hostSummary(status);
  $('#host-error').textContent = status.host?.error || status.lastError || '';
  $('#host-error').hidden = !$('#host-error').textContent;
  $('#setup-link').hidden = !!status.host?.connected;
  $('#last-saved').textContent = status.lastSavedAt ? `Последняя запись: ${date(status.lastSavedAt)}` : 'Сохранённые страницы останутся на вашем компьютере.';
  $('#queue-count').textContent = status.pendingCount || 0;
  $('#queue-size').textContent = status.pendingCount ? `${bytes(status.queueBytes)} · ждут записи в SQLite` : 'Все снимки доставлены';
  $('#retry-queue').disabled = busy;
}
async function refresh() {
  try { render(await send('status')); } catch (error) { notice(message, errorText(error), 'error'); }
}
async function action(fn, success) {
  busy = true;
  if (current) render(current);
  notice(message);
  try { await fn(); notice(message, success, 'success'); } catch (error) { notice(message, errorText(error), 'error'); }
  finally { busy = false; await refresh(); }
}
$('#toggle-site').addEventListener('click', () => {
  const tab = current?.activeTab;
  if (!tab?.supported || busy) return;
  if (tab.enabled) {
    action(() => send('site.disable', {origin:tab.origin}), 'Автоматическая запись для сайта отключена.');
    return;
  }
  // Keep permission request in the original user gesture; do not await status first.
  const site = new URL(tab.origin);
  const permission = chrome.permissions.request({origins:[`${site.protocol}//${site.hostname}/*`]});
  action(async () => {
    if (!await permission) throw new Error('Разрешение не выдано. Запись для сайта не включена.');
    await send('site.enable', {origin:tab.origin});
  }, 'Запись включена. Содержимое страницы будет сохраняться автоматически.');
});
$('#capture-now').addEventListener('click', () => action(() => send('capture.now', {tabId:current?.activeTab?.id}), 'Снимок поставлен в очередь. Он появится в архиве после записи в SQLite.'));
$('#retry-queue').addEventListener('click', () => action(() => send('queue.flush'), 'Повторная доставка выполнена. Проверьте состояние очереди.'));
refresh();
setInterval(() => { if (!busy) refresh(); }, 5000);
