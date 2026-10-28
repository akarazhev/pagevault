# Data format

A schema 1 capture contains `url`, `title`, `lang`, RFC3339 `captured_at`, `reason`, `session_id`, `source`, `metadata`, ordered `blocks`, `warnings` and `stats`. Optional sanitized `html` is included only when enabled. Warnings contain `code` and `message`.

Blocks include headings (`level`, `text`), paragraphs and text (`text`, optional `links`), code (`text`, optional `language`), lists (`ordered`, `items`, possibly nested `blocks`), tables (`rows`, optional `cell_blocks`), links (`url`, `text`), images (`src`, `alt`) and media (`kind`, `src`, optional `poster`). Unknown types display as JSON. Relative links are resolved to absolute HTTP(S) URLs.

Page identity is the exact URL, including query and fragment. `metadata.canonical_url` does not replace it. Semantic capture hashes cover schema, URL, title, language, source, metadata and blocks; timestamps and HTML do not create new versions. Repeated content updates last-seen time. Accumulated export uses `archive_kind: "observed_blocks"` and includes unique blocks from multiple moments in first-seen order; use a snapshot for a single state. Limited summaries can report `summary_truncated`; long URLs may appear as `url_display` with an empty `url`. Obtain complete values through chunked export.

Example capture:

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
  "blocks": [{"type": "heading", "level": 1, "text": "Discussion"}, {"type": "paragraph", "text": "Example"}],
  "warnings": [],
  "stats": {"block_count": 2, "text_chars": 17}
}
```

An accumulated export contains `schema_version`, `archive_kind`, `page` and `blocks`. Its order represents first observation, not a single point in time. Snapshot JSON preserves one state and can optionally contain sanitized HTML.
