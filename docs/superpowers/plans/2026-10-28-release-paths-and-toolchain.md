# Release Paths and Current Toolchain Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix output aliases and verify current stable tooling and dependencies.

**Architecture:** Resolve the output parent directory, preserving the final filename and the existing release whitelist.

**Tech Stack:** Go 1.27.3, Node.js 26.12.1, npm 12.2.1, Playwright 1.64.1, modernc.org/libc 1.79.1.

**Spec:** docs/superpowers/specs/2026-10-28-release-paths-and-toolchain-design.md

## Global Constraints

- Preserve final archive/checksum symlink replacement semantics and source bytes.
- Support relative paths, missing output directories, Unicode, and spaces.
- Pin the exact stable versions listed above; preserve already-current modules.
- Preserve system installations and archive schema; retain Linux-only skips.

## Review Focus

- Source aliases must not be overwritten: add a preservation regression in Task 1.
- Nested missing output directories must be creatable: exercise repeated packaging in Task 1.
- Final archive symlinks must not redirect writes: add a regression in Task 1.
- Relative/default output must still work: retain existing suite coverage.
- Source ZIPs must carry the Node version pin: add a ZIP-content regression in Task 2.

### Task 1: Fix release output aliases

**Files:** internal/release/release.go and internal/release/release_test.go.

**Interfaces:** Preserve release.Run(context.Context, Options) error and its ZIP/checksum outputs.

- [x] Add portable regressions for directory aliases, nested missing directories, source overlap, and final archive symlink replacement.
- [x] Run release tests with ordinary temporary paths. Expected: checksum inclusion and source-overlap assertions fail before the fix.
- [x] Resolve existing output-directory ancestors without following the final filename; propagate filesystem errors.
- [x] Run the full Go race suite and npm test. Expected: zero failures with explicit platform skips.
- [x] Commit as fix: canonicalize release output directories.

### Task 2: Update and verify tooling

**Files:** go.mod, go.sum, package.json, package-lock.json, .nvmrc, README.md, THIRD_PARTY_NOTICES.md, docs/VERIFICATION.md, internal/release/release.go, internal/release/release_test.go.

**Interfaces:** Consume fixed release.Run; produce pinned tooling and a six-platform release ZIP.

- [x] Add a Node-version-pin ZIP-content regression. Expected: fail before updating the release whitelist.
- [x] Pin versions, regenerate lockfiles with package tools, ship .nvmrc, and update documentation/notices.
- [x] Run Go race tests, vet, module verification, npm test/check, and shell syntax checks with pinned tools. Expected: zero failures with explicit platform skips.
- [x] Build all six targets, verify ZIP/checksums, and smoke-test the macOS helper with isolated SQLite.
- [x] Record actual results and commit as chore: use current stable toolchains and dependencies.
