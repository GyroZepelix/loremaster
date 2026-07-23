# Repository Map

## Entry points and packages

| Path | Responsibility | Change here when |
| --- | --- | --- |
| `cmd/lore/main.go` | Process entry point and exit status | Changing process startup only |
| `cmd/root.go` | Root Cobra command, version, completion commands | Adding global CLI metadata or completion behavior |
| `cmd/init.go` | Provider detection and config skeleton creation | Changing `lore init` or generated YAML |
| `cmd/sync.go` | End-to-end sync and prune orchestration | Changing profiles, lifecycle sequencing, manifest commits, or CLI summaries |
| `internal/config/` | YAML decoding, config discovery, profiles, path and collision validation | Changing the `lore.yml` contract |
| `internal/provider/` | Provider registry, detection markers, config roots, resource destinations | Adding or changing provider path semantics |
| `internal/cache/` | XDG/home cache location and URL-derived cache keys | Changing cache ownership or layout |
| `internal/git/` | Source-fetch abstraction and Git implementations | Changing clone, pull, checkout, or authentication behavior |
| `internal/sync/` | Resource resolution, linking/copying, ownership checks, stale reconciliation, rollback | Changing filesystem synchronization or safety invariants |
| `internal/manifest/` | Versioned ownership model, v1 migration input, atomic v2 persistence | Changing local state format or ownership queries |
| `internal/gitignore/` | `# Managed by loremaster` section reconciliation | Changing ignored-path ownership behavior |
| `README.md` | User-facing behavior and commands | Changing CLI, schema, provider paths, or supported workflows |
| `spec/` | Intended changes and historical rationale | Investigating why a behavior was introduced |

## Runtime call path

```text
cmd/lore/main.go
  -> cmd.Execute
  -> runInit or runSync
  -> config + provider + manifest
  -> sync.FetchSources
  -> Syncer.Sync for each provider
  -> manifest.Save
  -> gitignore.SetManagedEntries
```

The production fetcher is `internal/git.ExecGitFetcher`; `internal/git.GoGitFetcher` and test mocks satisfy `internal/git.Fetcher` for alternate and isolated use (`cmd/sync.go`, `internal/git/git.go`).

## Tests

Tests are colocated as `*_test.go`. High-signal suites include:

- `cmd/resource_sync_test.go` and `cmd/resource_lifecycle_test.go` for command-level fan-out, ownership, migration, prune, and rollback.
- `internal/config/resource_test.go` for the dynamic YAML contract.
- `internal/sync/resource_sync_test.go` and `internal/sync/linker_test.go` for path containment, mapping transitions, checksums, and overwrite refusal.
- `internal/manifest/v2_test.go` for manifest compatibility and validation.

## Common change paths

- New provider: update the provider interface implementation and registry, discovery/root tests, README matrix, then command-level fan-out coverage.
- Config schema change: update `internal/config/`, preserve strict source-field validation, and add parser plus sync tests.
- Resource lifecycle change: trace `cmd/sync.go`, `internal/sync/`, `internal/manifest/`, and `internal/gitignore/` together.
- Version change: update `cmd/root.go` and the README version presentation together.
