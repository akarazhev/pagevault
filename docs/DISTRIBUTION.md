# Releases

PageVault is distributed free through [GitHub Releases](https://github.com/akarazhev/pagevault/releases). Chromium uses an unpacked extension. Firefox supports temporary development installation or a Mozilla-signed XPI for permanent installation. See [installation](INSTALLATION.md) and [upgrading](UPGRADING.md).

## Build

Run from the repository root with the [development tools](../CONTRIBUTING.md):

```sh
go run ./cmd/release
go run ./cmd/release --extension-only
go run ./cmd/release --extension-only --browser firefox
```

The full ZIP includes six host binaries, `extension/` (Chromium), generated `extension-firefox/`, source and documentation. Standalone `PageVault-Extension-1.5.0.zip` and `PageVault-Firefox-1.5.0.zip` contain the selected extension and license with `manifest.json` at the root. All ZIPs are written to `dist/` with SHA-256 files. Application assets come from the same sources; only the Firefox manifest is transformed. Chromium's public key and ID remain unchanged.

The Firefox ZIP is unsigned. Mozilla signing through AMO is required for permanent release/beta installation, including self-distribution. Keep `pagevault@pagevault.local` fixed across updates and helper registrations. Use an AMO account to submit the generated ZIP as a listed or unlisted add-on. Do not publish the unsigned ZIP as a permanently installable XPI. See [Mozilla signing and distribution](https://extensionworkshop.com/documentation/publish/signing-and-distribution-overview/).

On macOS, build the universal installer with:

```sh
go run ./cmd/package-macos --unsigned
```

The PKG is unsigned and not notarized. Its executable has an ad-hoc signature, which does not establish developer identity. It installs the helper separately from the extension.

## Publish

1. Run the [verification checks](VERIFICATION.md) and require passing CI on the release commit.
2. Build the full ZIP, both standalone extension ZIPs and the macOS PKG. Check versions, contents, licenses and SHA-256 files. Sign the Firefox add-on separately if distributing a permanent installation.
3. Upload to a draft release. Download its assets and verify their bytes and checksums before publishing with the matching tag.
4. Keep existing published assets and tags intact.

Artifact and checksum writes roll back ordinary failures, but an interrupted process can leave an incomplete pair. Recheck both files after an interruption.
