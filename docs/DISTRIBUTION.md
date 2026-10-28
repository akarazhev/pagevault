# Distribution and support

PageVault is free and distributed as named GitHub Release assets, including the full ZIP, standalone extension ZIP, unsigned macOS PKG and their SHA-256 files. The extension is loaded unpacked and upgraded manually. The PKG is not signed or notarized. No store submission or paid account is required. See [installation](INSTALLATION.md) and [upgrading](UPGRADING.md).

For a release, run the local gates in [verification](VERIFICATION.md), build all three artifacts, inspect their content and verify checksums, and require the hosted CI checks on the release commit. Stage a draft, download and verify its assets, then publish the matching tag. Keep previous release assets and tags intact. Do not claim cross-compilation or CI is certification of every hardware and browser combination.

Use GitHub issues for reproducible bugs and feature requests; remove private archive data from reports. For security vulnerabilities use [private reporting](https://github.com/akarazhev/pagevault/security/advisories/new). See [support and readiness](PRODUCTION_READINESS.md) and [contributing](../CONTRIBUTING.md).
