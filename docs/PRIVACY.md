# Privacy

PageVault stores captures in local SQLite and queues undelivered captures in the browser's IndexedDB. It has no telemetry or remote collection service. Recording requires permission for each site.

Captures may include private text, URLs, titles and metadata. Optional HTML excludes scripts, event handlers and form values, but can still contain sensitive content. Media references are saved; media files are not downloaded. Protect the database, backups and exports accordingly.

Firefox declares `browsingActivity` and `websiteContent` data permissions because Native Messaging transfers URLs and captured content out of the browser to the local helper. These declarations do not enable network transmission. Recording remains disabled until enabled for a site. Each browser keeps its own site permissions, settings and pending queue even when browsers share the same SQLite archive.

Disabling recording keeps saved data. Delete pages in the archive UI, or stop all browser, helper and MCP processes before removing the database. Removing the extension deletes its queue and settings.

An MCP client can read the entire selected archive. A client using a cloud model may send retrieved content to its provider. See [MCP access](MCP.md).
