# Development

## Prerequisites and layout

The repository declares Go 1.24 in `go.mod`. Production Git operations shell out to `git`, so command-level Git workflows require the system executable and the user's existing authentication configuration (`internal/git/exec.go`).

Domain code belongs under `internal/`; Cobra wiring belongs under `cmd/`. Tests are colocated with their packages.

## Documented workflows

`README.md` documents these source workflows:

```bash
gofmt -w $(find cmd internal -type f -name '*.go')
go test ./... -count=1
go vet ./...
go test -race ./...
go build ./cmd/lore
```

The README also documents installation with:

```bash
go install github.com/GyroZepelix/loremaster/cmd/lore@latest
```

Tests and builds can write Go caches or artifacts and were not executed during the initial wiki ingest because the ingest scope prohibited them.

## Verification strategy

- Parser and path-contract changes: start with `internal/config/` tests.
- Provider path changes: use `internal/provider/` tests and command-level provider fan-out cases.
- Ownership, replacement, stale cleanup, or migration changes: cover `internal/sync/`, `internal/manifest/`, and `cmd/` together.
- Git behavior: `internal/git/exec_test.go` exercises the system-Git adapter against temporary local repositories; `internal/git/git_test.go` covers the go-git implementation.
- Filesystem tests should isolate paths and environment through `t.TempDir()` and `t.Setenv()`.

No CI workflow, Makefile, task runner, linter configuration, migration directory, or generator is tracked at analysis commit `22c4d4e2f53292a8b640274380a82707cd86c3d7` (`git ls-files`). Runtime manifest v1-to-v2 migration is implemented in `cmd/sync.go` and `internal/manifest/manifest.go`; it is not a separate operator command.

## Initial ingest checks

Executed on 2026-07-23:

```bash
gofmt -d $(git ls-files '*.go')
```

Result: passed with no diff. The command definition was inspected first; `-d` displays differences without rewriting files.

Unverified during ingest: unit tests, race tests, vet, builds, application commands, and Git/network flows. They were skipped by explicit ingest safety constraints, not because of known failures.

## Post-ingest verification

- **Sync reporting changes passed the full local matrix** - Focused tests, `go test ./... -count=1`, `go vet ./...`, `go test -race ./...`, formatting/diff checks, a temporary-output build, and disposable Git acceptance passed before commits `95614b5` and `923f579`. `Verified` ([session](./dreams/2026-07-23-2017-sync-reporting-and-wiki-integration.md))
- **The wiki merge preserved the test baseline** - `go test ./...` passed after merge commit `c442d9c`; no remote CI, push, release, or production install was performed. `Verified` ([session](./dreams/2026-07-23-2017-sync-reporting-and-wiki-integration.md))

## Release and version notes

The CLI version is a source variable in `cmd/root.go`; the README also displays the current version. Keep both aligned. There is no tracked release automation or deployment configuration. `README.md` is the source for build and installation instructions.
