# Native Messaging protocol

The host `com.pagevault.host` exchanges UTF-8 JSON frames prefixed by a four-byte native-endian length. stdout carries frames; diagnostics go to stderr. Requests contain `id` and `type`. Responses echo `id` and contain `ok`, with either `data` or an `error` containing `code` and `message`.

Requests are limited to 32 MiB; responses must be smaller than 1 MiB. The extension uses one persistent `connectNative` port and matches responses by ID. Reads time out after 20 seconds without closing the port. Mutations wait for acknowledgement or disconnection; a hung mutation may wait until the process ends. The next request reconnects after a disconnect.

## Commands

| Type | Parameters | Result |
| --- | --- | --- |
| `ping` | none | Versions, schema, database path, host ID, process ID and connection count |
| `capture.save` | `capture` | Page ID, capture ID, creation status and block count after commit |
| `pages.list` | `query`, `domain`, `limit`, `offset` | Page summaries and total |
| `pages.get` | `page_id`, `limit`, `offset` | Page summary and capture summaries |
| `captures.get` | `capture_id`, `offset`, `chunk_size` | Snapshot JSON chunk |
| `pages.export` | `page_id`, `offset`, `chunk_size` | Accumulated archive JSON chunk |
| `pages.domains` | `limit`, `offset` | Hostnames, page counts, total and `next_offset` |
| `stats` | none | Page, capture and block counts, database size and path |
| `pages.delete` | `page_id` | Cascading deletion confirmation |

The UI requires confirmation before deletion. Capture delivery removes a queued revision only after its commit; see [architecture](ARCHITECTURE.md) and [queue recovery](PRODUCTION_READINESS.md#queue-recovery).

## Pagination and chunks

For `pages.domains`, `limit` defaults to 200 and cannot exceed 200; `offset` defaults to zero. Response size limits may shorten a page. Follow the returned `next_offset`; `null` marks the end. Hostnames are returned in full so they remain valid filters. Totals may change while the archive is being updated.

Large snapshots and exports use `json_chunk`, `offset`, `next_offset`, `total_chars`, `done` and `document_hash`. Offsets count Unicode code points, not JavaScript UTF-16 code units. Join all chunks before parsing JSON. Restart if the hash changes.

Summary fields may be shortened and listed in `summary_truncated`. A long URL has an empty `url` and a `url_display` preview. Use a document export for complete values.

Native Messaging and the extension use JavaScript numbers for archive identifiers. Values above `Number.MAX_SAFE_INTEGER` (9007199254740991) have not been verified. [MCP](MCP.md) preserves positive signed 64-bit identifiers.
