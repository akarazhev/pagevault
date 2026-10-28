'use strict';
const { test, before, after } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const http = require('node:http');
const root = path.join(__dirname, '..');
const { chromium } = require(process.env.CODEX_PRIMARY_RUNTIME_NODE_MODULES ? path.join(process.env.CODEX_PRIMARY_RUNTIME_NODE_MODULES, 'playwright') : 'playwright');
let browser, server, origin;
before(async () => {
  server = http.createServer((req, res) => {
    res.setHeader('Content-Type', 'text/html; charset=utf-8');
    res.end(req.url === '/frame' ? '<p>Содержимое фрейма</p>' : fs.readFileSync(path.join(__dirname, 'fixtures/extractor-universal.html')));
  });
  await new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
  origin = `http://127.0.0.1:${server.address().port}`;
  browser = await chromium.launch({headless:true, executablePath:process.env.PAGEVAULT_CHROMIUM_PATH || undefined, args:['--no-sandbox']});
});
after(async () => { if (browser) await browser.close(); if (server) await new Promise(resolve => server.close(resolve)); });
async function fixture(fn, html) {
  const page = await browser.newPage();
  try {
    await page.goto(origin + '/project?tab=1#issue', {waitUntil:'load'});
    if (html !== undefined) await page.setContent(html);
    for (const file of ['extension/extractor.js', 'extension/adapters/gitlab.js']) {
      if (fs.existsSync(path.join(root, file))) await page.addScriptTag({path:path.join(root, file)});
    }
    assert.equal(await page.evaluate(() => typeof globalThis.PageVaultExtractor?.extract), 'function', 'extractor synchronous API exists');
    return await fn(page);
  } finally { await page.close(); }
}
test('captures generic div text once in DOM order and retains source URL state', () => fixture(async page => {
  const c = await page.evaluate(() => PageVaultExtractor.extract(document));
  assert.equal(c.schema_version, 1); assert.equal(c.url, origin + '/project?tab=1#issue');
  assert.equal(c.title, 'Проверка 📚'); assert.equal(c.lang, 'ru'); assert.equal(c.metadata.description, 'Описание');
  const texts = c.blocks.map(b => b.text).filter(Boolean);
  assert.deepEqual(texts.slice(0, 5), ['Заголовок', 'Первый общий текст Документы', 'Вложенный текст', 'Конец', 'До кода']);
  assert.equal(texts.filter(t => t.includes('Вложенный текст')).length, 1);
  assert.deepEqual(c.blocks.find(b => b.text === 'Первый общий текст Документы').links, [{text:'Документы',url:origin + '/docs?q=2#part'}]);
}));
test('preserves code bytes, nested lists, tables and media with paragraph order', () => fixture(async page => {
  const c = await page.evaluate(() => PageVaultExtractor.extract(document));
  assert.deepEqual(c.blocks.find(b => b.type === 'code'), {type:'code',text:'def f():\n    print("Привет 🌍")\n\treturn 2\n',language:'python'});
  const lists = c.blocks.filter(b => b.type === 'list');
  assert.deepEqual(lists[0].items, ['Первый пункт', 'Второй пункт']);
  assert.ok(lists.some(b => b.ordered && b.items.includes('Вложенный')));
  assert.deepEqual(c.blocks.find(b => b.type === 'table').rows, [['Имя','Значение'],['A','42']]);
  const imageIndex = c.blocks.findIndex(b => b.type === 'image' && b.alt === 'Фото');
  assert.equal(c.blocks[imageIndex - 1].text, 'Перед картинкой'); assert.equal(c.blocks[imageIndex + 1].text, 'после картинки');
  assert.equal(c.blocks[imageIndex].src, origin + '/photo.png');
  assert.ok(c.blocks.some(b => b.type === 'media' && b.src === origin + '/movie.mp4'));
}));
test('includes open shadow roots and same-origin frames, warning on inaccessible frames', () => fixture(async page => {
  const c = await page.evaluate(() => PageVaultExtractor.extract(document));
  assert.ok(c.blocks.some(b => b.text === 'Текст тени'));
  assert.ok(c.blocks.some(b => b.text === 'Содержимое фрейма'));
  assert.ok(c.warnings.some(w => w.code === 'inaccessible_frame'));
}));
test('sanitizes HTML and excludes form field values from structured text', () => fixture(async page => {
  const c = await page.evaluate(() => PageVaultExtractor.extract(document));
  const all = JSON.stringify(c);
  assert.doesNotMatch(all, /SECRET_INPUT|SECRET_TEXTAREA|SECRET_OPTION|EXECUTABLE_SECRET/);
  const unsafe = await page.evaluate(html => {
    const d = new DOMParser().parseFromString(html, 'text/html');
    return {scripts:d.querySelectorAll('script,iframe,object,embed').length, attrs:[...d.querySelectorAll('*')].flatMap(e => [...e.attributes].filter(a => /^on/i.test(a.name) || /javascript:/i.test(a.value)).map(a => a.name))};
  }, c.html);
  assert.deepEqual(unsafe, {scripts:0, attrs:[]});
  assert.equal((await page.evaluate(() => PageVaultExtractor.extract(document,{saveHtml:false}))).html, undefined);
}));
test('reports block, text and HTML limits without cutting a surrogate pair', () => fixture(async page => {
  const blocks = await page.evaluate(() => PageVaultExtractor.extract(document,{maxBlocks:2}));
  assert.equal(blocks.blocks.length,2); assert.ok(blocks.warnings.some(w => w.code === 'max_blocks'));
  const chars = await page.evaluate(() => PageVaultExtractor.extract(document,{maxTextChars:4,maxHtmlChars:50}));
  assert.ok(chars.stats.text_chars <= 4); assert.ok(chars.warnings.some(w => w.code === 'max_text_chars')); assert.ok(chars.html.length <= 50); assert.ok(chars.warnings.some(w => w.code === 'max_html_chars'));
  const emoji = await page.evaluate(() => { document.body.textContent = '🌍🌍'; return PageVaultExtractor.extract(document,{maxTextChars:3,saveHtml:false}); });
  assert.equal(emoji.blocks[0].text,'🌍'); assert.equal(emoji.stats.text_chars,2);
}));
test('adapter exceptions preserve generic extraction and later matching adapters can add metadata', () => fixture(async page => {
  const c = await page.evaluate(() => {
    PageVaultExtractor.registerAdapter({id:'broken',matches:()=>true,extract:()=>{throw new Error('fixture');}});
    PageVaultExtractor.registerAdapter({id:'working',matches:()=>true,extract:()=>({issue:7})});
    return PageVaultExtractor.extract(document);
  });
  assert.equal(c.source.adapter, 'working'); assert.deepEqual(c.metadata.site_data, {issue:7});
  assert.ok(c.blocks.some(b => b.text === 'Заголовок')); assert.ok(c.warnings.some(w => w.code === 'adapter_error'));
}));
test('GitLab adapter adds metadata without replacing universal text', () => fixture(async page => {
  await page.evaluate(() => { document.head.insertAdjacentHTML('beforeend','<meta name="application-name" content="GitLab"><meta name="project_id" content="52">'); });
  const c = await page.evaluate(() => PageVaultExtractor.extract(document));
  assert.equal(c.source.adapter,'gitlab'); assert.equal(c.metadata.site_data.project_id,'52');
  assert.ok(c.blocks.some(b => b.text === 'Вложенный текст'));
}));
test('does not lose code, images or nested blocks inside lists and tables', () => fixture(async page => {
  const c = await page.evaluate(() => PageVaultExtractor.extract(document,{saveHtml:false}));
  const serialized = JSON.stringify(c.blocks);
  assert.match(serialized,/before/); assert.match(serialized,/after/); assert.match(serialized,/cell tail/);
  assert.match(serialized,/x\\n  y/); assert.match(serialized,/nested table/);
  assert.match(serialized,/diagram/); assert.match(serialized,/diagram\.png/);
}, '<ul><li>before<pre>x\n  y</pre>after<img src="/diagram.png" alt="diagram"></li></ul><table><tr><td>cell<table><tr><td>nested table</td></tr></table>cell tail</td></tr></table>'));
test('handles slotted text once and warns rather than overflowing deeply nested documents', () => fixture(async page => {
  const c = await page.evaluate(() => {
    document.body.innerHTML = '<div id="host"><b>slotted once</b></div>';
    document.querySelector('#host').attachShadow({mode:'open'}).innerHTML = '<p>shadow <slot></slot> end</p>';
    const host = document.createElement('section'); document.body.append(host);
    let e = host; for (let i = 0; i < 600; i++) { const child = document.createElement('div'); e.append(child); e = child; } e.textContent = 'deep';
    return PageVaultExtractor.extract(document);
  });
  assert.equal(c.blocks.map(b=>b.text || '').join('\n').match(/slotted once/g)?.length,1);
  assert.ok(c.warnings.some(w => w.code === 'max_depth'));
}));
test('reports depth truncation inside code rather than silently dropping its text', () => fixture(async page => {
  const c = await page.evaluate(() => {
    document.body.innerHTML = '<pre>prefix</pre>';
    let e = document.querySelector('pre');
    for (let i=0;i<250;i++) { const span=document.createElement('span'); e.append(span); e=span; }
    e.textContent = 'unreachable';
    return PageVaultExtractor.extract(document,{saveHtml:false});
  });
  assert.equal(c.blocks[0].text,'prefix');
  assert.ok(c.warnings.some(w => w.code === 'max_depth'));
}));
test('keeps semantic caption content and table sections in DOM order', () => fixture(async page => {
  const c = await page.evaluate(() => PageVaultExtractor.extract(document,{saveHtml:false}));
  assert.equal(c.blocks[0].text,'Caption');
  assert.equal(c.blocks[1].type,'image'); assert.equal(c.blocks[1].alt,'caption image');
  assert.deepEqual(c.blocks.find(b=>b.type==='table').rows,[['footer first'],['body next']]);
}, '<table><caption>Caption<img src="/caption.png" alt="caption image"></caption><tfoot><tr><td>footer first</td></tr></tfoot><tbody><tr><td>body next</td></tr></tbody></table>'));
test('retains image-only anchor destination alongside media without requiring HTML', () => fixture(async page => {
  const c = await page.evaluate(() => PageVaultExtractor.extract(document,{saveHtml:false}));
  assert.deepEqual(c.blocks,[
    {type:'text',text:'Before'},
    {type:'link',text:'',url:origin+'/image-target'},
    {type:'image',src:origin+'/thumb.png',alt:'Thumbnail'},
    {type:'text',text:'After'}
  ]);
}, 'Before<a href="/image-target"><img src="/thumb.png" alt="Thumbnail"></a>After'));
test('retains block-containing anchor destinations without duplicating card text', () => fixture(async page => {
  const c = await page.evaluate(() => PageVaultExtractor.extract(document,{saveHtml:false}));
  const links = c.blocks.flatMap(b => b.type==='link' ? [{url:b.url}] : b.links || []);
  assert.ok(links.some(b=>b.url === origin+'/nested-card'));
  assert.ok(links.some(b=>b.url === origin+'/rich-card'));
  const text = c.blocks.map(b=>b.text || '').filter(Boolean);
  assert.deepEqual(text,['Nested card','Card heading','Card description','Tail']);
  assert.deepEqual(c.blocks.filter(b=>b.type==='image').map(b=>b.alt),['Card image']);
}, '<a href="/nested-card"><div>Nested card</div></a><a href="/rich-card"><h2>Card heading</h2><p>Card description<img src="/card.png" alt="Card image">Tail</p></a>'));
test('retains links in ordinary table cells and linked media nested inside lists', () => fixture(async page => {
  const c = await page.evaluate(() => PageVaultExtractor.extract(document,{saveHtml:false}));
  const table = c.blocks.find(b=>b.type==='table');
  assert.deepEqual(table.rows,[['Row title','prefix Documentation suffix']]);
  assert.ok(table.cell_blocks,'linked cells retain destination-bearing block details');
  assert.equal(table.cell_blocks[0][0][0].url,origin+'/row-target');
  assert.deepEqual(table.cell_blocks[0][1][0].links,[{text:'Documentation',url:origin+'/documentation'}]);
  const marker = c.blocks.findIndex(b=>b.type==='link' && b.url===origin+'/list-target');
  assert.ok(marker>=0); assert.equal(c.blocks[marker+1].type,'image');
  assert.equal(c.blocks[marker+1].alt,'Linked list image');
}, '<table><tr><td><a href="/row-target">Row title</a></td><td>prefix <a href="/documentation">Documentation</a> suffix</td></tr></table><ul><li>start<a href="/list-target"><img src="/list.png" alt="Linked list image"></a>end</li></ul>'));
test('preserves boundaries between adjacent block-containing links', () => fixture(async page => {
  const c = await page.evaluate(() => PageVaultExtractor.extract(document,{saveHtml:false}));
  assert.deepEqual(c.blocks.map(b=>b.text).filter(Boolean),['One card','Two card']);
  assert.deepEqual(c.blocks.filter(b=>b.type==='link').map(b=>b.url),[origin+'/one',origin+'/two']);
}, '<a href="/one"><div>One card</div></a><a href="/two"><div>Two card</div></a>'));
