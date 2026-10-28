# Local MCP and distribution

The user approved a local MCP interface for an agent to read the PageVault archive, requested documenting the distribution discussion, and asked to implement MCP first.

## Selected approach

Add `pagevault-host mcp --db PATH` to the existing Go executable using the official Go MCP SDK v1.8.0. The MCP client launches a separate process over stdio; Chrome remains responsible for collection through the existing Native Messaging mode. A single executable avoids a second platform-specific component. Remote HTTP hosting is outside this iteration.

Open an existing schema-1 SQLite archive with `mode=ro` and `query_only`. Do not create databases, initialize tables, or change schema/journal mode. Reuse archive queries and support reading committed changes from a concurrent browser writer.

## Tools

- `search_pages`: optional query/domain and bounded limit/offset; page summaries and continuation offset.
- `get_page`: accumulated `observed_blocks` JSON for one page; its blocks can originate from different snapshots. Require the previous document hash for continuation so changes cannot mix.
- `list_versions`: bounded snapshot summaries for one page.
- `get_version`: immutable capture JSON with the optional stored HTML omitted; preserve Unicode, numeric tokens, structured blocks, metadata and warnings.
- `list_sites`: bounded domain/count discovery.

Document tools return JSON chunks with Unicode-code-point offsets, next offset, total length, SHA-256 and done. Default chunk size 8,000, maximum 16,000 code points; actual chunks and list payloads are bounded to 32 KiB before MCP encoding. Lists default to 20 items, maximum 100. Every tool is read-only and closed-world; no arbitrary SQL, writes, filesystem paths or website navigation are exposed as tool parameters. Archived website content is untrusted data.

Implementation clarification: both document tools require the preceding document hash for continuation, because SQLite may reuse a deleted capture ID even though the stored capture itself is immutable.

## CLI and compatibility

Resolve database selection in the existing order: `--db`, PAGEVAULT_DB, adjacent pagevault.json, OS default. MCP stdout contains only protocol messages; errors go to stderr. Preserve Native Messaging framing and the SQLite schema. Release version 1.2.0 identifies the new capability; preserve the extension ID.

## Documentation and distribution

Document actual MCP usage and client launch parameters in docs/MCP.md. Document open-source MIT distribution, third-party notices, Chrome Web Store packaging/ID/privacy requirements, separate native installer onboarding, and separate extension/native updates in docs/DISTRIBUTION.md. CWS publication, GUI installers and updater remain future work. Update runtime third-party notices for the exact linked dependencies and ship both documents in releases.

## Verification

Prove read-only enforcement, no missing-database creation, schema rejection, concurrent writer visibility, paginated lists, bounded Unicode documents, immutable versions, changing-archive detection, omitted HTML and invalid/unknown requests. Use real SDK clients in memory and over a compiled stdio subprocess, run existing Go race/browser checks, and build all six targets.
