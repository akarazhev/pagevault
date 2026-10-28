# macOS installer and onboarding implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement task-by-task in this session. Checkboxes track verified work.

**Goal:** Install the macOS helper through a PKG, guide connection in the extension, and prepare separate store artifacts and documentation.

**Architecture:** Reuse the per-user Go installer; add opt-in active-registration DB adoption, a native macOS package builder, a small connection-state helper, and an extension-only release mode.

**Tech Stack:** Go 1.27.3, Node.js 26.12.1/npm 12.2.1/Playwright 1.64.1, macOS pkgbuild/productbuild/lipo/codesign. No new runtime dependencies.

**Spec:** docs/superpowers/specs/2026-10-28-macos-installer-design.md

## Global Constraints

- Version 1.3.0, existing extension ID and SQLite schema 1.
- PKG installs only in currentUserHome; macOS 13.0+, universal Intel/Apple Silicon.
- Preserve active archives, including an existing custom runtime; never execute the old registered program.
- Public signed mode needs both Developer ID identities. Local testing requires explicit --unsigned and a clearly named unsigned artifact.
- Downloads link to the existing releases page; no fabricated direct asset, automatic download/execution or new extension permission.
- Tests use temporary homes, SQLite and browser fixtures; do not overwrite the user's active registration or archive.
- Store publishing, notarization submission, Windows/Linux GUI installers and automatic updates remain out of scope.

## Review Focus

- A custom runtime registration must retain its active DB even when a stale default runtime config exists.
- Bad/missing registered configuration must fail before touching a working registration; explicit --db can repair it.
- User-home package scripts must handle space/Unicode homes and cannot select root's HOME or a system domain.
- UI retry failures, unknown platform/version and untrusted error/path values must remain useful and safe.
- Extension-only ZIP and PKG destinations must not overwrite sources or include their own generated outputs via aliases.

### Task 1: Preserve the actively registered archive

**Files:** internal/install/install.go, new registered.go/registered_test.go, cmd/pagevault/main.go and tests.

**Interfaces:** Add Options.PreserveRegisteredDB bool and CLI --preserve-registered-db. When DBPath is empty, adopt a validated existing native manifest's adjacent schema-1 pagevault.json before consulting default runtime config.

- [x] Add failing tests: custom runtime plus stale default config, explicit DB precedence, no manifest fallback, malformed name/type/paths/config and unchanged files after failure.
- [x] Run `go test ./internal/install ./cmd/pagevault`; expected RED before the option exists.
- [x] Implement preflight adoption without executing the old binary and retain original behavior unless opted in.
- [x] Run `go test -race ./internal/install ./cmd/pagevault ./internal/native ./internal/archive`; expected PASS.
- [x] Commit `feat: preserve registered archive during installer migration`.

### Task 2: Build the macOS PKG

**Files:** cmd/package-macos/main.go, internal/macpkg/{package,distribution,scripts}.go and tests, buildinfo/manifest/package version files.

**Interfaces:** macpkg.Build(context.Context,Options) (Artifact,error); Options has Root, Output, Go, Unsigned, ApplicationIdentity, InstallerIdentity. Artifact returns Path and SHA256. Package scripts consume Task 1's flag.

- [x] Write failing tests for explicit unsigned/signing modes, output safety, user-home domain, browser choices, minimum OS and script target resolution.
- [x] Implement source builds for both Darwin architectures, lipo, ad-hoc or Developer ID binary signing, native component/product packaging, atomic output/checksum and cleanup. Bump release identity to 1.3.0.
- [x] On macOS, test a real package: expand, inspect universal payload and Installer metadata, run postinstall in a temporary Unicode/space home, preserve a custom registered DB on upgrade, and query captured data through real installed native and SDK MCP processes.
- [x] Run `go test -race ./internal/macpkg ./internal/install ./cmd/pagevault`; expected PASS. Build a local unsigned PKG and inspect it with pkgutil/installer metadata commands.
- [x] Commit `feat: build per-user macOS installer packages`.

### Task 3: Connection and download screen

**Files:** extension/lib/connection.js, extension/ui/options.html/options.js/styles.css, tests/connection.test.cjs and onboarding browser tests.

**Interfaces:** Pure connection-state and platform helpers drive existing status rendering; consume ping version/schema/DB path, expose ready/unavailable/incompatible/update states.

- [x] Add failing unit/browser tests for missing/ready/older/newer/incompatible helpers, malicious text, OS detection failure, stable HTTPS download links and failed/repeated checks recovering.
- [x] Implement the download/install/check flow, helper version/DB/error display, manual platform choice and existing Windows/Linux script fallback without changing permissions or queue behavior.
- [x] Run `npm test` and `npm run check`; expected PASS with explicit existing Linux skips on macOS. Inspect desktop/mobile screenshots from synthetic fixtures.
- [x] Commit `feat: guide helper installation and connection`.

### Task 4: Store artifacts and release verification

**Files:** cmd/release/main.go, internal/release/release.go/release_test.go, docs/INSTALLATION.md/PRIVACY.md/CHROME_WEB_STORE.md, existing README/distribution/architecture/MCP/upgrading/verification docs and release whitelist.

**Interfaces:** release.Options.ExtensionOnly bool and CLI --extension-only; store ZIP roots at manifest.json while the full release retains pagevault/ prefix. Include new docs in full releases.

- [ ] Write failing tests for extension ZIP root/content/checksum, mutually exclusive flags and source/self-output aliases; implement by reusing existing source and output protection.
- [ ] Prepare privacy policy, listing/review instructions, signed/unsigned PKG usage, preserved DB behavior and downloads/publication prerequisites.
- [ ] Run Go race suite, vet/module verification, npm/browser/static checks, shell syntax, six-target build, full ZIP, extension ZIP, real unsigned PKG and SHA-256 checks. Record actual results and limits.
- [ ] Commit `feat: prepare store artifacts and installer documentation`.
- [ ] Run one fresh whole-branch read-only review, address material findings with RED→GREEN regressions, then integrate under existing git/push authorization.
