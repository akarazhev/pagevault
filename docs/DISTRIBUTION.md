# Releases

PageVault is distributed free through [GitHub Releases](https://github.com/akarazhev/pagevault/releases). The extension is loaded unpacked and updated manually. See [installation](INSTALLATION.md) and [upgrading](UPGRADING.md).

## Build

Run from the repository root with the [development tools](../CONTRIBUTING.md):

```sh
go run ./cmd/release
go run ./cmd/release --extension-only
```

The full ZIP includes six host binaries, the extension, source and documentation. The extension ZIP contains the extension and license with `manifest.json` at its root. Both are written to `dist/` with SHA-256 files.

On macOS, build the universal installer with:

```sh
go run ./cmd/package-macos --unsigned
```

The PKG is unsigned and not notarized. Its executable has an ad-hoc signature, which does not establish developer identity. It installs the helper separately from the extension.

## Publish

1. Run the [verification checks](VERIFICATION.md) and require passing CI on the release commit.
2. Build all three artifacts. Check versions, contents, licenses and SHA-256 files.
3. Upload to a draft release. Download its assets and verify their bytes and checksums before publishing with the matching tag.
4. Keep existing published assets and tags intact.

Artifact and checksum writes roll back ordinary failures, but an interrupted process can leave an incomplete pair. Recheck both files after an interruption.
