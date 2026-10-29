# Architecture

The Chromium and Firefox packages share all JavaScript, UI and assets in `extension/`. The release builder keeps Chromium's manifest and generates a Firefox Manifest V3 manifest with an event-page background module, Gecko ID and data disclosure. Shared modules select `browser` promise APIs when available and otherwise Chromium's `chrome` APIs. The classic content script uses the callback messaging API supported by both browsers.

The extension extracts loaded DOM content into structured blocks. Content scripts observe page changes and SPA navigation; site adapters add metadata. The background validates senders and permissions, queues captures in IndexedDB and delivers them over a persistent Native Messaging connection. A successful SQLite commit acknowledges the matching queue revision. Revision checks prevent an old response from removing a newer capture.

The Go helper opens one SQLite connection per native session. The archive has three main tables:

- `pages`: exact URLs and page metadata.
- `captures`: unique snapshots stored as JSON.
- `blocks`: unique observed blocks in first-seen order.

Transactions, foreign keys and WAL support consistent writes and concurrent readers. Semantic hashes exclude timestamps and HTML. The installer registers a helper for each browser; no daemon runs between sessions.

Each browser has separate settings, site permissions and an IndexedDB outbox. Browser helpers use the same default archive path and can share an archive; a custom database must be selected explicitly for each browser. Firefox Native Messaging uses `allowed_extensions` and its own registration paths. The host ignores Firefox's launch-time manifest path and add-on ID when resolving its database configuration.

The archive UI searches, displays and exports structured data without executing saved HTML. MCP runs as a separate stdio process, opens an existing archive read-only and sees subsequent commits. Its five tools expose archive queries without arbitrary SQL or filesystem access. Large documents use chunks with Unicode offsets and content hashes.

See [data format](SCHEMA.md), [Native Messaging](NATIVE_PROTOCOL.md), [MCP](MCP.md) and [site adapters](ADAPTERS.md).
