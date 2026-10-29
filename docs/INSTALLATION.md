# Installation

PageVault uses a Chromium extension and a local helper. The helper includes SQLite and needs no separate service or language runtime.

## Downloads

Get the files and matching `.sha256` checksums from [GitHub Releases](https://github.com/akarazhev/pagevault/releases).

| File | Contents |
| --- | --- |
| `PageVault-1.4.0.zip` | Extension, installers, source and binaries for macOS, Linux and Windows on amd64 and arm64 |
| `PageVault-Extension-1.4.0.zip` | Extension and license |
| `PageVault-1.4.0-macOS-unsigned.pkg` | macOS helper for Intel and Apple Silicon; requires macOS 13 or later |

GitHub's automatic source archives do not contain binaries. Verify downloads before installing. On macOS or Linux:

```sh
shasum -a 256 -c PageVault-1.4.0.zip.sha256
```

On Windows, compare `Get-FileHash .\PageVault-1.4.0.zip -Algorithm SHA256` with the checksum file. Substitute the filename for other downloads.

## Install the helper

Extract the full bundle and open a terminal in its directory.

```sh
./install.sh chrome
```

On Windows, run `install-windows.cmd` or:

```powershell
./install-windows.ps1 -Browser chrome
```

Repeat for each browser, using `chrome`, `chromium`, `edge` or `brave`. To choose a database, pass `--db PATH` on macOS/Linux or `-Db PATH` on Windows. This selects a file; it does not move existing data.

On macOS, you can instead open the PKG and choose browsers in Installer. Close them when prompted. The package is unsigned and not notarized. If macOS blocks it, follow [Apple's instructions](https://support.apple.com/en-us/102445) for an individual exception. Organization policy may prevent installation. Do not disable system protections globally.

## Load the extension

Open `chrome://extensions`, `edge://extensions` or `brave://extensions`, enable developer mode and choose Load unpacked. Select the full bundle's `extension` folder or the extracted standalone extension folder. Keep that directory in place.

Check the helper connection and database path in PageVault settings. The extension ID is `ijkioljfdaaplhaldbdpabckkfcieiee`. Read [upgrading](UPGRADING.md) before replacing an installation.

## Paths and troubleshooting

| OS | Default database | Per-browser helper directory |
| --- | --- | --- |
| Linux | `~/.local/share/pagevault/archive.sqlite3` | `~/.local/share/pagevault/host/<browser>/` |
| macOS | `~/Library/Application Support/PageVault/archive.sqlite3` | `~/Library/Application Support/PageVault/host/<browser>/` |
| Windows | `%USERPROFILE%\AppData\Local\PageVault\archive.sqlite3` | `%USERPROFILE%\AppData\Local\PageVault\host\<browser>\` |

Settings shows the configured path. The macOS PKG also installs its shared payload in `~/Library/Application Support/PageVault/Installer/`.

If the helper is unavailable, check the browser registration, extension ID, helper version and database path, then restart the browser. Run the installed helper with `diagnose` for details. A helper from the bundle needs `--db PATH` to diagnose a custom archive; diagnostics run in a separate process from the browser session.

For custom paths, use the helper's `install --help`. It supports `--runtime-dir`, `--manifest-dir`, `--extension-id`, `--db` and `--dry-run`. When changing the helper directory, `--preserve-registered-db` retains the registered database path. The PKG uses this option. An invalid registration stops the update; repair it with an explicit `--db` pointing to the existing archive. Old custom helper directories are not removed.
