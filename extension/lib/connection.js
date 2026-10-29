export const DOWNLOADS_URL = 'https://github.com/akarazhev/pagevault/releases';

function versionParts(value) {
  if (typeof value !== 'string' || !/^\d+\.\d+\.\d+(?:\.\d+)?$/.test(value)) return null;
  const parts = value.split('.').map(Number);
  return parts.every(Number.isSafeInteger) ? parts : null;
}
function isOlder(current, expected) {
  const a = versionParts(current),
    b = versionParts(expected);
  if (!a || !b) return false;
  for (let i = 0; i < Math.max(a.length, b.length); i++) {
    if ((a[i] || 0) !== (b[i] || 0)) return (a[i] || 0) < (b[i] || 0);
  }
  return false;
}
export function connectionState(host, extensionVersion) {
  const version = typeof host?.version === 'string' ? host.version : 'Unknown';
  const error = typeof host?.error === 'string' ? host.error : '';
  const dbPath = typeof host?.db_path === 'string' ? host.db_path : '';
  const base = { version, dbPath, error, usable: false };
  if (!host?.connected)
    return {
      ...base,
      dbPath: '',
      kind: 'unavailable',
      label: 'Local helper unavailable',
      detail:
        'Install or update the helper for your browser, then check the connection. If it is already installed, see the error below.',
    };
  if (host.schema_version != null && host.schema_version !== 1)
    return {
      ...base,
      kind: 'incompatible',
      label: 'Incompatible helper',
      detail:
        'The helper uses an unsupported archive schema. Install a compatible version before using this database.',
    };
  const update = isOlder(host.version, extensionVersion);
  return {
    ...base,
    usable: true,
    kind: update ? 'update' : 'ready',
    label: `SQLite connected · helper ${version}`,
    detail: update
      ? 'Connection established. The helper is an older version; reconnect the browser and MCP client after updating.'
      : 'Helper connected. The browser starts it automatically; no separate server is required.',
  };
}
export function normalizePlatform(value) {
  return ['mac', 'win', 'linux'].includes(value) ? value : 'unknown';
}
export function platformInstructions(platform, browser = 'chromium') {
  const firefox = browser === 'firefox';
  switch (normalizePlatform(platform)) {
    case 'mac':
      return {
        detail:
          'macOS 13 or later: open GitHub Releases and choose the -unsigned.pkg package for Intel and Apple Silicon.',
        install: `Open the .pkg and select ${firefox ? 'Firefox' : 'your browsers'}. The package is not signed by Apple; macOS may require permission in Privacy & Security. Installation preserves the existing archive path.`,
        command: '',
      };
    case 'win':
      return {
        detail:
          'Windows: download and extract the complete PageVault bundle with prebuilt programs.',
        install:
          'Run the command below from the extracted bundle. Choose the browser where PageVault is installed.',
        command: '.\\install-windows.cmd' + (firefox ? ' firefox' : ''),
      };
    case 'linux':
      return {
        detail: 'Linux: download and extract the complete PageVault bundle with prebuilt programs.',
        install:
          'Run the command in a terminal from the bundle directory. For Edge, Brave or Chromium, replace chrome with your browser.',
        command: 'bash install.sh ' + (firefox ? 'firefox' : 'chrome'),
      };
    default:
      return {
        detail:
          'Select your operating system. Published PageVault releases are available on the downloads page.',
        install:
          'Installation instructions for the local helper will appear here after you select a system.',
        command: '',
      };
  }
}
