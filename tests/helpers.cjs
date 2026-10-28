const path = require('node:path');
const fs = require('node:fs');
const http = require('node:http');
function playwright() {
  try {
    return require('playwright');
  } catch (error) {
    if (process.env.CODEX_PRIMARY_RUNTIME_NODE_MODULES)
      return require(path.join(process.env.CODEX_PRIMARY_RUNTIME_NODE_MODULES, 'playwright'));
    throw error;
  }
}
function browserOptions() {
  const result = { headless: true, args: ['--no-sandbox', '--disable-dev-shm-usage'] };
  if (process.env.PAGEVAULT_CHROMIUM_PATH)
    result.executablePath = process.env.PAGEVAULT_CHROMIUM_PATH;
  return result;
}
async function serveProject() {
  const root = path.resolve(__dirname, '..');
  const types = {
    '.html': 'text/html; charset=utf-8',
    '.js': 'text/javascript; charset=utf-8',
    '.css': 'text/css; charset=utf-8',
    '.json': 'application/json',
    '.png': 'image/png',
  };
  const server = http.createServer((req, res) => {
    const route = new URL(req.url, 'http://localhost').pathname;
    if (route === '/blank') {
      res.writeHead(200, { 'Content-Type': 'text/html' });
      res.end('<!doctype html><title>Test sandbox</title>');
      return;
    }
    const file = path.resolve(root, '.' + decodeURIComponent(route));
    if (!file.startsWith(root + path.sep) || !fs.existsSync(file) || !fs.statSync(file).isFile()) {
      res.writeHead(404);
      res.end('Not found');
      return;
    }
    res.writeHead(200, {
      'Content-Type': types[path.extname(file)] || 'application/octet-stream',
      'Cache-Control': 'no-store',
    });
    fs.createReadStream(file).pipe(res);
  });
  await new Promise((resolve, reject) => {
    server.once('error', reject);
    server.listen(0, '127.0.0.1', resolve);
  });
  return {
    url: `http://127.0.0.1:${server.address().port}`,
    close: () =>
      new Promise((resolve) => {
        server.closeAllConnections();
        server.close(resolve);
      }),
  };
}
module.exports = { playwright, browserOptions, serveProject };
