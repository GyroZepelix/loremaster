---
title: 'Add Pi and Codex Skill Sync Providers'
type: 'feature'
created: '2026-05-15'
status: 'draft'
context:
  - '{project-root}/spec/prd.md'
  - '{project-root}/spec/tech-spec-loremaster-v02x-profiles-multiprovider-subdir.md'
---

<frozen-after-approval reason="human-owned intent -- do not modify unless human renegotiates">

## Intent

**Problem:** Loremaster currently accepts only `claude` and `opencode`, so users cannot declare Pi or Codex in `lore.yml` even though both tools consume Agent Skills. This blocks one-command skill sync across the current tools Domagoj uses.

**Approach:** Add `pi` and `codex` as first-class providers in the existing provider registry, config validation, config lookup, provider detection, sync cleanup, tests, and README. Use native project skill directories: Pi `.pi/skills/<name>` and Codex `.agents/skills/<name>`; for Pi global scope, when the resolved project root is the user's home directory, sync to `~/.pi/agent/skills/<name>`.

## Boundaries & Constraints

**Always:** Preserve existing `claude` and `opencode` behavior. Keep provider names lowercase: `pi`, `codex`. Manifest and `.gitignore` entries must use the actual project-root-relative skill path, not a provider marker shortcut. Codex sync targets `.agents/skills`, matching current OpenAI Codex docs; Pi project sync targets `.pi/skills`, matching current Pi docs. Keep sync orchestration provider-agnostic: provider-specific path differences belong in the provider layer.

**Ask First:** Changing target directories, migrating existing user content from `.codex/skills`, changing the current `provider:` schema, or altering Claude/OpenCode paths.

**Never:** Do not implement skill package installation, skill validation, settings-file mutation, or conversion of existing `.agents/skills` / `.pi/skills` content. Do not special-case Pi or Codex inside sync logic when the provider abstraction can express the behavior.

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|----------|--------------|---------------------------|----------------|
| Multi-provider sync | `provider: [claude, pi, codex]`, include `code-review` | Links to `.claude/skills/code-review`, `.pi/skills/code-review`, and `.agents/skills/code-review`; `.gitignore` and manifest record all three actual paths | Source-level failures stay isolated as today |
| Codex config location | `lore.yml` or `lore-dev.yml` lives in `.agents/` or `.codex/` | Project root resolves to the parent directory; skills still sync to `.agents/skills/<name>` | Existing "no config found" error remains when no candidate exists |
| Codex global scope | Running from `$HOME` with `provider: codex` | Skills sync to `$HOME/.agents/skills/<name>` | Existing config-location errors remain unchanged |
| Pi global scope | Running from `$HOME` with `provider: pi` | Skills sync to `$HOME/.pi/agent/skills/<name>` | If `$HOME` cannot be resolved, fall back to project-style `$HOME/.pi/skills/<name>` only with a warning |
| Removed provider cleanup | Manifest has `.agents/skills/foo`; config no longer includes `codex` | Cleanup removes only managed `.agents/skills/foo` and its gitignore entry; unrelated `.agents/skills` content remains | Modified hard-copy skills are skipped using existing checksum safeguards |

</frozen-after-approval>

## Code Map

- `internal/provider/provider.go` -- Provider interface and registry; must expose actual skill roots and provider config directories.
- `internal/provider/claude.go`, `internal/provider/opencode.go` -- Existing provider behavior to preserve while extending interface.
- `internal/provider/pi.go`, `internal/provider/codex.go` -- New providers and path rules.
- `internal/provider/detect.go` -- Auto-detection across one or more marker directories per provider.
- `internal/config/config.go` -- `provider:` validation and supported-provider error messages.
- `internal/config/locate.go` -- `lore.yml` / profile lookup under provider config directories.
- `cmd/init.go` -- Provider selection prompt and default marker/config directory creation.
- `cmd/sync.go` -- Project-root resolution and removed-provider cleanup currently assume marker dir equals skill path prefix.
- `internal/sync/sync.go` -- Uses `Provider.SkillDir`; should continue to use provider-owned path resolution.
- `README.md` -- Supported tools, schema, examples, and config-location docs.

## Tasks & Acceptance

**Execution:**
- [ ] `internal/provider/provider.go` -- Extend the provider contract with `SkillRoot(projectRoot string) string`, `ConfigDirs() []string`, and `MarkerDirs() []string`; derive `SkillDir` from `SkillRoot`; register `pi` and `codex`; expose stable provider names/config dirs for validation and lookup.
- [ ] `internal/provider/pi.go` and `internal/provider/codex.go` -- Implement Pi and Codex path rules: Pi project `.pi/skills`, Pi home/global `.pi/agent/skills`, Codex `.agents/skills`; allow Codex detection from `.agents` or `.codex`.
- [ ] `internal/provider/detect.go` and `cmd/init.go` -- Detect any provider marker dir without duplicates; when no provider is detected and the user selects one, create that provider's default marker/config directory before writing the root `lore.yml`.
- [ ] `internal/config/config.go` and `internal/config/locate.go` -- Accept `pi` and `codex`, list all supported names in errors, and search root plus provider config dirs for default and profile config files.
- [ ] `cmd/sync.go` -- Replace marker-prefix cleanup with actual `SkillRoot`-relative prefixes; update `resolveProjectRoot` so configs under nested provider dirs such as `.pi/agent` resolve to the real project root.
- [ ] Tests in `internal/provider`, `internal/config`, `internal/sync`, and `cmd` -- Cover new providers, config lookup, sync destinations, gitignore/manifest paths, Pi home/global behavior, Codex `.agents` cleanup, and invalid-provider errors.
- [ ] `README.md` -- Document Pi and Codex in supported tools, `provider:` schema, config search locations, and multi-provider examples.

**Acceptance Criteria:**
- Given `provider: [pi, codex]`, when `lore sync` runs for include `foo`, then `foo` is linked into `.pi/skills/foo` and `.agents/skills/foo`, and both relative paths are added to `.gitignore` and manifest entries.
- Given `provider: codex` and config in `.codex/lore.yml`, when `lore sync` runs, then the project root is the parent directory and the skill path is `.agents/skills/<name>`.
- Given `provider: pi` and the resolved project root is `$HOME`, when `lore sync` runs, then the skill path is `$HOME/.pi/agent/skills/<name>`.
- Given a previous manifest contains `.agents/skills/foo` and the next config omits `codex`, when sync cleanup runs, then only the managed Codex entry is removed and unrelated `.agents/skills` entries are preserved.
- Given `provider: [claude, opencode]`, when existing tests run, then all current destination paths and behaviors remain unchanged.

## Spec Change Log

## Design Notes

The PRD defines Loremaster as stateless declarative glue: parse one config, fetch sources once, link per provider, reconcile managed stale entries, and update `.gitignore`. The v0.2 spec adds profiles, provider lists, subdirectory includes, and manifest-scoped ownership; manifest entries and gitignore entries are always actual project-root-relative skill paths.

Current Codex docs list repo-scoped skills under `.agents/skills`, user skills under `$HOME/.agents/skills`, and note symlinked skill folders are supported. Current Pi docs list project skills under `.pi/skills` and global skills under `~/.pi/agent/skills`. Pi also discovers `.agents/skills`; with `provider: [pi, codex]`, Pi may see both native target trees and warn about duplicate skill names. That is acceptable for this feature; do not mutate Pi settings or suppress Codex output.

The v0.2 spec and README say `.lore-manifest.yml` is lazy: default single-provider sync should not create it, while non-default profiles or default multi-provider sync do. Current `cmd/sync.go` appears to save the manifest unconditionally. Treat that as an existing discrepancy, not part of this feature unless a touched test or implementation path forces correction.

## Verification

**Commands:**
- `go test ./...` -- expected: all tests pass.
