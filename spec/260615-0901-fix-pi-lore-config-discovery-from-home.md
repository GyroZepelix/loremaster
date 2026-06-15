# Fix Pi lore config discovery from home

## Purpose

Investigate and fix the reported behavior where running `lore sync` from `~` detects `~/.claude/lore.yml` but does not detect `~/.pi/lore.yml` or `~/.pi/agent/lore.yml`. The desired outcome is that Pi global configs are discovered consistently from the home directory and remain covered by CLI-level regression tests.

## Context

Repository: `/Users/dgjalic/Documents/1-Projects/10-software-development/loremaster`.

Detected stack: Go module `github.com/GyroZepelix/loremaster` with Cobra CLI entrypoint under `cmd/` and internal packages for config, provider metadata, sync orchestration, manifests, and gitignore management.

Relevant docs and files inspected:

- `README.md` says `lore.yml` may live in the project root or provider config directories, including `.pi/` and `.pi/agent/`. It also says global Pi sync from `~` writes skills to `~/.pi/agent/skills/`.
- `internal/config/locate.go` resolves config paths through `configCandidates(dir, filename)`, starting with `dir/lore.yml` and then each directory returned by `provider.ConfigDirs()`.
- `internal/provider/provider.go` builds global provider config directory order from provider registry order: `claude`, `opencode`, `pi`, `codex`, de-duplicated.
- `internal/provider/pi.go` currently returns `[]string{".pi", filepath.Join(".pi", "agent")}` from `Pi.ConfigDirs()` and routes the Pi skill root to `~/.pi/agent/skills` when `projectRoot` equals `os.UserHomeDir()`.
- Existing tests already assert `Locate` and `LocateProfile` can find `.pi/lore.yml` and `.pi/agent/lore.yml`: `internal/config/config_test.go` and `internal/config/locate_test.go`.
- Existing tests assert `resolveProjectRoot` handles `.pi/lore.yml` and `.pi/agent/lore.yml`: `cmd/sync_test.go`.
- Existing tests assert Pi global sync writes to `~/.pi/agent/skills/foo`: `internal/sync/sync_test.go`.

Planning-time verification already run against the current checkout:

- `go test ./internal/config ./internal/provider ./cmd -run 'TestLocate|TestResolveProjectRoot|TestPi'` passed.
- `go test ./...` passed.

Important finding: the current source code already includes Pi config directories and the existing unit tests pass. That means the reported behavior may be one of these cases:

- The installed `lore` binary being run from `~` is stale or built from an older source version that only included `.claude` discovery.
- The reported case depends on CLI-level behavior not covered by the existing unit tests, such as working directory, environment, profile flag state, root command state, config precedence, or command invocation through the built binary.
- A different config path, profile filename, parse error, or provider validation error is being interpreted as "not detected" because `runSync` currently returns a generic `no config found for profile ...` error when `LocateProfile` fails.

No wiki routing files exist: `wiki/index.md`, `wiki/INDEX.md`, and `wiki/AGENTS.md` were not present.

## Requirements

- Running the source-built CLI from a directory that represents `HOME` must discover `./.pi/lore.yml` for the default profile.
- Running the source-built CLI from a directory that represents `HOME` must discover `./.pi/agent/lore.yml` for the default profile.
- Running with `--profile <name>` from a directory that represents `HOME` must discover `./.pi/lore-<name>.yml` and `./.pi/agent/lore-<name>.yml`.
- Existing priority order must remain unchanged: `./lore.yml` first, then provider config dirs in provider registry order, meaning `.claude` can still win over `.pi` when both default configs exist and no root config exists.
- Pi global skill output must remain `~/.pi/agent/skills/` when the resolved project root is the user home directory.
- The implementation must add regression coverage that exercises the CLI or command-level path, not only `config.Locate` unit tests.
- If the current source cannot reproduce the failure, the implementation must document the finding in the plan progress or decision log and focus the code change on regression coverage and clearer diagnosis rather than changing already-correct discovery logic.

## Out of Scope

- Do not change config precedence semantics unless reproduction proves precedence is the actual bug and the user approves the scope expansion.
- Do not add automatic fallback from arbitrary directories to `~/...`; README explicitly says global scope works because `Locate()` searches relative to the invocation directory.
- Do not change provider names, config schema, sync source parsing, manifest format, or git fetch behavior.
- Do not publish a release, run `go install`, modify the user's installed binary, commit, or push without explicit approval.
- Do not run destructive cleanup in the user's real `~/.pi`, `~/.claude`, or skill directories.

## Assumptions

- The user's intended command is `cd ~ && lore sync` with no `--profile` for default config discovery, plus possible profile variants worth covering because `LocateProfile` shares the candidate path logic.
- The intended Pi global config paths are `~/.pi/lore.yml` and `~/.pi/agent/lore.yml`, matching README and `Pi.ConfigDirs()`.
- Existing provider precedence is intentional because README states each invocation uses exactly one config file and tests already assert root config wins over provider config. If `.claude/lore.yml` and `.pi/lore.yml` both exist, detecting `.claude/lore.yml` first is expected, not a Pi discovery failure.
- It is safe to use temporary directories in tests to model `HOME`; no test should operate on the real user home directory.

## Plan

1. Reproduce the reported behavior with the source-built CLI in a temporary home directory.
   - Target areas: new or expanded tests in `cmd/sync_test.go`, optionally a small test helper in the same file.
   - Build a local skill source under `t.TempDir()` with a minimal skill directory, set `HOME` to a separate temporary directory, set `XDG_DATA_HOME` to a temporary cache directory, and run the sync command from the temporary home.
   - Create one test case for `~/.pi/lore.yml` and one for `~/.pi/agent/lore.yml` using minimal valid YAML with `provider: pi` and a local source path.
   - Expected result: tests should prove whether command-level sync discovers each config and creates `~/.pi/agent/skills/<skill>`.

2. Add profile-specific command-level coverage for Pi config directories.
   - Target areas: `cmd/sync_test.go` and existing profile flag handling in `cmd/sync.go`.
   - Add cases for `~/.pi/lore-dev.yml` and `~/.pi/agent/lore-dev.yml` invoked with `--profile dev`, or test `runSync` with the profile flag set and restored safely.
   - Reset package-level flags such as `profileFlag` and `pruneFlag` with `t.Cleanup` to avoid cross-test contamination.
   - Expected result: profile config discovery uses the same Pi directories as default discovery.

3. If the new command-level tests fail, fix the smallest failing code path.
   - Likely target areas: `internal/config/locate.go`, `internal/provider/pi.go`, `internal/provider/provider.go`, or `cmd/sync.go` depending on the reproduction.
   - Preserve `configCandidates` ordering unless the failing test proves it omits Pi paths.
   - Preserve `Pi.SkillRoot` behavior for home directory sync.
   - Expected result: `config.LocateProfile(cwd, profile)` returns the Pi config path and `runSync` uses `resolveProjectRoot(configPath)` as the temporary home.

4. If the new command-level tests pass immediately, treat the current source as correct and improve diagnostics only if useful and minimal.
   - Target areas: possibly `cmd/sync.go` error reporting when config lookup fails.
   - Consider changing the no-config error to preserve or include the lower-level locate error, such as searched profile name and working directory, without changing behavior.
   - Do not add verbose candidate listing unless it follows existing CLI style and is needed to explain this specific failure.
   - Expected result: future users can distinguish stale binary, wrong working directory, and missing config cases more easily.

5. Update docs only if implementation changes user-visible behavior or clarifies an ambiguity found during reproduction.
   - Target areas: `README.md` configuration scope section only if needed.
   - If tests confirm `.claude/lore.yml` wins over `.pi/lore.yml` when both exist, keep code unchanged and optionally clarify that provider directory search is ordered and uses the first config found.
   - Expected result: documentation matches observed and tested behavior.

6. Run verification and inspect the final diff.
   - Target areas: entire Go test suite and changed files only.
   - Ensure no dependency changes, no generated artifacts, and no edits outside the targeted source/docs/tests.
   - Expected result: all tests pass and the diff is limited to the reproduction/fix scope.

## Verification

- `go test ./internal/config ./internal/provider ./cmd -run 'TestLocate|TestResolveProjectRoot|TestPi|TestSync.*Pi|TestRunSync.*Pi'`
  - Expected: passes, including new command-level Pi discovery tests.
- `go test ./...`
  - Expected: passes across all packages.
- Manual acceptance check with a temporary home directory, not the real home:
  - Build or run the source CLI from a temp working directory where only `.pi/lore.yml` exists and contains a valid local source config.
  - Run `HOME=<temp-home> XDG_DATA_HOME=<temp-cache> go run ./cmd/lore sync` from that temp home.
  - Expected: sync succeeds and creates `<temp-home>/.pi/agent/skills/<skill>`.
- Manual acceptance check with `.pi/agent/lore.yml`:
  - Repeat the previous check with the config at `<temp-home>/.pi/agent/lore.yml` and no root or `.claude` config.
  - Expected: sync succeeds and creates `<temp-home>/.pi/agent/skills/<skill>`.
- If a real installed binary is compared after source verification, run only non-destructive diagnostics such as `which lore` and `lore --help`; do not modify or reinstall the binary without approval.

## Risks and Blockers

- Existing unit tests already pass for Pi discovery, so the reported issue may be a stale installed binary rather than a source bug. Mitigation: add CLI-level tests first, then only change source logic if the new tests reproduce a failure.
- If both `~/.claude/lore.yml` and `~/.pi/lore.yml` exist, current precedence will choose `.claude` before `.pi`. This may look like Pi is not detected, but it is current priority behavior. Changing this would be a scope change requiring approval.
- CLI tests can be flaky if they touch global package state. Mitigation: reset `profileFlag`, `pruneFlag`, and working directory with `t.Cleanup`, and isolate `HOME` and `XDG_DATA_HOME` per test.
- Sync tests that fetch git sources are slower and externally dependent. Mitigation: use local source paths and temporary directories so no network access is required.
- Safety boundaries for later implementation: destructive file operations, dependency changes, migrations, external service changes, commits, pushes, force operations, production actions, edits to the user's real home config, and broad unrequested refactors require explicit approval before proceeding.

## Progress

- [x] Planning complete and saved.
- [x] Implementation completed.
- [x] Planning-time baseline tests run: focused Go tests passed.
- [x] Planning-time baseline tests run: `go test ./...` passed.
- [x] New command-level regression tests implemented in `cmd/sync_test.go`.
- [x] Source fix implemented in `internal/provider/pi.go` for symlink-equivalent HOME/project-root comparison.
- [x] Final focused verification passed: `go test ./internal/config ./internal/provider ./cmd -run 'TestLocate|TestResolveProjectRoot|TestPi|TestSync.*Pi|TestRunSync.*Pi'`.
- [x] Final full verification passed: `go test ./...`.
- [x] Manual temp-home checks passed for `.pi/lore.yml` and `.pi/agent/lore.yml`.

## Decision Log

- Decision: Start implementation by adding command-level regression coverage rather than editing `Pi.ConfigDirs()` immediately.
  Rationale: Current source already lists `.pi` and `.pi/agent`, and existing unit tests pass. The untested gap is the actual `lore sync` path from a home-like working directory.
  Date/Author: 2026-06-15 / PI planning agent

- Decision: Preserve provider config precedence unless a failing test proves the issue is not precedence-related.
  Rationale: Existing tests and README imply the first located config wins. When `.claude/lore.yml` and `.pi/lore.yml` both exist, `.claude` discovery first is expected with current ordering.
  Date/Author: 2026-06-15 / PI planning agent

- Decision: Use temporary local sources and temporary HOME/XDG paths for all reproduction and acceptance checks.
  Rationale: The task concerns home-directory behavior, but tests must not read from or write to the user's real `~/.pi`, `~/.claude`, or cache directories.
  Date/Author: 2026-06-15 / PI planning agent

- Decision: Fix Pi global HOME detection by treating symlink-equivalent paths as the same directory.
  Rationale: Command-level tests showed config discovery worked, but on macOS temporary directories `HOME` used a `/var/...` path while `os.Getwd()` returned `/private/var/...`; the clean string comparison failed and Pi synced to `.pi/skills` instead of `.pi/agent/skills`.
  Date/Author: 2026-06-15 / PI implementation agent

## Execution Handoff

Use PI Agent in a fresh session with this prompt:

    Read the saved plan file path reported by the planning agent.
    Implement it step by step. Before editing, re-read the Requirements, Out of Scope, Risks and Blockers, and Verification sections.
    Start by adding command-level regression tests for `lore sync` from a temporary home directory with `.pi/lore.yml` and `.pi/agent/lore.yml`.
    If the tests fail, fix the smallest source path that makes them pass. If the tests pass, do not force a source change; record that the current source is correct and consider only minimal diagnostic or documentation clarification.
    Update the Progress and Decision Log sections as work proceeds.
    Run the Verification commands before reporting done.
    Do not commit, push, run migrations, add dependencies, perform destructive operations, modify the user's real home config, or expand scope without explicit approval.
    If PI plan mode is active, use a numbered Plan: section and mark completed implementation steps with [DONE:n].

No PI subagent is required for implementation because the relevant code path is small and already localized to config discovery, provider metadata, and sync command tests. An optional review subagent can be used after implementation to compare the final diff against this saved plan.

## Notes

- `provider.ConfigDirs()` currently returns a de-duplicated list in provider order. With the current registry this is `.claude`, `.opencode`, `.pi`, `.pi/agent`, `.agents`, `.codex`.
- `config.LocateProfile(cwd, "")` delegates to `Locate(cwd)`, so default profile behavior and profile-specific behavior share most of the candidate path logic.
- `resolveProjectRoot(configPath)` walks back through provider config dirs, including nested `.pi/agent`, so configs under `~/.pi/agent/` should resolve the project root as `~`.
- If a user reports that `.claude/lore.yml` is detected while `.pi/lore.yml` is ignored and both files exist at the same time, that is probably config precedence rather than missing Pi support.
- Planning-time commands were run against the current checkout and passed before implementation.
- Implementation changed `cmd/sync_test.go` and `internal/provider/pi.go` only.
- The source-built CLI was manually checked with temporary homes for both `.pi/lore.yml` and `.pi/agent/lore.yml`; both created `.pi/agent/skills/foo`.
