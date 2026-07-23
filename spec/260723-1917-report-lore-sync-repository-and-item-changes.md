# Report Lore Sync Repository and Item Changes

## Purpose

Make `lore sync` clearly report whether work occurred, which repositories were cloned or fast-forwarded, and which managed destination items were added, updated, or deleted. The result should let users distinguish a true no-op from repository updates and synchronized resource changes without weakening existing sync safety or failure behavior.

## Context

- Loremaster is a Go 1.24 CLI. `cmd/sync.go` currently orchestrates source fetching, provider sync, manifest persistence, transactional commit/rollback, and a single success line: `Synced <items> items from <sources> sources`.
- `internal/sync/sync.go` deduplicates sources in `FetchSources`, calls the internal Git fetcher, and returns `SyncResult`. `SyncResult` records all successfully reconciled items and stale ownership removals, but does not classify semantic changes.
- `internal/git/git.go` defines the internal `Fetcher` interface and go-git implementation. `internal/git/exec.go` provides the system-Git implementation used by `lore sync`. Neither currently returns clone, no-op, fast-forward, commit, or changed-path metadata.
- `internal/sync/linker.go` transactionally replaces an existing managed destination on each sync, even when its effective state is unchanged. Transactional `Change` records therefore cannot be used as user-visible semantic changes.
- Manifest version 2 in `internal/manifest/manifest.go` already records enough metadata to compare existing and resulting hard copies, link targets, modes, and kinds. Avoid a manifest schema change by using Git commit-range changed paths for effective content changes beneath stable soft links.
- There is no repository wiki. Relevant local guidance is `README.md`, including the documented sync lifecycle and development commands.
- Baseline checks on 2026-07-23 passed: `go test ./... -count=1` and `go vet ./...`.
- Git documentation confirms that `git pull --ff-only` only updates on a fast-forward, `git rev-parse HEAD` identifies the current commit, and commit-range `git diff --name-status` can supply added, modified, deleted, and rename paths. The pinned go-git API exposes tree diffs with before and after paths.
- Research sources:
  - https://git-scm.com/docs/git-pull
  - https://git-scm.com/docs/git-rev-parse/2.45.0.html
  - https://git-scm.com/docs/git-diff.html
  - https://pkg.go.dev/github.com/go-git/go-git/v5/plumbing/object

## Requirements

- Replace the count-only normal-sync success output with deterministic repository and managed-item status output.
- Report each first-time repository clone with its source and resulting full commit ID.
- Report each successful repository fast-forward with its source and full old and new commit IDs.
- Do not list unchanged repositories individually.
- Report successful managed destination changes at item granularity, using paths such as `.pi/skills/foo` or `.claude/prompts/review.md`, labeled `added`, `updated`, or `deleted`.
- Classify `added` only when the active profile successfully installs a newly managed destination.
- Classify `updated` when an existing managed item's effective synchronized state changes, including mode, kind, target, hard-copy checksum, or source content within a configured include changed by the effective Git revision transition.
- For a configured file include, match the exact changed source path. For a configured directory include, match the directory itself or descendants. Consider both sides of renames so moving a path into or out of an include marks the destination updated.
- A Git change outside every configured include must not mark a synchronized item updated. A source change must only be reported for a destination whose sync succeeded.
- Classify `deleted` only when the managed destination was actually removed. Do not label shared-ownership release, an already-absent destination, or a preserved modified/unverifiable item as deleted.
- Deduplicate item statuses and sort repositories by source and items by normalized destination path. Use a stable status order within each section.
- Show repository and item sections only when they contain entries. If there are no clones, fast-forwards, additions, updates, or deletions, print exactly one clear all-unchanged summary.
- A clone by itself is repository activity, but it must not falsely mark an already-owned item updated. Newly installed destinations still report as added.
- Render successful repository and item changes before returning a partial-sync error. Preserve existing warnings on stderr, safe partial-success behavior, rollback semantics, and nonzero failure exits.
- Keep source-fetch deduplication across resources and providers. A repository fetched once must produce one repository status while all successfully affected provider destinations can be reported.
- Update user documentation to describe the new normal-sync output and no-change behavior.

## Out of Scope

- Per-commit logs, commit messages, patches, or nested file-by-file output inside a managed directory.
- Reporting every unchanged repository or managed item.
- Changing prune output, init output, completion behavior, configuration syntax, provider path rules, or ownership policy.
- Persisting source-content fingerprints in the manifest or changing the manifest version solely for this feature.
- Treating live content edits under a stable soft link to a local, non-Git source as a sync mutation. Link target or configuration changes remain reportable; repository revision changes are the required effective-content case.
- Refactoring unrelated Git error handling, including the existing detached/diverged/offline cache behavior, unless a focused correction is necessary to return truthful status metadata.
- Dependency additions, migrations, broad refactors, unrelated cleanup, commits, pushes, production actions, or external service changes.

## Assumptions

- Full commit IDs are preferred over abbreviated IDs because they are deterministic and unambiguous across both Git implementations.
- Repository `fast-forwarded` status describes the successful pull/update phase. A ref checkout or revision transition that is not a fast-forward may still cause affected managed items to be `updated`, but must not be mislabeled as a fast-forward.
- The Git result model may need to retain pre-operation, post-pull, and final post-checkout revisions so repository status and effective changed paths remain accurate when `ref` is configured.
- Semantic item status is independent from `internal/sync.Change`: identical relinking or copying during an otherwise no-op sync must not produce user-visible `updated` output.
- Existing Cobra and direct `runSync` tests should remain supported by routing normal output through a testable writer or rendering helper, with an `os.Stdout` fallback where current direct invocation requires it.
- No implementation subagent is necessary by default because the affected flow is bounded to the Git adapter, sync result modeling, command rendering, and focused tests. An independent review subagent is optional after implementation.

## Plan

1. Define repository update metadata in `internal/git/git.go`.
   - Add an internal structured result that can distinguish cloned, unchanged, and fast-forwarded repositories; retain the relevant pre-operation, post-pull, and final revisions; and carry normalized changed paths for the effective old-to-final revision transition.
   - Adjust the internal `Fetcher` contract so one repository operation can return partial status metadata with an error, including the optional checkout phase. Keep this API internal and avoid exposing command-formatting concerns in the Git package.
   - Preserve the existing clone, pull, checkout, warm-cache, and cleanup behavior while making status truthfully observable.
   - Expected result: callers can tell whether a repository cloned or fast-forwarded, display commit IDs, and determine which source paths changed without parsing human Git output.

2. Implement and test status collection for both Git backends in `internal/git/exec.go`, `internal/git/git.go`, `internal/git/exec_test.go`, and `internal/git/git_test.go`.
   - For system Git, capture revisions with machine-readable Git commands, retain the current `pull --ff-only` behavior, and parse NUL-safe commit-range name/status output. Include both old and new paths for renames.
   - For go-git, capture HEAD hashes around clone/pull/checkout and derive changed paths from old/new commit tree diffs, retaining both populated sides of each change.
   - Return clone metadata after a successful first clone, unchanged metadata for an up-to-date repository, and old/new IDs plus changed paths after a successful fast-forward. Do not report a failed or non-fast-forward operation as successful.
   - Add focused tests for clone, no-op pull, fast-forward, commit IDs, modified/added/deleted paths, rename path coverage where practical, configured ref transitions, and failure metadata.
   - Expected result: both implementations satisfy the same deterministic status contract and existing fetch behavior remains covered.

3. Propagate deduplicated source update metadata through `internal/sync/sync.go` and its tests.
   - Extend `FetchSources` to return one structured source result per distinct Git source alongside resolved base directories and existing error reporting. Local sources should remain resolvable without fabricated repository status.
   - Add a small path-overlap matcher for exact file includes and directory descendants using normalized slash-separated repository paths. Its input must support both sides of a Git rename.
   - Make effective changed paths available while each configured include is synchronized so only successful destination items can receive source-driven `updated` status.
   - Update `mockFetcher` and all affected `FetchSources` call sites in `internal/sync/sync_test.go` and `internal/sync/resource_sync_test.go`, preserving fetch deduplication assertions.
   - Expected result: one fetched repository status can drive accurate update classification for every affected resource/provider destination without duplicate repository lines.

4. Add semantic managed-item classification in `internal/sync/sync.go`, `internal/sync/linker.go` only if needed, and focused sync tests.
   - Introduce a user-facing-neutral item change record in `SyncResult`, separate from rollback `Change`, with status and normalized destination path.
   - Mark successful newly owned destinations as added. Compare prior manifest metadata with the newly produced item to detect meaningful mode, kind, target, and hard checksum updates.
   - Mark an existing successfully synchronized soft item updated when its configured Git include intersects effective changed paths, even if the symlink target string is stable.
   - Record deletion only after a managed destination was actually staged or removed. Keep ownership-only releases and preserved items out of deleted output.
   - Handle stale reconciliation, ancestor/descendant mapping transitions, and removed-provider cleanup without double reporting. Extend `reconcileRemovedProviderItems` to return actual removed paths separately from warnings and transactional rollback data.
   - Sort and deduplicate semantic changes before returning them.
   - Add tests for first sync additions, repeated no-op sync, hard content update, soft-linked Git content update, unrelated repository changes, exact-file and directory include matching, actual deletion, shared ownership release, preserved modified hard copy, provider removal, and mapping transitions.
   - Expected result: semantic statuses describe observable managed-item changes rather than internal replacement mechanics.

5. Aggregate and render the new output in `cmd/sync.go` with command-level tests in `cmd/sync_test.go` and related resource lifecycle tests.
   - Snapshot prior profile items before reconciliation, collect repository results once, and aggregate item changes across providers and removed-provider cleanup.
   - Add a small deterministic rendering function that writes to an injected `io.Writer`. Use the Cobra output writer when available and preserve compatibility with direct `runSync` calls in existing tests.
   - Render non-empty repository and item sections with explicit status labels, source/path, and commit IDs. Omit empty sections.
   - Print one all-unchanged message only when there is no clone, fast-forward, addition, update, or deletion.
   - Render successful status before the existing partial-error return; leave warnings and detailed errors on stderr.
   - Add exact-output tests for unchanged, clone, fast-forward, added/updated/deleted ordering, repo update outside configured includes, duplicate-source deduplication, partial success with final error, and no false deletion for shared/preserved items.
   - Expected result: `lore sync` communicates all confirmed activity and produces one concise no-op message when nothing changed.

6. Document the behavior in `README.md`.
   - Update the Commands or How It Works section with concise examples of clone, fast-forward, managed-item changes, and the all-unchanged message.
   - State that directory resources are reported as managed destination items, not expanded nested files, and that warnings/errors retain their existing channels and exit behavior.
   - Expected result: users can interpret the new output without inferring unsupported nested-file or commit-log behavior.

7. Run the complete verification suite and review the diff against the confirmed rules.
   - Format changed Go files, run focused and full tests, vet, race tests, and a temporary-output build.
   - Manually exercise a local source and a disposable local Git remote to confirm first clone, no-op, fast-forward inside an include, fast-forward outside includes, hard-copy update, and deletion output.
   - Confirm no manifest schema change, no temporary lore transaction paths, and no regression to rollback or partial-failure behavior.
   - Expected result: automated and manual checks provide pass/fail evidence for every requested output state.

## Verification

- `gofmt -w <changed-go-files>`
  - Passing signal: `gofmt -d <changed-go-files>` produces no diff.
- `go test ./internal/git ./internal/sync ./cmd -count=1`
  - Passing signal: all focused status, classification, and exact-output tests pass.
- `go test ./... -count=1`
  - Passing signal: every package passes with no failures.
- `go vet ./...`
  - Passing signal: command exits zero with no diagnostics.
- `go test -race ./...`
  - Passing signal: all packages pass and the race detector reports no races.
- `tmp_bin="$(mktemp -d)/lore" && go build -o "$tmp_bin" ./cmd/lore && "$tmp_bin" --version`
  - Passing signal: build exits zero and the temporary binary runs successfully without creating a repository-root binary.
- Manual disposable Git acceptance sequence:
  - First sync prints one cloned repository with its resulting commit and `added` managed destination paths.
  - Immediate second sync prints exactly one all-unchanged summary.
  - A remote commit inside a configured include prints one fast-forward old-to-new pair and `updated` for each successfully affected provider destination.
  - A remote commit only outside configured includes prints the fast-forward but no managed-item update.
  - Removing a configured item prints `deleted` only when the destination was actually removed; shared or modified preserved content is not labeled deleted.
  - A partial failure still prints confirmed successful repository/item changes, emits errors on stderr, and exits nonzero.

## Risks and Blockers

- Git pull and checkout can move HEAD in different phases. Mitigation: capture enough revisions to distinguish actual fast-forward status from effective old-to-final content changes, and test configured refs explicitly.
- Rename detection can miss an affected include if only one path is retained. Mitigation: normalize and retain both before and after paths from name/status or tree changes.
- Transactional replacements happen during no-op syncs. Mitigation: never derive semantic status directly from rollback `Change`; compare manifest state and effective source changes instead.
- `SyncResult.Removed` currently includes ownership releases that do not delete disk content. Mitigation: add an explicit actual-removal signal and test shared ownership and already-absent destinations.
- System Git and go-git may expose diff details differently. Mitigation: define one backend-neutral result contract and run equivalent behavior tests for both implementations.
- Repository status may be available even when checkout or a later item sync fails. Mitigation: retain confirmed partial metadata, print only confirmed successful state transitions, and preserve the command's final nonzero exit.
- Later implementation must stop for approval before destructive operations, dependency changes, migrations, external service changes, commits, pushes, force operations, production actions, or scope expansion. No such action is required by this plan.

## Progress

- [x] Planning complete and saved.
- [x] Implementation complete.
- [x] Focused verification complete.
- [x] Full verification complete.
- [x] Documentation reviewed.

## Decision Log

- Decision: Report managed destination items rather than nested files.
  Rationale: This matches Loremaster's configured include and manifest ownership model while keeping output concise.
  Date/Author: 2026-07-23, PI Agent planning session
- Decision: Treat repository changes beneath stable soft links as managed-item updates.
  Rationale: Users observe changed skill/resource content even when the symlink itself is unchanged.
  Date/Author: 2026-07-23, PI Agent planning session
- Decision: Separate semantic statuses from transactional filesystem changes.
  Rationale: Existing reconciliation replaces managed destinations on no-op runs, so rollback records would create false update reports.
  Date/Author: 2026-07-23, PI Agent planning session
- Decision: Avoid a manifest schema change and persistent source fingerprints.
  Rationale: Git revision diffs cover the confirmed soft-link requirement without adding migration work or hashing every linked directory on every sync.
  Date/Author: 2026-07-23, PI Agent planning session
- Decision: Report only actual disk deletion as `deleted`.
  Rationale: Shared ownership release and preserved content do not represent a changed or deleted synced file.
  Date/Author: 2026-07-23, PI Agent planning session
- Decision: Treat configured ref switches separately from repository fast-forwards and restore the remote default branch when `ref` is cleared.
  Rationale: Switching branches, tags, or commits can change effective synced content without being a pull fast-forward, and cached refs must not silently select the wrong source revision.
  Date/Author: 2026-07-23, PI Agent implementation session

## Execution Handoff

Use PI Agent in a fresh session with this prompt:

    Read the saved plan file path reported by the planning agent.
    Implement it step by step. Before editing, re-read the Requirements, Out of Scope, Risks and Blockers, and Verification sections.
    Keep the saved plan updated as a living document, including Progress and Decision Log entries at stopping points.
    Run every applicable Verification command before reporting done.
    Do not commit, push, run migrations, add dependencies, perform destructive or force operations, change external services, take production actions, or expand scope without explicit approval.
    If PI plan mode is active, use a numbered Plan: section and mark completed implementation steps with [DONE:n].

## Notes

- This plan changes normal `lore sync` reporting only. Keep `lore sync --prune` messages unchanged.
- Prefer focused helper tests for rendering and path matching, plus a small number of real-Git integration tests. Do not make every command test depend on Git subprocesses.
- Preserve normalized forward-slash managed paths in output on every platform.
- Configured-ref transitions are represented by effective before/final changed paths, while repository status records only same-branch fast-forwards. Clearing `ref` restores the remote default branch.
- Final verification passed: focused and full tests, race tests, vet, formatting, diff checks, temporary-output build, and disposable Git CLI acceptance for clone, no-op, inside/outside fast-forwards, and deletion.
