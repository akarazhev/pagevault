# Release paths and current toolchain

The user requested a fix for the reproduced ZIP checksum inclusion bug and current stable language and library versions.

Resolve existing ancestors of the output directory before comparing output paths with release inputs. Preserve the final filename so replacing an archive or checksum symlink never follows it to its target. Support output directories that do not exist yet, relative paths, Unicode, and spaces.

Comparing raw paths misses directory aliases; resolving the complete output filename changes file-symlink replacement semantics. Resolving the parent directory is the selected implementation approach.

Use Go 1.27.3, Node.js 26.12.1 Current, npm 12.2.1, Playwright 1.64.1, and modernc.org/libc 1.79.1, verified against official release metadata on 2026-10-28. Preserve already-current Go modules. Pin project tooling, update lockfiles and notices, and ship the Node version pin in release sources.

Use temporary toolchain installations. Preserve the archive schema and extension behavior. Test ordinary macOS temporary paths without a TMPDIR workaround, and build all six supported targets. Keep Linux-only test skips visible on macOS.
