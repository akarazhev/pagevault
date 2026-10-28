# Upgrading and backups

1. Let the extension queue drain; review rejected captures in settings. Note the actual database path.
2. Back up SQLite with the SQLite CLI `.backup` command. Alternatively close all browser, helper and MCP processes and copy the database with any existing `-wal` and `-shm` files. Copying only the database during writes can omit WAL data.
3. Fully quit the browser, including background processes, and stop MCP clients. Replace the bundle while keeping the extension directory path and the installed extension, which owns settings and the queue.
4. Run the new installer for the same browsers. Existing configured database paths are preserved; explicitly pass the existing path with `--db` or `-Db` if needed. Refresh the extension on the browser extensions page, restart clients, and verify helper version and database path in settings.

Do not delete or recreate the archive during upgrade. SQLite schema remains version 1. An extension removal deletes its browser-owned queue; it does not delete the SQLite archive. See [installation](INSTALLATION.md) and [queue recovery](PRODUCTION_READINESS.md).
