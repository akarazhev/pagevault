# MCP access

PageVault exposes an existing schema 1 archive through read-only MCP over stdio. The client starts the helper process; no HTTP port or daemon is needed. Capture a page first and copy the database path from settings.

## Configuration

Use absolute paths to the helper for your OS and architecture and to the archive. For clients using `mcpServers`:

```json
{
  "mcpServers": {
    "pagevault": {
      "command": "/absolute/path/pagevault-host",
      "args": ["mcp", "--db", "/absolute/path/archive.sqlite3"]
    }
  }
}
```

For Codex:

```toml
[mcp_servers.pagevault]
command = "/absolute/path/pagevault-host"
args = ["mcp", "--db", "/absolute/path/archive.sqlite3"]
```

See the client documentation for [Codex](https://developers.openai.com/codex/mcp) or [Claude Code](https://code.claude.com/docs/en/mcp). Windows paths in JSON require escaped backslashes.

Database selection follows this order: `--db`, `PAGEVAULT_DB`, `pagevault.json` beside the executable, then the OS default. A PKG-installed per-browser helper has this configuration file; the shared installer binary does not.

MCP validates the archive, opens it read-only and sees subsequent committed writes. It does not create or initialize databases. SQLite may need directory permissions for WAL/SHM files.

## Tools

| Tool | Parameters | Result |
| --- | --- | --- |
| `list_sites` | `limit`, `offset` | Hostnames and page counts |
| `search_pages` | `query`, `domain`, `limit`, `offset` | Page summaries |
| `get_page` | `page_id`, `offset`, `max_chars`, `document_hash` | Accumulated archive JSON chunks |
| `list_versions` | `page_id`, `limit`, `offset` | Snapshot summaries, newest first |
| `get_version` | `capture_id`, `offset`, `max_chars`, `document_hash` | Snapshot JSON chunks without HTML |

IDs are positive signed 64-bit integers and retain full precision. Search uses Unicode case folding. A domain matches an exact hostname, case-insensitively, without including subdomains. An empty query lists all pages.

Lists default to 20 items, with a maximum of 100, and return `total` and `next_offset`. Summaries may shorten fields and report `summary_truncated`. A long URL has an empty `url` and a `url_display` preview. Read a document for full values. Concurrent updates can change totals and page positions.

Documents arrive as `text` chunks with Unicode code-point offsets, `next_offset`, `total_chars`, `document_hash` and `done`. Continue with the same ID and the returned offset and hash. The hash is required when `offset > 0`. If the document changes, discard earlier chunks and restart. Parse JSON only after joining all chunks. `get_page` combines blocks collected over time; `get_version` returns one snapshot without HTML.

`max_chars` defaults to 8,000 and accepts 1 to 16,000. Each tool result is limited to 32 KiB before MCP wrapping, so chunks and lists may be shorter than requested. Input frames are limited to 64 KiB.

## Access and privacy

The client can read the entire selected archive; a domain filter does not restrict access. Treat page contents as untrusted data. A cloud model may receive retrieved content from its client. Use a synthetic archive for connection tests. See [privacy](PRIVACY.md).
