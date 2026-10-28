# Contributing

Open an issue to discuss substantial changes and include reproduction steps for bugs. Do not upload private captures, database files, credentials or personal configuration. Security reports belong in [private reporting](https://github.com/akarazhev/pagevault/security/advisories/new).

Use Go 1.27.3, Node 26.12.1, npm 12.2.1 and Playwright Chromium. Run `npm ci`, `go test ./...`, `go vet ./...`, `npm test`, `npm run lint`, `npm run format:check`, and `npm run check`. Browser tests use temporary profiles and data. Keep multilingual fidelity fixtures and add behavior tests for regressions. Explain changes and verification in a pull request. See [verification](docs/VERIFICATION.md) for platform coverage.
