// Render actual PageVault UI with artificial data; never open a personal profile.
const fs = require('node:fs');
const path = require('node:path');
const assert = require('node:assert/strict');
const { playwright, browserOptions, serveProject } = require('../tests/helpers.cjs');

(async () => {
  const root = path.resolve(__dirname, '..');
  const output = path.join(root, 'dist', 'store-assets');
  fs.mkdirSync(output, { recursive: true });
  const version = JSON.parse(
    fs.readFileSync(path.join(root, 'extension', 'manifest.json'), 'utf8'),
  ).version;
  const server = await serveProject();
  let browser;
  const files = [];
  try {
    browser = await playwright().chromium.launch(browserOptions());
    async function scene(name, route, connected) {
      const page = await browser.newPage({ viewport: { width: 1280, height: 800 } });
      const errors = [];
      page.on('pageerror', (e) => errors.push(e.message));
      await page.addInitScript(
        ({ version, connected }) => {
          const stamp = '2026-10-28T12:00:00Z';
          const rows = [
            'Заметки проекта: планы и решения',
            'Обсуждение новой функции',
            'Документация по установке',
          ].map((title, i) => ({
            id: i + 1,
            url: `https://example.org/notes/${i + 1}`,
            title,
            hostname: 'example.org',
            first_seen: stamp,
            last_seen: stamp,
            capture_count: 1,
            block_count: 6,
            latest_capture_id: i + 1,
          }));
          const blocks = [
            { type: 'heading', level: 2, text: 'Решения по проекту' },
            {
              type: 'paragraph',
              text: 'Сохраняем важные заметки, обсуждения и примеры в личный архив. Текст, таблицы и код остаются доступны для поиска и чтения агентом.',
            },
            {
              type: 'list',
              items: [
                'Выбрать сайты для записи',
                'Сохранить загруженное содержимое',
                'Найти нужное в локальном архиве',
              ],
            },
            { type: 'heading', level: 3, text: 'План работы' },
            {
              type: 'table',
              rows: [
                ['Задача', 'Состояние'],
                ['Локальное хранение', 'Готово'],
                ['Доступ через MCP', 'Готово'],
              ],
            },
            {
              type: 'code',
              language: 'javascript',
              text: 'const notes = ["идея", "решение"];\nconsole.log(notes.join("\\n"));',
            },
          ];
          const host = {
            connected,
            version,
            schema_version: 1,
            db_path: connected
              ? '/Users/demo/Library/Application Support/PageVault/archive.sqlite3'
              : '',
            error: connected ? '' : 'Локальный помощник недоступен',
          };
          window.chrome = {
            runtime: {
              id: 'ijkioljfdaaplhaldbdpabckkfcieiee',
              getManifest: () => ({ version }),
              getPlatformInfo: async () => ({ os: 'mac' }),
              sendMessage: async (message) => {
                if (message.type === 'status')
                  return {
                    ok: true,
                    data: {
                      host,
                      pendingCount: 0,
                      queueBytes: 0,
                      enabledOrigins: [],
                      settings: {
                        saveHtml: true,
                        captureIntervalSeconds: 30,
                        debounceMs: 1200,
                        maxBlocks: 5000,
                        maxTextChars: 1000000,
                        maxHtmlChars: 2000000,
                      },
                    },
                  };
                const r = message.request;
                let data;
                if (r.type === 'stats')
                  data = { pages: 3, captures: 3, blocks: 18, db_bytes: 49152 };
                else if (r.type === 'pages.domains')
                  data = { items: [{ hostname: 'example.org', page_count: 3 }] };
                else if (r.type === 'pages.list')
                  data = { items: rows, total: 3, limit: 25, offset: 0 };
                else if (r.type === 'pages.get')
                  data = {
                    page: rows.find((p) => p.id === r.page_id),
                    captures: [
                      { id: r.page_id, captured_at: stamp, block_count: 6, reason: 'manual' },
                    ],
                    total: 1,
                  };
                else if (r.type === 'pages.export') {
                  const text = JSON.stringify({
                    schema_version: 1,
                    archive_kind: 'observed_blocks',
                    page: rows.find((p) => p.id === r.page_id),
                    blocks,
                  });
                  const chars = Array.from(text),
                    offset = r.offset || 0,
                    end = Math.min(chars.length, offset + r.chunk_size);
                  data = {
                    json_chunk: chars.slice(offset, end).join(''),
                    offset,
                    next_offset: end < chars.length ? end : null,
                    total_chars: chars.length,
                    document_hash: 'synthetic-store-scene',
                    done: end === chars.length,
                  };
                } else throw Error('Unexpected screenshot request ' + r.type);
                return { ok: true, data };
              },
            },
          };
        },
        { version, connected },
      );
      try {
        await page.goto(server.url + route);
        if (route.includes('archive')) {
          await page.waitForFunction(() => document.querySelectorAll('.page-item').length === 3);
          await page.locator('.page-item').first().click();
          await page.waitForFunction(() => !document.querySelector('#export').disabled);
        } else {
          await page.waitForFunction(() => !document.querySelector('#settings-fields').disabled);
          if (route.includes('#installation'))
            await page.locator('#installation').scrollIntoViewIfNeeded();
        }
        assert.deepEqual(errors, []);
        await page.screenshot({ path: path.join(output, name) });
        const png = fs.readFileSync(path.join(output, name));
        assert.equal(png.readUInt32BE(16), 1280);
        assert.equal(png.readUInt32BE(20), 800);
        files.push(name);
      } finally {
        await page.close();
      }
    }
    await scene('01-archive.png', '/extension/ui/archive.html', true);
    await scene('02-connection.png', '/extension/ui/options.html', true);
    await scene('03-installation.png', '/extension/ui/options.html#installation', false);
    fs.copyFileSync(
      path.join(root, 'extension', 'icons', 'icon128.png'),
      path.join(output, 'icon128.png'),
    );
    fs.writeFileSync(
      path.join(output, 'metadata.json'),
      JSON.stringify({ version, synthetic: true, screenshotSize: '1280x800', files }, null, 2) +
        '\n',
    );
    console.log(`Created ${files.length} synthetic UI screenshots and icon in ${output}.`);
  } finally {
    await browser?.close();
    await server.close();
  }
})().catch((error) => {
  console.error(error);
  process.exitCode = 1;
});
