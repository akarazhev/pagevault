# Verification

Use the versions and setup in [contributing](../CONTRIBUTING.md). Tests must use synthetic data and temporary databases, homes and browser profiles.

Run from the repository root:

```sh
go test ./...
go vet ./...
go test -race ./...
npm test
npm run test:firefox
npm run lint
npm run format:check
npm run check
npm audit
go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./cmd/... ./internal/...
```

[CI](../.github/workflows/ci.yml) runs Go and JavaScript tests on Ubuntu, Windows and macOS. Native Messaging end-to-end tests and the race detector run on Linux CI. Windows also checks PowerShell syntax and runs the installer with `-DryRun` in a temporary profile. Build jobs cover amd64 and arm64 on all three operating systems.

Install both Playwright browsers (`npx playwright install chromium firefox`). The Firefox suite repeats extraction, queue, archive UI and onboarding checks in Firefox. Its extension smoke test loads a generated Firefox package in a temporary profile, using a unique test native-host name. Native registration/SQLite browser integration runs only with an isolated Linux home; macOS and Windows runs do not establish this coverage. Runtime smoke checks do not verify Mozilla signing or permanent installation. `PAGEVAULT_FIREFOX_PATH` can select a compatible Firefox executable for diagnostics.

Before release, inspect ZIP and PKG contents and verify each SHA-256 file. On macOS, expand the PKG and test its scripts against a temporary home. Record interactive installer checks separately: package inspection and cross-compilation do not verify installation on every target. See [releases](DISTRIBUTION.md) and [support limits](PRODUCTION_READINESS.md).
