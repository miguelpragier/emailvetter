# Contributing

Open an issue for bugs or proposed features. Include a small reproducer, expected
behavior, and the application version. Remove credentials, personal email
addresses, and private infrastructure details from reports and fixtures.

For local setup and configuration, follow the README. Before submitting a pull
request, run:

```sh
task check
task gleak
task db:up
task test:integration
```

Integration tests modify their database: use a dedicated development instance.
Add focused tests for changes in behavior and format Go files with `gofmt`.
CI also runs the race detector and scans Git history for secrets. Run
`task release:check` with GoReleaser v2 to verify release packaging. When updating
runtime dependencies or the Go toolchain, update the bundled license texts and
THIRD_PARTY_NOTICES.md as needed.

Use Conventional Commits, for example `fix: reject invalid domains` or
`feat: add import validation`. Mark incompatible changes with `!` after the type
or scope and explain them in a `BREAKING CHANGE:` footer. Maintainers should use
squash merges with a semantic commit message; that message determines the release.
Changes limited to documentation, tests, or maintenance do not normally release.

Keep changes portable. Examples should use loopback addresses and reserved
example domains, and configuration must come from documented environment variables.
Do not commit `.env`, credentials, logs, database files, or environment-specific
manifests.

Do not copy domain lists into this repository without documenting the upstream
URL, exact revision, license, required notices, and modifications in
THIRD_PARTY_NOTICES.md. Include any required license text. Public availability
alone does not grant redistribution rights. Contributions of original code are
made under the repository's MIT license; third-party material retains its own
license.
