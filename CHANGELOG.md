# Changelog

## 1.5.0

- Added a Firefox 140+ package generated from the shared Chromium extension sources.
- Added Firefox Native Messaging registration and installer support on macOS, Linux and Windows.
- Added Firefox browser tests and installation/signing documentation; archive schema 1 and Chromium's extension identity remain unchanged.

## 1.4.0

- Improved queue recovery, Native Messaging validation, archive checks and installer rollback.
- Added English interface text, documentation and contribution guidelines.
- Added cross-platform CI, vulnerability checks and release verification.
- Kept SQLite schema 1 and the extension ID. See [upgrading](docs/UPGRADING.md) to preserve the configured archive.

## 1.3.0

Added an unsigned universal macOS installer, helper connection guidance and a standalone extension ZIP.

## 1.2.0

Added read-only MCP access to the archive.

## 1.1.0

Moved the native helper and SQLite implementation to Go with a persistent Native Messaging connection.
