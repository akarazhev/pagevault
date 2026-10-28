# Installation

Download the named artifacts and matching SHA-256 files from [GitHub Releases](https://github.com/akarazhev/pagevault/releases). `PageVault-1.4.0.zip` contains six binaries and the extension. `PageVault-Extension-1.4.0.zip` contains the extension and license only. `PageVault-1.4.0-macOS-unsigned.pkg` installs the macOS helper for Intel and Apple Silicon, but not the extension. GitHub's automatic source archives have no binaries.

Extract the bundle to a permanent directory. On macOS or Linux run `./install.sh chrome`; replace `chrome` with `chromium`, `edge`, or `brave` as needed. On Windows run `install-windows.cmd`, or `./install-windows.ps1 -Browser edge`. Install separately for each browser. The installer displays the runtime and database paths. Use `--db PATH` (Windows: `-Db PATH`) to select a database; selecting a different path does not move data. The helper and SQLite do not require a separate service, Python, Node or Go at runtime.

Alternatively, on macOS 13 or later, open the unsigned PKG and choose the browsers to register. Close those browsers when prompted. The package is neither Developer ID signed nor notarized. macOS may block it; see [Apple's instructions for opening an individually blocked app](https://support.apple.com/en-us/102445). Organization policy may prevent an exception. Do not disable system protections globally.

Load the unpacked extension from the bundle's `extension` folder, or the extracted standalone ZIP folder, on `chrome://extensions`, `edge://extensions`, or `brave://extensions`: enable developer mode and choose Load unpacked. Keep the directory in place. Check the connection and database path in PageVault settings. The fixed extension ID is `ijkioljfdaaplhaldbdpabckkfcieiee`. See [upgrading](UPGRADING.md) before replacing an installation.

For troubleshooting, confirm the installer targeted the same browser, restart it, and check the settings error and database path. For custom registration paths, run the installer with `--help`; `--runtime-dir`, `--manifest-dir`, `--extension-id`, `--db` and `--dry-run` are supported. `--preserve-registered-db` preserves an existing registration's database path when changing runtime directory. The PKG uses this option.

## Paths and diagnostics

| OS | Default database | Default per-browser runtime |
| --- | --- | --- |
| Linux | `~/.local/share/pagevault/archive.sqlite3` | `~/.local/share/pagevault/host/<browser>/` |
| macOS | `~/Library/Application Support/PageVault/archive.sqlite3` | `~/Library/Application Support/PageVault/host/<browser>/` |
| Windows | `%USERPROFILE%\AppData\Local\PageVault\archive.sqlite3` | `%USERPROFILE%\AppData\Local\PageVault\host\<browser>\` |

The actual configured path appears in settings. On macOS PKG stores the universal binary and licenses in `~/Library/Application Support/PageVault/Installer/`; selected browsers have separate runtime copies and configuration. A damaged existing registration stops preservation before replacement; explicitly select the correct existing database with the CLI if repair is needed. Old custom runtime files are not removed.

On macOS/Linux verify a downloaded sidecar in its directory with `shasum -a 256 -c PageVault-1.4.0.zip.sha256`. On Windows compare `Get-FileHash .\PageVault-1.4.0.zip -Algorithm SHA256` to the sidecar. Substitute the corresponding file name for the other artifacts. For diagnostics, run the appropriate binary, for example `./bin/darwin-arm64/pagevault-host diagnose` or `bin\windows-amd64\pagevault-host.exe diagnose`. This separate process reports its own PID and connection, not the browser's running session. On connection errors confirm registration and browser, fixed extension ID, helper version and database path; rerun the installer if needed.

For development, `go run ./cmd/release` builds the full ZIP and six hosts, `go run ./cmd/release --extension-only` builds the extension ZIP, and `go run ./cmd/package-macos --unsigned` builds the universal PKG. The universal executable is ad-hoc signed, which does not establish developer identity or notarization. See [verification](VERIFICATION.md).
