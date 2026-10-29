# Architecture

The extension extracts loaded DOM content into structured blocks. Content scripts observe page changes and SPA navigation; site adapters add metadata. The background validates senders and permissions, queues captures in IndexedDB and delivers them over a persistent Native Messaging connection. A successful SQLite commit acknowledges the matching queue revision. Revision checks prevent an old response from removing a newer capture.

The Go helper opens one SQLite connection per native session. The archive has three main tables:

- `pages`: exact URLs and page metadata.
- `captures`: unique snapshots stored as JSON.
- `blocks`: unique observed blocks in first-seen order.

Transactions, foreign keys and WAL support consistent writes and concurrent readers. Semantic hashes exclude timestamps and HTML. The installer registers a helper for each browser; no daemon runs between sessions.

The archive UI searches, displays and exports structured data without executing saved HTML. MCP runs as a separate stdio process, opens an existing archive read-only and sees subsequent commits. Its five tools expose archive queries without arbitrary SQL or filesystem access. Large documents use chunks with Unicode offsets and content hashes.

See [data format](SCHEMA.md), [Native Messaging](NATIVE_PROTOCOL.md), [MCP](MCP.md) and [site adapters](ADAPTERS.md).
