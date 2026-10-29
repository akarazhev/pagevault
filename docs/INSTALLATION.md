# Installation

PageVault uses a Chromium or Firefox extension and a local helper. The helper includes SQLite and needs no separate service or language runtime. Chromium requires Chrome 116+; Firefox requires desktop Firefox 140+.

## Downloads

Get the files and matching `.sha256` checksums from [GitHub Releases](https://github.com/akarazhev/pagevault/releases).

| File | Contents |
| --- | --- |
| `PageVault-1.5.0.zip` | Chromium and Firefox extension folders, installers, source and binaries for macOS, Linux and Windows on amd64 and arm64 |
| `PageVault-Extension-1.5.0.zip` | Chromium extension and license |
| `PageVault-Firefox-1.5.0.xpi` | Mozilla-signed Firefox extension for permanent installation |
| `PageVault-Firefox-1.5.0.zip` | Unsigned Firefox extension and license; build locally if not yet published |
| `PageVault-1.5.0-macOS-unsigned.pkg` | macOS helper for Intel and Apple Silicon; requires macOS 13 or later |

GitHub's automatic source archives do not contain binaries. Verify downloads before installing. On macOS or Linux:

```sh
shasum -a 256 -c PageVault-1.5.0.zip.sha256
```

On Windows, compare `Get-FileHash .\PageVault-1.5.0.zip -Algorithm SHA256` with the checksum file. Substitute the filename for other downloads.

## Install the helper

Extract the full bundle and open a terminal in its directory.

```sh
./install.sh chrome
```

On Windows, run `install-windows.cmd` or:

```powershell
./install-windows.ps1 -Browser chrome
```

Repeat for each browser, using `chrome`, `chromium`, `edge`, `brave` or `firefox`. For Firefox, run `./install.sh firefox` or `./install-windows.ps1 -Browser firefox`. To choose a database, pass `--db PATH` on macOS/Linux or `-Db PATH` on Windows. This selects a file; it does not move existing data. The default SQLite archive is shared across browsers; their queues, settings and site permissions are separate.

On macOS, you can instead open the PKG and choose browsers in Installer. Close them when prompted. The package is unsigned and not notarized. If macOS blocks it, follow [Apple's instructions](https://support.apple.com/en-us/102445) for an individual exception. Organization policy may prevent installation. Do not disable system protections globally.

## Load the extension

Open `chrome://extensions`, `edge://extensions` or `brave://extensions`, enable developer mode and choose Load unpacked. Select the full bundle's `extension` folder or the extracted standalone extension folder. Keep that directory in place.

Check the helper connection and database path in PageVault settings. The extension ID is `ijkioljfdaaplhaldbdpabckkfcieiee`. Read [upgrading](UPGRADING.md) before replacing an installation.

### Firefox

For permanent installation, download `PageVault-Firefox-1.5.0.xpi`. Open `about:addons`, choose the gear menu, select **Install Add-on From File**, and select the XPI. Install the helper for `firefox` separately, then check the connection and database path in PageVault settings. The full bundle's `extension-firefox` folder is an unsigned development copy; the signed XPI is a separate download.

Use the bundle's `extension-firefox` folder or build a Firefox ZIP with `npm run package:firefox`. For development, open `about:debugging#/runtime/this-firefox`, choose **Load Temporary Add-on**, and select `extension-firefox/manifest.json` or `PageVault-Firefox-1.5.0.zip`. Temporary add-ons are removed when Firefox closes; drain the queue before removing or changing installations.

Permanent installation in Firefox release/beta requires a Mozilla-signed XPI. The generated ZIP is unsigned and cannot be installed permanently by renaming it to `.xpi`. Maintainers obtain a signature through AMO (public listing or unlisted self-distribution); see [Mozilla's signing guide](https://extensionworkshop.com/documentation/publish/signing-and-distribution-overview/). The build does not submit or sign an add-on. Self-distributed updates are installed manually; this package does not configure an automatic update feed.

The Firefox add-on ID is `pagevault@pagevault.local`. Firefox may show a browsing-activity and website-content disclosure because selected pages are sent to the **local** Native Messaging helper. There is no remote collection service. Install the helper for `firefox`, then check the connection and archive path in settings.

## Paths and troubleshooting

| OS | Default database | Per-browser helper directory |
| --- | --- | --- |
| Linux | `~/.local/share/pagevault/archive.sqlite3` | `~/.local/share/pagevault/host/<browser>/` |
| macOS | `~/Library/Application Support/PageVault/archive.sqlite3` | `~/Library/Application Support/PageVault/host/<browser>/` |
| Windows | `%USERPROFILE%\AppData\Local\PageVault\archive.sqlite3` | `%USERPROFILE%\AppData\Local\PageVault\host\<browser>\` |

Settings shows the configured path. The macOS PKG also installs its shared payload in `~/Library/Application Support/PageVault/Installer/`.

Firefox's native manifest is `~/.mozilla/native-messaging-hosts/com.pagevault.host.json` on Linux and `~/Library/Application Support/Mozilla/NativeMessagingHosts/com.pagevault.host.json` on macOS. On Windows, the per-user `Software\Mozilla\NativeMessagingHosts\com.pagevault.host` registry key points to the manifest. Firefox Snap/Flatpak confinement may prevent access to a host installed outside its sandbox; use a Firefox distribution with Native Messaging access or configure the distribution's supported integration.

If the helper is unavailable, check the browser registration, extension ID, helper version and database path, then restart the browser. Run the installed helper with `diagnose` for details. A helper from the bundle needs `--db PATH` to diagnose a custom archive; diagnostics run in a separate process from the browser session.

For custom paths, use the helper's `install --help`. It supports `--runtime-dir`, `--manifest-dir`, `--extension-id`, `--db` and `--dry-run`. When changing the helper directory, `--preserve-registered-db` retains the registered database path. The PKG uses this option. An invalid registration stops the update; repair it with an explicit `--db` pointing to the existing archive. Old custom helper directories are not removed.
