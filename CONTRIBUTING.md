# Contributing

Use the Go version in [go.mod](go.mod) or a newer supported patch. No external
Go modules are currently required. To build the same entry point as the image:

```sh
go build -o labdash ./cmd/labdash
go test -race ./...
go vet ./...
```

To try a local image with [compose.yaml](compose.yaml), build it under the tag
the file uses; Compose then starts it instead of pulling:

```sh
docker build -f Containerfile -t "ghcr.io/vordenken/labdash:$(head -n 1 semver.txt)" .
docker compose up -d
```

CI runs the same checks plus `gofmt`, `govulncheck`, the Node tests and a
Docker smoke test on every pull request.

Static assets under `web/static` are embedded at compile time. Rebuild the
binary or image after frontend changes. For interface changes, check both EN
and DE, including accessible names and dialogs. Use English source messages
and add German translations in [i18n.js](web/static/core/i18n.js); never
translate user-configured content. With a recent Node.js installation you can
run `node --test tests/*.test.mjs` without installing any npm packages.
Use a copy of [demo.json](docs/demo.json) for public screenshots instead
of a real dashboard layout.

## Version and release notes

[semver.txt](semver.txt) starts with the current `X.Y.Z` version, without a
`v` prefix. The second line is `---`, followed by Markdown notes for that
version. Bump the version and update the notes with every application change
shipped in a new build, including builds handed off for deployment. Follow
SemVer and keep the previous notes under `## X.Y.Z`.

Also update the image tag in [compose.yaml](compose.yaml); CI fails if it
does not match. To release, merge to `main` and push the tag `vX.Y.Z`. The
release workflow re-runs CI, checks that the tag matches `semver.txt`,
publishes the multi-architecture image to GHCR and creates a GitHub release
with the notes for that version.

The Docker build reads the first line. The resulting build reports that
version in Settings, the startup log and `/healthz`. An explicit
`--build-arg VERSION=...` overrides it. An unversioned `go build` reports `dev`.

## Checking dependencies

1. Compare `go version` with the [Go releases](https://go.dev/dl/) and run
   `go list -m -u all`. The standard library follows the compiler version;
   it does not appear as a module update. Run the Go team's scanner:

   ```sh
   go run golang.org/x/vuln/cmd/govulncheck@latest ./...
   ```

2. Check both pinned multi-platform image digests in the [Containerfile](Containerfile)
   against the official Go and Alpine tags. The certificate stage installs CA
   certificates and timezone data from that Alpine branch. Rebuild without
   cache to pick up package updates even when a base digest has not changed.
   Scan the shipped image **and** the build stages: findings in a compiler or
   package manager are not necessarily in the final `scratch` image. For
   example, with a locally installed Trivy:

   ```sh
   docker build -f Containerfile --pull --no-cache -t labdash:audit .
   trivy image --scanners vuln labdash:audit
   docker build -f Containerfile --target build -t labdash:audit-build .
   trivy image --scanners vuln labdash:audit-build
   docker build -f Containerfile --target certs -t labdash:audit-certs .
   trivy image --scanners vuln labdash:audit-certs
   ```

3. There is no npm manifest and the frontend loads nothing from other
   origins. Keep it that way: the Content Security Policy in
   [server.go](internal/server/server.go) blocks external scripts, styles,
   images and connections. Dependabot also updates the pinned GitHub Actions.

When reporting a finding, distinguish between source dependencies, build
stages, the shipped binary and an already-running instance. Rebuilding the
repository does not update an existing instance.
