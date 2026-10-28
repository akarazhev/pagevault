import {$, el, send, native, notice, date, number, bytes, errorText, externalLink, readChunked, renderBlocks, downloadJSON} from './common.js';
const LIMIT = 25;
let offset = 0, total = 0, pages = [], selected = null, captures = [], capturesTotal = 0;
let accumulated = null, mode = 'accumulated', listRequest = 0, detailRequest = 0;
let listLoading = false, listFailed = false;
const message = $('#message');
function updatePagination() {
  $('#previous').disabled = listLoading || listFailed || offset === 0;
  $('#next').disabled = listLoading || listFailed || offset + LIMIT >= total;
  if (listFailed) { $('#page-range').textContent = 'Недоступно'; return; }
  $('#page-range').textContent = total ? `${number(offset + 1)}–${number(Math.min(offset + LIMIT, total))} из ${number(total)}` : '0 страниц';
}
function showList() {
  const list = $('#page-list'); list.replaceChildren();
  if (!pages.length && total > 0) { list.append(el('p', 'empty-state', 'На этой странице нет записей. Нажмите «Обновить», чтобы перечитать архив.')); return; }
  if (!pages.length) { list.append(el('p', 'empty-state', $('#search').value || $('#domain').value ? 'По этому запросу ничего не найдено. Измените текст поиска или выберите другой сайт.' : 'Пока здесь пусто. Разрешите запись для сайта в меню расширения и сохраните первую страницу.')); return; }
  for (const page of pages) {
    const button = el('button', 'page-item');
    button.type = 'button';
    button.classList.toggle('selected', selected?.id === page.id);
    button.setAttribute('aria-pressed', String(selected?.id === page.id));
    button.append(el('strong', 'page-item-title', page.title || page.url || page.url_display || 'Без названия'), el('span', 'page-item-host', page.hostname), el('span', 'page-item-meta', `${number(page.capture_count)} снимков · ${date(page.last_seen)}`));
    button.addEventListener('click', () => selectPage(page));
    list.append(button);
  }
}
async function loadList() {
  const request = ++listRequest;
  listLoading = true; listFailed = false;
  updatePagination();
  $('#page-list').setAttribute('aria-busy', 'true');
  let requestedOffset = Math.min(Math.max(0, offset), Math.max(0, Math.floor((total - 1) / LIMIT) * LIMIT));
  try {
    for (;;) {
      const data = await native('pages.list', {query:$('#search').value.trim(), domain:$('#domain').value, limit:LIMIT, offset:requestedOffset});
      if (request !== listRequest) return;
      total = data.total;
      const lastOffset = Math.max(0, Math.floor((total - 1) / LIMIT) * LIMIT);
      // Another tab or a deletion may have reduced the archive while this read was pending.
      if (requestedOffset > lastOffset) { requestedOffset = lastOffset; continue; }
      offset = requestedOffset; pages = data.items;
      $('#result-count').textContent = `${number(total)} страниц`;
      showList();
      break;
    }
  } catch (error) {
    if (request !== listRequest) return;
    listFailed = true;
    notice(message, `Не удалось загрузить архив. ${errorText(error)}`, 'error');
    $('#result-count').textContent = 'Архив недоступен';
    $('#page-list').replaceChildren(el('p', 'empty-state', 'Архив недоступен. Подключите локального помощника в настройках и нажмите «Обновить».'));
  } finally {
    if (request === listRequest) {
      listLoading = false;
      $('#page-list').removeAttribute('aria-busy');
      updatePagination();
    }
  }
}
async function loadSummary() {
  const [statsResult, domainResult, statusResult] = await Promise.allSettled([native('stats'), native('pages.domains'), send('status')]);
  if (statsResult.status === 'fulfilled') {
    const s = statsResult.value;
    $('#stat-pages').textContent = number(s.pages); $('#stat-captures').textContent = number(s.captures); $('#stat-blocks').textContent = number(s.blocks); $('#stat-size').textContent = bytes(s.db_bytes);
    $('#connection-status').textContent = '● SQLite подключена'; $('#connection-status').classList.add('teal-text');
  } else {
    $('#connection-status').textContent = '● Помощник недоступен'; $('#connection-status').classList.remove('teal-text');
    notice(message, `Нет связи с SQLite. Откройте настройки для подключения помощника. ${errorText(statsResult.reason)}`, 'error');
  }
  if (domainResult.status === 'fulfilled') {
    const old = $('#domain').value;
    $('#domain').replaceChildren(new Option('Все сайты', ''));
    for (const d of domainResult.value.items) $('#domain').append(new Option(`${d.hostname} (${number(d.page_count)})`, d.hostname));
    $('#domain').value = old;
  }
  if (statusResult.status === 'fulfilled' && statusResult.value.pendingCount) {
    $('#connection-status').textContent += ` · ${number(statusResult.value.pendingCount)} в очереди`;
  }
}
function setMode(next) {
  mode = next;
  for (const [id, key] of [['view-accumulated','accumulated'],['view-versions','versions']]) {
    $(`#${id}`).classList.toggle('selected', next === key);
    $(`#${id}`).setAttribute('aria-pressed', String(next === key));
  }
  $('#version-bar').hidden = next !== 'versions';
}
function renderAccumulated() {
  setMode('accumulated');
  $('#capture-warnings').hidden = true;
  $('#view-note').textContent = 'Все уникальные блоки в порядке первого появления. Они могли быть видны в разные моменты и не обязательно находились на странице одновременно.';
  renderBlocks(accumulated?.blocks, $('#blocks'));
}
function renderCapture(capture) {
  $('#view-note').textContent = `Состояние страницы на ${date(capture.captured_at)} · ${number(capture.blocks?.length)} блоков · ${capture.source?.adapter || 'generic'}. Исходный HTML не исполняется.`;
  const warnings = Array.isArray(capture.warnings) ? capture.warnings : [];
  notice($('#capture-warnings'), warnings.map(w => typeof w === 'string' ? w : `${w.message || w.code}`).join(' '), 'warning');
  renderBlocks(capture.blocks, $('#blocks'));
}
function updateVersions() {
  const select = $('#version-select'); const old = select.value;
  select.replaceChildren();
  for (const c of captures) select.append(new Option(`${date(c.captured_at)} · ${number(c.block_count)} блоков`, String(c.id)));
  if (captures.some(c => String(c.id) === old)) select.value = old;
  $('#more-versions').hidden = captures.length >= capturesTotal;
}
async function showVersion() {
  if (!selected) return;
  setMode('versions');
  const captureId = $('#version-select').value;
  if (!captureId) { $('#blocks').replaceChildren(el('p', 'empty-state', 'У страницы пока нет доступных снимков.')); return; }
  const request = ++detailRequest;
  $('#capture-warnings').hidden = true;
  $('#view-note').textContent = 'Загрузка выбранного снимка…';
  $('#blocks').replaceChildren(el('p', 'empty-state', 'Загрузка снимка…'));
  try {
    const capture = await readChunked('captures.get', {capture_id:Number(captureId)});
    if (request === detailRequest && mode === 'versions') renderCapture(capture);
  } catch (error) {
    if (request === detailRequest) {
      notice(message, errorText(error), 'error');
      $('#view-note').textContent = 'Снимок недоступен.';
      $('#blocks').replaceChildren(el('p', 'empty-state', 'Не удалось загрузить снимок. Выберите другую версию или повторите попытку после восстановления связи.'));
    }
  }
}
async function selectPage(page) {
  selected = page; accumulated = null; captures = []; capturesTotal = 0;
  const request = ++detailRequest;
  notice(message); showList(); setMode('accumulated');
  $('#detail-empty').hidden = true; $('#detail').hidden = false;
  $('#detail-title').textContent = page.title || 'Без названия'; $('#detail-host').textContent = page.hostname;
  $('#detail-link').replaceChildren(externalLink(page.url, page.url || page.url_display));
  $('#detail-meta').textContent = `Впервые: ${date(page.first_seen)} · обновлено: ${date(page.last_seen)} · ${number(page.capture_count)} снимков`;
  $('#version-select').replaceChildren(); $('#capture-warnings').hidden = true;
  $('#blocks').replaceChildren(el('p', 'empty-state', 'Читаем сохранённые блоки…'));
  $('#view-note').textContent = '';
  $('#export').disabled = true;
  $('#view-accumulated').disabled = true; $('#view-versions').disabled = true;
  try {
    const [data, archive] = await Promise.all([native('pages.get', {page_id:page.id, limit:100, offset:0}), readChunked('pages.export', {page_id:page.id})]);
    if (request !== detailRequest) return;
    selected = {...data.page, ...archive.page}; captures = data.captures; capturesTotal = data.total; accumulated = archive;
    $('#detail-title').textContent = selected.title || 'Без названия';
    $('#detail-link').replaceChildren(externalLink(selected.url, selected.url || selected.url_display));
    updateVersions(); renderAccumulated(); $('#export').disabled = false;
    $('#view-accumulated').disabled = false; $('#view-versions').disabled = false;
  } catch (error) {
    if (request !== detailRequest) return;
    notice(message, errorText(error), 'error');
    $('#blocks').replaceChildren(el('p', 'empty-state', 'Не удалось прочитать страницу. Выберите её ещё раз после восстановления связи.'));
  }
}
$('#search-form').addEventListener('submit', event => { event.preventDefault(); offset = 0; loadList(); });
$('#search').addEventListener('search', () => { offset = 0; loadList(); });
$('#domain').addEventListener('change', () => { offset = 0; loadList(); });
$('#previous').addEventListener('click', () => { if (listLoading || listFailed) return; offset = Math.max(0, offset - LIMIT); loadList(); });
$('#next').addEventListener('click', () => { if (listLoading || listFailed) return; offset = Math.min(offset + LIMIT, Math.max(0, Math.floor((total - 1) / LIMIT) * LIMIT)); loadList(); });
$('#refresh').addEventListener('click', async () => { notice(message); await loadSummary(); await loadList(); if (selected) await selectPage(selected); });
$('#view-accumulated').addEventListener('click', () => { ++detailRequest; if (accumulated) renderAccumulated(); });
$('#view-versions').addEventListener('click', showVersion);
$('#version-select').addEventListener('change', showVersion);
$('#more-versions').addEventListener('click', async () => {
  const pageId = selected?.id; if (!pageId) return;
  $('#more-versions').disabled = true;
  try {
    const data = await native('pages.get', {page_id:pageId, limit:100, offset:captures.length});
    if (selected?.id !== pageId) return;
    captures.push(...data.captures); capturesTotal = data.total; updateVersions();
  } catch (error) { notice(message, errorText(error), 'error'); }
  finally { $('#more-versions').disabled = false; }
});
$('#export').addEventListener('click', async () => {
  if (!selected) return;
  const page = selected; $('#export').disabled = true;
  try {
    // Export the accumulated object read using the chunk protocol, not a synthetic snapshot.
    const data = accumulated || await readChunked('pages.export', {page_id:page.id});
    downloadJSON(data, `pagevault-${page.id}.json`);
    notice(message, 'JSON с накопленными блоками подготовлен к скачиванию.', 'success');
  } catch (error) { notice(message, errorText(error), 'error'); }
  finally { $('#export').disabled = false; }
});
$('#delete-page').addEventListener('click', async () => {
  if (!selected) return;
  const page = selected;
  if (!confirm(`Удалить страницу «${page.title || page.url}», все её снимки и накопленные блоки из SQLite, а также ожидающие отправки копии этой страницы? Это действие нельзя отменить. Если запись сайта включена, новые снимки могут создать архив заново.`)) return;
  $('#delete-page').disabled = true;
  try {
    await native('pages.delete', {page_id:page.id});
    ++detailRequest; selected = null; accumulated = null; $('#detail').hidden = true; $('#detail-empty').hidden = false;
    offset = 0; await loadSummary(); await loadList(); notice(message, 'Страница, её снимки и ожидающие отправки копии удалены.', 'success');
  } catch (error) { notice(message, errorText(error), 'error'); }
  finally { $('#delete-page').disabled = false; }
});
await loadSummary(); await loadList();
