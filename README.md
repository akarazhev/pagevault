<p align="center">
  <img src="extension/icons/icon128.png" alt="PageVault icon" width="96" height="96">
</p>

<h1 align="center">PageVault</h1>

<p align="center">
  <a href="https://github.com/akarazhev/pagevault/releases">
    <img src="https://img.shields.io/github/v/release/akarazhev/pagevault?label=release&amp;color=087f78" alt="Latest release">
  </a>
  <a href="https://github.com/akarazhev/pagevault/actions/workflows/ci.yml">
    <img src="https://github.com/akarazhev/pagevault/actions/workflows/ci.yml/badge.svg?branch=main" alt="CI status on main">
  </a>
  <a href="LICENSE">
    <img src="https://img.shields.io/badge/license-MIT-blue" alt="License: MIT">
  </a>
  <a href="docs/INSTALLATION.md">
    <img src="https://img.shields.io/badge/browsers-Chrome%20%7C%20Chromium%20%7C%20Edge%20%7C%20Brave%20%7C%20Firefox-087f78" alt="Browsers: Chrome, Chromium, Edge, Brave and Firefox">
  </a>
</p>

PageVault saves web page content in a local SQLite archive. Enable recording for a site to collect structured snapshots as you browse. Search saved pages, view earlier snapshots, export JSON or read the archive through an MCP client.

The Chromium and Firefox extensions share one implementation and capture loaded text, headings, lists, tables, code, links and media references. A GitLab adapter adds project metadata. The extension and Go helper run locally, without telemetry or a remote service.

## Installation

Download `PageVault-1.5.0.zip` and its SHA-256 file from [GitHub Releases](https://github.com/akarazhev/pagevault/releases). Extract the bundle to a permanent directory and verify the checksum.

1. Install the helper: run `./install.sh chrome` on macOS or Linux, or `install-windows.cmd` on Windows. Select `chromium`, `edge`, `brave` or `firefox` as needed; for example, `./install.sh firefox` or `install-windows.cmd firefox`.
2. For Chromium, open the browser's extensions page, enable developer mode and load the bundle's `extension` folder. For Firefox 140+, use the Firefox package or the bundle's `extension-firefox` folder as described in [installation](docs/INSTALLATION.md#firefox).
3. Check the helper connection in PageVault settings. Open an HTTP(S) page, enable recording for its site and grant access. Use Capture now for a manual snapshot.

macOS 13 and later also support an unsigned installer for Intel and Apple Silicon. See [installation](docs/INSTALLATION.md) for downloads, checksums and troubleshooting, and [upgrading](docs/UPGRADING.md) before replacing an existing installation.

## Capture and storage

Only loaded content is captured. Scroll or expand a page first. Media files are not downloaded, and the archive does not recreate an interactive site. An accumulated page contains unique blocks collected over time; select a snapshot to view one recorded state.

Saved pages may contain private data. Read the [privacy policy](docs/PRIVACY.md) and [capture limits and queue recovery](docs/PRODUCTION_READINESS.md). Drain the queue before removing the extension.

## Development

See [contributing](CONTRIBUTING.md) for setup, [verification](docs/VERIFICATION.md) for checks and [distribution](docs/DISTRIBUTION.md) for release builds.

Technical reference: [architecture](docs/ARCHITECTURE.md), [data format](docs/SCHEMA.md), [Native Messaging](docs/NATIVE_PROTOCOL.md), [MCP](docs/MCP.md), [site adapters](docs/ADAPTERS.md).

PageVault is MIT licensed. See [LICENSE](LICENSE), [dependency notices](THIRD_PARTY_NOTICES.md) and the [security policy](SECURITY.md).
