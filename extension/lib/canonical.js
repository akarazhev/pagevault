export function stableStringify(value) {
  if (value === null || typeof value !== 'object') return JSON.stringify(value);
  if (Array.isArray(value))
    return '[' + value.map((item) => stableStringify(item) ?? 'null').join(',') + ']';
  return (
    '{' +
    Object.keys(value)
      .sort()
      .filter((key) => value[key] !== undefined)
      .map((key) => JSON.stringify(key) + ':' + stableStringify(value[key]))
      .join(',') +
    '}'
  );
}
export async function captureDigest(capture) {
  const value = {};
  for (const key of ['schema_version', 'url', 'title', 'lang', 'source', 'metadata', 'blocks'])
    if (capture[key] !== undefined) value[key] = capture[key];
  const digest = await crypto.subtle.digest(
    'SHA-256',
    new TextEncoder().encode(stableStringify(value)),
  );
  return Array.from(new Uint8Array(digest), (byte) => byte.toString(16).padStart(2, '0')).join('');
}
