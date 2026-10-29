# Data format

Schema 1 captures contain a URL, title, language, RFC3339 timestamp, capture reason, session ID, source, metadata, ordered blocks, warnings and statistics. Optional `html` contains sanitized markup. Each warning has a `code` and `message`.

```json
{
  "schema_version": 1,
  "url": "https://example.org/thread/42?page=2#reply",
  "title": "Project discussion",
  "lang": "en",
  "captured_at": "2026-10-28T12:00:00.000Z",
  "reason": "mutation",
  "session_id": "local-document-id",
  "source": {"kind": "dom", "adapter": "generic"},
  "metadata": {"description": "", "canonical_url": "", "site_data": {}},
  "blocks": [{"type": "heading", "level": 1, "text": "Discussion"}],
  "warnings": [],
  "stats": {"block_count": 1, "text_chars": 10}
}
```

## Blocks

| Type | Fields |
| --- | --- |
| Heading | `level`, `text` |
| Paragraph or text | `text`, optional `links` |
| Code | `text`, optional `language` |
| List | `ordered`, `items`, optional nested `blocks` |
| Table | `rows`, optional `cell_blocks` |
| Link | `url`, `text` |
| Image | `src`, `alt` |
| Media | `kind`, `src`, optional `poster` |

Relative links resolve to absolute HTTP(S) URLs. Unknown block types display as JSON.

## Identity and versions

A page is identified by its exact URL, including query and fragment. `metadata.canonical_url` does not replace it. Snapshot hashes cover schema, URL, title, language, source, metadata and blocks. Timestamps and HTML do not create new versions; repeated content updates the page's last-seen time.

An accumulated export contains `schema_version`, `archive_kind: "observed_blocks"`, `page` and unique `blocks` in first-seen order. Blocks may come from several snapshots. Use a snapshot to retrieve one recorded state.

List summaries may shorten fields and report `summary_truncated`. Long URLs use `url_display` with an empty `url`. Chunked document exports retain the full values.
