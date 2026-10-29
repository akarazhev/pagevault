// Firefox's browser namespace supplies promises. Chromium 116 uses chrome.
export const extensionAPI = globalThis.browser ?? globalThis.chrome;
