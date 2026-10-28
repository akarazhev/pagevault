/* PageVault universal DOM extraction. Classic script, no extension API or scheduler. */
(() => {
  'use strict';
  if (globalThis.PageVaultExtractor) return;
  const adapters = new Map();
  const SKIP = new Set([
    'SCRIPT',
    'STYLE',
    'NOSCRIPT',
    'TEMPLATE',
    'HEAD',
    'INPUT',
    'TEXTAREA',
    'SELECT',
    'OPTION',
    'DATALIST',
  ]);
  const STRUCTURAL = new Set([
    'ADDRESS',
    'ARTICLE',
    'ASIDE',
    'BLOCKQUOTE',
    'DD',
    'DETAILS',
    'DIALOG',
    'DIV',
    'DL',
    'DT',
    'FIELDSET',
    'FIGCAPTION',
    'FIGURE',
    'FOOTER',
    'FORM',
    'HEADER',
    'HGROUP',
    'HR',
    'MAIN',
    'NAV',
    'SECTION',
    'SUMMARY',
  ]);
  const HTML_TAGS = new Set(
    'html head title body a abbr address article aside audio b bdi bdo blockquote br button caption cite code col colgroup dd del details dfn div dl dt em fieldset figcaption figure footer form h1 h2 h3 h4 h5 h6 header hgroup hr i img ins kbd label legend li main mark nav ol p picture pre q rp rt ruby s samp section small source span strong sub summary sup table tbody td tfoot th thead time tr track u ul var video wbr'.split(
      ' ',
    ),
  );
  const MAX_DEPTH = 200;
  const MAX_NODES = 100000;
  function limit(value, fallback, ceiling) {
    return Number.isFinite(value) && value >= 0 ? Math.min(Math.floor(value), ceiling) : fallback;
  }
  // Limits count JavaScript UTF-16 code units. Never leave half a surrogate pair.
  function sliceSafe(value, size) {
    let end = Math.max(0, size);
    if (end < value.length && end > 0 && /[\uD800-\uDBFF]/.test(value[end - 1])) end--;
    return value.slice(0, end);
  }
  const cleanText = (value) =>
    String(value || '')
      .replace(/\s+/gu, ' ')
      .trim();
  function safeUrl(value, base) {
    if (!value) return '';
    try {
      const url = new URL(value, base);
      return url.protocol === 'http:' || url.protocol === 'https:' ? url.href : '';
    } catch {
      return '';
    }
  }
  function registerAdapter(adapter) {
    if (
      !adapter ||
      !/^[a-z0-9][a-z0-9_-]{0,63}$/i.test(adapter.id || '') ||
      typeof adapter.matches !== 'function' ||
      typeof adapter.extract !== 'function'
    ) {
      throw new TypeError('Adapter requires id, matches and extract');
    }
    adapters.set(adapter.id, adapter);
  }
  function extract(document, options = {}) {
    if (!document || document.nodeType !== 9) throw new TypeError('A DOM Document is required');
    options = options || {};
    const maxBlocks = limit(options.maxBlocks, 5000, 50000);
    const maxTextChars = limit(options.maxTextChars, 1000000, 10000000);
    const maxHtmlChars = limit(options.maxHtmlChars, 2000000, 10000000);
    const warnings = [];
    const warned = new Set();
    const warn = (code, message) => {
      if (!warned.has(code)) {
        warned.add(code);
        warnings.push({ code, message });
      }
    };
    let url;
    try {
      url = document.location.href;
    } catch {
      url = document.URL || '';
    }
    const capture = {
      schema_version: 1,
      url,
      title: document.title || '',
      lang: document.documentElement?.lang || '',
      captured_at: new Date().toISOString(),
      reason: options.reason || 'initial',
      session_id: options.sessionId || '',
      source: { kind: 'dom', adapter: 'generic' },
      metadata: {
        description: document.querySelector('meta[name="description" i]')?.content || '',
        canonical_url: safeUrl(
          document.querySelector('link[rel~="canonical" i]')?.getAttribute('href'),
          document.baseURI,
        ),
        site_data: {},
      },
      blocks: [],
      warnings,
      stats: { block_count: 0, text_chars: 0 },
    };
    let nodeCount = 0,
      chars = 0,
      exhausted = false;
    const visited = new WeakSet();
    function enter(node, depth) {
      if (!node || exhausted || visited.has(node)) return false;
      if (depth > MAX_DEPTH) {
        warn('max_depth', 'DOM depth limit reached; some nested content was skipped.');
        return false;
      }
      if (++nodeCount > MAX_NODES) {
        exhausted = true;
        warn('max_nodes', 'DOM node limit reached; capture truncated.');
        return false;
      }
      visited.add(node);
      return true;
    }
    // Apply the shared text budget even to rich list/table cell structures.
    function budget(block) {
      function text(value) {
        const result = sliceSafe(String(value || ''), maxTextChars - chars);
        if (result.length < String(value || '').length) {
          warn('max_text_chars', 'Text limit reached; capture truncated.');
          exhausted = true;
        }
        chars += result.length;
        return result;
      }
      const b = { ...block };
      if (typeof b.text === 'string') b.text = text(b.text);
      if (typeof b.alt === 'string') b.alt = text(b.alt);
      if (b.items) b.items = b.items.map(text);
      if (b.cell_blocks) {
        b.cell_blocks = b.cell_blocks.map((row) => row.map((cell) => cell.map(budget)));
        b.rows = b.cell_blocks.map((row) =>
          row.map((cell) =>
            cell
              .filter((part) => ['text', 'paragraph', 'heading', 'link'].includes(part.type))
              .map((part) => part.text)
              .filter(Boolean)
              .join(' '),
          ),
        );
      } else if (b.rows) b.rows = b.rows.map((row) => row.map(text));
      // Link labels are already represented in the owning block text/items.
      if (b.links)
        b.links = b.links.map((link) => ({ text: sliceSafe(link.text, 8192), url: link.url }));
      return b;
    }
    function output(block) {
      if (capture.blocks.length >= maxBlocks) {
        exhausted = true;
        warn('max_blocks', 'Block limit reached; capture truncated.');
        return;
      }
      const bounded = budget(block);
      if (bounded.text === '' && !['code', 'link'].includes(bounded.type)) return;
      capture.blocks.push(bounded);
    }
    const baseFor = (node) => node.ownerDocument?.baseURI || document.baseURI;
    function inlineText(node, depth = 0, state = { nodes: 0 }) {
      if (!node) return '';
      if (depth > MAX_DEPTH) {
        warn('max_depth', 'DOM depth limit reached; some nested content was skipped.');
        return '';
      }
      if (++state.nodes > MAX_NODES) {
        warn('max_nodes', 'DOM node limit reached; capture truncated.');
        return '';
      }
      if (node.nodeType === 3) return node.nodeValue || '';
      if (node.nodeType !== 1 && node.nodeType !== 11) return '';
      if (SKIP.has(node.tagName) || node.isContentEditable) return '';
      if (node.tagName === 'BR') return '\n';
      const parts = [];
      for (const child of node.childNodes) {
        if (state.nodes >= MAX_NODES) {
          warn('max_nodes', 'DOM node limit reached; capture truncated.');
          break;
        }
        parts.push(inlineText(child, depth + 1, state));
      }
      return parts.join('');
    }
    function language(node) {
      const code = node.tagName === 'PRE' ? node.querySelector('code') || node : node;
      const match = String(code.className || '').match(/(?:^|\s)(?:language|lang)-([\w+-]+)/i);
      return code.getAttribute('data-language') || (match ? match[1] : '');
    }
    function childrenFor(node) {
      if (node.nodeType === 1 && node.tagName === 'SLOT') {
        const assigned = node.assignedNodes?.({ flatten: true });
        if (assigned?.length) return assigned;
      }
      return node.childNodes;
    }
    // A flow collects adjacent inline content, flushing at semantic boundaries.
    function flow(container, sink, kind = 'text', level, depth = 0) {
      let run = '',
        links = [],
        hasBoundary = false;
      function flush() {
        const text = cleanText(run);
        if (text) {
          const b = { type: kind, ...(level ? { level } : {}), text };
          if (links.length) b.links = links;
          if (kind === 'text' && links.length === 1 && text === links[0].text) {
            b.type = 'link';
            b.url = links[0].url;
            delete b.links;
          }
          sink(b);
        }
        run = '';
        links = [];
      }
      function boundary() {
        hasBoundary = true;
        flush();
      }
      function process(node, currentDepth) {
        if (!enter(node, currentDepth)) return;
        if (node.nodeType === 3) {
          run += node.nodeValue || '';
          return;
        }
        if (node.nodeType !== 1 && node.nodeType !== 11) return;
        const tag = node.tagName;
        if (SKIP.has(tag) || node.isContentEditable) return;
        if (tag === 'BR') {
          run += '\n';
          return;
        }
        if (tag === 'A') {
          const href = safeUrl(node.getAttribute('href'), baseFor(node));
          const parts = [];
          const shape = flow(node, (b) => parts.push(b), 'text', undefined, currentDepth + 1);
          const raw = inlineText(node);
          // Ordinary inline links stay in their surrounding text run. Rich anchors
          // retain a separate destination marker, followed by each descendant once.
          if (
            !shape.hasBoundary &&
            parts.length === 1 &&
            parts[0].type === 'text' &&
            !parts[0].links &&
            cleanText(raw) === parts[0].text
          ) {
            run += raw;
            if (href) links.push({ text: parts[0].text, url: href });
          } else {
            boundary();
            if (href) sink({ type: 'link', text: '', url: href });
            for (const part of parts) sink(part);
          }
          return;
        }
        if (tag === 'PRE' || tag === 'CODE') {
          boundary();
          sink({ type: 'code', text: inlineText(node), language: language(node) });
          return;
        }
        if (/^H[1-6]$/.test(tag || '')) {
          boundary();
          flow(node, sink, 'heading', Number(tag[1]), currentDepth + 1);
          return;
        }
        if (tag === 'P') {
          boundary();
          flow(node, sink, 'paragraph', undefined, currentDepth + 1);
          return;
        }
        if (tag === 'UL' || tag === 'OL') {
          boundary();
          list(node, sink, currentDepth + 1);
          return;
        }
        if (tag === 'TABLE') {
          boundary();
          table(node, sink, currentDepth + 1);
          return;
        }
        if (tag === 'IMG') {
          boundary();
          const src = safeUrl(node.currentSrc || node.getAttribute('src'), baseFor(node));
          if (src || node.alt) sink({ type: 'image', src, alt: node.alt || '' });
          return;
        }
        if (['VIDEO', 'AUDIO'].includes(tag)) {
          boundary();
          const src = safeUrl(
            node.currentSrc ||
              node.getAttribute('src') ||
              node.querySelector('source')?.getAttribute('src'),
            baseFor(node),
          );
          sink({
            type: 'media',
            kind: tag.toLowerCase(),
            src,
            poster: safeUrl(node.getAttribute('poster'), baseFor(node)),
          });
          flow(node, sink, 'text', undefined, currentDepth + 1);
          return;
        }
        if (tag === 'IFRAME' || tag === 'FRAME') {
          boundary();
          try {
            const doc = node.contentDocument;
            if (!doc || !doc.documentElement) throw new Error('inaccessible');
            // Touch location to distinguish a cross-origin Window from an accessible frame.
            void doc.location.href;
            flow(doc.body || doc.documentElement, sink, 'text', undefined, currentDepth + 1);
          } catch {
            warn(
              'inaccessible_frame',
              'An inaccessible iframe was skipped (cross-origin, sandboxed, or loading).',
            );
          }
          return;
        }
        if (['CANVAS', 'OBJECT', 'EMBED'].includes(tag)) {
          boundary();
          warn('unreadable_media', 'Canvas and embedded objects are not converted to text.');
          if (tag !== 'CANVAS')
            sink({
              type: 'media',
              kind: tag.toLowerCase(),
              src: safeUrl(node.getAttribute('data') || node.getAttribute('src'), baseFor(node)),
            });
          flow(node, sink, 'text', undefined, currentDepth + 1);
          return;
        }
        let block = STRUCTURAL.has(tag);
        if (
          !block &&
          tag &&
          !['A', 'SPAN', 'B', 'I', 'EM', 'STRONG', 'SMALL', 'S', 'U', 'LABEL', 'SLOT'].includes(tag)
        ) {
          try {
            block = /^(block|flex|grid|table|list-item|flow-root)/.test(
              node.ownerDocument.defaultView.getComputedStyle(node).display,
            );
          } catch {
            /* detached document */
          }
        }
        if (node.shadowRoot) {
          boundary();
          flow(node.shadowRoot, sink, 'text', undefined, currentDepth + 1);
          // Assigned light DOM nodes were visited at their slots; remaining nodes are still accessible DOM.
          flow(node, sink, 'text', undefined, currentDepth + 1);
          return;
        }
        if (block) {
          boundary();
          flow(node, sink, 'text', undefined, currentDepth + 1);
          return;
        }
        for (const child of childrenFor(node)) process(child, currentDepth + 1);
      }
      for (const child of childrenFor(container)) {
        if (exhausted) break;
        process(child, depth + 1);
      }
      flush();
      return { hasBoundary };
    }
    function list(node, sink, depth) {
      let items = [],
        links = [];
      const ordered = node.tagName === 'OL';
      function flush() {
        if (items.length) {
          sink({ type: 'list', ordered, items, ...(links.length ? { links } : {}) });
          items = [];
          links = [];
        }
      }
      for (const child of node.children) {
        if (exhausted) break;
        if (child.tagName !== 'LI') {
          flush();
          flow(child, sink, 'text', undefined, depth + 1);
          continue;
        }
        if (!enter(child, depth)) continue;
        const pieces = [];
        flow(child, (b) => pieces.push(b), 'text', undefined, depth + 1);
        let item = '';
        for (const b of pieces) {
          if (b.type === 'link' && !b.text) {
            if (item) {
              items.push(item);
              item = '';
            }
            flush();
            sink(b);
            continue;
          }
          if (['text', 'paragraph', 'link'].includes(b.type)) {
            item += (item ? ' ' : '') + b.text;
            if (b.links) links.push(...b.links);
            if (b.type === 'link') links.push({ text: b.text, url: b.url });
          } else {
            if (item) {
              items.push(item);
              item = '';
            }
            flush();
            sink(b);
          }
        }
        if (item || !pieces.length) items.push(item);
      }
      flush();
    }
    function table(node, sink, depth) {
      const rows = [],
        details = [];
      let rich = false;
      if (node.caption) flow(node.caption, sink, 'paragraph', undefined, depth + 1);
      // HTMLTableElement.rows moves tfoot after tbody; archive actual DOM order.
      for (const row of node.querySelectorAll('tr')) {
        if (exhausted) break;
        if (row.closest('table') !== node || !enter(row, depth)) continue;
        const cells = [],
          cellDetails = [];
        for (const cell of row.cells) {
          if (!enter(cell, depth + 1)) continue;
          const parts = [];
          flow(cell, (b) => parts.push(b), 'text', undefined, depth + 2);
          cells.push(
            parts
              .filter((b) => ['text', 'paragraph', 'heading', 'link'].includes(b.type))
              .map((b) => b.text)
              .filter(Boolean)
              .join(' '),
          );
          if (
            parts.some(
              (b) =>
                b.type === 'link' ||
                b.links?.length ||
                !['text', 'paragraph', 'heading'].includes(b.type),
            )
          )
            rich = true;
          cellDetails.push(parts);
        }
        rows.push(cells);
        details.push(cellDetails);
      }
      sink({ type: 'table', rows, ...(rich ? { cell_blocks: details } : {}) });
    }
    flow(document.body || document.documentElement, output);
    for (const adapter of adapters.values()) {
      try {
        if (!adapter.matches({ document, url })) continue;
        const data = adapter.extract({ document, url });
        if (!data || typeof data !== 'object' || Array.isArray(data))
          throw new TypeError('Adapter result must be a metadata object');
        // JSON copy rejects executable values/cycles and makes captures transportable.
        const json = JSON.stringify(data);
        if (json.length > 65536) throw new RangeError('Adapter metadata exceeds 65536 characters');
        capture.metadata.site_data = { ...capture.metadata.site_data, ...JSON.parse(json) };
        capture.source.adapter = adapter.id;
      } catch {
        warn('adapter_error', 'Adapter error; the generic DOM capture was retained.');
      }
    }
    if (options.saveHtml !== false) capture.html = sanitizedHtml(document, maxHtmlChars, warn);
    capture.stats = { block_count: capture.blocks.length, text_chars: chars };
    return capture;
  }
  function sanitizedHtml(source, maxChars, warn) {
    const target = source.implementation.createHTMLDocument('');
    let nodes = 0,
      remaining = maxChars;
    const seen = new WeakSet();
    function boundedValue(value, allowance, attribute = false) {
      let end = 0,
        cost = 0;
      for (const char of value) {
        const size =
          char === '\u00a0'
            ? 6
            : char === '&'
              ? 5
              : char === '<' || char === '>'
                ? 4
                : attribute && char === '"'
                  ? 6
                  : char.length;
        if (cost + size > allowance) break;
        cost += size;
        end += char.length;
      }
      return { value: value.slice(0, end), cost };
    }
    function clone(node, depth) {
      if (!node || seen.has(node)) return null;
      seen.add(node);
      if (++nodes > MAX_NODES) {
        warn('max_nodes', 'DOM node limit reached; capture truncated.');
        return null;
      }
      if (depth > MAX_DEPTH) {
        warn('max_depth', 'DOM depth limit reached; some nested content was skipped.');
        return null;
      }
      if (remaining <= 0) {
        warn('max_html_chars', 'HTML limit reached; source truncated.');
        return null;
      }
      if (node.nodeType === 3) {
        const bounded = boundedValue(node.nodeValue || '', remaining);
        const text = bounded.value;
        remaining -= bounded.cost;
        if (text.length < (node.nodeValue || '').length)
          warn('max_html_chars', 'HTML limit reached; source truncated.');
        return target.createTextNode(text);
      }
      if (node.nodeType !== 1 && node.nodeType !== 11) return null;
      const tag = node.tagName?.toLowerCase();
      if (
        tag &&
        ([
          'script',
          'style',
          'noscript',
          'template',
          'iframe',
          'frame',
          'object',
          'embed',
          'input',
          'textarea',
          'select',
          'option',
          'datalist',
          'base',
          'meta',
          'link',
        ].includes(tag) ||
          node.isContentEditable)
      )
        return null;
      const safeTag = tag && (HTML_TAGS.has(tag) ? tag : 'div');
      if (tag) {
        remaining -= safeTag.length * 2 + 5;
        if (remaining <= 0) {
          warn('max_html_chars', 'HTML limit reached; source truncated.');
          return null;
        }
      }
      const result = tag ? target.createElement(safeTag) : target.createDocumentFragment();
      function attribute(name, value) {
        // Reserve for escaping each character, delimiters, and the attribute name.
        const room = remaining - name.length - 4;
        if (room <= 0) {
          remaining = 0;
          warn('max_html_chars', 'HTML limit reached; source truncated.');
          return;
        }
        const bounded = boundedValue(value, room, true);
        remaining -= name.length + 4 + bounded.cost;
        result.setAttribute(name, bounded.value);
        if (bounded.value.length < value.length) {
          remaining = 0;
          warn('max_html_chars', 'HTML limit reached; source truncated.');
        }
      }
      if (tag) {
        for (const attr of node.attributes) {
          const name = attr.name.toLowerCase();
          if (
            [
              'lang',
              'dir',
              'title',
              'alt',
              'colspan',
              'rowspan',
              'start',
              'width',
              'height',
              'datetime',
            ].includes(name)
          )
            attribute(name, sliceSafe(attr.value, 8192));
          if (['href', 'src', 'poster'].includes(name)) {
            const url =
              attr.value.length <= Math.min(8192, remaining)
                ? safeUrl(attr.value, node.ownerDocument.baseURI)
                : null;
            if (url) attribute(name, url);
            else if (attr.value.length > Math.min(8192, remaining))
              warn('max_html_chars', 'HTML limit reached; source truncated.');
          }
          if (remaining <= 0) break;
        }
        if (tag === 'a') attribute('rel', 'noopener noreferrer');
        if (['button', 'audio', 'video'].includes(tag)) attribute('disabled', '');
      }
      if (node.shadowRoot && remaining <= 100)
        warn('max_html_chars', 'HTML limit reached; source truncated.');
      if (node.shadowRoot && remaining > 100) {
        remaining -= 100;
        const shadow = target.createElement('section');
        shadow.setAttribute('data-pagevault-shadowroot', 'open');
        for (const child of node.shadowRoot.childNodes) {
          const c = clone(child, depth + 1);
          if (c) shadow.append(c);
        }
        result.append(shadow);
      }
      let children = node.childNodes;
      if (tag === 'slot') {
        const assigned = node.assignedNodes?.({ flatten: true });
        if (assigned?.length) children = assigned;
      }
      for (const child of children) {
        const c = clone(child, depth + 1);
        if (c) result.append(c);
        if (remaining <= 0 || nodes > MAX_NODES) break;
      }
      return result;
    }
    const root = clone(source.documentElement, 0);
    const html = root?.outerHTML || '';
    if (html.length > maxChars) warn('max_html_chars', 'HTML limit reached; source truncated.');
    return sliceSafe(html, maxChars);
  }
  globalThis.PageVaultExtractor = Object.freeze({ extract, registerAdapter });
})();
