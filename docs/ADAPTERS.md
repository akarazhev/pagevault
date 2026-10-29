# Site adapters

Adapters add site metadata to generic DOM extraction. Implement synchronous `matches({document, url})` and `extract({document, url})` methods. The latter returns JSON-compatible data for `metadata.site_data`.

```javascript
(() => {
  PageVaultExtractor.registerAdapter({
    id: 'forum',
    matches({ url }) {
      return new URL(url).hostname === 'forum.example.org';
    },
    extract({ document }) {
      return { topic_id: document.querySelector('[data-topic-id]')?.dataset.topicId || '' };
    },
  });
})();
```

Save the file under `extension/adapters/` and add it to `CAPTURE_FILES` in `extension/lib/config.js`, after `extractor.js` and before `content.js`. Reload the extension and page, then capture manually. Check `source.adapter`, `metadata.site_data` and `warnings` in the export. Existing examples are [example.js](../extension/adapters/example.js) and [gitlab.js](../extension/adapters/gitlab.js).

Matching adapters merge metadata in registration order. The last successful adapter sets `source.adapter`. Errors become warnings and generic extraction continues.

Read the loaded DOM and return useful source IDs and links. Do not return DOM nodes or cyclic objects. Test present and absent elements and adapter exceptions with synthetic fixtures. Adapters are bundled locally; remote code is not loaded.
