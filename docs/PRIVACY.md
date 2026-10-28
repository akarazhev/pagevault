# Privacy

PageVault stores captures in a local SQLite database and queues undelivered captures in the browser's IndexedDB. It does not send archive content to a PageVault service or collect telemetry. Recording is enabled per site with browser permission. Captures can include private page text, URLs, titles and metadata; protect the database, backups and exported JSON accordingly.

Optional sanitized HTML excludes scripts, event handlers and form values, but other page content can still be sensitive. Media references are stored; media files are not downloaded. Disabling recording does not erase existing data. Delete individual pages in the archive UI or remove the database after stopping the helper; deleting the extension also removes its queue and settings.

MCP provides read-only access to the selected archive to the client running it. If that client uses a cloud model, retrieved content can be sent to the model provider. Local storage alone does not prevent that disclosure. See [MCP](MCP.md).
