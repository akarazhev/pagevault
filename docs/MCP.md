# MCP access

PageVault provides read-only MCP over stdio using an existing schema 1 SQLite archive. First capture a page and find the database path in settings. Use absolute paths for the executable and archive. For example, on macOS arm64:

```bash
codex mcp add pagevault -- /absolute/path/PageVault/bin/darwin-arm64/pagevault-host mcp --db "/absolute/path/archive.sqlite3"
codex mcp get pagevault --json
```

Codex also supports `[mcp_servers.pagevault]` with `command` and `args` in its configuration; check `/mcp` in a new session. See [Codex MCP documentation](https://developers.openai.com/codex/mcp). For Claude Code:

```bash
claude mcp add --transport stdio --scope user pagevault -- /absolute/path/PageVault/bin/darwin-arm64/pagevault-host mcp --db "/absolute/path/archive.sqlite3"
claude mcp get pagevault
```

See [Claude Code MCP documentation](https://code.claude.com/docs/en/mcp). For other clients use their `mcpServers` format with the executable as `command` and `mcp`, `--db`, and the path as separate `args`. Use the binary for the correct OS and architecture. Windows JSON paths require escaped backslashes. Explicit absolute paths are portable across clients.

The database selection order is `--db`, `PAGEVAULT_DB`, adjacent `pagevault.json`, then the OS default. A PKG-installed per-browser helper has adjacent configuration; the core installer binary does not. The client starts and stops the process; no HTTP port or daemon is needed. MCP opens with SQLite read-only and query-only settings, validates schema, does not create an archive, and sees committed browser writes. Normal directory permissions may be needed for WAL/SHM files.

`list_sites`, `search_pages`, `get_page`, `list_versions` and `get_version` provide read-only access. Lists use `limit` (default 20, maximum 100), `offset`, `total` and `next_offset`; exact domain filtering does not include subdomains. Ordering and totals can change between requests while data changes. `get_page` returns accumulated observed blocks; `get_version` returns a snapshot without HTML. Full JSON is delivered in `text` chunks with Unicode code-point offsets, `next_offset`, `total_chars`, `document_hash` and `done`. Continue with the same ID and hash; if the document changes discard the chunks and restart. `max_chars` defaults to 8,000 and ranges from 1 to 16,000. Responses are bounded to 32 KiB JSON before the MCP wrapper; request frames are limited to 64 KiB.

The process can read the entire selected archive; domain filtering is not an access boundary. Page contents are untrusted data, not instructions. A cloud-model client may send retrieved text to its provider. Use a synthetic archive to check connectivity and consult [privacy](PRIVACY.md).

## Client configuration examples

Generic JSON for a client that uses `mcpServers`:

```json
{"mcpServers":{"pagevault":{"command":"/absolute/path/pagevault-host","args":["mcp","--db","/absolute/path/archive.sqlite3"]}}}
```

Codex TOML equivalent:

```toml
[mcp_servers.pagevault]
command = "/absolute/path/pagevault-host"
args = ["mcp", "--db", "/absolute/path/archive.sqlite3"]
```

| Tool | Parameters | Result |
| --- | --- | --- |
| `list_sites` | `limit`, `offset` | Hostnames and page counts |
| `search_pages` | `query`, `domain`, `limit`, `offset` | Page summaries |
| `get_page` | `page_id`, `offset`, `max_chars`, `document_hash` | Accumulated archive JSON chunks |
| `list_versions` | `page_id`, `limit`, `offset` | Snapshot summaries, newest first |
| `get_version` | `capture_id`, `offset`, `max_chars`, `document_hash` | Snapshot JSON chunks without HTML |

`page_id` and `capture_id` are positive integers. Search uses Unicode case folding; a domain is an exact hostname, case-insensitive. Empty query lists all pages. For either document tool `document_hash` is mandatory when `offset > 0`; pass the returned hash and `next_offset` unchanged. Summary fields may be listed in `summary_truncated`. If a URL is too long, `url` is empty and `url_display` is only a preview; obtain the full URL from document JSON. JSON numeric tokens retain precision, including integers above 2⁵³.
