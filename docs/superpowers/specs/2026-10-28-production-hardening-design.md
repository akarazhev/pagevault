# PageVault production hardening and English project

## Intent and authority

The user requested a comprehensive professional/production readiness pass and explicitly chose an English-only project: UI, messages and documentation, without a Russian variant. Existing authorization covers implementation, tests, commits/pushes and GitHub Releases. Distribution remains free and exclusively through GitHub Releases. Do not request another procedural approval; continue with concrete, reviewable changes and meaningful verification.

Retain the local Go host, JavaScript Manifest V3 extension, SQLite schema 1, extension ID `ijkioljfdaaplhaldbdpabckkfcieiee`, stdio MCP and existing archives. No paid developer accounts, store submissions, telemetry, cloud service or automatic downloads. Published v1.3.0 assets/tag stay intact. The next minor release is 1.4.0, chosen by the implementer for the English interface and reliability improvements.

## Approach

Harden the existing architecture and prove its failure behavior with regression tests. The current end-to-end path works, but an independent baseline audit reproduced data-loss, queue-blocking, extraction-limit and protocol-boundary defects. A wholesale rewrite adds migration risk without addressing these specific failures. Documentation-only polish would leave them in the published runtime. Focused fixes, maintainable source formatting, cross-platform CI and an honest support/readiness report are the selected implementation approach.

## Confirmed findings and required behavior

### Archive and protocols

- Opening an existing foreign/incomplete SQLite file must reject it before journal/schema changes. Initialize only a new/empty database; recognize existing schema-1 PageVault databases. Read-only access must remain read-only.
- Native site discovery must paginate with `limit` (default/max 200), `offset`, `total`, `items` and `next_offset` (null at end). Bound hostname summaries and update archive UI to traverse all pages. A multi-megabyte site list must remain usable without exceeding a native frame.
- MCP must preserve int64 archive IDs through both request parsing and output; IDs above 2^53 must never silently select another record. Avoid the SDK generic-tool float64 normalization while retaining explicit schemas, defaults and validation.
- Enforce a strict 64 KiB MCP input frame bound before exposing a frame to the SDK, including concatenated frames and EOF boundaries. Preserve stdio cancellation and clean shutdown.
- Make URI/path tests portable to Windows while retaining Unix-only question-mark injection coverage and multilingual Unicode fixture data.

### Extension reliability

- Reject captures outside native validation limits before enqueueing, including URLs over 65,536 Unicode code points. Existing permanently rejected queue entries must remain recoverable/visible and must not block unrelated valid captures. Preserve data until successful commit or an explicit user discard; distinguish permanent invalid_request from retryable delivery failures.
- Charge table cell text once across plain/rich projections; preserve visible rich link labels at the exact text budget and account for following blocks.
- Disconnect observers for removed/navigated same-origin iframe documents. Repeated iframe replacement must not retain orphaned document trees.
- Bound HTML construction before allocation: attribute-heavy pages must not create arbitrarily large cloned DOM/serialized strings before applying maxHtmlChars. Retain sanitization and warnings.
- Translate all user-facing extension text, HTML language, dates, warnings and errors to English. Multilingual synthetic test data remains coverage, not a separate interface locale.

### Installation and release safety

- A macOS core preinstall must run before payload replacement, using the bundled universal Go program from package Scripts. No Python/Node/jq runtime requirement. Check archives of all four browsers, including unselected browsers/default runtime configs, against every core payload destination (host and both license files), following existing alias-aware path safety. Abort before touching an archive if any collision or unreadable active registration prevents a safe decision.
- New CLI preflight behavior is read-only and accepts an explicit user home; real user registrations/archives are never modified by tests. Keep current-user-home-only installation and reject root.
- Publishing an artifact/checksum pair must preserve the previous pair on failure. Stage both, reject directory destinations and roll back partial publication; symlink/hardlink protections must retain their existing behavior. Reuse one internal publication helper for ZIP and PKG.
- Installer/CLI/shell/PowerShell copy is English. Unsigned PKG remains explicitly labelled, and Gatekeeper restrictions stay documented.

## Professional maintenance

- English README and all maintained Markdown docs; archive older store material as historical rather than a current distribution route. Add CONTRIBUTING.md, SECURITY.md, CHANGELOG.md, issue/PR templates and a concise production readiness/support matrix. No invented email or support SLA.
- Add consistent source formatting and static linting using pinned development tools. Preserve the plain JavaScript/Go architecture; no runtime frameworks or gratuitous code reorganization.
- GitHub Actions checks on Ubuntu, Windows and macOS: Go tests/vet, JS/static/format/lint, browser tests, real Linux Native Messaging E2E and six-target build. Pin actions by full commit SHA, grant contents:read by default, and do not use pull_request_target or execute PR code with write tokens/secrets.
- Add npm and Go vulnerability checks, weekly Dependabot for Go/npm/actions, and enable private vulnerability reporting if the existing admin permission allows it. Standard public runners only; avoid paid services and unnecessary uploaded build artifacts.
- Validate final ZIP/PKG/source alignment, version/ID consistency, licenses, absence of personal artifacts and SHA-256. Preserve reproducible build commands and document manual release steps; automation must not publish unreviewed PR code.

## Acceptance and limits

Every confirmed Important runtime finding receives a failing regression before its fix and a passing relevant suite afterward. Test changes prove user-visible behavior, not implementation text. Copy/config/docs-only changes need validation rather than mirrored unit tests.

Before final integration, run the complete local checks, get independent task and whole-branch reviews, and inspect actual GitHub CI on all three OS families. Build all six targets; do not equate cross-compilation with runtime verification of Windows/Linux arm64 or macOS Intel. Record exact tests/skips and any remaining findings. Production readiness means the supported, tested scenarios meet these gates; it is not a claim of universal security or platform certification.

Publish 1.4.0 through GitHub Releases only after required checks pass, with exact source commit, English release notes, three named artifacts and checksum sidecars. Stage a draft and verify downloaded bytes before publication. Do not replace v1.3.0 files. Do not reconfigure the user's live helper or agent clients merely to run automated checks.
