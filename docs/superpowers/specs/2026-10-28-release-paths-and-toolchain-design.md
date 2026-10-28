# Release paths and current toolchain

The user requested a fix for the reproduced ZIP checksum inclusion bug and current stable language and library versions.

Compare filesystem directory-entry identities before writing output over release inputs or including its checksum. Match directory aliases through filesystem identity, and distinguish separate hard links from aliases of one entry. Use Lstat so replacing an archive or checksum symlink never follows it to its target. Support output directories that do not exist yet, relative paths, Unicode, spaces, and case-insensitive filesystems.

Comparing raw paths and resolving only directory symlinks both miss case aliases; resolving complete filenames changes file-symlink replacement semantics. Filesystem directory-entry identity is the selected implementation approach.

Use Go 1.27.3, Node.js 26.12.1 Current, npm 12.2.1, Playwright 1.64.1, and modernc.org/libc 1.79.1, verified against official release metadata on 2026-10-28. Preserve already-current Go modules. Pin project tooling, update lockfiles and notices, and ship the Node version pin in release sources.

Use temporary toolchain installations. Preserve the archive schema and extension behavior. Test ordinary macOS temporary paths without a TMPDIR workaround, and build all six supported targets. Keep Linux-only test skips visible on macOS.
