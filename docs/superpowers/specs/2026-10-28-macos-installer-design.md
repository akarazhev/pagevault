# macOS installer and browser onboarding

The user approved implementing the proposed next stage: a macOS PKG, a clear connection screen, and Chrome Web Store preparation. They delegated implementation choices. Continue through implementation and verification without another procedural approval round. Store submission and public release publication are not requested here.

## Goals

- A person installs the local component with the macOS Installer GUI, selects their browser, checks the extension connection, saves a page, and reads it through the existing MCP.
- Preserve every existing archive path and archive contents on upgrade. Keep per-user registration and compatibility with existing shell installers.
- Provide separate distribution artifacts: the complete six-platform bundle, a macOS PKG, and a Chrome Web Store ZIP with manifest.json at its root.
- Prepare an accurate privacy policy, store listing/review instructions, and installation/update documentation.

Windows/Linux GUI installers, automatic updates, a network service, new SQLite schema, and store submission remain later work.

## Approaches and implementation choice

1. System-wide PKG: uses administrator installation and global native registrations. It introduces account selection and migration problems for the existing per-user configurations.
2. Per-user PKG: fits the existing Go installer, permits a current-user-home installation, and preserves configuration per browser. This is the selected implementation choice within the user's authorized scope.
3. A new desktop setup app/DMG: adds an application and an extra launch step without helping the existing CLI registration. Defer it.

Use Apple's native pkgbuild/productbuild/lipo/codesign tools, with no new runtime dependencies. Apple documents that a current-user-home installation runs as that user and cannot write outside that home. The existing Go binary's minimum macOS version is 13.0; the PKG must require at least macOS 13.0. Release version: 1.3.0. Preserve extension ID and schema 1.

## Package structure

- A core component installs a universal x86_64/arm64 host and license materials under the current user's `Library/Application Support/PageVault/Installer/`.
- Four script components register Chrome (selected by default), Chromium, Edge, and Brave (optional) through the existing Go `install` command. The Installer displays these choices; the core is always selected.
- Distribution domains: currentUserHome enabled, localSystem and anywhere disabled. Component install location is `/`, interpreted relative to the selected user-home domain.
- Browser registration uses the unchanged `~/Library/Application Support/PageVault/host/<browser>/` runtime and standard native manifest paths. Add opt-in `install --preserve-registered-db`: when no explicit `--db` is supplied, read the existing standard native manifest and the pagevault.json beside its absolute executable path. This preserves an active archive even when the old installation used a custom `--runtime-dir`. Explicit `--db` wins. With no old manifest, keep the existing default-config behavior.
- Read and validate the old host name, stdio type, absolute executable path and schema-1 absolute DB configuration; do not execute the old program. Invalid/unreadable existing registration fails before replacements and tells the user to repair with an explicit DB path. Migrate the registration to the default per-user runtime while leaving old custom runtime files intact. A nonstandard browser profile/manifest directory still requires the existing CLI options.
- Postinstall resolves the actual payload under the user-home target or the current user's HOME, refuses to run as root, and invokes the copied host with an explicit `--home`. This also permits isolated verification by placing the expanded payload under a temporary home.
- Declare selected browsers in Installer's must-close metadata. Do not terminate processes silently. Tell MCP users to reconnect after updating.
- Registration is transactional per browser, as today. If a selected browser fails, report the failure; previously successful browser registrations remain usable and retries preserve their DB paths. Do not claim a cross-browser atomic transaction.

The builder is a Go command `cmd/package-macos` backed by `internal/macpkg`. It builds both Darwin hosts from current sources by default, combines them with lipo, packages the components, and publishes the final PKG/checksum only after successful build. Validate output filename/path so packaging cannot overwrite project sources or its inputs. Clean private staging directories on success and failure.

## Signing and publication

There are no code-signing identities available on this machine. Provide explicit local-test mode `--unsigned`, producing `PageVault-1.3.0-macOS-unsigned.pkg`. It may use an ad-hoc executable signature for Apple Silicon, but is not a trusted signed/notarized public installer.

Provide signing options for Developer ID Application and Developer ID Installer identities. Signed mode must require both, sign the executable before packaging, enable hardened runtime, and sign the product package. Document subsequent notarytool/stapler steps; do not access a developer account or submit without credentials/authorization. Do not call an unsigned package ready for normal public distribution.

No GitHub Releases are currently published. The connection screen therefore links honestly to the repository's releases page, labelled as opening downloads, rather than promising a direct asset that does not exist. PKG artifacts are delivered locally for review. Public release publication remains a separate final action after signing is configured.

## Connection screen

Keep the established extension UI and permissions. Extend options.html/options.js and a small pure connection helper module; reuse existing CSS components.

- Display helper availability, helper version, SQLite path, and current error.
- A failed ping means 'helper unavailable', not proof it is uninstalled. Retain the concrete error and a retry action.
- A known unsupported schema is incompatible; show update guidance. A helper older than the extension with schema 1 remains usable and receives an update suggestion. A newer schema-compatible helper is usable.
- Provide an OS selector (macOS, Windows, Linux), with best-effort chrome.runtime.getPlatformInfo detection and a manual fallback. macOS uses the PKG workflow; Windows/Linux retain existing script instructions.
- Explain three steps: open downloads, install/register for the correct browser, check connection. Include the actual extension ID for diagnosis.
- Use normal user-click HTTPS links with noopener/noreferrer. No automatic download, execution, remote code, extra permission or background request is added.
- Keep the queue, recording settings, sites, and existing archive behavior unchanged. Use textContent for all host-provided values. Recover properly from failed and repeated connection checks.

## Release and store preparation

Extend `cmd/release` with an extension-only mode, mutually exclusive with source-only and skip-build. It must include only extension files with manifest.json at the ZIP root, preserve the source/alias protections of the full packager, avoid self-inclusion, and emit SHA-256.

Add installation, privacy, and store-preparation documents and include them in the complete release. The privacy policy describes selected-site DOM/URL/text/metadata/optional HTML collection, IndexedDB queue and SQLite, deletion, local processing, optional agent access and cloud providers, absence of telemetry/remote collection by PageVault itself, and support contact via repository issues. It does not claim fully offline processing when a cloud agent is used.

The store preparation document provides listing text, permission purposes, stable privacy-policy URL, package instructions, review steps and remaining publication prerequisites. No store fee, account registration, ID replacement, upload, or publication is performed.

## Verification

- TDD for active registered DB adoption (including a custom runtime), explicit DB precedence, missing registrations, malformed/relative/incomplete configurations, and no replacement on failure.
- TDD for the package's user-home-only domain, universal payload, browser choices, version/minimum OS, signing requirement, missing binary/tool failures, and safe output handling.
- On macOS, build a real PKG, expand it with pkgutil, inspect Distribution/PackageInfo, and execute the expanded postinstall against a temporary Unicode/space home. Verify the registered native host, initial/default DB, preserved custom DB on update, and no archive deletion.
- Run a real installed native process and SDK MCP client against the temporary archive to verify capture → SQLite → MCP. Do not replace the user's active browser registration or archive during tests.
- Browser tests for unavailable/ready/update/incompatible states, malicious host text, platform fallback, download destination, and retry failures/recovery. Inspect screenshots using synthetic data.
- Regression tests for the extension-only ZIP's root, contents, checksum, and source/output alias protection.
- Run the Go race suite, vet, module verification, npm/browser/static checks, six-target release build and artifact checksums. Record platform-specific skips and distinguish package expansion/script checks from an actual GUI installation.
- One fresh read-only review of the finished branch. Build unsigned locally unless valid signing identities are supplied.

References: [Apple distribution domains](https://developer.apple.com/library/archive/documentation/DeveloperTools/Reference/DistributionDefinitionRef/Chapters/Distribution_XML_Ref.html), [Apple packaging](https://developer.apple.com/documentation/xcode/packaging-mac-software-for-distribution), [Chrome Native Messaging](https://developer.chrome.com/docs/extensions/develop/concepts/native-messaging).
