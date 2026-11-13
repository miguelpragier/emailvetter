# Third-party material

## Domain data

The original seed list was created manually by the project author, as confirmed
by the maintainer. It remains in historical Git revisions under the repository's
MIT license. This version does not bundle or automatically load that seed.
Existing databases are not modified by its removal.

Before adding any external list, record its repository or download URL, exact
commit or release, retrieval date, SPDX license identifier (if available), required
attribution, accompanying license text, and local modifications. Review whether
combining and distributing the data is permitted. Preserve each source's notices
and terms; the code's MIT license does not relicense external data.

Operators importing data through the API are responsible for checking the terms
of that data. No lists are downloaded automatically.

## Software dependencies

The binary includes the Go runtime and standard library under the BSD-style
license in [LICENSES/go.txt](LICENSES/go.txt).

The distributed binary includes these MIT-licensed libraries; their original
license and copyright notices are included in `LICENSES/`:

| Module | Version | License text |
|---|---|---|
| `github.com/caarlos0/env/v11` | v11.4.1 | [caarlos0-env.txt](LICENSES/caarlos0-env.txt) |
| `github.com/danielgtaylor/huma/v2` | v2.40.0 | [huma.txt](LICENSES/huma.txt) |

Direct and transitive Go dependencies and their exact versions are recorded in
`go.mod` and `go.sum`. They retain their upstream licenses. To locate their module
directories and inspect accompanying notices after downloading dependencies:

```sh
go list -m -json all
```

Release tooling and GitHub Actions retain their respective upstream licenses.
