export function pageOrigin(url) {
  try { const parsed = new URL(url); return ['http:', 'https:'].includes(parsed.protocol) && !parsed.username && !parsed.password ? parsed.origin : null; }
  catch { return null; }
}
export function permissionPattern(origin) {
  if (!pageOrigin(origin)) throw new Error('Only HTTP and HTTPS pages are supported.');
  const parsed = new URL(origin);
  return `${parsed.protocol}//${parsed.hostname}/*`;
}
export function isExtensionPage(sender, extensionOrigin) {
  return typeof sender?.url === 'string' && sender.url.startsWith(extensionOrigin);
}
export function validateCaptureSource(capture, sender, { enabledOrigins = [], oneShots = {}, now = Date.now() }) {
  if (!Number.isInteger(sender?.tab?.id) || (sender.frameId ?? 0) !== 0) throw new Error('The capture must come from the main frame of the tab.');
  if (typeof capture?.url !== 'string' || Array.from(capture.url).length > 65536) throw new Error('Capture URL exceeds the 65,536 character limit.');
  const origin = pageOrigin(sender.url);
  if (!origin || origin !== pageOrigin(capture?.url)) throw new Error('The capture URL does not match the message source.');
  const once = oneShots[sender.tab.id];
  if (!enabledOrigins.includes(origin) && !(once?.origin === origin && once.expires > now)) throw new Error('Recording is disabled for this site.');
  if (capture.schema_version !== 1 || !Array.isArray(capture.blocks)) throw new Error('Unsupported capture format.');
  return origin;
}
