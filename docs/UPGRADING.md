# Upgrading and backups

1. Wait for the capture queue to empty and review rejected entries in settings. Note the database path.
2. Back up the archive with SQLite's `.backup` command. Alternatively, stop all browser, helper and MCP processes and copy the database with any `-wal` and `-shm` files. Copying only the database during writes can miss recent data.
3. Fully quit the browser, including background processes, and stop MCP clients. Replace the bundle while keeping the extension folder at the same path. Keep the extension installed to retain its settings and queue.
4. Run the new installer for the same browsers. Use the existing helper directory or `--preserve-registered-db` to retain a custom registration's database path. You can also pass the path explicitly with `--db` or `-Db`.
5. Reload the extension on the browser's extensions page, restart clients and check the helper version and database path in settings.

Firefox packages use the stable add-on ID `pagevault@pagevault.local`. Update a signed XPI in place to retain its queue and settings; temporary development installations are removed at browser shutdown. Install or upgrade the helper with the `firefox` browser option. Chromium's extension key and ID are unchanged. If another browser uses a custom archive, explicitly select that same database when first installing Firefox; per-browser configuration is not copied automatically.

Version 1.5 uses SQLite schema 1. Upgrading does not require recreating the archive. Removing the extension deletes its queue and settings but leaves the SQLite archive.

See [installation](INSTALLATION.md) and [queue recovery](PRODUCTION_READINESS.md#queue-recovery).
