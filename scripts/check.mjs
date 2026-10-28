import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { createHash } from 'node:crypto';
import { spawnSync } from 'node:child_process';
import { CAPTURE_FILES } from '../extension/lib/config.js';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const manifest = JSON.parse(fs.readFileSync(path.join(root, 'extension/manifest.json'), 'utf8'));
const required = [manifest.background.service_worker, manifest.action.default_popup, manifest.options_page, ...CAPTURE_FILES, ...Object.values(manifest.icons)];
for (const name of required) {
  const file = path.resolve(root, 'extension', name);
  if (!file.startsWith(path.join(root, 'extension') + path.sep) || !fs.existsSync(file)) throw new Error(`Missing or invalid extension asset: ${name}`);
}
const digest = createHash('sha256').update(Buffer.from(manifest.key, 'base64')).digest('hex').slice(0, 32);
const id = [...digest].map(c => String.fromCharCode(97 + parseInt(c, 16))).join('');
if (id !== fs.readFileSync(path.join(root, 'docs/extension-id.txt'), 'utf8').trim()) throw new Error('Native extension ID does not match public key');

function walk(dir) {
  return fs.readdirSync(dir, { withFileTypes: true }).flatMap(item => item.isDirectory() ? walk(path.join(dir, item.name)) : [path.join(dir, item.name)]);
}
const scripts = walk(path.join(root, 'extension')).filter(file => file.endsWith('.js'));
for (const file of scripts) {
  const checked = spawnSync(process.execPath, ['--check', file], { encoding: 'utf8' });
  if (checked.status !== 0) throw new Error(checked.stderr || `Cannot parse ${file}`);
}
for (const file of walk(path.join(root, 'extension/ui')).filter(file => file.endsWith('.html'))) {
  const html = fs.readFileSync(file, 'utf8');
  if (/<script(?![^>]*\bsrc\s*=)[^>]*>/i.test(html) || /\son[a-z]+\s*=/i.test(html)) throw new Error(`Inline executable HTML is incompatible with CSP: ${file}`);
  for (const [, source] of html.matchAll(/<script[^>]*\bsrc="([^"]+)"/g)) if (!fs.existsSync(path.resolve(path.dirname(file), source))) throw new Error(`Missing script ${source}`);
}
for (const [size, file] of Object.entries(manifest.icons)) {
  const bytes = fs.readFileSync(path.join(root, 'extension', file));
  if (bytes.readUInt32BE(16) !== Number(size) || bytes.readUInt32BE(20) !== Number(size)) throw new Error(`Incorrect icon dimensions: ${file}`);
}
console.log(`Checked ${scripts.length} extension scripts, UI CSP, assets, PNG dimensions and native extension ID ${id}.`);
