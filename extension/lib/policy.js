export function pageOrigin(url) {
  try { const parsed = new URL(url); return ['http:', 'https:'].includes(parsed.protocol) && !parsed.username && !parsed.password ? parsed.origin : null; }
  catch { return null; }
}
export function permissionPattern(origin) {
  if (!pageOrigin(origin)) throw new Error('Поддерживаются обычные страницы HTTP и HTTPS.');
  const parsed = new URL(origin);
  return `${parsed.protocol}//${parsed.hostname}/*`;
}
export function isExtensionPage(sender, extensionOrigin) {
  return typeof sender?.url === 'string' && sender.url.startsWith(extensionOrigin);
}
export function validateCaptureSource(capture, sender, { enabledOrigins = [], oneShots = {}, now = Date.now() }) {
  if (!Number.isInteger(sender?.tab?.id) || (sender.frameId ?? 0) !== 0) throw new Error('Снимок должен поступить из основной страницы вкладки.');
  const origin = pageOrigin(sender.url);
  if (!origin || origin !== pageOrigin(capture?.url)) throw new Error('Адрес снимка не соответствует источнику сообщения.');
  const once = oneShots[sender.tab.id];
  if (!enabledOrigins.includes(origin) && !(once?.origin === origin && once.expires > now)) throw new Error('Сохранение для этого сайта выключено.');
  if (capture.schema_version !== 1 || !Array.isArray(capture.blocks)) throw new Error('Неподдерживаемый формат снимка.');
  return origin;
}
