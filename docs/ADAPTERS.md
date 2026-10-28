# Site adapters

The generic extractor handles permitted HTTP(S) pages. An adapter adds site-specific JSON metadata without replacing generic extraction. See `extension/adapters/example.js` and `gitlab.js`. Register an adapter with synchronous `matches({document, url})` and `extract({document, url})` functions; the latter returns JSON-compatible data for `metadata.site_data`. Add its local file to `CAPTURE_FILES` in `extension/lib/config.js` after `extractor.js` and before `content.js`, reload the extension and tab, and capture a page.

Multiple matching adapters merge metadata in registration order; the last successful adapter sets `source.adapter`. An adapter error becomes a warning and generic extraction continues. Use loaded DOM, preserve useful source IDs and links, avoid DOM nodes and cycles in results, and add synthetic fixtures for missing and present site elements. Remote adapter code is not loaded.

Example adapter:

```javascript
(() => {
  PageVaultExtractor.registerAdapter({
    id: 'forum',
    matches({ url }) { return new URL(url).hostname === 'forum.example.org'; },
    extract({ document }) {
      return { topic_id: document.querySelector('[data-topic-id]')?.dataset.topicId || '' };
    },
  });
})();
```

Add `adapters/forum.js` between `extractor.js` and `content.js` in `CAPTURE_FILES`; reload and take a manual capture. Inspect `source.adapter`, `metadata.site_data` and warnings in exported JSON. Test absent elements and adapter exceptions as well as the normal case.
