/* Disabled example. To enable: replace the hostname and matches return value,
 * then add this file to CAPTURE_FILES in extension/lib/config.js before content.js.
 * Return extra site_data only. Never replace/limit generic blocks, schedule work,
 * access chrome APIs, execute source strings, or send network requests.
 */
(() => {
  'use strict';
  const ENABLED = false;
  globalThis.PageVaultExtractor.registerAdapter({
    id: 'example',
    matches({ url }) {
      return ENABLED && new URL(url).hostname === 'forum.example.org';
    },
    extract({ document, url }) {
      return {
        platform: 'example-forum',
        thread_id: document.querySelector('[data-thread-id]')?.getAttribute('data-thread-id') || '',
        pathname: new URL(url).pathname,
      };
    },
  });
})();
