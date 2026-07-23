# Architecture

## System boundaries

Loremaster is a single-process CLI. Its inputs are a working directory, one profile-specific YAML config, local or Git source trees, environment-derived cache paths, and any existing manifest/provider files. Its outputs are provider resource paths, `.lore-manifest.yml`, the managed `.gitignore` section, terminal diagnostics, and an exit status (`cmd/sync.go`).

External boundaries are:

- The filesystem for configuration, cache, provider roots, symlinks, copies, backups, and atomic state writes.
- The system `git` command for production clone, pull, fetch, and checkout operations (`internal/git/exec.go`).
- User environment values such as `HOME` and `XDG_DATA_HOME` (`internal/cache/cache.go`, `internal/provider/pi.go`).

## Sync flow

1. `cmd/sync.go` resolves `lore.yml` or `lore-<profile>.yml` in the current directory and known provider config directories.
2. `internal/config` decodes ordered dynamic resources, validates providers and exact relative paths, rejects overlaps, and defaults link mode to `soft`.
3. `internal/manifest` loads ownership state. Version 1 path entries are treated as legacy migration input; valid version 2 entries carry provider, resource, mode, kind, checksum, and symlink target.
4. `internal/sync.FetchSources` resolves local directories or fetches each distinct Git source into the shared cache.
5. `cmd/sync.go` creates one `Syncer` per configured provider. Each sync resolves literal destinations and isolates item failures.
6. Filesystem replacements and removals remain staged until `manifest.Save` succeeds. A save failure triggers reverse-order rollback; success commits backup removal (`cmd/sync.go`, `internal/sync/linker.go`).
7. Final manifest ownership drives the sorted managed `.gitignore` entries (`cmd/sync.go`, `internal/gitignore/gitignore.go`).

## Data ownership

| Data | Owner and authority |
| --- | --- |
| `lore.yml`, `lore-<profile>.yml` | User-authored desired state |
| Cache under `$XDG_DATA_HOME/loremaster` or `~/.local/share/loremaster` | Loremaster-managed source working trees |
| Provider resource destinations | User filesystem; mutable only when active-profile ownership and current state are proven |
| `.lore-manifest.yml` | Local ownership and integrity authority, version 2 on save |
| `.gitignore` managed section | Derived from all manifest-owned paths plus `.lore-manifest.yml` |

## Safety invariants

- Resource and include paths must be clean relative paths without escape, backslashes, colons, controls, or glob characters (`internal/config/include.go`).
- Resolved source includes cannot escape their source root through symlinks (`internal/sync/sync.go`).
- Destination parent components cannot be symlinks, and managed removal must remain below the declared provider resource root (`internal/sync/sync.go`).
- Existing unmanaged or differently owned destinations are never overwritten (`internal/sync/linker.go`, `internal/sync/sync.go`).
- Hard copies are replaced or removed only when their checksum still matches. Soft links are verified against recorded type and target (`internal/sync/linker.go`).
- Shared paths are not deleted or unignored while another profile owns them (`cmd/sync.go`, `internal/manifest/manifest.go`).
- Failed desired items retain prior ownership so partial failure does not orphan existing content (`cmd/sync.go`).

## Tradeoffs

- Resource names are literal and provider-agnostic. This keeps transport simple but allows paths a provider may ignore (`README.md`).
- Partial item/source failures do not block unrelated work, but the command returns an error after safe successes and state persistence (`cmd/sync.go`, `internal/sync/sync.go`).
- Cache keys normalize Git URLs and use a truncated SHA-256-derived directory name. The source URL is not stored in the path (`internal/cache/cache.go`).
- Concurrent syncs are unsupported; there is no locking around manifest or `.gitignore` updates (`README.md`).
