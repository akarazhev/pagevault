# PageVault

PageVault saves loaded web page content as structured snapshots in a local SQLite archive. Enable recording for a site and browse normally; changes produce snapshots and the archive retains unique observed blocks. A generic extractor handles common content, and a GitLab adapter adds project metadata. Search, view versions, export JSON, or read the archive through a local MCP client.

PageVault is free and distributed through [GitHub Releases](https://github.com/akarazhev/pagevault/releases). The browser extension and native helper run locally; there is no separate server or telemetry. Saved content can contain private data. Read the [privacy policy](docs/PRIVACY.md) before enabling a site.

## Install and use

Download `PageVault-1.4.0.zip` from Releases. It includes the extension, installers, six host binaries, source and documentation. The automatically generated GitHub source archives do not contain binaries. On macOS 13 or newer, `PageVault-1.4.0-macOS-unsigned.pkg` installs the helper for Intel and Apple Silicon; use it with `PageVault-Extension-1.4.0.zip` or the full bundle. Verify each download against its `.sha256` file. The PKG is unsigned and not notarized; macOS may block it. See [installation](docs/INSTALLATION.md) for Apple's individual-file exception and restrictions.

1. Extract the full bundle to a permanent location. On macOS/Linux run `./install.sh chrome`; on Windows run `install-windows.cmd` or `./install-windows.ps1 -Browser chrome`. Choose `chromium`, `edge`, or `brave` as appropriate, and repeat for each browser. The installer displays the helper and database paths.
2. Open the browser's extensions page, enable developer mode, and load the unpacked `extension` folder (or the root of the standalone extension ZIP). Keep the folder in place. The fixed extension ID is `ijkioljfdaaplhaldbdpabckkfcieiee`.
3. Open PageVault settings and check the helper connection and SQLite path. Visit an HTTP(S) page, enable recording for its site in the extension popup, and grant the requested site permission. Use Capture now for a manual snapshot. Disable recording at any time; saved data stays in SQLite.

See [installation](docs/INSTALLATION.md), [upgrading and backups](docs/UPGRADING.md), and [queue recovery](docs/PRODUCTION_READINESS.md). Wait for the queue to empty before removing the extension: its IndexedDB queue belongs to the browser.

## What is captured

PageVault extracts loaded DOM content including text, headings, lists, tables, code, links and media references. Open shadow roots and accessible same-origin frames are handled where possible. Scroll or expand content to load it first. Closed shadow roots, cross-origin frames, image text, canvas and PDF viewers may be missing; media files are not downloaded. The archive does not recreate an interactive site offline. Snapshot warnings report limits reached. Defaults include 5,000 blocks, 1,000,000 text characters and 2,000,000 HTML characters. Optional sanitized HTML omits scripts, event handlers and form values and is never executed by the archive UI.

The queue accepts up to 500 captures or 128 MiB; new captures are rejected when full, without deleting accepted entries. Restore the helper, retry delivery, then capture again. Permanently rejected entries can be exported, retried or discarded in settings. Accumulated blocks can include multiple revisions; choose a snapshot to see one state.

## MCP and development

An MCP client launches `pagevault-host mcp --db /absolute/path/archive.sqlite3` over stdio, opening an existing schema 1 database read-only. Tools list sites, search pages, and retrieve accumulated content or snapshot versions. An agent using a cloud model may send retrieved content to its provider. See [MCP setup and limits](docs/MCP.md).

Development requires Go 1.27.3, Node 26.12.1, npm 12.2.1 and Playwright Chromium. Run `npm ci`, `go test ./...`, `npm test`, `npm run lint`, `npm run format:check`, and `npm run check`. Build release bundles with `go run ./cmd/release`, `go run ./cmd/release --extension-only`, and on macOS `go run ./cmd/package-macos --unsigned`. See [verification](docs/VERIFICATION.md) and [contributing](CONTRIBUTING.md).

Documentation: [architecture](docs/ARCHITECTURE.md), [schema](docs/SCHEMA.md), [native protocol](docs/NATIVE_PROTOCOL.md), [adapters](docs/ADAPTERS.md), [distribution](docs/DISTRIBUTION.md), [readiness and support](docs/PRODUCTION_READINESS.md). Report security issues privately via [GitHub](https://github.com/akarazhev/pagevault/security/advisories/new). PageVault is MIT licensed; dependency notices are in [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md).
