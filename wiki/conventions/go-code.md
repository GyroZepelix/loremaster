# Go Code Conventions

## Package boundaries

- Keep Cobra command construction and cross-package workflow orchestration in `cmd/` (`cmd/root.go`, `cmd/init.go`, `cmd/sync.go`).
- Keep reusable domain logic in focused `internal/` packages. The repository has no public library package.
- Use narrow interfaces at external boundaries. `internal/git.Fetcher` allows the sync flow to use the system Git adapter, go-git, or test doubles.

## Formatting and naming

- Format Go source with `gofmt`; `README.md` documents formatting all files under `cmd` and `internal`.
- Provider names, resource names, profile names, and manifest paths are lowercase string contracts. Preserve the validation and slash-normalization paths in `internal/config/`.
- Prefer resource-neutral names such as `Item`, `Resource`, and `ConfigRoot`; `skills` remains a compatibility-specific resource contract (`internal/config/config.go`).

## Errors and output

- Wrap causal errors with operation context using `fmt.Errorf("...: %w", err)` at package boundaries. Production code repeatedly follows this pattern across `cmd/` and `internal/`.
- Include provider, resource, source item, and destination context in per-item sync errors (`internal/sync/sync.go`).
- Preserve partial failure isolation: collect independent item/source failures, persist safe successes, then return a non-nil summary error (`cmd/sync.go`).
- Warnings go to stderr; normal completion summaries go to stdout (`cmd/sync.go`).

## Determinism and persistence

- Sort manifest items, warning lists, result paths, and managed ignore entries before persistence or reporting (`cmd/sync.go`, `internal/manifest/manifest.go`, `internal/gitignore/gitignore.go`).
- Use same-directory temporary files plus rename for state that must commit atomically (`internal/manifest/manifest.go`).
- Do not weaken strict YAML field decoding or path validation to make malformed input appear successful.
