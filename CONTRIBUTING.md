# Contributing

Open an issue before substantial changes. Bug reports should include reproduction steps, PageVault version, OS and browser. Remove private URLs, captures, credentials and personal paths. Report vulnerabilities through the [security policy](SECURITY.md).

Use Go 1.27.3, Node.js 26.12.1 and npm 12.2.1. Install development dependencies and the test browser:

```sh
npm ci
npx playwright install chromium
```

On Linux, use `npx playwright install --with-deps chromium` to install browser system dependencies. Run the [verification checks](docs/VERIFICATION.md) before submitting a pull request. Describe the behavior changed and checks performed. Add regression tests for behavior changes and retain multilingual fixtures.

Keep generated files out of version control. Release output belongs in `bin/` and `dist/`; disposable local reports belong in `.test-artifacts/`. Tests should create temporary data and remove it when finished.
