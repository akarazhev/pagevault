# Native Messaging protocol

The host `com.pagevault.host` exchanges UTF-8 JSON frames prefixed by a four-byte native-endian length. stdout contains frames only; diagnostics go to stderr. Requests contain `id` and `type`; responses contain the same `id`, `ok`, and either `data` or an `error` with `code` and `message`.

Commands include `ping`, `capture.save`, `pages.list`, `pages.get`, `captures.get`, `pages.export`, `pages.domains`, `stats`, and `pages.delete`. Delete requires explicit confirmation in the UI. Requests are limited to 32 MiB and responses to less than 1 MiB. Large capture and export JSON responses use `json_chunk`, `offset`, `next_offset`, `total_chars`, `done`, and `document_hash`; offsets count Unicode code points. Join chunks and parse only after completion; restart if the hash changes. Do not use JavaScript UTF-16 string length to compute offsets.

The extension uses a persistent `connectNative` port and matches responses by ID. Read requests time out after 20 seconds without closing the shared port; mutations await acknowledgement or a real disconnect. The next request reconnects after disconnection. A hung mutation may wait until the process ends. Accepted captures remain in IndexedDB until their matching revision is committed. See [architecture](ARCHITECTURE.md).

## Commands and pagination

| Type | Parameters | Result |
| --- | --- | --- |
| `ping` | none | Versions, schema, database path, host ID, process ID and connection count |
| `capture.save` | `capture` | Page ID, capture ID, creation status and block count after commit |
| `pages.list` | `query`, `domain`, `limit`, `offset` | Page summaries and total |
| `pages.get` | `page_id`, `limit`, `offset` | Page summary and capture summaries |
| `captures.get` | `capture_id`, `offset`, `chunk_size` | Original snapshot JSON chunk |
| `pages.export` | `page_id`, `offset`, `chunk_size` | Observed-block archive JSON chunk |
| `pages.domains` | `limit`, `offset` | Exact hostnames, page counts, total and `next_offset` |
| `stats` | none | Page, capture and block counts, database size and path |
| `pages.delete` | `page_id` | Cascading deletion confirmation |

For `pages.domains`, limit defaults to 200 and is capped at 200; offset defaults to zero. The byte limit can return fewer items than requested. Use the returned `next_offset` rather than adding the limit; null means the end. Hostnames are preserved exactly rather than truncated into misleading filters. List totals may change as the archive changes. Summary fields can be shortened with `summary_truncated`; a long URL is represented by `url_display` and an empty `url`, not a truncated clickable URL. Full values are available via chunked exports.
