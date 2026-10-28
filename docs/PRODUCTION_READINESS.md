# Readiness and support

| Platform | Distribution | Evidence and limits |
| --- | --- | --- |
| Linux amd64/arm64 | Full ZIP, per-browser installer | Linux hosted Chromium runs native end-to-end tests; cross-builds alone do not prove both architectures at runtime. |
| Windows amd64/arm64 | Full ZIP, per-browser installer | Windows hosted portable tests, PowerShell parse and synthetic dry run; no full interactive installer/browser certification. |
| macOS 13+ Intel/Apple Silicon | Full ZIP or universal unsigned PKG plus extension ZIP | macOS hosted portable tests and local arm64 synthetic PKG scripts; no interactive 1.4 GUI upgrade certification. PKG is unsigned and not notarized. |

Desktop Chrome, Chromium, Edge and Brave use an unpacked extension (minimum Chrome 116). Real browser and OS configurations vary. Hosted evidence for the earlier hardening commit is in [verification](VERIFICATION.md); release publication requires passing checks on the final commit. Report bugs through GitHub issues and vulnerabilities through [private reporting](https://github.com/akarazhev/pagevault/security/advisories/new). Support targets the current release without a promised response time.

MCP preserves positive signed 64-bit archive identifiers exactly. Native Messaging and the extension UI use JavaScript numbers for archive page and capture identifiers; values above `Number.MAX_SAFE_INTEGER` (9007199254740991) have not been verified there.

## Queue recovery

Settings lists rejected captures by title, bounded URL and queued time, with the error. Export JSON explicitly before discarding if you need the data. Retry after correcting the cause; discard only when the data is no longer needed. Actions apply to the displayed revision, so stale actions cannot delete or retry a replacement. Accepted queue entries remain until committed or explicitly discarded. When the queue is full, new submissions are refused: restore the helper, retry delivery and capture again. See [upgrading](UPGRADING.md) for backups and manual upgrades.

## Design decisions and costs

- Domain pagination preserves exact hostnames and scans as needed for accurate counts; byte limits may shorten pages, so follow the actual `next_offset` rather than assuming the requested limit. Correct totals can cost more work on large archives.
- Queue totals include rejected data; revision-safe retry, discard and acknowledgement prevent stale operations from losing replacements. Export is an explicit user action.
- Existing databases must match recognized schema 1. Malformed or partial files are refused and require repair or restoration from backup; they are not silently migrated or overwritten. SQLite files at installer-owned payload paths are refused rather than replaced.
- Artifact and checksum publication rolls back ordinary paired write failures, but is not crash-atomic across both files. Verify both checksums after interruptions.
- The canonical Go module is `github.com/akarazhev/pagevault`; version 1.4 retains schema 1, the fixed extension ID, and the existing archive and configuration paths through manual upgrade.
