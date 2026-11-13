# emailvetter

Lightweight email and domain vetting API with a customizable disposable-email
blocklist, backed by [rqlite](https://rqlite.io).

Block domains, manage notes, and check individual addresses or batches through an
HTTP API. Blocking a domain also blocks its subdomains. Results are `allowed`,
`blocked`, or `invalid`; `allowed` means the domain is absent from the blocklist,
not that the mailbox exists or can receive mail. No DNS, MX, or SMTP checks are
performed.

## Quick start

Requirements: Git, Go 1.27.3 or newer (as declared in `go.mod`), and a running
rqlite HTTP endpoint. Docker is optional for running rqlite locally.

```sh
git clone https://github.com/miguelpragier/emailvetter.git
cd emailvetter
go build -o bin/emailvetter ./cmd/emailvetter
```

In another terminal, start the local database with Docker Compose:

```sh
docker compose up -d rqlite
docker compose logs -f rqlite
```

The Compose configuration pins rqlite to version 10.6.4 and stores data in a
named volume. `docker compose down` stops it and preserves data; bringing it up
again reuses that data. Use Ctrl+C to stop following logs. The database is exposed
only on loopback. Production deployments need their own storage and backup setup.

To deliberately delete the development database, use `docker compose down -v`.
This removes the volume and all its data. Integration tests should use a dedicated
development database.

Once rqlite is ready, start the API:

```sh
EMAILVETTER_ADDR=127.0.0.1:8080 ./bin/emailvetter
```

Defaults connect to rqlite at `http://127.0.0.1:4001`. On startup, the service
creates missing tables. New databases start with an empty blocklist: add domains
through the API or import a list after checking its license. No third-party domain
list is bundled or downloaded automatically. Upgrading preserves existing entries,
including any domains seeded by earlier versions.

In a third terminal:

```sh
curl --fail-with-body http://127.0.0.1:8080/health
curl --fail-with-body http://127.0.0.1:8080/v1/vet \
  -H 'Content-Type: application/json' \
  -d '{"email":"person@example.com"}'
```

Interactive API documentation is available at `/docs` and the OpenAPI contract at
`/openapi.json`.

## Configuration

Configuration comes from environment variables. The binary does not load `.env`
files itself. See [.env.example](.env.example) for a copyable template.

| Variable | Default | Purpose |
|---|---|---|
| `APP_ENV` | `development` | `production` requires at least one API key |
| `EMAILVETTER_ADDR` | `:8080` | HTTP listen address |
| `EMAILVETTER_RQLITE_ADDR` | `http://127.0.0.1:4001` | rqlite HTTP endpoint |
| `EMAILVETTER_RQLITE_USER` | empty | Optional database Basic Auth username |
| `EMAILVETTER_RQLITE_PASS` | empty | Optional database Basic Auth password |
| `EMAILVETTER_RQLITE_TIMEOUT` | `5s` | Timeout per database request |
| `EMAILVETTER_READ_KEYS` | empty | Comma-separated keys for vetting and registry reads |
| `EMAILVETTER_WRITE_KEYS` | empty | Comma-separated keys for reads and writes |

Without API keys, authentication is disabled in development. Bind to loopback for
local use. In production, set `APP_ENV=production`, configure strong API keys,
serve HTTPS through a reverse proxy, and restrict access to the database. Configure
both database credentials when using rqlite Basic Auth.

## Authentication and API examples

Authenticated requests use `Authorization: Bearer <key>`. Read keys can vet
addresses and read the blocklist; write keys can also modify it. `/health` is
public. Invalid or missing credentials return `401`; insufficient scope returns
`403`.

Set `EMAILVETTER_API_KEY` in your shell to a configured write key for the examples
below. Omit the Authorization header when running locally without authentication.

Block a domain:

```sh
curl --fail-with-body http://127.0.0.1:8080/v1/domains \
  -H "Authorization: Bearer $EMAILVETTER_API_KEY" \
  -H 'Content-Type: application/json' \
  -d '{"domain":"example.test","note":"Example block","added_by":"operator"}'
```

Check an email or domain, supplying exactly one field:

```sh
curl --fail-with-body http://127.0.0.1:8080/v1/vet \
  -H "Authorization: Bearer $EMAILVETTER_API_KEY" \
  -H 'Content-Type: application/json' \
  -d '{"domain":"sub.example.test"}'
```

After adding the domain above, the response is:

```json
{"requested":"sub.example.test","verdict":"blocked","matched_domain":"example.test"}
```

Check a batch (up to 1,000 entries in total):

```sh
curl --fail-with-body http://127.0.0.1:8080/v1/vet/batch \
  -H "Authorization: Bearer $EMAILVETTER_API_KEY" \
  -H 'Content-Type: application/json' \
  -d '{"emails":["person@example.com"],"domains":["example.test"]}'
```

Import domains from a JSON array:

```sh
curl --fail-with-body http://127.0.0.1:8080/v1/domains/import \
  -H "Authorization: Bearer $EMAILVETTER_API_KEY" \
  -H 'Content-Type: application/json' \
  -d '["another.example.test","third.example.test"]'
```

Import also accepts `text/plain` (one domain per line), `text/csv` (domain and
optional note, with an optional `domain,note` header), and structured JSON
(`{"domains":[{"domain":"example.test","note":"Example"}]}`). The import body
limit is 8 MiB. Invalid and duplicate domains are skipped and reported.

Upload a plain text domain list as a file (one domain per line):

```sh
curl --fail-with-body 'http://127.0.0.1:8080/v1/domains/import/file?added_by=operator' \
  -H "Authorization: Bearer $EMAILVETTER_API_KEY" \
  -F 'file=@disposable_email_domains_blocklist.txt;type=text/plain'
```

This endpoint requires a write key and accepts a single multipart field named
`file`. The complete request must fit within 8 MiB. UTF-8 BOM, CRLF and blank
lines are accepted. Empty files are rejected; invalid domains and duplicates
are reported in the same summary as the regular import. Existing entries and
notes are preserved; importing does not remove domains absent from the file.
Check the source list's license before importing or redistributing it.

| Method | Path | Description |
|---|---|---|
| GET | `/health` | Database availability and application version |
| POST | `/v1/vet` | Vet one email or domain |
| POST | `/v1/vet/batch` | Vet emails and domains in a batch |
| GET | `/v1/domains` | List entries; supports `q`, `limit` (1–200), and `offset` |
| GET | `/v1/domains/{domain}` | Get one entry |
| POST | `/v1/domains` | Add an entry |
| PATCH | `/v1/domains/{domain}` | Replace an entry's note |
| DELETE | `/v1/domains/{domain}` | Remove an entry |
| POST | `/v1/domains/import` | Import JSON, CSV, or plain text |
| POST | `/v1/domains/import/file` | Upload a domain-per-line text file |

## Development

[Task](https://taskfile.dev) is optional. `task run` also requires the
[godotenv CLI](https://github.com/joho/godotenv):

```sh
go install github.com/joho/godotenv/cmd/godotenv@latest
cp .env.example .env
# Edit .env as needed and ensure the Go binary installation directory is in PATH.
task run
```

The `.env` file must exist for `task run`. Keep it out of version control.

```sh
task db:up            # Start persistent local rqlite
task db:down          # Stop rqlite, preserving data
task db:logs          # Follow database logs
task build            # Compile to bin/emailvetter
task test             # Unit tests; integration tests skip without a database URL
task vet              # Go vet and fail on unformatted Go files
task check            # Vet, build, and test
task gleak            # Scan current files; requires Gitleaks in PATH
task test:integration # Integration tests against 127.0.0.1:4001
```

Integration tests modify database contents: use a dedicated development database.
To select another endpoint or run without Task:

```sh
go test ./...
EMAILVETTER_RQLITE_ADDR=http://127.0.0.1:4001 \
  go test -run Integration -count=1 ./internal/...
```

`task vet` fails when Go files need formatting. Run `gofmt -w` on the listed
files before submitting changes.

## Related projects

If you're exploring disposable-email domain lists, these related projects are
also well worth a visit:

- [ivolo/disposable-email-domains](https://github.com/ivolo/disposable-email-domains)
- [disposable-email-domains/disposable-email-domains](https://github.com/disposable-email-domains/disposable-email-domains)

Thank you to their maintainers and contributors for sharing their work with the
community!

## License

The original project code is licensed under the
[MIT License](https://github.com/miguelpragier/emailvetter/blob/main/LICENSE).
Third-party domain lists require separate review of their source licenses before
incorporation; this license does not relicense third-party data.

## Continuous integration

GitHub Actions checks pull requests and pushes to `main` for formatting, Go vet,
builds, unit and integration tests with the race detector, and secrets. Integration
tests use a temporary rqlite service; no external database credentials are needed.
The secrets job checks Git history with Gitleaks. CI also validates the GoReleaser
configuration and builds all six release archives without publishing.

## Releases

After the CI checks pass on a push to `main`,
[go-semantic-release](https://github.com/go-semantic-release/semantic-release)
calculates the version and release notes from Conventional Commits in dry-run mode.
[GoReleaser](https://goreleaser.com/) builds archives for Linux, macOS, and Windows (amd64 and arm64), with SHA-256
checksums and publishes the GitHub release after uploading the assets. There are no npm dependencies. The release version is included in
`/health` and OpenAPI; local builds report `dev`.

- `fix: correct domain matching` produces a patch release.
- `feat: add an endpoint` produces a minor release.
- `feat!: change the response format` or a `BREAKING CHANGE:` footer produces a major release.
- Changes such as `docs:`, `test:`, and `chore:` alone do not produce a release.

The first release starts at `1.0.0`. Use squash merges with a Conventional Commit
message, or ensure individual commits follow the convention when retaining them.
Release notes live in GitHub Releases; no generated version commit is pushed back
to `main`. The workflow uses the built-in `GITHUB_TOKEN` with `contents: write`
only for publishing; it requires no personal access token. The release and its
assets are produced in the same workflow, without depending on a tag-triggered run.

For a local packaging check with GoReleaser v2.20.0 installed:

```sh
task release:check
```

See [CONTRIBUTING.md](CONTRIBUTING.md), [SECURITY.md](SECURITY.md), and
[THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md) before contributing or importing
external data.

If a release exists but its asset upload failed, run the CI workflow manually from
`main`, setting `release_tag` to that existing tag (for example `v1.0.0`). This
rebuilds that tag and uploads its assets without calculating a new version. Both
validation and publication use GoReleaser v2.20.0. Completed immutable releases
cannot be modified; this recovery path is for releases that still permit uploads.
