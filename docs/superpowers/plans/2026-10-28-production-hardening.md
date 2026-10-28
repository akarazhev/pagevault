# Production Hardening Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Resolve confirmed production defects, make PageVault English-only, add maintainable checks/support materials, and deliver a verified GitHub release.

**Architecture:** Retain the existing local Go/SQLite host and JavaScript extension. Add small shared safety boundaries for archive recognition, protocol framing, queue rejection and paired artifact publication. CI tests the supported desktop OS families; packaged binaries remain runtime-independent of Go/Node.

**Tech Stack:** Go 1.27.3, Node 26.12.1, npm 12.2.1, Playwright 1.64.1, existing MCP SDK/SQLite dependencies; pinned development-only formatting/lint/vulnerability tools and GitHub Actions.

**Spec:** `docs/superpowers/specs/2026-10-28-production-hardening-design.md`

## Global Constraints

- English-only UI, messages and maintained documentation; multilingual synthetic data is test coverage, not a locale.
- SQLite schema 1 and extension ID `ijkioljfdaaplhaldbdpabckkfcieiee` remain unchanged; preserve current archives/configuration and accepted queue entries.
- Free GitHub Releases only; no paid signing/account/store work, telemetry or network daemon. v1.3.0 assets/tag remain untouched; next release 1.4.0.
- Automated tests use temporary synthetic archives/homes/profiles only; never replace the real browser registration/archive or global client configurations.
- TDD for behavioral fixes; do not add mirrored source-text tests for copy, configuration or prose. Run focused checks while iterating and the complete applicable suite before each task commit.
- Work on `codex/production-hardening` in the authorized checkout; managed worktrees outside writable roots were already shown unusable in this session.

## Review Focus

- A late permanent error must not quarantine or delete a newer queue revision; pin this in Task 2.
- Native domain pagination must preserve the exact filter value of long hostnames and shorten pages by bytes, not truncate identifiers; pin this in Task 1.
- Empty vs foreign/incomplete SQLite must be distinguished before WAL/DDL; pin this in Task 1 with unchanged-file evidence.
- Core package payload must protect archives for unselected browsers and symlink/hardlink aliases before replacement; pin this in Task 3.
- Second-file publication failure must restore both prior artifacts, including symlink entries; failed rollback preserves/report backups. Pin this in Task 3; crash-atomic two-file replacement is not promised.

---

### Task 1: Archive and protocol safety

**Files:** `internal/archive/{storage.go,readonly.go,schema_test.go,storage_test.go,readonly_test.go}`; new `internal/archive/schema.go`; `internal/native/{host.go,host_test.go}`; `internal/mcpserver/{server.go,server_test.go}` and new focused framing helpers/tests as needed; `cmd/pagevault/mcp_test.go` for portable process coverage.

**Interfaces:** Produce `Store.DomainsPage(limit, offset int64) (map[string]any,error)` with `items`, `total`, `limit`, `offset`, `next_offset` (integer or null). Native `pages.domains` accepts default/max limit 200 and offset >=0; preserve exact `hostname` values and keep each response below 768 KiB by ending a page early. Existing `Domains()` may delegate for compatibility. Task 2 consumes `next_offset` and unchanged item shape.

- [x] Add failing tests that Open rejects nonempty foreign schema-0 and incomplete schema-1 archives without changing their bytes/journal/schema; retain valid existing schema-1/fresh/concurrent initialization tests.
- [x] Add failing native pagination tests traversing 4,800 long valid hostnames completely, a long exact filter value, and invalid limit/offset; every frame fits 1 MiB and pagination progresses by actual returned items.
- [x] Add raw-wire MCP regressions for page/capture ID 9007199254740993 in requests, structured results and text fallback; add a 65,575-byte frame after a 151-byte initialize plus exact-boundary, split-read and EOF cases.
- [x] Run regressions before changes; record expected RED. Implement conservative archive recognition before mutation, streaming/bounded native domain pages, SDK registration avoiding float64 normalization, and strict bounded line input ahead of SDK decoding. Retain tool schemas/defaults/validation, read-only behavior, cancellation and clean stdout.
- [x] Make question-mark filesystem tests Unix-only cases while using Unicode/space/#/% portable filenames on Windows. Run `go test -race ./...` and record all platform skips. Commit the task with RED/GREEN evidence in its report.

### Task 2: English extension with durable recovery and bounded extraction

**Files:** `extension/**` (all user-facing strings/lang/formatting); `tests/{core,connection,native-client,extractor,queue,ui,onboarding,integration}.test.cjs`; new focused capture-validation helpers/tests only where needed.

**Interfaces:** Consume Task 1 native domain pagination, treating absent `next_offset` as end-of-list for compatible older hosts. Extend outbox entries with a revision-scoped permanent rejection state without dropping existing v1 entries. `markError(id, revision, error, {permanent})` and explicit retry/discard operations must compare revisions; total queued/rejected counts and bytes stay consistent in one IDB transaction, including replacement of a rejected entry. Delivery returns sent/rejected/error statistics and skips rejected entries, bounded by attempted items, while retryable transport errors stop the batch. Background exposes extension-page-only queue listing/export/retry/discard; options UI lists rejected captures with error and explicit controls. List responses use bounded summaries, not full 24 MiB captures; export retrieves one revision on explicit click using existing JSON-download behavior. No content-script queue-management privilege.

- [x] Add RED regressions for a >65,536-codepoint URL rejected before enqueue; an existing permanently invalid entry followed by valid entries; IDB persistence across reopen; late rejection vs replacement revision; retry/discard accounting and authorization.
- [x] Add RED Chromium regressions for rich link table text at maxTextChars boundary with following content, removed/navigated same-origin iframes returning observed roots to baseline, and attribute-heavy HTML enforcing maxHtmlChars before large cloning/allocation.
- [x] Implement aligned validation, visible recovery preserving rejected data, revision-safe delivery and bounded processing. Fix shared table text accounting, observer discovery/lifecycle and bounded sanitized HTML construction. Paginate archive domain discovery using exact hostnames.
- [x] Translate every interface, warning/error and HTML lang to English; use English locale formatting. Keep multilingual fixture data for fidelity. Update UI assertions to English; do not test literal source copy.
- [x] Run `npm test`, `npm run check`; inspect English desktop/mobile UI with synthetic data, including rejected queue recovery. Commit with complete test/TDD evidence.

### Task 3: Pre-payload installer protection and reliable artifacts

**Files:** `internal/install/` new package-preflight helper/tests; `cmd/pagevault/{main.go,main_test.go}`; `internal/macpkg/{package.go,scripts.go,distribution.go,package_test.go}`; `internal/release/{release.go,release_test.go}`; new `internal/artifact/` pair publisher/tests; `cmd/package-macos/main.go`, `install.sh`, `install-windows.ps1`, `install-windows.cmd` for English copy.

**Interfaces:** Produce read-only `install.CheckPackagePayload(home string) error`, exposed as CLI `package-preflight --home ABSOLUTE_HOME`. Check all browser manifests, active config paths and fallback standard runtime configurations against the three payload files under `Library/Application Support/PageVault/Installer/`. Also inspect existing destination headers read-only and reject SQLite files not identified by a standard registration; no DB connection/initialization. Core preinstall invokes the universal host bundled under core package Scripts, resolves the user target as postinstall does, and refuses root. Shared `artifact.PublishPair(source, output string) (sha256 string,error)` publishes a staged file plus checksum sidecar with rollback on ordinary errors.

- [x] Add RED tests placing real synthetic SQLite at each core payload destination, including an unselected browser, aliases and an archive with no standard registration. Expand the real PKG, run its core preinstall and prove it fails before payload deployment with unchanged database bytes; normal fresh/upgrade preinstall succeeds without creating a DB or registration.
- [x] Add RED paired-publication regressions for a checksum-directory blocker and failure after first publication; verify preservation of old file contents and symlink targets/entries. Exercise actual filesystem state; if error injection is needed, wrap only the failing OS operation.
- [x] Implement the preflight and bundle its universal executable in core Scripts before pkgbuild. Preserve existing per-user install/active DB adoption and unsigned mode. Implement one shared publisher, reject directory targets, restore prior entries on failure, and preserve/report backups if rollback itself fails.
- [x] Translate CLI/install scripts/PKG resources to English; help/errors remain actionable. Run full Go race suite plus shell/PowerShell validations available on the current OS, and real expanded-package integration under synthetic homes. Commit with evidence.

### Task 4: Maintainable source and cross-platform quality gates

**Files:** `package.json`, `package-lock.json`, new ESLint/Prettier/editor/git attributes configs; `.github/workflows/ci.yml`, `.github/dependabot.yml`; `scripts/` validation helpers only as needed. Apply formatting across maintained source/test files. Change Go module/import path to canonical `github.com/akarazhev/pagevault` in go.mod and Go sources.

**Interfaces:** Expose `npm run lint`, `npm run format`, `npm run format:check`, retain test/check/package commands. CI uses module Go version and .nvmrc, pinned full-SHA official actions, contents:read and bounded timeouts/concurrency. Matrix Ubuntu 24.04, Windows 2025 and macOS 15 runs Go tests/vet and JS/static/browser checks; use race where supported. A Linux job must execute existing Native Messaging E2E without skips. Verify six-target build. npm audit and govulncheck pinned to v1.9.0 run as vulnerability gates.

- [x] Resolve latest stable development-only ESLint, globals and Prettier from official package metadata; pin exact versions. Add a consistent formatter and useful recommended lint rules; fix genuine findings without blanket suppression. Ignore generated/private files and semantic HTML fixtures.
- [x] Add pinned Actions and weekly dependency update configuration; do not publish or expose write tokens on PR runs. Enable private vulnerability reporting through the existing repository admin access and verify the resulting public reporting route.
- [x] Format maintained source, standardize line endings and canonical Go module imports. Run formatter/lint/static checks and complete Go/JS suites; dependency vulnerability scans must be inspected rather than judged by exit code alone.
- [x] Push the feature branch under existing authorization and run actual GitHub checks. Inspect Linux native E2E and Windows/macOS runtime results; fix genuine platform failures, including URI fixtures. Record job URLs, counts and skips. Commit the task after local gates; add subsequent focused commits for CI failures. Read the CI preparation evidence from this plan workspace; do not hide native E2E failures by skipping Linux cases.

### Task 5: English public project and verified release

**Files:** English finishing copy in `extension/ui/{common.js,options.js,archive.js,popup.js}` and `scripts/store-screenshots.cjs`; English rewrite of `README.md`, maintained `docs/*.md`; new `CONTRIBUTING.md`, `SECURITY.md`, `CHANGELOG.md`, `docs/PRODUCTION_READINESS.md`; `.github/ISSUE_TEMPLATE/` and PR template. `internal/buildinfo/info.go`, `extension/manifest.json`, package manifests and version-dependent tests; `internal/release/release.go` source whitelist for community/config/CI materials. Finish `eslint.config.js` environment scopes from the Task4 minor finding. Historical design/plan files are already English; retain intentional multilingual test data only.

**Interfaces:** Version 1.4.0 synchronized in native/extension/npm metadata. Include new community docs, formatting/lint configs and .github sources in full release; extension ZIP remains extension plus LICENSE. Named artifacts use version 1.4.0 with existing naming conventions. Public security reporting uses the verified GitHub private-report URL, not invented contacts.

- [x] Rewrite README around installation/use, screenshots if already available, architecture/privacy, MCP, support and contribution links. Translate maintained docs fully, archive old store preparation accurately, add release/support process and concrete readiness evidence without unsupported production claims. No Russian README variant.
- [x] Add useful English issue/PR templates, contribution and security policies, changelog and support matrix. Polish singular/plural counts and identify rejected captures with a bounded URL/time as well as title. Scope ESLint browser globals to extension source, Node globals to tools, and combined globals to browser-evaluated tests. Document rejected-queue recovery, manual upgrades/backups, unsigned macOS behavior, limits and what CI actually tests.
- [x] Bump 1.4.0 and add a meaningful packaging regression for new development/community source inclusion while keeping personal artifacts excluded. Update release whitelist and version-dependent fixtures; run the complete Go/JS/format/lint/static/vulnerability gates.
- [x] Build all six hosts and final ZIP, extension ZIP and universal unsigned PKG; verify actual contents, licenses, source/version/ID alignment and every checksum. Inspect English UI and PKG resources with synthetic data.
- [x] Complete independent reviews, integrate/push only after required GitHub checks pass, stage v1.4.0 in a draft, download and verify all six assets, publish and verify anonymous access/exact tag. Preserve v1.3.0 assets/tag; translate its mutable release description to English and point to the new release. Update the repository About description/topics in English. Record exact evidence and all material limits in the report/final response.

## Release verification

[PageVault 1.4.0](https://github.com/akarazhev/pagevault/releases/tag/v1.4.0) was published as Latest from `6e8267e9bb88b776776c698d0d5ba933a54912b4` after independent task and whole-branch reviews. [All ten CI jobs passed on this source commit](https://github.com/akarazhev/pagevault/actions/runs/40741540669): Linux JavaScript 72 passed with no skips; Windows/macOS 63 passed with nine expected Linux-only skips each; Go tests/vet, Linux race, six cross-builds and vulnerability gates passed.

All six draft assets were downloaded and matched local bytes, sidecars and server SHA-256 digests before publication. Full and extension ZIPs passed content/source/version/ID/private-exclusion checks; the universal unsigned PKG was expanded and inspected. The public release page and all six asset URLs returned HTTP 200 without authentication. The v1.3.0 asset IDs, sizes, digests, original publication metadata and tag were unchanged; its mutable body is now English and points to v1.4.0. Repository About and topics are English, and private vulnerability reporting is enabled. Platform and signing limits remain in [the readiness report](../../PRODUCTION_READINESS.md).

This completion record changes the historical plan only; the release tag continues to identify the exact verified source above.
