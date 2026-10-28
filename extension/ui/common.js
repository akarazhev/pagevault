import {connectionState} from '../lib/connection.js';
export const $ = (selector, parent = document) => parent.querySelector(selector);
export function el(tag, className, text) {
  const node = document.createElement(tag);
  if (className) node.className = className;
  if (text !== undefined) node.textContent = String(text);
  return node;
}
export async function send(type, args = {}) {
  if (!globalThis.chrome?.runtime?.sendMessage) throw new Error('Откройте эту страницу из установленного расширения PageVault.');
  const response = await chrome.runtime.sendMessage({type, ...args});
  if (!response?.ok) throw new Error(response?.error || 'Расширение не ответило. Попробуйте ещё раз.');
  return response.data;
}
export const native = (type, args = {}) => send('native', {request: {type, ...args}});
export const errorText = error => error?.message || String(error);
export function notice(node, message = '', kind = 'info') {
  node.textContent = message;
  node.className = `notice ${kind}`;
  node.hidden = !message;
}
export function date(value) {
  if (!value) return '—';
  const d = new Date(value);
  return Number.isNaN(d.valueOf()) ? String(value) : d.toLocaleString();
}
export function number(value) { return Number(value || 0).toLocaleString(); }
export function bytes(value) {
  const n = Number(value || 0);
  if (n < 1024) return `${n} Б`;
  if (n < 1048576) return `${(n / 1024).toFixed(1)} КБ`;
  return `${(n / 1048576).toFixed(1)} МБ`;
}
export function safeUrl(value) {
  try { const u = new URL(String(value)); return ['http:', 'https:'].includes(u.protocol) ? u.href : null; } catch { return null; }
}
export function externalLink(url, label = url) {
  const href = safeUrl(url);
  if (!href) return el('span', 'break-text', label || 'Недоступная ссылка');
  const a = el('a', 'break-text', label || href);
  a.href = href;
  a.target = '_blank';
  a.rel = 'noopener noreferrer';
  return a;
}
export async function readChunked(type, args, onProgress = () => {}) {
  for (let attempt = 0; attempt < 3; attempt++) {
    let offset = 0, total = null, documentHash = null;
    const chunks = [];
    for (;;) {
      const data = await native(type, {...args, offset, chunk_size:120000});
      if (typeof data?.json_chunk !== 'string' || data.offset !== offset || !Number.isInteger(data.total_chars) || data.total_chars < 0) throw new Error('Помощник вернул неверную часть архива.');
      const hash = data.document_hash ?? null;
      if (total !== null && (data.total_chars !== total || hash !== documentHash)) {
        // Never concatenate chunks from different archive revisions, even at equal lengths.
        if (attempt === 2) throw new Error('Архив продолжает изменяться во время чтения. Повторите загрузку после завершения записи.');
        break;
      }
      total = data.total_chars; documentHash = hash;
      chunks.push(data.json_chunk);
      onProgress(data.next_offset ?? total, total);
      if (data.done) {
        try { return JSON.parse(chunks.join('')); } catch { throw new Error('Не удалось прочитать JSON архива.'); }
      }
      if (!Number.isInteger(data.next_offset) || data.next_offset <= offset || data.next_offset > total) throw new Error('Чтение архива остановилось: неверное смещение.');
      offset = data.next_offset;
    }
  }
}
export function downloadJSON(value, filename) {
  const blob = new Blob([JSON.stringify(value, null, 2)], {type:'application/json;charset=utf-8'});
  const url = URL.createObjectURL(blob);
  const a = el('a');
  a.href = url;
  a.download = filename;
  document.body.append(a);
  a.click();
  a.remove();
  setTimeout(() => URL.revokeObjectURL(url), 10000);
}
function metaLine(label, value) {
  const p = el('p', 'metadata-line');
  p.append(el('strong', '', `${label}: `), el('span', 'break-text', value));
  return p;
}
export function renderBlocks(blocks, target) {
  target.replaceChildren();
  if (!Array.isArray(blocks) || !blocks.length) {
    target.append(el('p', 'empty-state', 'В этом представлении нет сохранённых блоков.'));
    return;
  }
  const fragment = document.createDocumentFragment();
  for (const b of blocks) {
    if (!b || typeof b !== 'object') continue;
    let node;
    const text = typeof b.text === 'string' ? b.text : '';
    switch (b.type) {
      case 'heading': node = el(`h${Math.max(2, Math.min(6, Number(b.level) || 2))}`, '', text); break;
      case 'code': {
        node = el('div', 'code-block');
        if (b.language) node.append(el('div', 'code-language', b.language));
        const pre = el('pre');
        pre.append(el('code', '', text));
        node.append(pre);
        break;
      }
      case 'table': {
        node = el('div', 'table-scroll');
        const table = el('table', 'content-table');
        const tbody = el('tbody');
        const rows = Array.isArray(b.rows) ? b.rows : [];
        const richRows = Array.isArray(b.cell_blocks) ? b.cell_blocks : [];
        for (let r = 0; r < Math.max(rows.length, richRows.length); r++) {
          const row = Array.isArray(rows[r]) ? rows[r] : rows[r] === undefined ? [] : [rows[r]];
          const richRow = Array.isArray(richRows[r]) ? richRows[r] : [];
          const tr = el('tr');
          for (let c = 0; c < Math.max(row.length, richRow.length); c++) {
            const td = el('td');
            const richBlocks = richRow[c];
            if (Array.isArray(richBlocks) && richBlocks.length) renderBlocks(richBlocks, td);
            else {
              const cell = row[c];
              td.textContent = cell == null ? '' : typeof cell === 'object' ? JSON.stringify(cell) : String(cell);
            }
            tr.append(td);
          }
          tbody.append(tr);
        }
        table.append(tbody);
        node.append(table);
        break;
      }
      case 'list': {
        node = el(b.ordered ? 'ol' : 'ul');
        for (const item of Array.isArray(b.items) ? b.items : []) {
          const li = el('li');
          if (Array.isArray(item?.blocks) && item.blocks.length) renderBlocks(item.blocks, li);
          else li.textContent = typeof item === 'object' ? (item?.text ?? JSON.stringify(item)) : String(item ?? '');
          node.append(li);
        }
        break;
      }
      case 'image': case 'media': {
        node = el('section', 'media-reference');
        node.append(el('span', 'eyebrow', b.type === 'image' ? 'Изображение · ссылка' : `${b.kind || 'Медиа'} · ссылка`));
        if (b.alt || b.text) node.append(el('p', '', b.alt || text));
        if (b.src) node.append(externalLink(b.src));
        if (b.poster) node.append(metaLine('Обложка', b.poster));
        node.append(el('small', 'muted', 'Медиафайлы не загружаются автоматически.'));
        break;
      }
      case 'link': node = el('p'); node.append(externalLink(b.url, text || b.url)); break;
      case 'paragraph': case 'text': node = el('p', 'preserve-lines', text); break;
      default: node = el('pre', 'unknown-block', JSON.stringify(b, null, 2));
    }
    if (Array.isArray(b.links) && b.type !== 'link') {
      const links = el('div', 'block-links');
      for (const link of b.links) if (link?.url) links.append(externalLink(link.url, link.text || link.url));
      if (links.childNodes.length) {
        const wrap = el('div');
        wrap.append(node, links);
        node = wrap;
      }
    }
    fragment.append(node);
  }
  target.append(fragment);
}
export function hostSummary(status) {
  return hostConnection(status).label;
}
export function hostConnection(status) {
  return connectionState(status?.host, globalThis.chrome?.runtime?.getManifest?.()?.version);
}
