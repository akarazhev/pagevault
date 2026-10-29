# Support and limitations

PageVault supports desktop Chrome, Chromium, Edge and Brave with an unpacked extension. The minimum Chrome version is 116.

| Platform | Distribution | Automated coverage |
| --- | --- | --- |
| Linux amd64/arm64 | Full ZIP and shell installer | Go and browser tests; Native Messaging integration tests on Linux CI |
| Windows amd64/arm64 | Full ZIP and Windows installer | Go and browser tests; PowerShell syntax and installer dry run |
| macOS 13+, Intel/Apple Silicon | Full ZIP or universal PKG with extension ZIP | Go and browser tests; package metadata and scripts |

Binaries are built for all six targets. Runtime tests do not cover every architecture or browser configuration. Interactive Windows installation and the macOS 1.4 GUI upgrade have not been verified. The macOS PKG is unsigned and not notarized. See [verification](VERIFICATION.md) for checks.

Report bugs through [GitHub issues](https://github.com/akarazhev/pagevault/issues) and vulnerabilities through the [security policy](../SECURITY.md). Fixes target the current release; response times are not guaranteed.

## Capture limits

PageVault reads loaded DOM content, including accessible same-origin frames and open shadow roots. Closed shadow roots, cross-origin frames, image text, canvas and PDF viewers may be missing. Media files are not downloaded.

Default limits are 5,000 blocks, 1,000,000 text characters and 2,000,000 HTML characters. Snapshot warnings identify limits reached. Optional HTML is sanitized and never executed by the archive UI.

## Queue recovery

The queue holds up to 500 captures or 128 MiB, including rejected entries. When full, it refuses new captures without deleting accepted data. Restore the helper connection, retry delivery and capture the page again.

Settings lists rejected captures with their title, URL, time and error. Correct the cause before retrying. Export JSON before discarding data you need. Retry and discard apply to the displayed revision, so an old action cannot affect its replacement. Delivery removes an entry only after its matching revision is committed. Deleting an archived page also clears pending captures for that URL.

## Archive limits

Existing databases must match PageVault schema 1. Damaged or unrelated databases are rejected; restore a backup or repair the file before reopening it.

MCP preserves positive signed 64-bit identifiers. Native Messaging and the extension use JavaScript numbers; archive identifiers above `Number.MAX_SAFE_INTEGER` (9007199254740991) have not been verified. See the [protocol reference](NATIVE_PROTOCOL.md).
