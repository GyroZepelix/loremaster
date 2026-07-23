# Sync Arbitrary Provider Resources

## Purpose

Extend Loremaster from a skill-directory syncer into a provider-resource syncer. A `lore.yml` file will retain `provider` as its only reserved top-level key and treat every other validated top-level key as a literal provider-relative resource path. This enables exact files and directories such as prompts, commands, agents, hooks, or user-defined resource trees to be fetched once and safely synchronized across all configured providers without breaking existing `skills:` configurations.

## Context

- The repository is a Go 1.24 CLI using Cobra and `gopkg.in/yaml.v3`; no new dependency is needed (`go.mod`).
- `internal/config/config.go` currently decodes only `provider` and `skills`, silently ignores unknown top-level keys, requires at least one skill source, and validates refs only within `cfg.Skills`.
- `internal/config/include.go` already parses exact `src` and `src:dst` includes and rejects absolute and escaping paths, but it has no resource-name model or explicit glob rejection.
- `internal/provider/provider.go` and each concrete provider expose only skill roots. Current roots are `.claude/skills`, `.opencode/skills`, `.agents/skills`, project `.pi/skills`, and global `~/.pi/agent/skills`.
- `internal/sync/sync.go` iterates only `cfg.Skills`, requires each include to resolve to a directory, builds destinations through `Provider.SkillDir`, and reconciles stale entries by walking one skill root.
- `internal/sync/linker.go` symlinks directories in soft mode and recursively copies directories in hard mode. Soft mode currently removes any existing target. Hard mode protects directories through an in-directory `.lore-checksum`, so it cannot safely represent hard-copied individual files.
- `internal/manifest/manifest.go` stores manifest version 1 as profile-owned path strings. It lacks item kind, link mode, and checksums needed to protect hard-copied files. Prune and provider-removal logic in `cmd/sync.go` duplicates skill-specific removal rules.
- `internal/gitignore` already accepts arbitrary project-relative paths and should remain the single writer for managed ignore entries.
- There is no repository wiki index. `docs/` is empty; behavior documentation is in `README.md`.
- Pi's installed documentation at `/Users/dgjalic/.bun/install/global/node_modules/@earendil-works/pi-coding-agent/docs/prompt-templates.md` confirms project prompts at `.pi/prompts/*.md`, global prompts at `~/.pi/agent/prompts/*.md`, and non-recursive prompt discovery.
- Claude Code's official documentation lists `.claude/commands/*.md` as its flat single-file prompt mechanism and does not define `.claude/prompts`. Dynamic resource names must therefore remain literal transport paths, not claims of provider compatibility: https://code.claude.com/docs/en/claude-directory and https://code.claude.com/docs/en/skills.
- Planning baseline on 2026-07-23: `go test ./... -count=1` and `go vet ./...` both pass, and the worktree is clean on branch `v0.4.0`.

## Requirements

- Keep `provider` as the only reserved top-level YAML key. Parse every other top-level mapping key, in document order, as a resource containing the existing source objects (`source`, optional `ref`, `include`, optional `type`).
- Preserve existing `skills:` YAML and behavior. `skills` includes must continue to resolve to directories; no existing valid skill configuration should require migration.
- Allow non-skill resources to include exact regular files or directories. Selected directories remain recursive. Preserve `src:dst` remapping for both kinds.
- Do not expand globs. Reject glob metacharacters in resource names and include paths with a clear validation error so `prompts/*` cannot be mistaken for a supported selector.
- Validate dynamic resource names as clean, non-empty relative slash paths. Reject absolute paths, root escape through `..`, backslashes, colons, control characters, and destinations outside the provider configuration root.
- Require at least one non-empty resource section, but allow configurations with no `skills:` section, such as a prompts-only configuration.
- Strictly validate fields inside each source object so misspellings such as `incldue` fail. Document that a validly shaped top-level typo such as `skils:` is necessarily treated as an intentional literal resource name.
- Apply source validation, default `type: soft`, supported type checks, SCP-port diagnostics, and same-source/different-ref rejection across all resources.
- Fetch each distinct source/ref once per command and reuse its resolved base directory across resources and providers. Preserve partial failure isolation.
- Add a provider configuration-root abstraction and resolve destinations as `<config-root>/<resource>/<mapped-include>`:
  - Claude project or home: `.claude/<resource>/<item>`.
  - OpenCode project or home: `.opencode/<resource>/<item>`.
  - Codex project or home: `.agents/<resource>/<item>`; `.codex` remains a config discovery location only.
  - Pi project: `.pi/<resource>/<item>`.
  - Pi when the sync root resolves to `$HOME`: `.pi/agent/<resource>/<item>`.
- Treat resource names literally for every provider. Do not alias `prompts` to Claude `commands`, validate provider consumption, or suppress provider/resource combinations.
- Detect exact and parent-child destination conflicts using the complete logical path `<resource>/<dst>` across all resources and source blocks before writing. The same logical item across different configured providers is expected, not a collision.
- Generalize soft mode to symlink either a file or a directory and hard mode to copy either kind while preserving current recursive-copy and nested-source-symlink safety behavior.
- Refuse to overwrite any existing destination not owned by the active Loremaster profile. Also reject a destination owned by a different profile. An owned destination may be replaced only when its recorded kind/mode and current filesystem state prove it has not been replaced or locally modified.
- Upgrade the manifest to a backward-compatible structured version that records at least project-relative path, provider, resource, mode, kind, and checksum for hard copies. Load version 1 path-string manifests, safely classify legacy skill entries from the filesystem, and save version 2 only after migration. Existing legacy `.lore-checksum` directories must remain verifiable; new hard-file safety data must not create sidecar files inside provider resource directories.
- If the manifest is missing or corrupt, treat existing destinations as unmanaged and preserve them. Retain the current warning for a corrupt manifest.
- Reconcile ownership centrally from manifest entries rather than walking only skill roots. Handle removed items, removed resources, removed providers, orphaned profiles, and soft or hard files/directories through one shared removal policy.
- Remove owned symlinks directly. Remove hard copies only when their current checksum matches the recorded checksum or a verified legacy marker. Preserve modified or unverifiable content, emit a warning, retain its ownership record, and keep its `.gitignore` entry.
- Preserve prior ownership for desired items whose fetch, provider sync, or write failed. A partial success must not drop failed-provider entries from the manifest or make their existing files unowned.
- Remove a `.gitignore` entry only when no manifest profile still owns the path. Sort and deduplicate entries for deterministic output.
- Keep config discovery, profile naming, local/Git source support, ref checkout, and soft/hard defaults otherwise unchanged.
- Replace skill-only CLI summaries and diagnostics with resource-neutral terms such as `items` and `sources`, while including provider, resource, include source, and resolved destination in actionable errors.
- Update `lore init`, root/sync/prune help, and `README.md` to document the dynamic schema, literal path semantics, provider roots, file-versus-directory rules, no-glob rule, overwrite refusal, manifest migration, and provider-specific consumption caveat.

## Out of Scope

- Provider-specific aliases or capability mappings, including translating `prompts:` to Claude's `commands/` directory.
- Glob expansion, recursive source discovery, extension filtering, or automatic prompt/skill format validation.
- Merging into structured destination files such as `settings.json`; each include manages one complete file or directory path.
- Destinations outside a provider's configuration root or absolute destination paths.
- Changing Git authentication, cache layout, profile discovery, config search precedence, or local-source resolution semantics.
- Adding providers, changing current provider directory conventions, or fixing unrelated README discrepancies.
- New dependencies, broad unrelated refactors, commits, pushes, or production/external service actions.

## Assumptions

- `skills` is the only resource with directory-only source semantics. This preserves the current contract while allowing exact files everywhere else.
- Resource keys represent directories beneath a provider configuration root. For example, `hooks/tools:` plus `include: [validate.sh]` produces `.claude/hooks/tools/validate.sh`; managing a provider-root file directly is not part of this schema.
- YAML declaration order should be preserved for deterministic validation, fetching, diagnostics, and tests; an ordered resource slice is preferable to a Go map as the primary model.
- Literal transport is intentional. A successfully synchronized path may still be ignored by the target tool, and README examples must distinguish filesystem success from provider discovery.
- Both `soft` and `hard` continue to be valid for all resources. Structured manifest checksums are the source of truth for new hard copies, while legacy directory markers are migration input.
- Refusing unmanaged overwrite is an intentional safety tightening for existing soft skills as well as new resources; users must remove or relocate conflicts explicitly.
- Current nested source-symlink handling in hard copies remains unchanged unless tests expose a regression directly caused by generic item support.

## Plan

1. Refactor the configuration model and dynamic YAML decoder.
   - Target `internal/config/config.go`, `internal/config/include.go`, `internal/config/config_test.go`, and `internal/config/include_test.go`.
   - Introduce ordered resource and generic source models, while retaining a clear compatibility path for the exact `skills` resource.
   - Decode the top-level YAML mapping through `yaml.Node` so `provider` is handled specially and all other keys become ordered resources. Decode each source object strictly against the allowed fields.
   - Centralize relative path validation for resource names and include source/destination paths. Add explicit glob-metacharacter rejection and full `<resource>/<dst>` overlap validation.
   - Flatten source declarations for ref-conflict validation and later fetch orchestration. Ensure prompts-only and arbitrary-resource-only configs parse successfully.
   - Add table tests for legacy skills, multiple ordered resources, nested resource names, exact files, defaults, empty resources, invalid keys, strict source fields, top-level typo behavior, globs, cross-resource collisions, and same-source/different-ref conflicts.
   - Expected result: the parsed config represents every declared resource deterministically, with unsafe or ambiguous paths rejected before filesystem work.

2. Add generic provider configuration roots without changing current skill destinations.
   - Target `internal/provider/provider.go`, `internal/provider/claude.go`, `internal/provider/opencode.go`, `internal/provider/pi.go`, `internal/provider/codex.go`, and `internal/provider/provider_test.go`.
   - Add `ConfigRoot(projectRoot)` and a generic resource destination helper. Derive or preserve `SkillRoot` as `<ConfigRoot>/skills` so existing callers and tests remain correct during the refactor.
   - Keep Pi's real-home comparison as the project/global discriminator. Keep Codex resource output under `.agents` even when config discovery found `.codex/lore.yml`.
   - Add provider matrix tests for project roots, Pi global roots, nested resources, and mapped file destinations.
   - Expected result: all providers resolve the confirmed literal resource paths consistently, and every existing skill path remains unchanged.

3. Introduce manifest version 2 and ownership-aware migration.
   - Target `internal/manifest/manifest.go`, `internal/manifest/manifest_test.go`, and manifest-related helpers in `cmd/sync.go` or a narrowly scoped new internal file if needed to avoid command-layer duplication.
   - Replace profile path strings in the in-memory model with structured entries containing path, provider, resource, mode, kind, and checksum. Keep deterministic ordering.
   - Implement custom loading for both version 1 scalar entries and version 2 structured entries. Reject unsupported future versions clearly rather than treating them as an empty manifest.
   - Add migration/enrichment logic that verifies legacy symlinks and legacy hard directories with `.lore-checksum`; do not infer ownership for arbitrary unmanaged paths. Continue retro-registering only provably managed legacy default skills when no manifest exists.
   - Add lookup helpers for owner-by-path, active-profile entries, cross-profile conflicts, retained entries after failures, and global path ownership used by `.gitignore` cleanup.
   - Test v1 load and v2 save, mixed provider paths, legacy hard markers, corrupted/unsupported manifests, deterministic round trips, cross-profile ownership, and missing filesystem entries.
   - Expected result: file ownership and hard-copy integrity can be proven without placing metadata beside arbitrary provider files, while existing manifests continue to work.

4. Generalize linking, copying, checksums, and safe replacement.
   - Target `internal/sync/linker.go` and focused linker tests in `internal/sync/sync_test.go` or a new `internal/sync/linker_test.go`.
   - Replace `LinkSkill` with a generic item operation that classifies regular files and directories, creates parent directories, symlinks either kind in soft mode, and dispatches hard copies to file or directory copy logic.
   - Add file and directory checksum helpers. For legacy directories, ignore only the root legacy `.lore-checksum` marker during verification; preserve current behavior for source symlink entries in hard directory copies.
   - Move overwrite authorization out of unconditional `RemoveAll`: require a verified active-profile manifest entry before removal. Verify an existing soft item is still the expected symlink type; verify hard content against its manifest or legacy checksum before replacement.
   - Return an explicit outcome containing synced/skipped state, kind, mode, and checksum so callers never count a protected skip as success.
   - Test soft files, soft directories, hard files, hard directories, hidden/nested content, managed updates, mode changes, unmanaged files/directories/symlinks, tampered owned targets, and locally modified hard copies.
   - Expected result: exact files and directories share one safe transfer primitive, and no generic resource write can silently destroy unowned content.

5. Rework sync orchestration around resources and central reconciliation.
   - Target `internal/sync/sync.go`, `internal/sync/sync_test.go`, `cmd/sync.go`, and `cmd/sync_test.go`.
   - Flatten and deduplicate source fetches across all resources before the provider loop. Keep failures isolated and associate errors with each affected resource item.
   - Build the complete desired destination set for every provider and resource, validate realized collisions, and pass current ownership into the generic item operation.
   - Replace skill-root walking and `cleanRemovedProviders` duplication with one manifest-driven reconciliation helper used by ordinary stale cleanup, removed providers/resources, and `--prune`.
   - Make reconciliation return removed, retained-modified, failed-desired, and newly managed entries. Merge these outcomes into the active profile instead of replacing it only with successful writes.
   - Stop empty-parent cleanup at the provider configuration root. Never remove `.claude`, `.opencode`, `.pi`, `.pi/agent`, or `.agents` itself.
   - Update `.gitignore` once after final ownership reconciliation: add all currently owned paths, remove only paths with no remaining owner, and preserve `.lore-manifest.yml`.
   - Update source/item counts and diagnostics to resource-neutral language. Ensure command errors still return non-zero when any item fails while recording safe partial successes.
   - Add regression tests for resource removal, provider removal, Pi project/global paths, all-provider fanout, cross-profile conflicts, failed fetch retention, partial provider failure retention, stale files/directories, modified hard-copy retention, prune behavior, gitignore shared ownership, and no false success count on a skipped target.
   - Expected result: profiles and cleanup remain safe and deterministic across arbitrary resource roots, files, directories, and partial failures.

6. Update initialization, help, and user documentation.
   - Target `cmd/init.go`, `cmd/root.go`, command help strings in `cmd/sync.go`, their tests, and `README.md`.
   - Keep the generated `skills:` starter but add concise commented examples for `prompts:` and a nested arbitrary resource. Use generic CLI descriptions and summaries.
   - Document the exact YAML grammar, reserved-key rule, resource-name restrictions, source mapping, directory-only skills, exact non-skill files/directories, literal paths, no globs, and conflict behavior.
   - Add a provider resource-root table covering project and home behavior, explicitly noting Pi's `.pi` versus `.pi/agent` split and Codex's `.agents` output root.
   - State that `.claude/prompts` is only a literal directory and is not a Claude Code prompt-discovery location; point users to `commands:` when they want Claude flat commands. State that Pi prompt discovery is non-recursive.
   - Document safe overwrite refusal, hard-copy checksums in the manifest, v1 migration, partial failure behavior, and cleanup ownership.
   - Expected result: users can predict both filesystem output and target-tool consumption without relying on the old skill-only terminology.

7. Format and verify the complete change.
   - Run formatting, focused package tests while iterating, then the full test, vet, race, and build commands below.
   - Run the temporary end-to-end acceptance scenario to prove a single config fans a skill directory, prompt file, and arbitrary file to both global Pi and Claude literal roots.
   - Review the final diff against every requirement and confirm no dependency, cache, config-discovery, or unrelated behavior changed.
   - Expected result: all automated and manual checks pass, legacy skill configs remain valid, and the example produces the confirmed paths without overwriting unmanaged targets.

## Verification

- Format all Go changes:

      gofmt -w $(find cmd internal -type f -name '*.go')

- Run focused tests during implementation:

      go test ./internal/config ./internal/provider ./internal/manifest ./internal/sync ./cmd -count=1

- Run the full regression suite:

      go test ./... -count=1

  Passing signal: every package reports `ok` and no package fails.

- Run static analysis:

      go vet ./...

  Passing signal: exit status 0 with no diagnostics.

- Run race-enabled tests:

      go test -race ./...

  Passing signal: all tests pass with no race report.

- Build the CLI:

      go build ./cmd/lore

  Passing signal: exit status 0.

- Automated acceptance tests must prove:
  - The original `provider: claude` plus `skills:` examples still create `.claude/skills/<name>`.
  - A prompts-only config parses and syncs without `skills:`.
  - `prompts:` with a file include creates `.pi/prompts/<file>` for a project and `~/.pi/agent/prompts/<file>` when the sync root is `$HOME`.
  - The same literal resource creates `.claude/prompts/<file>`, even though Claude does not consume that path.
  - `commands:` creates `.claude/commands/<file>` with no provider aliasing.
  - A nested key such as `hooks/tools:` creates the matching nested path for all four providers.
  - `skills` rejects a file include, non-skill resources accept files and directories, and all resource includes reject globs.
  - Existing unmanaged targets and targets owned by another profile are preserved and reported as conflicts.
  - Modified hard files/directories survive sync, stale reconciliation, provider removal, and prune with ownership and `.gitignore` retained.
  - Version 1 manifests load and migrate without losing existing skill ownership.
  - A failed source/provider retains prior manifest entries while successful items are recorded.

- Manual end-to-end check from a temporary home directory:
  1. Build `lore` into a temporary directory.
  2. Create a local source containing `example-skill/SKILL.md`, `some-prompt.md`, and `tool.json`.
  3. Write `lore.yml` under the temporary home with `provider: [pi, claude]`, `skills:`, `prompts:`, and `hooks/tools:` sections pointing to that source.
  4. Run `HOME=<temp-home> XDG_DATA_HOME=<temp-cache> <temp-bin>/lore sync` with the temporary home as the working directory.
  5. Verify these paths exist and are symlinks in default soft mode:

         <temp-home>/.pi/agent/skills/example-skill
         <temp-home>/.claude/skills/example-skill
         <temp-home>/.pi/agent/prompts/some-prompt.md
         <temp-home>/.claude/prompts/some-prompt.md
         <temp-home>/.pi/agent/hooks/tools/tool.json
         <temp-home>/.claude/hooks/tools/tool.json

  6. Place an unmanaged file at one target, rerun sync, and verify the command reports the conflict without changing the file.

## Risks and Blockers

- Dynamic top-level keys make a correctly shaped typo indistinguishable from an intended resource. Mitigation: strict nested source fields, deterministic parsing, clear README warnings, and CLI errors that print the resource name and final destination.
- Arbitrary resources can overlap through nested keys and mapped destinations. Mitigation: canonicalize and validate complete `<resource>/<dst>` paths before any fetch result is written.
- Refusing unmanaged overwrite changes existing soft-skill behavior. Mitigation: make the error actionable, preserve all content, and test migration/retro-registration so genuinely managed legacy skills remain updateable.
- Manifest v1 migration is load-bearing. Treating v1 as corrupt could orphan managed content; treating arbitrary paths as owned could delete user data. Mitigation: accept scalar entries explicitly, classify only provable symlinks or legacy checksum directories, preserve unknown entries, and save v2 atomically.
- Hard-file integrity cannot use the current in-directory marker. Mitigation: store checksums in manifest v2 and treat a missing/corrupt manifest as no authority to overwrite or delete.
- Current command orchestration can lose ownership after partial provider failures. Generic resources increase the impact. Mitigation: merge outcomes per desired path and retain previous entries for desired failures before saving.
- Multiple profiles may currently list the same path. Mitigation: detect this during load/sync, refuse new cross-profile claims, and never delete or unignore a path while another profile owns it.
- Pi prompt discovery is non-recursive, and Claude does not discover `.claude/prompts`. Mitigation: document that Loremaster guarantees path transport only and include provider-native examples without introducing aliases.
- Cross-platform path handling may diverge if YAML backslashes are accepted. Mitigation: continue using slash-form config paths, reject backslashes, and compare normalized project-relative paths.
- During later implementation, destructive operations, dependency changes, additional data migrations beyond the approved manifest v1-to-v2 migration, external service changes, commits, pushes, force operations, production actions, and scope expansion require explicit user approval.

## Progress

- [x] Requirements clarified and confirmed.
- [x] Repository, tests, provider documentation, and relevant external documentation explored.
- [x] Planning baseline verification completed.
- [x] Planning complete and saved.
- [x] Implementation complete.
- [x] Configuration and provider phases complete.
- [x] Manifest and generic linking phases complete.
- [x] Sync orchestration phase complete.
- [x] Documentation and CLI phase complete.
- [x] Post-change verification complete.

## Decision Log

- Decision: Use dynamic top-level resource keys, reserving only `provider`.
  Rationale: This matches the requested concise YAML and permits arbitrary provider-relative directories without a wrapper schema.
  Date/Author: 2026-07-23, planning agent with user confirmation.
- Decision: Keep `skills` directory-only; allow exact files and directories for all other resources; do not support globs.
  Rationale: This preserves the current skill contract while enabling prompt and arbitrary file synchronization with deterministic ownership.
  Date/Author: 2026-07-23, planning agent with user confirmation.
- Decision: Use literal resource paths across all existing providers and preserve Pi project/global scope.
  Rationale: Loremaster remains a transport layer instead of encoding changing provider-specific capability aliases.
  Date/Author: 2026-07-23, planning agent with user confirmation.
- Decision: Refuse unmanaged overwrite and cross-profile claims.
  Rationale: Existing soft-mode deletion is not acceptable when the destination may be a provider configuration file rather than an isolated skill directory.
  Date/Author: 2026-07-23, planning agent with user confirmation.
- Decision: Introduce a backward-compatible structured manifest rather than payload sidecar files.
  Rationale: Hard-copied files need checksums and ownership metadata, while extra files inside prompts, commands, or arbitrary directories could affect provider discovery.
  Date/Author: 2026-07-23, planning agent.
- Decision: Centralize stale/provider/profile cleanup around manifest entries.
  Rationale: Walking one skill root cannot safely reconcile files distributed across arbitrary resource roots, and the current duplicated removal logic already diverges.
  Date/Author: 2026-07-23, planning agent.
- Decision: Keep filesystem changes transactional until manifest persistence succeeds.
  Rationale: Staged replacements and removals must roll back if ownership metadata cannot be saved, otherwise valid content can become unmanaged or unrecoverable.
  Date/Author: 2026-07-23, implementation agent after independent safety review.
- Decision: Reject legacy state in v2 and migrate v1 conservatively.
  Rationale: Only v1 loading may establish legacy ownership. Unverifiable legacy symlinks, empty directories, or modified checksums stop migration instead of being claimed by a forged or incomplete v2 entry.
  Date/Author: 2026-07-23, implementation agent after independent safety review.

## Execution Handoff

Use PI Agent in a fresh session with this prompt:

    Read spec/260723-1029-sync-arbitrary-provider-resources.md.
    Implement it step by step. Before editing, re-read the Requirements, Out of Scope, Risks and Blockers, and Verification sections.
    Keep the plan updated as a living document by marking Progress and recording implementation decisions or changed assumptions in Decision Log and Notes.
    Start with tests for dynamic parsing, exact file sync, overwrite refusal, and manifest v1 migration before changing production behavior.
    Run every Verification command and the manual acceptance check before reporting done.
    Do not commit, push, add dependencies, perform destructive operations, change external services, run unapproved migrations, or expand scope without explicit approval.
    If PI plan mode is active, use a numbered Plan: section and mark completed implementation steps with [DONE:n].

An optional independent review subagent may be used after implementation. Give it the saved plan and final diff, and ask it to report only unmet requirements, ownership/data-loss risks, and missing tests. Do not delegate the implementation synthesis itself.

## Notes

- The requested `.claude/propmts` example is treated as a typo; the literal resource path is `.claude/prompts`.
- Prefer generic names such as `Resource`, `Source`, `Item`, `ConfigRoot`, and `LinkItem` where they improve clarity, but avoid compatibility shims or abstractions that are not needed by the confirmed behavior.
- `gitignore.EnsureEntries` and `RemoveEntries` are already path-generic. Favor changing orchestration and ownership decisions rather than rewriting the package unless deterministic sorting or shared-owner behavior requires a narrow API change.
- `cmd/sync.go` currently contains repeated removal and checksum code. Extract only the shared ownership-aware removal operation required by normal reconciliation, removed providers/resources, and prune.
- Official Claude references used during planning:
  - https://code.claude.com/docs/en/claude-directory
  - https://code.claude.com/docs/en/skills
- Official Pi behavior was verified from the installed Pi documentation:
  - `/Users/dgjalic/.bun/install/global/node_modules/@earendil-works/pi-coding-agent/docs/prompt-templates.md`
  - `/Users/dgjalic/.bun/install/global/node_modules/@earendil-works/pi-coding-agent/docs/skills.md`
- Final verification passed on 2026-07-23: focused tests, `go test ./... -count=1`, `go vet ./...`, `go test -race ./...`, `go build ./cmd/lore`, `git diff --check`, and the temporary-home Pi plus Claude manual acceptance scenario.
