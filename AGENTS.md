# Working on PageVault

These instructions apply throughout this repository. Use the linked project documentation for detailed contracts and keep it consistent with behavior changes.

## Project and entry points

PageVault saves loaded web page content in a local SQLite archive. It consists of a Chromium Manifest V3 extension in plain JavaScript and a Go helper. The helper provides Native Messaging, a read-only MCP server over stdio, and installation commands. There is no remote collection service or telemetry.

Capture flow: DOM → content script → background worker → IndexedDB outbox → Native Messaging → SQLite.

Read [README.md](README.md), [CONTRIBUTING.md](CONTRIBUTING.md), and [architecture](docs/ARCHITECTURE.md) before substantial changes. Inspect the working tree and preserve unrelated user changes. Keep edits focused on the requested behavior.

| Location | Responsibility |
| --- | --- |
| `extension/extractor.js` | DOM extraction and optional HTML sanitization; classic script without extension APIs |
| `extension/content.js` | Capture scheduling, mutation observers, SPA navigation, frames and shadow roots |
| `extension/background.js` | Message authorization, site permissions, settings and capture delivery |
| `extension/lib/` | Durable outbox, native client, semantic hashes, policy and configuration |
| `extension/adapters/` | Site-specific metadata; currently includes GitLab |
| `extension/ui/` | Popup, settings, archive browsing and safe block rendering |
| `cmd/pagevault/` | Helper CLI entry point |
| `internal/archive/`, `internal/native/`, `internal/mcpserver/` | SQLite storage and the two transport protocols |
| `internal/install/`, `internal/macpkg/`, `internal/release/`, `internal/artifact/` | Installation, packaging and artifact replacement |
| `cmd/release/`, `cmd/package-macos/` | Release build commands |
| `tests/`, `internal/**/*_test.go`, `cmd/**/*_test.go` | Node/Playwright tests and Go tests |
| `scripts/`, `.github/workflows/ci.yml` | Static checks, screenshot tooling and CI |

## Setup and local commands

Run commands from the repository root. Follow the toolchain versions in `go.mod`, `.nvmrc` and `package.json`; use `package-lock.json` with `npm ci`. Do not change version pins merely to accommodate an older local toolchain.

```sh
npm ci
npx playwright install chromium
npm run build
```

On Linux, browser tests may need `npx playwright install --with-deps chromium`. `npm run build` writes the local helper to `bin/pagevault-host`; it does not register the helper with a browser.

Common validation commands:

```sh
go test ./...
go vet ./...
npm test
npm run lint
npm run format:check
npm run check
```

`npm run check` verifies extension scripts, assets, CSP constraints, icon dimensions and the derived extension ID. It does not replace runtime tests.

## Verification by change

- Start with the affected Go package or Node test file, then run the relevant broader suites. For example: `go test ./internal/archive` or `node --test --test-concurrency=1 tests/queue.test.cjs`.
- Add regression coverage for behavior changes. Keep tests beside Go code or in `tests/*.test.cjs`, and retain multilingual and Unicode fixtures.
- For extension changes, run `npm test`, lint, formatting and asset checks. For Go changes, run Go tests and vet; changes to shared protocol behavior also need the extension tests.
- For documentation-only edits, verify paths, commands and the diff. Application tests are unnecessary unless executable behavior or build inputs change.
- Before a pull request or release, follow [verification](docs/VERIFICATION.md), including the race detector and dependency vulnerability checks. CI configuration is in [.github/workflows/ci.yml](.github/workflows/ci.yml).
- Native Messaging browser integration tests run only on Linux with an isolated Chromium profile. Report skipped or unavailable checks explicitly; passing macOS or Windows tests does not establish native browser integration coverage.
- Summarize the behavior changed, checks actually run and relevant limitations. Do not claim tests or interactive installation passed without running them.

## Behavioral contracts

### Capture, authorization and rendering

- Preserve per-site recording permissions and main-frame/sender-origin validation. Privileged archive and queue commands must remain limited to extension pages.
- Capture loaded DOM content and media references. Preserve extraction limits and warnings for incomplete content, including inaccessible frames and unsupported media.
- Treat page content, URLs, titles and adapter metadata as untrusted data. Keep form values and executable content out of sanitized HTML. Render archived blocks safely without executing saved HTML or adding remote scripts.
- Adapters add JSON-compatible metadata without replacing generic extraction. Register new bundled adapters in `CAPTURE_FILES` in `extension/lib/config.js`, after `extractor.js` and before `content.js`. See [adapters](docs/ADAPTERS.md).

### Queue and native connection

- Accept captures durably in IndexedDB before acknowledging enqueue. Remove an entry only after SQLite commits and only for the acknowledged revision.
- Preserve revision checks on acknowledgements, errors, retry and discard. A stale response must not remove or alter a newer capture. Queue capacity limits must not evict accepted captures.
- Keep native mutations serialized and capture acceptance independent of native I/O. Page deletion must also clear queued captures for the exact URL in the established order.
- Reuse the persistent `connectNative` port and correlate responses by request ID. Read timeouts must not close a shared connection or release an in-flight mutation; saves and deletes wait for acknowledgement or disconnection.

### Storage and protocols

- Page identity is the exact URL, including query and fragment. `metadata.canonical_url` is metadata, not a replacement identity.
- Preserve semantic deduplication: timestamps and HTML do not create snapshot versions. Accumulated blocks are unique blocks in first-seen order and may span multiple visits; they are not one historical snapshot.
- Preserve transactions, foreign keys, cascading deletion, WAL and schema validation. Unknown or damaged databases must be rejected without automatic recreation or repair. Current archive and capture schemas are version 1; schema changes need an explicit migration and compatibility plan.
- Keep Native Messaging framed JSON separate from MCP JSON-RPC. In both serving modes, stdout is reserved for protocol data and diagnostics go to stderr. Preserve frame limits, pagination and request validation.
- Chunk offsets count Unicode code points, not JavaScript UTF-16 code units. Preserve document hashes and restart semantics so clients cannot combine different revisions.
- MCP opens an existing archive read-only, without creating or initializing it. Preserve its bounded responses, positive signed 64-bit IDs and five query tools. Do not add arbitrary SQL, filesystem access, network fetching or mutation capabilities as an incidental change.

For wire formats and limits, consult [schema](docs/SCHEMA.md), [Native Messaging](docs/NATIVE_PROTOCOL.md) and [MCP](docs/MCP.md). Keep implementations, clients, tests and these documents aligned when changing a contract.

## Style and compatibility

- Follow `.editorconfig`, ESLint and Prettier configuration. Use `gofmt` for changed Go files and format only the relevant JavaScript, JSON, CSS or HTML files to avoid unrelated churn.
- Keep the extension in plain JavaScript with the existing module/classic-script split. Preserve Manifest V3 CSP: no inline executable scripts, event-handler attributes or remote code.
- Keep UI text and project documentation in English, consistent with the existing files.
- Preserve the extension public key and derived ID, native host name and installed archive paths. They connect existing browser registrations, settings and queued captures to the helper.
- When a release version changes, keep `package.json`, `package-lock.json`, `extension/manifest.json` and `internal/buildinfo/info.go` consistent, and update affected release documentation.
- Preserve macOS, Linux and Windows support for amd64 and arm64. Keep platform-specific behavior isolated and maintain builds with `CGO_ENABLED=0`.

## User data, installers and releases

- Use synthetic captures and temporary databases, homes and browser profiles for tests and diagnostics. Never use the user's real archive or browser registration as a test fixture. The helper's `diagnose` command can create a database, so always supply a temporary database path during testing.
- Keep private page content, URLs, credentials and personal paths out of logs, fixtures, screenshots and reports. Follow [privacy](docs/PRIVACY.md) and [security](SECURITY.md).
- Installer changes must preserve the configured database and rollback behavior. Retain destination-alias checks and recovery backups when replacement or rollback fails. Test against temporary destinations or use `--dry-run`.
- Generated binaries and release artifacts belong in ignored `bin/` and `dist/`; disposable reports belong in `.test-artifacts/`. Do not commit archives, SQLite files, dependencies or local environment files.
- Packaging uses explicit source allowlists in `internal/release/release.go`. Check them and the release tests when adding files that must ship in a bundle.
- Follow [distribution](docs/DISTRIBUTION.md) for ZIP/PKG builds and checksums, and [upgrading](docs/UPGRADING.md) for archive preservation. Build the unsigned macOS installer on macOS with `go run ./cmd/package-macos --unsigned`.
- Preserve existing published assets and tags. Distinguish cross-compilation and package inspection from interactive installation checks; record platform limits as described in [support and limitations](docs/PRODUCTION_READINESS.md).
