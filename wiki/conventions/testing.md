# Testing Conventions

## Structure

- Tests use Go's standard `testing` package and live beside implementation files as `*_test.go`.
- Table-driven cases and `t.Run` are common for validators, provider matrices, and failure variants (`internal/config/resource_test.go`, `internal/provider/provider_test.go`).
- Assertions use direct conditionals with `t.Fatal`, `t.Fatalf`, and `t.Errorf`; no assertion framework is declared.

## Isolation

- Use `t.TempDir()` for source trees, project roots, cache roots, manifests, and Git repositories.
- Use `t.Setenv()` for `HOME` and `XDG_DATA_HOME` when behavior depends on home or cache resolution.
- Command tests that change the working directory restore it with `t.Cleanup()` (`cmd/resource_sync_test.go`).
- Reset package-level Cobra flag state with cleanup when invoking command handlers directly (`cmd/resource_lifecycle_test.go`).
- Prefer local source directories or temporary local Git repositories. Do not make tests depend on remote services.

## Coverage expectations

- Add parser tests for both valid compatibility input and unsafe or ambiguous input.
- Pair provider path changes with project and home-root cases.
- Treat ownership and data-loss prevention as command-level concerns: test manifest contents, filesystem state, `.gitignore`, and rollback together.
- For sync changes, cover idempotence, partial failure, stale removal, unmanaged targets, cross-profile ownership, modified hard copies, and transactional failure where applicable (`internal/sync/resource_sync_test.go`, `cmd/resource_lifecycle_test.go`).
- Manifest format changes require load, save, malformed input, prior-version migration, and deterministic round-trip coverage (`internal/manifest/v2_test.go`).

The documented full verification command is `go test ./... -count=1`; race-sensitive changes should also use `go test -race ./...` (`README.md`).
