# Sync Safety Conventions

## Validate before filesystem work

- Parse exact relative resource and include paths in `internal/config/` and reject escapes, globs, control characters, backslashes, colons, duplicate destinations, and parent-child overlaps.
- Resolve source symlinks and prove the result remains inside the source root before reading or linking (`internal/sync/sync.go`).
- Reject symlinked destination parents and paths outside the provider resource root.

## Ownership is required

- Treat `.lore-manifest.yml` as the authority to replace or remove an existing destination.
- Never overwrite an unmanaged destination or one owned by another profile.
- Release active-profile ownership without deleting a path that another profile still owns.
- Preserve ownership and `.gitignore` entries when desired content fails to fetch, sync, or verify (`cmd/sync.go`).

A missing or corrupt manifest does not grant ownership. Existing destinations remain unmanaged (`cmd/sync.go`, `internal/manifest/manifest.go`).

## Verify current state

- For soft links, verify that the destination is still a symlink and matches the recorded target when available.
- For hard copies, verify kind and checksum before replacement or deletion. Version 2 checksums include file content, permissions, directory structure, empty directories, and symlink targets (`internal/sync/linker.go`).
- Migrate version 1 entries conservatively. Missing legacy items may be dropped or recreated, but present unverifiable entries stop migration (`cmd/sync.go`, `internal/sync/sync.go`).

## Commit or roll back

Filesystem replacement and removal use sibling staging and backup paths. `cmd/sync.go` saves the updated manifest before deleting backups. If manifest persistence fails, call `RollbackChanges` in reverse order and leave the prior destinations in place (`internal/sync/linker.go`).

Reconcile `.gitignore` only from final manifest ownership. Keep `.lore-manifest.yml` ignored, sort entries, and preserve user-authored content outside the `# Managed by loremaster` section (`internal/gitignore/gitignore.go`).

## Reporting and cache safety

- **Strip URL user information only at the display boundary** - Repository success output removes parsed URL usernames and authentication material without changing the source string passed to clone or fetch (`cmd/sync.go`). `Verified` ([session](../dreams/2026-07-23-2017-sync-reporting-and-wiki-integration.md))
- **Never force checkout over a dirty cached worktree** - A same-branch no-op preserves local cache changes; revision movement returns a clear error before changing the local branch, and a later checkout failure restores the prior ref (`internal/git/git.go`). `Verified` ([session](../dreams/2026-07-23-2017-sync-reporting-and-wiki-integration.md))

## Concurrency

Concurrent syncs targeting the same project are unsupported because manifest and `.gitignore` updates are not locked (`README.md`). Do not imply concurrency safety without adding and verifying a repository-wide locking design.
