---
title: 'Loremaster MVP'
slug: 'loremaster-mvp'
created: '2026-03-21'
status: 'completed'
stepsCompleted: [1, 2, 3, 4]
tech_stack: ['Go 1.22+', 'Cobra', 'go-git/v5', 'gopkg.in/yaml.v3']
files_to_modify:
  - 'go.mod'
  - 'main.go'
  - 'cmd/root.go'
  - 'cmd/init.go'
  - 'cmd/sync.go'
  - 'internal/config/config.go'
  - 'internal/config/locate.go'
  - 'internal/config/config_test.go'
  - 'internal/provider/provider.go'
  - 'internal/provider/claude.go'
  - 'internal/provider/opencode.go'
  - 'internal/provider/detect.go'
  - 'internal/cache/cache.go'
  - 'internal/cache/cache_test.go'
  - 'internal/git/git.go'
  - 'internal/git/git_test.go'
  - 'internal/sync/sync.go'
  - 'internal/sync/linker.go'
  - 'internal/sync/sync_test.go'
  - 'internal/gitignore/gitignore.go'
  - 'internal/gitignore/gitignore_test.go'
code_patterns:
  - 'internal/ for all domain logic, cmd/ for CLI wiring'
  - 'Provider interface for tool-agnostic skill directory resolution'
  - 'Sync orchestrator pattern: config → git → linker → reconcile → gitignore'
  - 'GitFetcher interface for testable git operations'
  - 'Single-responsibility packages'
test_patterns:
  - 'Table-driven tests with _test.go per package'
  - 't.TempDir() for filesystem operations'
  - 'Interface-based mocking for git operations'
  - 'Local git repo fixtures for integration tests'
---

# Tech-Spec: Loremaster MVP

**Created:** 2026-03-21

## Overview

### Problem Statement

Developers managing AI coding skills across multiple projects have no declarative, tool-agnostic way to sync skills from git repos. They manually copy files, lose track of versions, and risk committing private skills into project repositories.

### Solution

A stateless Go CLI (`lore init` + `lore sync`) that reads a declarative `lore.yml` manifest, clones/pulls skill repos into a central cache using go-git, symlinks (or copies) skills into the target tool's skills directory, and auto-manages `.gitignore` entries. Built with Cobra for subcommand structure and shell completions.

### Scope

**In Scope:**
- `lore init` — bootstrap `lore.yml` with provider auto-detection (Claude Code, OpenCode)
- `lore sync` — clone/pull repos, symlink or copy skills, reconcile stale links, update `.gitignore`
- `lore.yml` parsing — repo-grouped format with `source`, `ref`, `include`, `type`
- Local path sources with `include` list support
- `go-git` for all git operations (pure Go, zero runtime dependencies)
- Cobra CLI framework with shell completions (bash, zsh, fish) registered in root command
- Central cache at `~/.local/share/loremaster/` (respects `XDG_DATA_HOME`)
- Symlink-first design, hard copy opt-in per skill
- Auto-gitignore management (idempotent, append-only, with stale entry cleanup)
- Stale symlink reconciliation (remove skills no longer declared in config)
- `.lore-checksum` for hard copy modification detection
- Linux and macOS support
- 32 functional requirements from the PRD (FR33 dropped — no `name` field)

**Out of Scope:**
- `lore status`, `lore verify`, `.lore.lock` (Phase 2)
- Cursor, Windsurf, or other provider support (Phase 2)
- Windows support (Phase 3)
- Community skill discovery and sharing (Phase 3)
- Interactive prompts (except tool selection ambiguity in `lore init`)
- Global/project config merging — project config wins, no merge behavior
- Single-skill imports without `include` list

## Context for Development

### Project Structure

```
loremaster/
├── go.mod
├── go.sum
├── main.go                          # Entry point, calls cmd.Execute()
├── cmd/
│   ├── root.go                      # Root command, version flag, shell completions
│   ├── init.go                      # lore init
│   └── sync.go                      # lore sync
├── internal/
│   ├── config/
│   │   ├── config.go                # lore.yml parsing & validation
│   │   ├── config_test.go
│   │   └── locate.go                # Config file discovery (CWD-scoped)
│   ├── provider/
│   │   ├── provider.go              # Provider interface + registry
│   │   ├── claude.go                # Claude Code provider
│   │   ├── opencode.go              # OpenCode provider
│   │   └── detect.go                # Auto-detection logic
│   ├── cache/
│   │   ├── cache.go                 # Central cache management + URL normalization
│   │   └── cache_test.go
│   ├── git/
│   │   ├── git.go                   # GitFetcher interface + go-git implementation
│   │   └── git_test.go
│   ├── sync/
│   │   ├── sync.go                  # Orchestrator: config → git → linker → reconcile → gitignore
│   │   ├── linker.go                # Symlink/copy logic + .lore-checksum
│   │   └── sync_test.go
│   └── gitignore/
│       ├── gitignore.go             # Idempotent .gitignore management + stale entry cleanup
│       └── gitignore_test.go
```

### Codebase Patterns

- **Confirmed Clean Slate** — greenfield Go project, no existing code or legacy constraints
- Stateless CLI design — no daemon, no background processes, no database
- Single binary distribution — Go's static compilation
- Config-driven — `lore.yml` is the single source of truth
- `internal/` for all domain logic, `cmd/` for CLI wiring only
- Provider interface pattern for tool-agnostic skill directory resolution
- `GitFetcher` interface for testable git operations
- Sync orchestrator pattern: resolve config → fetch git sources → create links → reconcile stale → update gitignore
- Single-responsibility packages — each package does one thing

### Files to Reference

| File | Purpose |
| ---- | ------- |
| `_bmad-output/prd.md` | Full PRD with functional requirements, user journeys, and non-functional requirements |

### Technical Decisions

- **go-git** over shelling out to system git — pure Go binary with zero runtime dependencies
- **Cobra** for CLI framework — mature, built-in shell completion, subcommand structure
- **Shell completions in `root.go`** — no separate `completion.go`; Cobra's built-in generation registered directly on root command
- **Symlink-first** design — skills point to cache, enabling in-place editing and automatic upstream propagation
- **Single interactive moment** — only `lore init` when provider auto-detection is ambiguous; everything else is non-interactive
- **Local path sources** support `include` lists just like git sources — `source: /path` + `include: [foo]` resolves to `/path/foo`
- **`internal/` over `pkg/`** — this is a CLI, not a library; no public API needed
- **Provider as interface** — `SkillDir(name string) string` — minimal surface, easy to extend for Phase 2 providers
- **Config discovery is CWD-scoped** — walk from CWD checking `lore.yml`, `.claude/lore.yml`, `.opencode/lore.yml`; first match wins. No special "global mode" — running from `~` naturally targets global scope
- **No config merging** — project config wins entirely; no global+project merge behavior in MVP
- **Cache key via URL normalization** — strip protocol, strip trailing `.git`, lowercase host to compute cache directory name. Clone always uses the exact URL from `lore.yml`. First clone's remote URL wins for subsequent pulls
- **Stale symlink reconciliation** — before linking, scan provider skill directory for symlinks pointing into cache that aren't in current `include` list; remove them and their `.gitignore` entries
- **`.lore-checksum`** — stored alongside hard-copied skills to detect local modifications (FR17); skip overwrite and warn if checksum mismatch
- **FR33 dropped** — no `name` field in MVP; `include` is always required as a list
- **Error message format** — structured as: `error: sync failed for source "<source>": <reason> (<actionable hint>)`

### lore.yml Format

```yaml
provider: claude
skills:
  - source: git@github.com:user/skills.git
    ref: main
    include: [commit-message, code-review]
    type: soft

  - source: git@github.com:org/shared-skills.git
    ref: v1.2.0
    include: [frontend-design]
    type: hard

  - source: /home/user/local-skills
    include: [my-custom-skill]
    type: soft
```

**Field definitions:**
- `provider` — target AI tool (`claude` or `opencode`); required
- `skills[].source` — git URL or local absolute/relative path; required
- `skills[].ref` — git ref (branch, tag, or commit SHA); optional, defaults to default branch
- `skills[].include` — list of skill directory names to sync from this source; required
- `skills[].type` — `soft` (symlink, default) or `hard` (copy); optional

## Implementation Plan

### Tasks

#### Task 1: Project Scaffolding

- [x]1.1: Initialize Go module
  - File: `go.mod`
  - Action: Run `go mod init github.com/dgjalic/loremaster` (or chosen module path). Set Go version to 1.22+.

- [x]1.2: Create entry point
  - File: `main.go`
  - Action: Create minimal `main.go` that calls `cmd.Execute()`. Import `cmd` package. Handle non-zero exit on error.

- [x]1.3: Create root command with shell completions
  - File: `cmd/root.go`
  - Action: Define root `lore` command via Cobra. Add `--version` flag. Register `completion` subcommand using `cobra.Command` with `GenBashCompletionV2`, `GenZshCompletion`, `GenFishCompletion` as sub-subcommands (bash, zsh, fish). Export `Execute()` function.

- [x]1.4: Add dependencies
  - Action: `go get github.com/spf13/cobra github.com/go-git/go-git/v5 gopkg.in/yaml.v3`

#### Task 2: Config Parsing & Discovery

- [x]2.1: Define config types
  - File: `internal/config/config.go`
  - Action: Define Go structs mapping the `lore.yml` format:
    ```go
    type Config struct {
        Provider string        `yaml:"provider"`
        Skills   []SkillSource `yaml:"skills"`
    }
    type SkillSource struct {
        Source  string   `yaml:"source"`
        Ref     string   `yaml:"ref,omitempty"`
        Include []string `yaml:"include"`
        Type    string   `yaml:"type,omitempty"` // "soft" (default) or "hard"
    }
    ```
  - Action: Implement `Parse(r io.Reader) (*Config, error)` that unmarshals YAML and validates:
    - `provider` must be non-empty and one of `claude`, `opencode`
    - `skills` must have at least one entry
    - Each skill must have non-empty `source` and non-empty `include`
    - `type` defaults to `"soft"` if empty; must be `"soft"` or `"hard"` if set
  - Action: Implement `IsGitSource(source string) bool` — returns true if source looks like a git URL (contains `://`, starts with `git@`, or ends with `.git`)

- [x]2.2: Implement config file discovery
  - File: `internal/config/locate.go`
  - Action: Implement `Locate(dir string) (string, error)` that searches from the given directory:
    1. `<dir>/lore.yml`
    2. `<dir>/.claude/lore.yml`
    3. `<dir>/.opencode/lore.yml`
    - Return the first path found. Return error if none found.
  - Notes: No upward directory walking. No global `~/lore.yml` search — that happens naturally when CWD is `~`.

- [x]2.3: Write config tests
  - File: `internal/config/config_test.go`
  - Action: Table-driven tests covering:
    - Valid minimal config (provider + one source with include)
    - Valid full config (all fields populated)
    - Missing provider → error
    - Missing include → error
    - Invalid provider value → error
    - Invalid type value → error
    - Type defaults to "soft" when omitted
    - `IsGitSource` with SSH URL, HTTPS URL, local path
    - `Locate` with config in CWD, in `.claude/`, in `.opencode/`, not found

#### Task 3: Provider System

- [x]3.1: Define provider interface and registry
  - File: `internal/provider/provider.go`
  - Action: Define:
    ```go
    type Provider interface {
        Name() string
        SkillDir(projectRoot string, skillName string) string
        GlobalSkillDir(skillName string) string
        MarkerDir() string // e.g. ".claude" — used for detection
    }
    ```
  - Action: Implement `Get(name string) (Provider, error)` registry function that returns the matching provider or an error for unknown names.

- [x]3.2: Implement Claude Code provider
  - File: `internal/provider/claude.go`
  - Action: Implement `Provider` interface:
    - `Name()` → `"claude"`
    - `SkillDir(projectRoot, skillName)` → `<projectRoot>/.claude/skills/<skillName>`
    - `GlobalSkillDir(skillName)` → `~/.claude/skills/<skillName>`
    - `MarkerDir()` → `".claude"`

- [x]3.3: Implement OpenCode provider
  - File: `internal/provider/opencode.go`
  - Action: Implement `Provider` interface:
    - `Name()` → `"opencode"`
    - `SkillDir(projectRoot, skillName)` → `<projectRoot>/.opencode/skills/<skillName>`
    - `GlobalSkillDir(skillName)` → `~/.config/opencode/skills/<skillName>`
    - `MarkerDir()` → `".opencode"`

- [x]3.4: Implement provider auto-detection
  - File: `internal/provider/detect.go`
  - Action: Implement `Detect(projectRoot string) ([]Provider, error)` that scans `projectRoot` for marker directories (`.claude/`, `.opencode/`). Return all matching providers. Caller decides: if exactly one, use it; if multiple, prompt user; if zero, error.

#### Task 4: Cache Management

- [x]4.1: Implement cache directory management
  - File: `internal/cache/cache.go`
  - Action: Implement `Dir() string` that returns `$XDG_DATA_HOME/loremaster/` or `~/.local/share/loremaster/` as fallback.
  - Action: Implement `NormalizeURL(rawURL string) string` that:
    1. Strips protocol (`https://`, `git://`, `ssh://`)
    2. Converts `git@host:path` to `host/path`
    3. Strips trailing `.git`
    4. Lowercases the host portion
    5. Returns the normalized string
  - Action: Implement `RepoDir(rawURL string) string` that returns `<cacheDir>/<sha256(NormalizeURL(rawURL))[:16]>` — a truncated hash (16 bytes / 32 hex chars) for the directory name.
  - Action: Implement `SkillPath(rawURL string, skillName string) string` that returns `<RepoDir(rawURL)>/<skillName>`.
  - Action: Implement `EnsureDir() error` that creates the cache directory if it doesn't exist.

- [x]4.2: Write cache tests
  - File: `internal/cache/cache_test.go`
  - Action: Table-driven tests covering:
    - `NormalizeURL`: SSH and HTTPS URLs for the same repo produce the same normalized string
    - `NormalizeURL`: trailing `.git` stripped
    - `NormalizeURL`: host lowercased
    - `RepoDir`: deterministic output for same URL
    - `RepoDir`: different URLs produce different dirs
    - `SkillPath`: correct path composition
    - `Dir`: respects `XDG_DATA_HOME` env var (set in test, unset after)

#### Task 5: Git Operations

- [x]5.1: Define GitFetcher interface and implement with go-git
  - File: `internal/git/git.go`
  - Action: Define interface:
    ```go
    type Fetcher interface {
        CloneOrPull(url string, targetDir string) error
        Checkout(repoDir string, ref string) error
    }
    ```
  - Action: Implement `GoGitFetcher` struct satisfying `Fetcher`:
    - `CloneOrPull`: If `targetDir` doesn't exist, clone using `go-git`. If it exists, open the repo and pull (fetch + merge fast-forward on current branch). Use default auth (SSH agent, git credential helpers via go-git's transport).
    - `Checkout`: Open repo at `repoDir`, resolve `ref` as branch/tag/commit, checkout the worktree to that ref.
  - Notes: If `ref` is empty, skip `Checkout` — use whatever the default branch is after clone/pull.

- [x]5.2: Write git tests
  - File: `internal/git/git_test.go`
  - Action: Integration-style tests using `t.TempDir()`:
    - Create a bare git repo with `go-git` in a temp dir, add a commit with a skill directory
    - Test `CloneOrPull` clones into new directory
    - Test `CloneOrPull` pulls into existing clone (add another commit, pull, verify new content)
    - Test `Checkout` to a specific tag
    - Test `CloneOrPull` with invalid URL returns descriptive error

#### Task 6: Gitignore Management

- [x]6.1: Implement idempotent gitignore management
  - File: `internal/gitignore/gitignore.go`
  - Action: Implement `EnsureEntries(gitignorePath string, entries []string) error`:
    1. Read existing `.gitignore` (create if doesn't exist)
    2. Parse existing entries into a set
    3. Append only entries not already present
    4. Add a `# Managed by loremaster` header comment before the first loremaster entry (if not already present)
    5. Write back with preserved formatting (trailing newline)
  - Action: Implement `RemoveEntries(gitignorePath string, entries []string) error`:
    1. Read existing `.gitignore`
    2. Remove matching lines
    3. Remove the `# Managed by loremaster` header if no loremaster entries remain
    4. Write back
  - Action: Implement `ManagedEntries(gitignorePath string) ([]string, error)`:
    1. Read `.gitignore`
    2. Return all entries in the loremaster-managed section (between `# Managed by loremaster` and the next blank line or section header)

- [x]6.2: Write gitignore tests
  - File: `internal/gitignore/gitignore_test.go`
  - Action: Table-driven tests using `t.TempDir()`:
    - `EnsureEntries` on non-existent file → creates file with header + entries
    - `EnsureEntries` on existing file → appends without duplicating
    - `EnsureEntries` idempotent — running twice produces same file
    - `EnsureEntries` preserves existing non-loremaster content
    - `RemoveEntries` removes specified entries
    - `RemoveEntries` cleans up header when section becomes empty
    - `ManagedEntries` returns correct entries

#### Task 7: Sync Orchestrator & Linker

- [x]7.1: Implement linker (symlink/copy with checksum)
  - File: `internal/sync/linker.go`
  - Action: Implement `LinkSkill(src string, dst string, linkType string) error`:
    - If `linkType == "soft"`: remove existing `dst` if present, create symlink `dst → src`
    - If `linkType == "hard"`:
      1. If `dst` exists and `.lore-checksum` exists in `dst`:
         - Compute current checksum of `dst` contents
         - Compare with stored checksum
         - If mismatch: print warning `warning: skipping "<dst>": local modifications detected`, return nil (skip, don't error)
      2. Copy `src` directory recursively to `dst`
      3. Compute checksum of copied contents, write to `<dst>/.lore-checksum`
  - Action: Implement `ComputeDirChecksum(dir string) (string, error)` — walk directory, hash all file contents in sorted order, return hex SHA256.

- [x]7.2: Implement sync orchestrator
  - File: `internal/sync/sync.go`
  - Action: Define `Syncer` struct with dependencies:
    ```go
    type Syncer struct {
        GitFetcher git.Fetcher
        CacheDir   string
        Provider   provider.Provider
        ProjectRoot string
    }
    ```
  - Action: Implement `Sync(cfg *config.Config) error`:
    1. **Resolve provider** from `cfg.Provider` using `provider.Get()`
    2. **Collect desired skill set** — iterate all sources and their includes to build a set of expected skill names
    3. **For each source in `cfg.Skills`**:
       a. **Determine source type**: `config.IsGitSource(source)` → git path, else local path
       b. **If git source**:
          - Compute cache dir via `cache.RepoDir(source.Source)`
          - Call `GitFetcher.CloneOrPull(source.Source, cacheDir)`
          - If `source.Ref` is set, call `GitFetcher.Checkout(cacheDir, source.Ref)`
       c. **If local source**:
          - Verify `source.Source` path exists, error if not
       d. **For each skill in `source.Include`**:
          - Compute `srcPath`: for git → `cache.SkillPath(source.Source, skill)`, for local → `<source.Source>/<skill>`
          - Verify `srcPath` exists as a directory
          - Compute `dstPath`: `provider.SkillDir(projectRoot, skill)`
          - Call `LinkSkill(srcPath, dstPath, source.Type)`
          - Collect skill name for gitignore
       e. **If source fails** (clone error, path not found): log error with structured format, continue to next source (partial failure isolation — FR29)
    4. **Reconcile stale skills**: scan provider skill directory for symlinks pointing into the cache directory that aren't in the desired skill set → remove them
    5. **Update gitignore**: call `gitignore.EnsureEntries()` with all successfully synced skill paths. Call `gitignore.RemoveEntries()` for any reconciled (removed) stale entries.
    6. **Return**: nil if all sources succeeded, aggregated error if any failed
  - Notes: Each failed source is logged but does not abort the entire sync. Final return includes all errors.

- [x]7.3: Write sync tests
  - File: `internal/sync/sync_test.go`
  - Action: Tests using a mock `Fetcher` implementation and `t.TempDir()`:
    - Sync with one git source, two skills → both symlinked, both in gitignore
    - Sync with one local source → skills symlinked from local path (no git calls)
    - Sync with hard copy type → directory copied, `.lore-checksum` written
    - Sync with hard copy + local modifications → warning logged, skill skipped
    - Sync with failed git source → other sources still synced, error returned
    - Sync with invalid skill name (directory doesn't exist in source) → error for that skill, others proceed
    - Stale reconciliation: previously synced skill removed from config → symlink deleted, gitignore entry removed
    - Idempotent sync: running sync twice produces identical filesystem state

#### Task 8: CLI Commands

- [x]8.1: Implement `lore init`
  - File: `cmd/init.go`
  - Action: Register `init` subcommand on root. Implementation:
    1. Check if `lore.yml` already exists (via `config.Locate(cwd)`) → if yes, print `"lore.yml already exists at <path>"` and exit 0
    2. Call `provider.Detect(cwd)` to scan for marker directories
    3. If exactly one provider found → use it
    4. If multiple providers found → prompt user with numbered selection (the only interactive moment)
    5. If no providers found → error: `"error: no supported AI tool detected (looked for .claude/, .opencode/)"`
    6. Generate skeleton `lore.yml` with detected provider and a commented-out example source
    7. Write `lore.yml` to CWD
    8. Print: `"Created lore.yml for <provider>"`

- [x]8.2: Implement `lore sync`
  - File: `cmd/sync.go`
  - Action: Register `sync` subcommand on root. Implementation:
    1. Get CWD
    2. Call `config.Locate(cwd)` → error if not found: `"error: no lore.yml found (run 'lore init' first)"`
    3. Read and parse config via `config.Parse()`
    4. Resolve project root from config file location (parent of `lore.yml`, or parent of `.claude/` if found there)
    5. Initialize `Syncer` with `GoGitFetcher`, cache dir, resolved provider, project root
    6. Call `Syncer.Sync(cfg)`
    7. Print summary: `"Synced N skills from M sources"` or error details
    8. Exit with non-zero code if any errors occurred

#### Task 9: Tests & Verification

- [x]9.1: Integration test for full sync flow
  - File: `internal/sync/sync_test.go` (append)
  - Action: End-to-end test:
    1. Create a bare git repo in `t.TempDir()` with two skill directories
    2. Write a `lore.yml` pointing at the bare repo
    3. Run `Syncer.Sync()` with real `GoGitFetcher`
    4. Verify: skills symlinked, `.gitignore` updated, cache directory populated
    5. Remove one skill from config, re-sync
    6. Verify: removed skill's symlink is gone, gitignore entry removed, remaining skill untouched

- [x]9.2: Verify all tests pass
  - Action: Run `go test ./...` and ensure zero failures
  - Action: Run `go vet ./...` and ensure zero warnings

- [x]9.3: Build and smoke test
  - Action: Run `go build -o lore .`
  - Action: Verify `./lore --version`, `./lore init --help`, `./lore sync --help`, `./lore completion bash` all produce expected output

### Acceptance Criteria

#### Config Parsing (FR5-FR10)

- [x]AC1: Given a valid `lore.yml` with provider and one source, when parsed, then a `Config` struct is returned with all fields correctly populated
- [x]AC2: Given a `lore.yml` with missing `provider`, when parsed, then an error is returned indicating the missing field
- [x]AC3: Given a `lore.yml` with a source missing `include`, when parsed, then an error is returned indicating the missing field
- [x]AC4: Given a `lore.yml` with `type` omitted, when parsed, then `type` defaults to `"soft"`
- [x]AC5: Given a `lore.yml` with an invalid `type` value, when parsed, then an error is returned
- [x]AC6: Given a project with `lore.yml` in `.claude/`, when `Locate()` is called from the project root, then the path `.claude/lore.yml` is returned
- [x]AC7: Given a project with no `lore.yml` anywhere, when `Locate()` is called, then an error is returned

#### Provider Detection (FR2-FR4, FR22-FR24)

- [x]AC8: Given a project with only `.claude/` directory, when `Detect()` is called, then a single Claude provider is returned
- [x]AC9: Given a project with both `.claude/` and `.opencode/`, when `Detect()` is called, then both providers are returned (caller prompts user)
- [x]AC10: Given provider name `"claude"`, when `SkillDir("project", "foo")` is called, then `"project/.claude/skills/foo"` is returned
- [x]AC11: Given provider name `"opencode"`, when `GlobalSkillDir("bar")` is called, then `"~/.config/opencode/skills/bar"` is returned

#### Cache Management (FR25-FR27)

- [x]AC12: Given SSH URL `git@github.com:User/Repo.git` and HTTPS URL `https://github.com/user/repo.git`, when `NormalizeURL()` is called on both, then the same normalized string is returned
- [x]AC13: Given a repo URL, when `RepoDir()` is called twice, then the same cache path is returned (deterministic)
- [x]AC14: Given `XDG_DATA_HOME=/tmp/test`, when `Dir()` is called, then `/tmp/test/loremaster/` is returned

#### Git Operations (FR12-FR14)

- [x]AC15: Given a valid git repo URL and an empty cache directory, when `CloneOrPull()` is called, then the repo is cloned into the target directory
- [x]AC16: Given an already-cloned repo in cache, when `CloneOrPull()` is called, then new commits are pulled
- [x]AC17: Given a cloned repo and a valid tag ref, when `Checkout()` is called, then the worktree is at the specified tag
- [x]AC18: Given an invalid git URL, when `CloneOrPull()` is called, then an error with actionable context is returned

#### Sync Operations (FR11, FR15-FR18, FR28-FR31)

- [x]AC19: Given a config with two git sources each with two skills, when `Sync()` is called with cold cache, then all four skills are symlinked into the provider skill directory
- [x]AC20: Given a config with a local path source and `include: [foo, bar]`, when `Sync()` is called, then `<source>/foo` and `<source>/bar` are symlinked (no git operations)
- [x]AC21: Given a config with `type: hard`, when `Sync()` is called, then the skill directory is copied (not symlinked) and a `.lore-checksum` file is written
- [x]AC22: Given a hard-copied skill with local modifications (checksum mismatch), when `Sync()` is called, then a warning is printed and the skill is not overwritten
- [x]AC23: Given a config where source A fails (bad URL) and source B succeeds, when `Sync()` is called, then source B's skills are synced and source A's error is reported
- [x]AC24: Given a previously synced skill that is removed from `include`, when `Sync()` is called, then the stale symlink is removed and its `.gitignore` entry is cleaned up

#### Git Exclusion (FR19-FR21)

- [x]AC25: Given a project with no `.gitignore`, when sync creates symlinks, then a `.gitignore` is created with a `# Managed by loremaster` section containing the skill paths
- [x]AC26: Given a project with an existing `.gitignore`, when sync adds new skills, then existing entries are preserved and new entries are appended without duplication
- [x]AC27: Given a sync that runs twice with no config changes, when `.gitignore` is checked, then it is identical after both runs (idempotent)

#### Init Command (FR1-FR4)

- [x]AC28: Given a project with `.claude/` directory and no `lore.yml`, when `lore init` is run, then a `lore.yml` is created with `provider: claude`
- [x]AC29: Given a project where `lore.yml` already exists, when `lore init` is run, then it prints a message and exits without overwriting
- [x]AC30: Given a project with no `.claude/` or `.opencode/` directory, when `lore init` is run, then an error is printed: `"no supported AI tool detected"`

#### Shell Completions (FR32)

- [x]AC31: Given `lore completion bash` is run, then valid bash completion script is output to stdout
- [x]AC32: Given `lore completion zsh` is run, then valid zsh completion script is output to stdout

#### Error Handling (FR28-FR31)

- [x]AC33: Given any sync failure, when the error is printed, then it follows the format: `error: sync failed for source "<source>": <reason> (<hint>)`
- [x]AC34: Given any sync failure, when the process exits, then the exit code is non-zero

## Additional Context

### Dependencies

- `github.com/spf13/cobra` — CLI framework
- `github.com/go-git/go-git/v5` — pure Go git implementation
- `gopkg.in/yaml.v3` — YAML parsing

### Testing Strategy

- **Unit tests** per package with `_test.go` files using standard `go test`
- **Filesystem tests** using `t.TempDir()` for symlink, copy, gitignore, cache, and reconciliation operations
- **Interface-based mocking** — `GitFetcher` interface allows sync package tests without real git operations
- **Integration tests** — end-to-end `lore sync` using a local bare git repo as a fixture (created in test setup via go-git)
- **Table-driven tests** for config parsing (valid YAML, invalid YAML, missing fields, edge cases)
- **No external test dependencies** — standard library `testing` package only

### Notes

- Cache location: `$XDG_DATA_HOME/loremaster/` or `~/.local/share/loremaster/`
- One cache entry per unique normalized repo URL
- Provider skill directories: `.claude/skills/<name>/` and `.opencode/skills/<name>/`
- Global skill directories: `~/.claude/skills/<name>/` and `~/.config/opencode/skills/<name>/`
- Error format: `error: sync failed for source "<source>": <reason> (<hint>)`
- Hard copy checksum: SHA256 of all file contents in sorted order, stored as `.lore-checksum` in skill directory
- URL normalization is for cache deduplication only — clone always uses the exact URL from config

## Review Notes
- Adversarial review completed
- Findings: 18 total, 18 fixed, 0 skipped
- Resolution approach: auto-fix
- Key fixes: path traversal validation (F1/F2), copyFile error handling (F3), URL normalization edge case (F4), hash truncation increased to 16 bytes (F5), nil pointer guard (F14), gitignore section management (F8), stale hard-copy reconciliation (F9)
