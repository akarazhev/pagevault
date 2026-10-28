export const HOST_NAME = 'com.pagevault.host';
export const CAPTURE_FILES = ['extractor.js', 'adapters/gitlab.js', 'content.js'];
export const DEFAULT_SETTINGS = Object.freeze({
  saveHtml: true,
  captureIntervalSeconds: 30,
  debounceMs: 1200,
  maxBlocks: 5000,
  maxTextChars: 1000000,
  maxHtmlChars: 2000000,
});
const BOUNDS = {
  captureIntervalSeconds: [10, 300],
  debounceMs: [500, 10000],
  maxBlocks: [100, 20000],
  maxTextChars: [1000, 2000000],
  maxHtmlChars: [1000, 4000000],
};
export function normalizeSettings(input = {}) {
  const result = { ...DEFAULT_SETTINGS };
  if (typeof input.saveHtml === 'boolean') result.saveHtml = input.saveHtml;
  for (const [key, [min, max]] of Object.entries(BOUNDS)) {
    if (typeof input[key] === 'number' && Number.isFinite(input[key]))
      result[key] = Math.max(min, Math.min(max, Math.round(input[key])));
  }
  return result;
}
