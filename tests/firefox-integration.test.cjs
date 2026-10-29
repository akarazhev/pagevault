const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { randomUUID } = require('node:crypto');
const { spawn, execFileSync } = require('node:child_process');
const { once } = require('node:events');
const { playwright, serveProject } = require('./helpers.cjs');

// BiDi can install Firefox extensions without a WebDriver dependency. Test copies
// use a unique native host name, so they can never contact the user's real helper.
async function connect(browser) {
  const endpoint = await new Promise((resolve, reject) => {
    const timeout = setTimeout(() => reject(Error('Firefox BiDi startup timed out')), 20000);
    let output = '';
    const read = (chunk) => {
      output += chunk.toString();
      const match = output.match(/WebDriver BiDi listening on (ws:\/\/[^\s]+)/);
      if (match) {
        clearTimeout(timeout);
        resolve(match[1] + '/session');
      }
    };
    browser.stdout.on('data', read);
    browser.stderr.on('data', read);
    browser.once('error', (error) => {
      clearTimeout(timeout);
      reject(error);
    });
    browser.once('exit', () => {
      clearTimeout(timeout);
      reject(Error('Firefox exited before BiDi startup'));
    });
  });
  const socket = new WebSocket(endpoint);
  await once(socket, 'open');
  let next = 0;
  const pending = new Map();
  socket.addEventListener('message', (event) => {
    const reply = JSON.parse(event.data);
    const entry = pending.get(reply.id);
    if (!entry) return;
    pending.delete(reply.id);
    clearTimeout(entry.timeout);
    if (reply.type === 'error') entry.reject(Error(`${reply.error}: ${reply.message}`));
    else entry.resolve(reply.result);
  });
  socket.addEventListener('close', () => {
    for (const entry of pending.values()) {
      clearTimeout(entry.timeout);
      entry.reject(Error('Firefox BiDi connection closed'));
    }
    pending.clear();
  });
  return {
    close: () => socket.close(),
    command(method, params = {}) {
      return new Promise((resolve, reject) => {
        const id = ++next;
        const timeout = setTimeout(() => {
          pending.delete(id);
          reject(Error(`Firefox ${method} timed out`));
        }, 25000);
        pending.set(id, { resolve, reject, timeout });
        socket.send(JSON.stringify({ id, method, params }));
      });
    },
  };
}

test(
  'Firefox package starts its background, captures Unicode and preserves an offline queue',
  {
    timeout: 120000,
    skip:
      process.env.PAGEVAULT_TEST_BROWSER !== 'firefox'
        ? 'Run npm run test:firefox for Firefox extension checks'
        : process.platform === 'win32'
          ? 'Extension smoke uses the Unix unzip utility; portable Firefox DOM/UI tests still run'
          : false,
  },
  async (t) => {
    const root = path.resolve(__dirname, '..');
    const temp = fs.mkdtempSync(path.join(os.tmpdir(), 'pagevault-firefox-'));
    const server = await serveProject();
    let processFirefox, bidi;
    try {
      const go = process.env.PAGEVAULT_GO || 'go';
      const zip = path.join(temp, 'firefox.zip');
      execFileSync(
        go,
        ['run', './cmd/release', '--extension-only', '--browser', 'firefox', '--output', zip],
        { cwd: root, stdio: 'pipe' },
      );
      const extension = path.join(temp, 'extension');
      execFileSync('unzip', ['-q', zip, '-d', extension]);
      const manifestPath = path.join(extension, 'manifest.json');
      const manifest = JSON.parse(fs.readFileSync(manifestPath, 'utf8'));
      assert.equal(manifest.background.service_worker, undefined);
      assert.deepEqual(manifest.background.scripts, ['background.js']);
      manifest.host_permissions = ['http://127.0.0.1/*'];
      fs.writeFileSync(manifestPath, JSON.stringify(manifest));
      const id = manifest.browser_specific_settings.gecko.id;
      const uuid = randomUUID();
      const hostName = 'com.pagevault.test_' + randomUUID().replaceAll('-', '');
      const configPath = path.join(extension, 'lib/config.js');
      fs.writeFileSync(
        configPath,
        fs.readFileSync(configPath, 'utf8').replace('com.pagevault.host', hostName),
      );
      const profile = path.join(temp, 'profile');
      fs.mkdirSync(profile);
      fs.writeFileSync(
        path.join(profile, 'user.js'),
        `user_pref("extensions.webextensions.uuids", ${JSON.stringify(JSON.stringify({ [id]: uuid }))});\n` +
          'user_pref("browser.shell.checkDefaultBrowser", false);\n' +
          'user_pref("browser.startup.homepage_override.mstone", "ignore");\n',
      );
      const home = path.join(temp, 'home');
      fs.mkdirSync(home);
      processFirefox = spawn(
        process.env.PAGEVAULT_FIREFOX_PATH || playwright().firefox.executablePath(),
        [
          '--headless',
          '--no-remote',
          '--profile',
          profile,
          '--remote-debugging-port=0',
          '--remote-allow-system-access',
        ],
        {
          env: { ...process.env, ...(process.platform === 'linux' ? { HOME: home } : {}) },
          stdio: ['ignore', 'pipe', 'pipe'],
        },
      );
      bidi = await connect(processFirefox);
      await bidi.command('session.new', { capabilities: {} });
      const tree = await bidi.command('browsingContext.getTree');
      const installed = await bidi.command('webExtension.install', {
        extensionData: { type: 'path', path: extension },
      });
      assert.equal(installed.extension, id);
      const { context } = tree.contexts[0];
      await bidi.command('browsingContext.navigate', {
        context,
        url: `moz-extension://${uuid}/ui/archive.html`,
        wait: 'complete',
      });
      async function evaluate(expression) {
        const result = await bidi.command('script.evaluate', {
          target: { context },
          awaitPromise: true,
          expression: `(async () => JSON.stringify(await (${expression})))()`,
        });
        assert.equal(result.type, 'success', result.exceptionDetails?.text);
        return result.result.type === 'undefined' ? undefined : JSON.parse(result.result.value);
      }
      async function message(type, args = {}) {
        const reply = await evaluate(
          `browser.runtime.sendMessage(${JSON.stringify({ type, ...args })})`,
        );
        assert.equal(reply?.ok, true, reply?.error);
        return reply.data;
      }
      async function until(read, predicate) {
        const deadline = Date.now() + 15000;
        let value;
        while (Date.now() < deadline) {
          value = await read();
          if (predicate(value)) return value;
          await new Promise((resolve) => setTimeout(resolve, 100));
        }
        assert.fail(`Firefox state did not settle: ${JSON.stringify(value)}`);
      }
      assert.equal((await message('status')).host.connected, false);
      const url = server.url + '/tests/fixtures/integration-forum.html';
      const site = await evaluate(`browser.tabs.create({ url: ${JSON.stringify(url)} })`);
      await message('site.enable', { origin: server.url });
      const state = await until(
        () => message('status'),
        (value) => value.pendingCount > 0,
      );
      assert.equal(state.host.connected, false);
      const queued = await evaluate(
        `import(browser.runtime.getURL('lib/outbox.js')).then(q => q.peek())`,
      );
      assert.equal(queued[0].capture.url, url);
      assert.ok(queued[0].capture.blocks.length > 0);
      assert.match(JSON.stringify(queued[0].capture.blocks), /Сохраним содержимое форума локально/);
      const [{ result: denied }] = await evaluate(`browser.scripting.executeScript({
      target: { tabId: ${site.id} },
      func: () => browser.runtime.sendMessage({ type: 'native', request: { type: 'stats' } })
    })`);
      assert.equal(denied.ok, false);
      assert.match(denied.error, /extension pages/);
      await bidi.command('browsingContext.reload', { context, wait: 'complete' });
      assert.ok((await message('status')).pendingCount > 0);

      await t.test(
        'Firefox Native Messaging commits and reads the queued page',
        {
          skip:
            process.platform !== 'linux'
              ? 'Native registration is isolated using a temporary Linux home'
              : false,
        },
        async () => {
          const binary = path.join(temp, 'bootstrap', 'pagevault-host');
          execFileSync(go, ['build', '-o', binary, './cmd/pagevault'], {
            cwd: root,
            stdio: 'pipe',
          });
          const plan = JSON.parse(
            execFileSync(
              binary,
              [
                'install',
                '--browser',
                'firefox',
                '--home',
                home,
                '--db',
                path.join(home, 'archive.sqlite3'),
              ],
              { encoding: 'utf8' },
            ),
          );
          const native = JSON.parse(fs.readFileSync(plan.manifest_path, 'utf8'));
          native.name = hostName;
          fs.writeFileSync(
            path.join(path.dirname(plan.manifest_path), hostName + '.json'),
            JSON.stringify(native),
          );
          await message('queue.flush');
          await until(
            () => message('status'),
            (value) => value.host.connected && value.pendingCount === 0,
          );
          const pages = await message('native', { request: { type: 'pages.list' } });
          assert.equal(pages.items[0].url, url);
          await message('site.disable', { origin: server.url });
        },
      );
    } finally {
      bidi?.close();
      if (processFirefox && processFirefox.exitCode === null) {
        const exited = once(processFirefox, 'exit');
        processFirefox.kill('SIGKILL');
        await exited;
      }
      await server.close();
      fs.rmSync(temp, { recursive: true, force: true });
    }
  },
);
