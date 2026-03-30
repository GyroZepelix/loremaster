---
title: 'Loremaster v0.2.x — Profiles, Multi-Provider, Subdirectory Skills'
slug: 'loremaster-v02x-profiles-multiprovider-subdir'
created: '2026-03-25'
status: 'ready-for-dev'
stepsCompleted: [1, 2, 3, 4]
tech_stack: ['Go 1.24', 'Cobra v1.10.2', 'gopkg.in/yaml.v3', 'git.Fetcher interface (ExecGitFetcher production default, GoGitFetcher in tests)']
files_to_modify:
  - 'internal/config/config.go'
  - 'internal/config/locate.go'
  - 'internal/config/config_test.go'
  - 'internal/provider/provider.go'
  - 'internal/sync/sync.go'
  - 'internal/sync/sync_test.go'
  - 'internal/sync/linker.go'
  - 'cmd/sync.go'
  - 'cmd/init.go'
files_to_create:
  - 'internal/config/include.go'
  - 'internal/config/include_test.go'
  - 'internal/manifest/manifest.go'
  - 'internal/manifest/manifest_test.go'
code_patterns:
  - 'internal/ for all domain logic, cmd/ for CLI wiring'
  - 'Provider interface for tool-agnostic skill directory resolution'
  - 'Sync orchestrator pattern: config -> git -> linker -> reconcile -> gitignore'
  - 'GitFetcher interface for testable git operations'
  - 'Table-driven tests with t.TempDir() for filesystem isolation'
  - 'Config.Provider is string, validated against validProviders map'
  - 'validateSkillName() rejects /, \\, .., empty — must be replaced with path-aware validation'
  - 'Syncer takes single Provider, caller loops for multi-provider'
  - 'reconcileStale() does flat ReadDir — must become recursive walk for nested skills'
  - 'desiredSkills map uses skill name as key — must use destination relative path'
  - 'SkillDir(root, name) uses filepath.Join which naturally handles nested paths'
test_patterns:
  - 'Table-driven tests with _test.go per package'
  - 't.TempDir() for filesystem operations'
  - 'Interface-based mocking for git operations'
  - 'Existing test files: config_test.go, sync_test.go, exec_test.go, git_test.go, provider_test.go, cache_test.go, gitignore_test.go'
---

# Tech-Spec: Loremaster v0.2.x — Profiles, Multi-Provider, Subdirectory Skills

**Created:** 2026-03-25

## Overview

### Problem Statement

Loremaster v0.1.x is limited to a single `lore.yml` config per project, a single provider per config, and flat skill names that must live at the root of skill repositories. Users need:

1. **Multiple configurations per project** — different skill sets for different contexts (e.g., development vs CI, frontend vs backend) without maintaining separate repos or branches.
2. **Multiple AI tool targets** — a single lore file that syncs skills to both Claude Code and OpenCode simultaneously.
3. **Subdirectory access in skill repos** — skill repositories that organize skills into nested directories (e.g., `loa/brainstorm/`) cannot be consumed without flattening the repo structure first.

### Solution

Three coordinated features:

1. **Profile flag (`-p <profile>`)** — `lore sync -p dev` reads `lore-dev.yml` instead of `lore.yml`. A lazy `.lore-manifest.yml` tracks per-profile ownership so profiles don't clobber each other's skills during stale reconciliation.
2. **Multi-provider** — `provider` field accepts a list (`provider: [claude, opencode]`), and sync iterates over all providers, placing skills into each provider's skill directory. Single string remains valid for backward compatibility.
3. **Subdirectory includes** — `include` values can contain `/`-delimited paths (`loa/brainstorm`) mapping to repo subdirectories. Optional `src:dst` mapping (`deep/nested/skill:my-skill`) allows remapping destination paths. Path traversal is validated on both sides.

### Scope

**In Scope:**
- `-p <profile>` flag on `sync` and `init` commands
- `lore-<profile>.yml` file naming convention with same search order as `lore.yml`
- Lazy `.lore-manifest.yml` for multi-profile stale reconciliation (only created when a non-default profile is first synced)
- `provider: [claude, opencode]` list syntax with backward compat for single string
- Sync iterates over all providers, placing skills in each provider's skill directory
- `.gitignore` management for all provider paths
- Subdirectory includes with `/` paths (e.g., `loa/brainstorm`)
- Optional `src:dst` mapping syntax (e.g., `skills/loa/brainstorm:loa/brainstorm`)
- Path traversal validation (reject `..`, absolute paths, paths escaping source root)
- Arbitrary nesting depth
- `--prune` flag on sync to clean up orphaned profile skills
- Atomic manifest writes (temp file + rename)
- Two-phase sync: fetch sources once, link per-provider
- v0.1.x → v0.2.x backward compatibility (single string `provider` field)

**Out of Scope:**
- Profile merging or inheritance (each profile is fully independent)
- Glob patterns in includes
- `.lore.lock` (deferred to Phase 2)
- Cross-profile dependency tracking
- Profile-specific provider overrides (all profiles use their own `provider` field)

## Context for Development

### Codebase Patterns

- Clean `internal/` for domain logic, `cmd/` for CLI wiring
- Provider interface pattern for tool-agnostic skill directory resolution
- Sync orchestrator pattern: resolve config → fetch git sources → create links → reconcile stale → update gitignore
- `git.Fetcher` interface with two implementations: `ExecGitFetcher` (shells to system `git`, production default in `cmd/sync.go` — chosen for full SSH config/key support) and `GoGitFetcher` (pure Go via go-git library, used in `sync_test.go` integration tests to avoid system git dependency in CI). `FetchSources` takes the interface — implementation-agnostic. v0.2.x does not change this wiring.
- Error wrapping with `fmt.Errorf("context: %w", err)` and actionable hints
- Table-driven tests with `t.TempDir()` for filesystem isolation
- Per-skill failure isolation within sources, source-level isolation across sources

### Files to Reference

| File | Purpose |
| ---- | ------- |
| `internal/config/config.go` | Config struct, YAML parsing, validation — needs `provider` list support + include path parsing |
| `internal/config/locate.go` | `Locate(dir)` searches `./lore.yml`, `.claude/lore.yml`, `.opencode/lore.yml` — needs profile variant |
| `internal/provider/provider.go` | Provider interface + registry — `SkillDir()` needs to handle nested paths |
| `internal/sync/sync.go` | Sync orchestrator — needs multi-provider loop + manifest integration + subdirectory resolution |
| `internal/sync/linker.go` | Symlink/copy logic — destination paths now include subdirectories |
| `internal/gitignore/gitignore.go` | `.gitignore` management — needs to handle entries for multiple providers |
| `cmd/sync.go` | CLI sync command — needs `-p` flag, multi-provider orchestration |
| `cmd/init.go` | CLI init command — needs `-p` flag for profile-specific init |
| `internal/cache/cache.go` | Cache directory + URL normalization — unchanged |
| `spec/prd.md` | Original PRD for reference |

### Technical Decisions

**TD-1: Profile file naming follows docker-compose convention.**
`lore-<profile>.yml` mirrors `docker-compose.override.yml` / `compose.<profile>.yml`. The default (no `-p`) always uses `lore.yml`. `-p default` is a reserved keyword that maps to `lore.yml` (not `lore-default.yml`). Profile names are validated: `[a-z0-9][a-z0-9_-]*`, max 64 chars. Note: config files found inside provider directories (e.g., `.claude/lore.yml`) with multi-provider `provider: [claude, opencode]` are valid — `resolveProjectRoot()` resolves the project root one level up from the provider dir regardless of provider list content. The config location is a filesystem convenience, not a provider scope constraint.

**TD-2: Manifest is lazily created.**
`.lore-manifest.yml` is not created until a non-default profile is synced for the first time OR the default profile uses multi-provider. On first creation by a non-default profile, existing managed skills in the provider directory are retroactively recorded as owned by the `default` profile by scanning for symlinks pointing into cache and directories with `.lore-checksum` (same logic as `reconcileStale` detection). If the default profile has never been synced (no managed entries found), the `default` profile entry is left empty. If a profile has no entries in the manifest yet (e.g., first run of default after another profile created the manifest), skip reconciliation for that profile — treat as fresh, register whatever gets synced.

**TD-3: Provider field uses YAML union type.**
`provider` accepts both `provider: claude` (string) and `provider: [claude, opencode]` (list). Parsing uses `yaml.v3` custom unmarshaling to handle both forms. Internally always stored as `[]string`. The `UnmarshalYAML` must handle null/empty scalar nodes: if `value.Kind == yaml.ScalarNode && value.Value == ""`, return error `"missing required field: provider"` rather than producing `[""]`.

**TD-4: Include paths use `/` delimiter with optional `src:dst` mapping.**
- `brainstorm` → source: `brainstorm/`, destination: `skills/brainstorm/` (flat, backward compat)
- `loa/brainstorm` → source: `loa/brainstorm/`, destination: `skills/loa/brainstorm/`
- `deep/nested/skill:my-skill` → source: `deep/nested/skill/`, destination: `skills/my-skill/`
- Both sides validated: `filepath.Clean()`, reject `..` and absolute paths
- Split on first `:` only (docker volume semantics) — invalid chars in dst will fail path validation
- Empty source or destination after split → error
- Overlapping includes: two-level validation. (1) **Within a source** — rejected at parse time in `Parse()` by calling `ValidateOverlaps()` on each source's `ParsedIncludes`. (2) **Across sources** — rejected at sync time in `Sync()` during collision detection, by aggregating all `entry.Dst` values across sources and calling `ValidateOverlaps()` on the combined set. Parse-time catches intra-source overlaps early; sync-time catches cross-source overlaps that `Parse()` cannot see (it only sees one config). Both prevent symlink-inside-symlink and hard-copy subdirectory conflicts.

**TD-5: Reconciliation scoping.**
- No manifest: reconcile all managed entries (current behavior, single-profile)
- With manifest: reconcile only entries owned by the current profile
- Ownership transfers on collision (last-write-wins with warning)
- Reconciliation runs per-provider inside the Syncer loop — each provider's skill tree is independent
- Cross-profile provider mismatches are natural: each profile is sovereign over its own manifest entries. Profile A with `[claude, opencode]` and profile B with `[claude]` don't interfere — manifest tracks full relative paths including provider prefix

**TD-6: `.lore-manifest.yml` is gitignored.**
Added to `.gitignore` via `gitignore.EnsureEntries()` alongside skill entries when the manifest is first created. It appears under the "# Managed by loremaster" section. It contains local state (provider-prefixed relative paths to synced skills) and should not be committed.

**TD-7: Two-phase sync: fetch once, link per-provider.**
Sync is split into two phases to avoid duplicate git operations with multi-provider. Phase 1 (fetch): `cmd/sync.go` orchestrates git clone/pull/checkout for all sources once, producing a map of `sourceKey → baseDir`. Phase 2 (link): for each provider, create a Syncer with the provider set and pass pre-resolved base directories — Syncer only does linking, reconciliation, and gitignore. This means `syncSource()` is refactored: git operations extracted to a separate `fetchSource()` method or handled by the caller, while `syncSource()` takes a `baseDir` parameter instead of resolving it internally. Without this, 2 providers × 5 sources = 10 git operations instead of 5.

**Map key for baseDirs:** The key must be `source+ref` (not just source URL) because two sources can reference the same repo with different refs. The cache already uses one directory per URL (`cache.RepoDir(url)`), so same-URL-different-ref requires sequential checkout: fetch the repo once, then for each ref, checkout and resolve the baseDir. The `baseDirs` map key should be the index into `cfg.Skills` (integer) or a composite `url+"|"+ref` string. Implementation: `FetchSources` groups sources by normalized URL, clones/pulls each URL once, then iterates refs in order — checkout ref, snapshot the baseDir (for local sources the dir is stable; for git sources the baseDir is the cache dir at that checkout state). **Important**: since a single cache dir can only have one ref checked out at a time, `FetchSources` must process sources sharing a URL sequentially and the link phase must run per-source immediately after checkout, NOT after all fetches. This changes the two-phase split: fetch-and-link must be interleaved per-source when URL+ref conflicts exist. Alternatively, reject same-URL-different-ref at config validation time with a clear error — simpler and avoids the interleaving complexity. **Decision: reject same-URL-different-ref at parse time** with error: `"skills[%d] and skills[%d] reference the same source %q with different refs (%q vs %q) — use separate cache entries or consolidate refs"`.

**TD-8: `reconcileStale()` needs recursive walk for nested skills.**
Current implementation does flat `os.ReadDir(skillsParent)` and checks `entry.Name()`. With nested includes (`loa/brainstorm`), the top-level entry `loa/` would be seen, not `loa/brainstorm`. Must walk recursively, match against relative paths, and clean up empty parent dirs after removing stale leaves. The `desiredSkills` map key changes from flat name to destination relative path.

**Intermediate directory handling:** The recursive walk only acts on **leaf entries** — symlinks pointing into cache or directories containing `.lore-checksum`. Plain directories (like `loa/`) are intermediate and are never directly removed during the walk. They are only cleaned up AFTER stale leaf removal, bottom-up, if they are empty (PM-8). If a user manually created a directory `loa/` with their own files inside, it will never be touched — it has no symlink-to-cache or `.lore-checksum` marker, so it's invisible to reconciliation. The walk should use `filepath.WalkDir` with `SkipDir` for directories that are desired skills (prevents walking into them and treating their contents as individual entries).

**TD-9: Include parsing produces structured `IncludeEntry` type.**
Raw include strings (`loa/brainstorm`, `deep/skill:my-skill`) are parsed into `IncludeEntry{Src: "loa/brainstorm", Dst: "loa/brainstorm"}` or `IncludeEntry{Src: "deep/skill", Dst: "my-skill"}`. Parsing + validation happens in config layer. Sync layer works with structured entries only.

**TD-10: Orphaned profile detection and `--prune` flag.**
On every `lore sync` (any profile), if manifest exists, check all profiles in the manifest for existing config files. For profiles whose config files don't exist, print: `warning: profile "experiment" has no config file (lore-experiment.yml) — run 'lore sync --prune' to clean up orphaned skills`. The `--prune` flag is **prune-only** — it removes skills, gitignore entries, and manifest entries for orphaned profiles, then exits without syncing. Non-destructive by default (warn only), explicit opt-in for cleanup. To sync AND prune, run `lore sync --prune && lore sync`. This separation keeps the commands simple and predictable — prune is a maintenance operation, not part of the sync flow.

**TD-11: Atomic manifest writes.**
Write manifest to a temp file in the same directory, then `os.Rename()` to the final path (atomic on POSIX). On load, if YAML parse fails, print `warning: .lore-manifest.yml is corrupted — treating as absent (profiles will re-register on next sync)` and proceed as if no manifest exists. The `version: 1` field in the manifest helps detect truncation (missing version = truncated).

**TD-12: Guard against nil Provider in `Sync()`.**
Add `if s.Provider == nil { return nil, fmt.Errorf("provider must be set before calling Sync()") }` at the top of `Sync()`. The old fallback to `cfg.Provider` is removed — caller is responsible for setting Provider.

**TD-13: Concurrent syncs are unsupported.**
Document in README: "Running multiple `lore sync` commands targeting the same project simultaneously is not supported and may corrupt `.lore-manifest.yml`." No file locking — follows docker-compose/terraform precedent.

## Deep Investigation Results

### Anchor Point Analysis

#### Feature 1: Profiles — Anchor Points

| Location | Current Code | Change Required |
|----------|-------------|-----------------|
| `config/locate.go:9-23` | `Locate(dir)` builds candidates `[dir/lore.yml, dir/.claude/lore.yml, dir/.opencode/lore.yml]` | Add `LocateProfile(dir, profile)` that builds candidates using `lore-<profile>.yml`. `Locate(dir)` unchanged for backward compat. |
| `cmd/sync.go:15-18` | `syncCmd` has no flags | Add `-p`/`--profile` string flag via `syncCmd.Flags().StringVarP()` in `init()` |
| `cmd/sync.go:31` | `config.Locate(cwd)` | Branch: if profile set, call `config.LocateProfile(cwd, profile)` |
| `cmd/init.go:13-17` | `initCmd` has no flags | Add `-p`/`--profile` string flag |
| `cmd/init.go:88` | `configPath := filepath.Join(cwd, "lore.yml")` | If profile set, use `lore-<profile>.yml` |
| `cmd/sync.go` (new) | No `--prune` flag | Add `--prune` bool flag. When set + manifest exists, scan for orphaned profiles (config file missing), remove their skills + gitignore + manifest entries |
| `sync/sync.go:71` | `reconcileStale(desiredSkills)` | When manifest exists: scope to current profile's owned skills only |
| New file | N/A | `internal/manifest/manifest.go` — Load/Save/Update manifest, track profile→skills ownership. Atomic writes (temp + rename). Corrupt YAML → warn + treat as absent. |

**Manifest data model:**
```yaml
# .lore-manifest.yml
version: 1
profiles:
  default:
    - .claude/skills/brainstorm
    - .claude/skills/commit
  dev:
    - .claude/skills/debug-tool
```
Entries are relative paths from project root including provider prefix (e.g., `.claude/skills/...`) — provider-aware by design, directly comparable to gitignore entries.

#### Feature 2: Multi-Provider — Anchor Points

| Location | Current Code | Change Required |
|----------|-------------|-----------------|
| `config/config.go:17-19` | `Config.Provider string` | Change to `Config.Providers []string` with custom `UnmarshalYAML` for union type |
| `config/config.go:29-32` | `validProviders` map + single validation | Loop validation over `cfg.Providers`, reject empty list |
| `config/config.go:41-45` | `if cfg.Provider == ""` + `if !validProviders[cfg.Provider]` | Validate each entry in slice |
| `cmd/sync.go:50-53` | `provider.Get(cfg.Provider)` + single Syncer | Two-phase: (1) `sync.FetchSources(fetcher, cfg.Skills)` → `baseDirs` (standalone, provider-agnostic), (2) loop providers: `for _, provName := range cfg.Providers { syncer.Provider = prov; syncer.Sync(cfg, baseDirs) }` |
| `cmd/sync.go:72` | Single result print | Aggregate results across providers |
| `sync/sync.go:29-35` | `Syncer.Sync()` falls back to `cfg.Provider` | Remove fallback — add nil guard error. Caller must set Provider. |
| `sync/sync.go:96-126` | `syncSource()` handles both git fetch and skill linking | Extract git operations to `fetchSource()` or accept pre-resolved `baseDir` parameter — Syncer only does linking + reconciliation in multi-provider mode |
| `cmd/init.go:80-86` | Skeleton uses `provider: %s` | Use `provider: [%s]` for list, or `provider: %s` for single (match user's choice count) |

**YAML union type implementation:**
```go
type ProviderList []string

func (p *ProviderList) UnmarshalYAML(value *yaml.Node) error {
    if value.Kind == yaml.ScalarNode {
        *p = []string{value.Value}
        return nil
    }
    var list []string
    if err := value.Decode(&list); err != nil {
        return err
    }
    *p = list
    return nil
}
```

#### Feature 3: Subdirectory Includes — Anchor Points

| Location | Current Code | Change Required |
|----------|-------------|-----------------|
| `config/config.go:80-93` | `validateSkillName()` rejects `/`, `\\`, `..`, empty | Replace with `ParseIncludeEntry(raw)` → `IncludeEntry{Src, Dst}` + `validateIncludePath()` that allows `/` but rejects `..`, absolute, traversal |
| `config/config.go:59-62` | `for _, skill := range s.Include { validateSkillName(skill) }` | `for _, raw := range s.Include { ParseIncludeEntry(raw) }` — store parsed entries |
| `config/config.go:17-27` | `SkillSource.Include []string` | Add `ParsedIncludes []IncludeEntry` (populated during Parse, used by sync) |
| `sync/sync.go:44-51` | Collision detection: `skillSource[skill]`, `desiredSkills[skill]` | Key changes to `entry.Dst` (destination path). For multi-provider, key should be `providerName/entry.Dst` or just `entry.Dst` since it's per-provider. |
| `sync/sync.go:134-135` | `srcPath := filepath.Join(baseDir, skill)` | `srcPath := filepath.Join(baseDir, entry.Src)` |
| `sync/sync.go:141` | `dstPath := s.Provider.SkillDir(s.ProjectRoot, skill)` | `dstPath := s.Provider.SkillDir(s.ProjectRoot, entry.Dst)` — `filepath.Join` in SkillDir naturally handles nested paths |
| `sync/sync.go:156-208` | `reconcileStale()` flat ReadDir + `desiredSkills[name]` | Recursive `filepath.WalkDir`, match against relative paths from skillsParent, clean up empty parent dirs |
| `sync/linker.go:22` | `os.MkdirAll(filepath.Dir(dst), 0755)` | Already handles nested parent creation — no change needed |

**Path traversal validation:**
```go
func validateIncludePath(path string) error {
    cleaned := filepath.Clean(path)
    if filepath.IsAbs(cleaned) {
        return fmt.Errorf("invalid include path %q: must be relative", path)
    }
    if cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
        return fmt.Errorf("invalid include path %q: must not escape source root", path)
    }
    return nil
}
```

**Include parsing validation table (from party mode review):**

| Input | Result |
|-------|--------|
| `brainstorm` | `{Src: "brainstorm", Dst: "brainstorm"}` — backward compat |
| `loa/brainstorm` | `{Src: "loa/brainstorm", Dst: "loa/brainstorm"}` — identity map |
| `deep/skill:my-tool` | `{Src: "deep/skill", Dst: "my-tool"}` — remap |
| `loa/brainstorm:` | **Error**: empty destination |
| `:brainstorm` | **Error**: empty source |
| `a:b:c` | `{Src: "a", Dst: "b:c"}` → **Error**: `:` invalid in path (split first `:`, then validate) |
| `../escape:foo` | **Error**: path traversal on source side |
| `foo:../escape` | **Error**: path traversal on destination side |
| `/absolute/path` | **Error**: absolute path |
| `loa` + `loa/brainstorm` (same or cross source) | **Error**: overlapping include (prefix conflict) |

### Cross-Feature Interactions

1. **Profile + Multi-Provider**: Manifest entries must include the provider path prefix (e.g., `.claude/skills/brainstorm`). When syncing profile X for providers [claude, opencode], the manifest records both `.claude/skills/brainstorm` and `.opencode/skills/brainstorm` under profile X.

2. **Profile + Subdirectory**: Manifest entries use the full relative path (e.g., `.claude/skills/loa/brainstorm`). Reconciliation compares against these exact paths.

3. **Multi-Provider + Subdirectory**: Each provider loop produces its own set of `syncedEntries` with paths like `.claude/skills/loa/brainstorm` and `.opencode/skills/loa/brainstorm`. Gitignore gets both.

4. **All Three Combined**: `lore sync -p dev` with `provider: [claude, opencode]` and `include: [loa/brainstorm]` produces manifest entries `[.claude/skills/loa/brainstorm, .opencode/skills/loa/brainstorm]` under profile `dev`, synced to both provider directories, both gitignored.

### Party Mode Decisions (2026-03-25)

**PM-1: Manifest first-run behavior for unknown profiles.**
If a profile has no entries in the manifest (first run), skip reconciliation entirely for that profile. Register whatever gets synced. No error, no retroactive scanning. This handles: (a) `lore sync -p dev` on fresh checkout creates manifest with dev entries, (b) subsequent `lore sync` (default) finds manifest but no default entries — just syncs and registers.

**PM-2: Overlapping includes rejected at parse time.**
`include: [loa, loa/brainstorm]` is invalid — `loa/` symlink would already contain `brainstorm/`, creating a symlink-inside-symlink conflict. Detection: for each pair of destination paths, check if one is a prefix of the other (with `/` separator). Applied both within a source and across sources during collision detection.

**PM-3: `-p default` is reserved.**
Maps to `lore.yml`, not `lore-default.yml`. The profile name "default" in the manifest corresponds to the no-flag invocation.

**PM-4: Multiple colons — split on first `:` only.**
Consistent with docker volume `src:dst` semantics. `a:b:c` splits to `Src="a", Dst="b:c"`, then `b:c` fails path validation (`:` not valid in file paths). No special-case error needed.

**PM-5: Profile name validation expanded.**
Pattern: `[a-z0-9][a-z0-9_-]*`, max 64 chars. Allows underscores in addition to hyphens for common file naming conventions.

**PM-6: Cross-profile provider mismatches are natural.**
Each profile has its own `provider` field. Manifest tracks full relative paths including provider prefix. Profile A with `provider: [claude, opencode]` and profile B with `provider: [claude]` coexist without interference — reconciliation is scoped per-profile per-provider.

**PM-7: Collision warnings include source and destination context.**
With `src:dst` mapping, two different sources can map different source paths to the same destination. Warning message must show both source path and destination path: `"warning: destination %q (from %q in %q) conflicts with %q in %q — last source wins"`.

**PM-8: Empty parent directory cleanup after stale removal.**
When removing a stale nested skill like `loa/brainstorm/`, check if `loa/` is now empty. If empty, remove it. If it contains other managed or user-created entries, leave it. Walk up from the removed leaf, cleaning empty dirs until a non-empty parent is found.

### Pre-mortem Findings (2026-03-25)

**PRE-1: Two-phase sync prevents duplicate git operations.**
Without this, 2 providers × 5 sources = 10 git operations instead of 5. Sync split into: (1) fetch all sources once → `map[source]baseDir`, (2) link per-provider. See TD-7 (updated).

**PRE-2: Orphaned profile detection with `--prune`.**
Deleted config files (`lore-experiment.yml`) leave skills on disk permanently. On every sync, if manifest exists, warn about orphaned profiles. `--prune` flag performs cleanup. See TD-10.

**PRE-3: Atomic manifest writes prevent corruption.**
Crash during write → truncated YAML → next sync misinterprets as empty profile → deletes skills. Fix: temp file + `os.Rename()`. Corrupted manifest on load → warn + treat as absent. See TD-11.

**PRE-4: Nil Provider guard in Sync().**
Old fallback removed. Explicit error instead of panic. See TD-12.

**PRE-5: Concurrent syncs documented as unsupported.**
No file locking — follows docker-compose/terraform precedent. See TD-13.

**PRE-6: v0.1.x → v0.2.x upgrade path test required.**
Add integration test: parse v0.1.x-format `lore.yml` (single string `provider: claude`), run full sync, verify identical behavior to v0.1.x. Ensures `ProviderList` custom unmarshaling + caller loop work for the single-provider case.

## Implementation Plan

### Tasks

Tasks are ordered by dependency — lowest-level changes first, then building up to CLI integration.

#### Task 1: Include path parsing and validation (`internal/config/include.go`)

- [ ] 1.1: Create `IncludeEntry` type and `ParseIncludeEntry()` function
  - File: `internal/config/include.go` (new)
  - Action: Define `type IncludeEntry struct { Src string; Dst string }`. Implement `ParseIncludeEntry(raw string) (IncludeEntry, error)` that:
    - Splits on first `:` to get src and dst
    - If no `:`, dst = src (identity mapping)
    - Calls `validateIncludePath()` on both src and dst
    - Returns error for empty src, empty dst, absolute paths, `..` traversal
  - Notes: `filepath.Clean()` both sides before validation. This replaces `validateSkillName()`.

- [ ] 1.2: Implement `validateIncludePath()` function
  - File: `internal/config/include.go`
  - Action: Validate a single path segment: `filepath.Clean()`, reject absolute paths (`filepath.IsAbs`), reject `..` prefix (`strings.HasPrefix(cleaned, "..")`), reject backslash (`strings.Contains(path, "\\")`), reject colon (`strings.Contains(cleaned, ":")`) — colon is valid on Linux but would break the `src:dst` parsing if used in paths, and is invalid on Windows. Note: this intentionally drops the `/` rejection from the old `validateSkillName()` since `/` is now the subdirectory delimiter.

- [ ] 1.3: Implement `ValidateOverlaps()` function
  - File: `internal/config/include.go`
  - Action: Given a slice of `IncludeEntry`, check all pairs for prefix overlaps on `Dst`. If `a.Dst == b.Dst` or `strings.HasPrefix(a.Dst, b.Dst+"/")` or vice versa, return error identifying the conflicting entries.

- [ ] 1.4: Create include path tests
  - File: `internal/config/include_test.go` (new)
  - Action: Table-driven tests covering the full validation table from the investigation: `brainstorm`, `loa/brainstorm`, `deep/skill:my-tool`, empty src/dst, `a:b:c`, `../escape:foo`, `/absolute/path`, and overlap detection. Test both `ParseIncludeEntry()` and `ValidateOverlaps()`.

#### Task 2: Multi-provider config parsing (`internal/config/config.go`)

- [ ] 2.1: Change `Config.Provider` to `Config.Providers` with union type
  - File: `internal/config/config.go`
  - Action: Define `type ProviderList []string` with custom `UnmarshalYAML` that handles both scalar (`provider: claude`) and sequence (`provider: [claude, opencode]`). Change `Config` struct: `Provider string` → `Providers ProviderList` with `yaml:"provider"` tag (YAML key stays `provider` for backward compat). Remove old `validProviders` map check. Replace with loop validation: each entry must be in `validProviders`, list must not be empty, reject duplicates.

- [ ] 2.2: Add same-URL-different-ref validation
  - File: `internal/config/config.go`
  - Action: After parsing all skills, check for sources that share the same URL (after normalization via `IsGitSource` check) but have different `ref` values. If found, return error: `"skills[%d] and skills[%d] reference the same source %q with different refs (%q vs %q) — consolidate into a single source entry or use different URLs"`. Same URL + same ref (or both empty) is fine — they'll share the cache entry.

- [ ] 2.3: Integrate `ParseIncludeEntry` into `Parse()`
  - File: `internal/config/config.go`
  - Action: Replace `validateSkillName(skill)` loop with `ParseIncludeEntry(raw)` loop. Store parsed entries in new `SkillSource.ParsedIncludes []IncludeEntry` field (no YAML tag — computed during Parse). Remove `validateSkillName()` function entirely.

- [ ] 2.4: Update config tests
  - File: `internal/config/config_test.go`
  - Action: Update existing tests referencing `cfg.Provider` to `cfg.Providers`. Add new test cases:
    - `provider: claude` → `Providers: ["claude"]`
    - `provider: [claude, opencode]` → `Providers: ["claude", "opencode"]`
    - `provider: [claude, claude]` → error (duplicate)
    - `provider: []` → error (empty)
    - `provider: [unknown]` → error (invalid)
    - Include paths with `/` now valid (were rejected before)
    - Include paths with `:` mapping parsed correctly
    - v0.1.x format lore.yml parses identically
    - Two sources with same URL but different refs → error
    - Two sources with same URL and same ref → valid (deduped in fetch)

#### Task 3: Profile-aware config location (`internal/config/locate.go`)

- [ ] 3.1: Add `LocateProfile()` function and profile validation
  - File: `internal/config/locate.go`
  - Action: Add `LocateProfile(dir, profile string) (string, error)`. If `profile == "" || profile == "default"`, delegate to `Locate(dir)`. Otherwise validate profile name (`regexp.MustCompile("^[a-z0-9][a-z0-9_-]*$")`, max 64 chars), then build candidates using `lore-<profile>.yml` in same search order as `Locate()`. Return error if not found: `"no lore-%s.yml found in %s"`.

- [ ] 3.2: Add `ConfigFileName()` helper
  - File: `internal/config/locate.go`
  - Action: Add `ConfigFileName(profile string) string` that returns `"lore.yml"` for empty/default profile, `"lore-<profile>.yml"` otherwise. Used by both `LocateProfile()` and `cmd/init.go`.

- [ ] 3.3: Add locate profile tests
  - File: `internal/config/config_test.go` (or new `locate_test.go`)
  - Action: Test `LocateProfile` with: empty profile → delegates to `Locate`, `"default"` → delegates to `Locate`, `"dev"` → searches for `lore-dev.yml`, invalid profile name → error, profile > 64 chars → error, file not found → error with helpful message.

#### Task 4: Manifest package (`internal/manifest/`)

- [ ] 4.1: Create manifest types and Load/Save functions
  - File: `internal/manifest/manifest.go` (new)
  - Action: Define:
    ```go
    type Manifest struct {
        Version  int                 `yaml:"version"`
        Profiles map[string][]string `yaml:"profiles"`
    }
    ```
    Implement `Load(path string) (*Manifest, error)` — reads YAML, returns nil + warning on parse error (treat as absent). Implement `Save(path string, m *Manifest) error` — atomic write via temp file + `os.Rename()`. Implement `Exists(path string) bool`.

- [ ] 4.2: Add manifest helper methods
  - File: `internal/manifest/manifest.go`
  - Action: Add methods:
    - `(m *Manifest) SetProfile(name string, entries []string)` — sets/replaces entries for a profile
    - `(m *Manifest) GetProfile(name string) ([]string, bool)` — returns entries + whether profile exists
    - `(m *Manifest) RemoveProfile(name string)` — deletes profile entry
    - `(m *Manifest) ProfileNames() []string` — returns all profile names
    - `New() *Manifest` — creates empty manifest with `Version: 1`

- [ ] 4.3: Add retroactive default profile registration
  - File: `internal/manifest/manifest.go`
  - Action: Add `ScanManagedEntries(skillsParentDir string, cacheDir string) ([]string, error)` — walks the skills directory, finds symlinks pointing into `cacheDir` and directories with `.lore-checksum`, returns their relative paths from project root. Used when creating a manifest for the first time to retroactively register existing managed skills under the `default` profile. If no managed entries found, returns empty slice (default profile will have empty entry in manifest).

- [ ] 4.4: Add orphaned profile detection
  - File: `internal/manifest/manifest.go`
  - Action: Add `(m *Manifest) FindOrphaned(dir string, locateFn func(dir, profile string) (string, error)) []string` — for each profile in the manifest, call `locateFn` to check if its config file exists. Return list of profile names whose config files are missing. The `default` profile uses `Locate(dir)`, others use `LocateProfile(dir, name)`.

- [ ] 4.5: Create manifest tests
  - File: `internal/manifest/manifest_test.go` (new)
  - Action: Table-driven tests using `t.TempDir()`:
    - Load valid manifest → correct profiles
    - Load corrupted YAML → nil (no error, treated as absent)
    - Load missing file → nil (no error)
    - Save + Load roundtrip → identical
    - Atomic write: verify temp file cleaned up on success
    - SetProfile/GetProfile/RemoveProfile operations
    - FindOrphaned with missing config files
    - ScanManagedEntries with symlinks + hard copies + user-created dirs
    - New() creates version 1 manifest

#### Task 5: Refactor sync to two-phase architecture (`internal/sync/sync.go`)

- [ ] 5.1: Extract `FetchSources()` as standalone function
  - File: `internal/sync/sync.go`
  - Action: Create `FetchSources(fetcher git.Fetcher, sources []config.SkillSource) (map[string]string, []string)` as a **package-level function** (not a Syncer method — fetch is provider-agnostic and Syncer's Provider field is irrelevant during fetch). Returns `map[sourceURL]baseDir` and a list of error strings. For each source: if git → clone/pull/checkout to cache via `fetcher`, if local → resolve to absolute path and verify directory exists. Both git and local sources produce entries in the `baseDirs` map. Errors are per-source (source-level isolation preserved).

- [ ] 5.2: Refactor `syncSource()` to accept pre-resolved `baseDir`
  - File: `internal/sync/sync.go`
  - Action: Change signature to `syncSource(src config.SkillSource, baseDir string, syncedEntries *[]string) ([]string, error)`. Remove all git/local resolution logic — just iterate `src.ParsedIncludes`, resolve `srcPath = filepath.Join(baseDir, entry.Src)`, `dstPath = provider.SkillDir(root, entry.Dst)`, call `LinkSkill`. The source-level error return is now only for cases where `baseDir` doesn't exist (shouldn't happen if FetchSources succeeded).

- [ ] 5.3: Update `Sync()` to use two-phase flow
  - File: `internal/sync/sync.go`
  - Action: Refactor `Sync()`:
    1. Add nil Provider guard: `if s.Provider == nil { return error }`
    2. Remove the `provider.Get(cfg.Provider)` fallback
    3. Accept `baseDirs map[string]string` parameter (pre-resolved by caller)
    4. Collision detection uses `entry.Dst` instead of flat skill name. Include source + destination in warning message (PM-7).
    5. Overlap detection across sources (call `config.ValidateOverlaps` on aggregated entries)
    6. `desiredSkills` map keyed by destination relative path
    7. Call `syncSource(src, baseDirs[src.Source], &syncedEntries)` for each source

- [ ] 5.4: Refactor `reconcileStale()` to recursive walk
  - File: `internal/sync/sync.go`
  - Action: Replace `os.ReadDir(skillsParent)` with `filepath.WalkDir(skillsParent, ...)`. For each entry that is a symlink pointing into cache or a dir with `.lore-checksum`: compute its relative path from `skillsParent`, check against `desiredSkills` map. If not desired and not owned by another profile (when manifest provided): remove it. After removing stale leaves, walk parent dirs bottom-up and remove empty ones (PM-8). Add `manifest *manifest.Manifest` and `profileName string` parameters to `reconcileStale()` for manifest-aware scoping.

- [ ] 5.5: Add manifest integration to `Sync()`
  - File: `internal/sync/sync.go`
  - Action: Add `Manifest *manifest.Manifest` and `ProfileName string` fields to `Syncer`. In `Sync()`:
    - If manifest exists and profile has entries → pass to `reconcileStale()` for scoped reconciliation
    - If manifest exists and profile has NO entries → skip reconciliation (PM-1)
    - If no manifest → reconcile all (current behavior)
    - After sync, update manifest with current profile's synced entries
    - Return updated manifest in `SyncResult` (caller saves it)

#### Task 6: Update CLI sync command (`cmd/sync.go`)

- [ ] 6.1: Add `-p`/`--profile` and `--prune` flags
  - File: `cmd/sync.go`
  - Action: Add module-level vars `var profileFlag string` and `var pruneFlag bool`. In `init()`, register: `syncCmd.Flags().StringVarP(&profileFlag, "profile", "p", "", "sync a named profile (reads lore-<profile>.yml)")` and `syncCmd.Flags().BoolVar(&pruneFlag, "prune", false, "remove skills from orphaned profiles")`.

- [ ] 6.2a: Config location + parsing with profile support
  - File: `cmd/sync.go`
  - Action: Update `runSync()` opening: locate config via `config.LocateProfile(cwd, profileFlag)` (handles empty/default), parse config, resolve project root, determine profile name (`profileFlag` or `"default"` if empty).

- [ ] 6.2b: Manifest load + orphan warning + prune early-return
  - File: `cmd/sync.go`
  - Action: After config parsing: load manifest if exists via `manifest.Load(filepath.Join(projectRoot, ".lore-manifest.yml"))`. If `--prune` flag set + manifest exists: delegate to prune handler (Task 6.3), return. If manifest exists: check for orphaned profiles and warn (TD-10).

- [ ] 6.2c: Two-phase fetch + per-provider link loop
  - File: `cmd/sync.go`
  - Action: Phase 1 — call `sync.FetchSources(fetcher, cfg.Skills)` → `baseDirs` + fetch errors. Phase 2 — `for _, provName := range cfg.Providers`: get provider, create/configure Syncer with provider + manifest + profile name, call `syncer.Sync(cfg, baseDirs)`. Collect per-provider results.

- [ ] 6.2d: Result aggregation + manifest save
  - File: `cmd/sync.go`
  - Action: Aggregate `SyncResult` across providers (sum Synced/Sources, merge Errors). If multi-provider used OR non-default profile: save manifest (lazy creation — TD-2). If manifest was just created for first time: add `.lore-manifest.yml` to gitignore. Print summary or errors.

- [ ] 6.3: Implement `--prune` handler
  - File: `cmd/sync.go`
  - Action: When `--prune` is set: load manifest, call `manifest.FindOrphaned()`, for each orphaned profile: remove its skills from disk. For symlinks: remove directly. For hard copies (directories with `.lore-checksum`): check checksum for local modifications — if modified, warn and skip (same protection as `LinkSkill`), if unmodified remove. Remove entries from gitignore and manifest. Save updated manifest. Print summary: removed count + skipped count (if any modified hard copies).

#### Task 7: Update CLI init command (`cmd/init.go`)

- [ ] 7.1: Add `-p`/`--profile` flag to init
  - File: `cmd/init.go`
  - Action: Add `var initProfileFlag string`. Register in `init()`. In `runInit()`:
    - Check for existing config using `config.LocateProfile(cwd, initProfileFlag)`
    - Write to `config.ConfigFileName(initProfileFlag)` instead of hardcoded `"lore.yml"`
    - Skeleton `provider` field: if single provider detected/selected → `provider: %s`, if multiple detected → `provider: [%s, %s]`
    - Skeleton `include` comment shows path example: `#   include: [skill-name, path/to/skill]`

#### Task 8: Update sync tests (`internal/sync/sync_test.go`)

- [ ] 8.1: Update existing sync tests for new API
  - File: `internal/sync/sync_test.go`
  - Action: Update all existing tests to use new `Sync()` signature (accepts `baseDirs`). Update skill source configs to use `ParsedIncludes` instead of raw `Include`. Ensure existing tests pass with the refactored code (no behavior change for flat skill names).

- [ ] 8.2: Add subdirectory include sync tests
  - File: `internal/sync/sync_test.go`
  - Action: New test cases:
    - Sync `loa/brainstorm` → creates `skills/loa/brainstorm/` with correct symlink
    - Sync `deep/skill:my-tool` → creates `skills/my-tool/` pointing to `deep/skill/`
    - Stale nested skill removed → parent dir cleaned up if empty
    - Stale nested skill removed → parent dir preserved if contains other entries
    - Collision detection on destination path with `src:dst` mapping

- [ ] 8.3: Add manifest-aware reconciliation tests
  - File: `internal/sync/sync_test.go`
  - Action: New test cases:
    - No manifest → reconcile all (backward compat)
    - Manifest with profile entries → only profile's stale entries removed
    - Manifest with unknown profile (first run) → skip reconciliation, register entries
    - Multi-provider: each provider reconciles independently

- [ ] 8.4: Add v0.1.x backward compatibility integration test
  - File: `internal/sync/sync_test.go`
  - Action: Parse a v0.1.x-style config (`provider: claude`, flat `include: [skill-a]`), run full sync through new code path, verify identical file placement and gitignore output as v0.1.x would produce.

- [ ] 8.5: Add prune, orphan detection, and corrupted manifest integration tests
  - File: `internal/sync/sync_test.go`
  - Action: New test cases:
    - Orphan detection: manifest has profile `dev`, config file `lore-dev.yml` doesn't exist → `FindOrphaned` returns `["dev"]`
    - Prune flow: orphaned profile's skills removed from disk + gitignore entries removed + manifest profile entry removed
    - `--prune` with no manifest → no-op, no error, clean message
    - Corrupted manifest → warning printed, sync proceeds as if no manifest exists (AC26)
    - Partial fetch failure: source A fails, source B succeeds → source B's skills linked, source A error reported (AC28)
  - Notes: For the prune flow test, create a manifest with orphaned entries, run prune logic, verify filesystem + gitignore + manifest are all cleaned up. Use `t.TempDir()` for full isolation.

#### Task 9: Update README documentation

- [ ] 9.1: Document profiles feature
  - File: `README.md`
  - Action: Add section documenting `-p`/`--profile` usage, `lore-<profile>.yml` naming, `--prune` for cleanup. Include examples.

- [ ] 9.2: Document multi-provider feature
  - File: `README.md`
  - Action: Document `provider: [claude, opencode]` syntax with backward compat note. Show example lore.yml.

- [ ] 9.3: Document subdirectory includes
  - File: `README.md`
  - Action: Document `/` path syntax and `src:dst` mapping with examples. Note path traversal restrictions.

- [ ] 9.4: Add concurrent sync warning
  - File: `README.md`
  - Action: Add note per TD-13: concurrent syncs targeting same project are unsupported.

#### Task 10: Version bump and final verification

- [ ] 10.1: Bump version to v0.2.0
  - File: `cmd/root.go`
  - Action: Update `var version = "0.1.0"` (line 9) to `var version = "0.2.0"`. Verify actual current value before editing — recent commits may have bumped it.

- [ ] 10.2: Run full test suite
  - Action: `go test ./...` — all tests pass. `go vet ./...` — no warnings. `go build -o lore ./cmd/lore` — successful build.

### Acceptance Criteria

#### Feature 1: Profiles

- [ ] AC1: Given no `-p` flag with single provider (`provider: claude`), when `lore sync` runs, then it locates and uses `lore.yml` (backward compat, no manifest created). Note: if the default profile uses multi-provider (`provider: [claude, opencode]`), a manifest IS created to track per-provider entries — this is expected and not a backward compat break since multi-provider configs didn't exist in v0.1.x.
- [ ] AC2: Given `-p dev` flag, when `lore sync -p dev` runs, then it locates and uses `lore-dev.yml` from the same search paths as `lore.yml`.
- [ ] AC3: Given `-p default` flag, when `lore sync -p default` runs, then it uses `lore.yml` (reserved keyword).
- [ ] AC4: Given an invalid profile name (e.g., `UPPER`, `has spaces`, > 64 chars), when `lore sync -p <invalid>` runs, then an actionable error is returned.
- [ ] AC5: Given `lore sync` was run (default profile synced skills A, B), when `lore sync -p dev` runs (syncing skills C, D), then skills A and B are NOT removed. A `.lore-manifest.yml` is created tracking both profiles.
- [ ] AC6: Given a manifest exists with profile `default` owning skills A, B, when default's config removes skill A, when `lore sync` runs, then skill A is removed but skill B and all other profiles' skills are untouched.
- [ ] AC7: Given a manifest exists with profile `dev` whose config file (`lore-dev.yml`) was deleted, when `lore sync` runs, then a warning is printed: `"warning: profile "dev" has no config file..."`.
- [ ] AC8: Given `--prune` flag with orphaned profile `dev`, when `lore sync --prune` runs, then dev's skills are removed from disk, gitignore, and manifest.
- [ ] AC9: Given `lore init -p staging`, when run, then `lore-staging.yml` is created with correct skeleton.

#### Feature 2: Multi-Provider

- [ ] AC10: Given `provider: claude` (v0.1.x string format), when parsed, then `cfg.Providers` equals `["claude"]` — full backward compatibility.
- [ ] AC11: Given `provider: [claude, opencode]`, when `lore sync` runs, then skills are placed in both `.claude/skills/` and `.opencode/skills/`.
- [ ] AC12: Given `provider: [claude, opencode]` with 3 git sources, when `lore sync` runs, then git clone/pull happens exactly 3 times (not 6) — two-phase sync verified.
- [ ] AC13: Given `provider: [claude, opencode]`, when skills are synced, then `.gitignore` contains entries for both `.claude/skills/<name>` and `.opencode/skills/<name>`.
- [ ] AC14: Given `provider: [claude, claude]` (duplicate), when parsed, then an error is returned.
- [ ] AC15: Given `provider: []` (empty list), when parsed, then an error is returned.
- [ ] AC15b: Given two skill sources with the same git URL but different `ref` values, when parsed, then an error is returned identifying the conflicting sources.
- [ ] AC15c: Given `provider:` (null/empty YAML value), when parsed, then an error is returned: "missing required field: provider".

#### Feature 3: Subdirectory Includes

- [ ] AC16: Given `include: [loa/brainstorm]`, when synced, then `<provider>/skills/loa/brainstorm/` is created with correct symlink to `<cache>/loa/brainstorm/`.
- [ ] AC17: Given `include: [deep/nested/skill:my-tool]`, when synced, then `<provider>/skills/my-tool/` is created with correct symlink to `<cache>/deep/nested/skill/`.
- [ ] AC18: Given `include: [brainstorm]` (flat, no `/`), when synced, then behavior is identical to v0.1.x — backward compatible.
- [ ] AC19: Given `include: [../escape]`, when parsed, then an error is returned: "must not escape source root".
- [ ] AC20: Given `include: [/absolute/path]`, when parsed, then an error is returned: "must be relative".
- [ ] AC21: Given `include: [loa/brainstorm:]`, when parsed, then an error is returned: empty destination.
- [ ] AC22: Given `include: [loa, loa/brainstorm]` (overlapping), when parsed, then an error is returned: prefix conflict.
- [ ] AC23: Given a stale nested skill `loa/brainstorm/` removed during reconciliation, when `loa/` is now empty, then `loa/` is also removed.
- [ ] AC24: Given a stale nested skill `loa/brainstorm/` removed during reconciliation, when `loa/` contains `loa/other-skill/`, then `loa/` is preserved.

#### Cross-Feature

- [ ] AC25: Given `lore sync -p dev` with `provider: [claude, opencode]` and `include: [loa/brainstorm]`, when synced, then manifest records both `.claude/skills/loa/brainstorm` and `.opencode/skills/loa/brainstorm` under profile `dev`.
- [ ] AC26: Given a corrupted `.lore-manifest.yml`, when `lore sync` runs, then a warning is printed and sync proceeds as if no manifest exists.
- [ ] AC27: Given all v0.2.x changes applied, when a v0.1.x-format `lore.yml` is used with `lore sync`, then behavior is identical to v0.1.x (no manifest, single provider, flat skills).
- [ ] AC28: Given `FetchSources` where source A fails (git clone error) but source B succeeds, when `Sync()` runs, then source B's skills are still linked successfully and source A's failure is reported in `result.Errors`.
- [ ] AC29: Given `--prune` flag with no manifest file, when `lore sync --prune` runs, then it prints "no manifest found — nothing to prune" and exits cleanly (no error).
- [ ] AC30: Given `provider: [claude, opencode]` with 3 git sources (each with a `ref`), when `FetchSources` runs, then mock `GitFetcher.CloneOrPull` is called exactly 3 times and `Checkout` is called exactly 3 times (not 6 of each — verified by counting calls on the mock).
- [x] ~~AC31~~ **REMOVED** — Redundant with AC26. Corrupted manifest is universally treated as absent regardless of profile. Users are warned and can re-sync to rebuild.

## Additional Context

### Dependencies

- No new external dependencies required. All features implemented with existing standard library + yaml.v3 + cobra.
- `os.Rename()` for atomic writes — POSIX standard, works on Linux/macOS. Windows behavior differs (not in scope).
- `filepath.WalkDir` for recursive stale detection — Go 1.16+ standard library.

### Testing Strategy

- **Unit tests** for each new package/function:
  - `internal/config/include_test.go` — ParseIncludeEntry, validateIncludePath, ValidateOverlaps
  - `internal/manifest/manifest_test.go` — Load, Save, atomic write, corruption handling, profile operations
  - `internal/config/config_test.go` — ProviderList unmarshaling, updated Parse with new fields
  - `internal/config/locate_test.go` — LocateProfile, profile validation, ConfigFileName
- **Integration tests** in `internal/sync/sync_test.go`:
  - Two-phase sync flow with mock git fetcher (verify call count for AC30)
  - Multi-provider linking (skills placed in both provider dirs)
  - Manifest-scoped reconciliation (profile isolation)
  - Nested skill sync + stale cleanup with empty parent removal
  - v0.1.x backward compatibility end-to-end
  - Partial fetch failure → partial link success (AC28)
  - Prune flow: orphaned profile cleanup (disk + gitignore + manifest)
  - Prune with no manifest → clean no-op (AC29)
  - Corrupted manifest → warning + proceeds as absent (AC26)
- **cmd/ orchestration testing** — `cmd/sync.go` is no longer a thin wiring layer after the two-phase sync, manifest logic, and prune handler. Key orchestration paths (profile resolution → fetch → per-provider link → manifest save) are tested indirectly via `sync_test.go` for the Syncer logic. The cmd-level decisions (manifest lazy creation trigger, prune early-return, orphan warning) should be extracted into testable helper functions in `cmd/sync.go` rather than inline in `runSync()`, so they can be unit tested without executing the full Cobra command.
- **Manual testing checklist:**
  - `lore sync` with existing v0.1.x `lore.yml` → no behavior change
  - `lore sync -p dev` creates manifest on first use
  - `lore sync` after `lore sync -p dev` → default skills preserved
  - `lore sync --prune` removes orphaned profile skills
  - Multi-provider `provider: [claude, opencode]` places skills in both dirs
  - Subdirectory `include: [loa/brainstorm]` creates nested structure
  - Mapped `include: [deep/skill:my-tool]` remaps correctly

### Notes

- **Task ordering is critical.** Tasks 1-4 are foundational (types, parsing, manifest). Task 5 is the core refactor. Tasks 6-7 are CLI wiring. Tasks 8-9 are tests/docs. Task 10 is final verification. A dev agent should follow this order strictly. **Key dependency:** Task 2.1 renames `Config.Provider` to `Config.Providers` — all code in Tasks 5+ that previously referenced `cfg.Provider` must use `cfg.Providers`. The compile will catch this but the dev agent should be aware.
- **The two-phase sync (Task 5.1-5.3) is the highest-risk refactor.** It changes the `Sync()` signature and restructures the core flow. All existing sync tests must pass after this change — run tests after each sub-task.
- **Manifest lazy creation (TD-2) interacts with multiple code paths.** The decision tree is: no `-p` flag + no manifest → don't create one. Non-default profile → create manifest + retroactively register default's entries. Manifest exists → always use it.
- **`reconcileStale()` recursive walk (Task 5.4) replaces a simple ReadDir.** The flat case must still work identically — add regression tests for flat skill names with the new recursive walk.
- **The `ProviderList` custom unmarshaler must preserve the YAML tag as `provider` (not `providers`)** for backward compatibility. The Go field name changes to `Providers` but the YAML key stays `provider`.
- **Stale local-source symlinks** (from v0.1.x gap analysis) remain un-reconciled. The recursive walk doesn't change this — local-source symlinks point outside the cache and don't have `.lore-checksum`. This is a known limitation — document in README: "Skills from local sources are not automatically cleaned up when removed from config. Remove the symlink manually."
- **Concurrent syncs (TD-13):** No file locking added. If this becomes a real issue in CI environments, revisit with `flock`-based advisory locking in a future version.
- **`filepath.Clean()` silently strips trailing slashes** — `include: [loa/brainstorm/]` becomes `loa/brainstorm`. This is intentional and acceptable (Clean normalizes paths).
- **Line number references in anchor points are informational.** Earlier tasks modify files, causing line drift. Dev agent should search for function/variable names rather than relying on line numbers.
- **`filepath.Dir(SkillDir(root, "dummy"))` trick** to find the skills parent directory works because "dummy" has no `/`. The recursive walk root is always `<root>/<provider>/skills/`. Document this in a code comment.
- **Platform scope:** This spec targets Linux and macOS. Windows is not supported (atomic `os.Rename`, symlink behavior, `:` in paths all differ). Not in scope for v0.2.x.
- **`go-git/v5` dependency** is retained for `GoGitFetcher` which is used in `sync_test.go` integration tests to avoid requiring system `git` in CI. `ExecGitFetcher` is the production default (SSH key/config support). Both implement `git.Fetcher` interface. Not user-selectable — wiring is hardcoded in `cmd/sync.go`. Future cleanup could replace GoGitFetcher with a mock fetcher in tests (option 3 from review), but that's out of scope for v0.2.x.
- **`lore init -p staging`** only checks for existing `lore-staging.yml` — it does NOT require a default `lore.yml` to exist first. Profiles are independent.

## Review Notes

- Adversarial review completed (27 findings)
- Findings addressed: F-1 through F-21 fixed inline, F-22 through F-27 acknowledged in Notes
- Critical fixes: F-2 (same-URL-different-ref rejected at parse time), F-3 (two-level overlap validation clarified), F-1 (go-git role clarified)
- High fixes: F-5 (AC1 updated for multi-provider), F-6 (--prune is prune-only), F-8 (retroactive registration task added as 4.3), F-9 (intermediate dir handling specified), F-10 (version string fixed)
- Medium fixes: F-11/F-24 (colon rejection added), F-13 (AC30 includes Checkout), F-14 (manifest gitignore handling), F-16 (null provider), F-19 (prune respects hard copy checksums), F-20 (cmd/ helper extraction)
- Acknowledged in Notes: F-12 (line numbers informational), F-15 (dummy trick documented), F-17 (stale local symlinks documented), F-22 (minor UX, accepted), F-23 (go-git retained for tests), F-25 (init -p independence documented), F-26 (Clean strips trailing slash, intentional), F-27 (platform scope stated)

