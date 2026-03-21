---
stepsCompleted: ['step-01-init', 'step-02-discovery', 'step-02b-vision', 'step-02c-executive-summary', 'step-03-success', 'step-04-journeys', 'step-05-domain', 'step-06-innovation', 'step-07-project-type', 'step-08-scoping', 'step-09-functional', 'step-10-nonfunctional', 'step-11-polish', 'step-12-complete']
inputDocuments: ['spec/brainstorm/202603211200-loremaster.md']
workflowType: 'prd'
documentCounts:
  briefs: 0
  research: 0
  brainstorming: 1
  projectDocs: 0
classification:
  projectType: cli_tool
  domain: developer_tooling
  complexity: low
  projectContext: greenfield
---

# Product Requirements Document - Loremaster

**Author:** Domagoj
**Date:** 2026-03-21

## Executive Summary

Loremaster is a stateless Go CLI that manages AI coding skills across multiple projects. It reads a declarative `lore.yml` manifest — per-project or global — fetches skills from git repositories into a central cache (`~/.local/share/loremaster/`), and symlinks them into the target tool's skills directory. A single `lore sync` command ensures every project has exactly the skills it declares, with automatic git-exclusion to prevent private skills from leaking into project repos.

The primary user is a developer working across multiple projects that each need different combinations of AI coding skills. The tool prioritizes personal workflow efficiency first, with team and community sharing as a secondary benefit.

### What Makes This Special

Loremaster is declarative and git-native. The `lore.yml` file is the artifact — it turns skill configuration into something reproducible and version-controlled using infrastructure developers already have: git repos and SSH keys. No registry, no accounts, no new infrastructure.

Skills are plain text files. Git already solves distribution. Loremaster is the minimal declarative glue between the two — a repo-grouped YAML manifest that maps sources to destinations, with symlink-first design so upstream improvements propagate automatically. Hard copy is available when isolation is needed.

There is no incumbent in this space. AI coding tools (Claude Code, OpenCode, Cursor) all use skill/prompt files with no package manager. Loremaster fills that gap with a tool-agnostic, provider-based architecture.

## Project Classification

- **Project Type:** CLI Tool — terminal-based, scriptable, config-driven
- **Domain:** Developer Tooling
- **Complexity:** Low — no regulated industry, straightforward git + symlink mechanics
- **Project Context:** Greenfield — new product, no existing codebase

## Success Criteria

### User Success

- A developer can go from `lore init` to a fully synced project in under a minute on a reasonable connection — the bottleneck is git clone speed, not loremaster overhead
- Running `lore sync` on an already-cached project completes in seconds (no network needed for symlink-only operations)
- The user never has to think about skill management — sync once, forget about it
- Private skills never appear in `git status` or accidentally get committed

### Business Success

- The author uses loremaster daily across personal projects without friction or failures
- Reliable enough to recommend to peers without caveats
- No "works on my machine" problems — consistent behavior across Linux and macOS

### Technical Success

- Zero silent failures — every sync error is reported clearly with actionable context
- No broken symlinks after a successful sync — if a symlink can't be created, the operation fails loudly
- Idempotent sync — running `lore sync` twice produces the same result
- Clean `.gitignore` management — entries are added without corrupting existing content

### Measurable Outcomes

- `lore sync` with warm cache completes in < 2 seconds for a typical project (5-10 skills)
- `lore sync` with cold cache is bounded only by git clone speed + negligible overhead
- Zero data loss scenarios — hard-copied skills with local modifications are never overwritten without warning


## User Journeys

### Journey 1: First-Time Project Setup

**Domagoj** starts a new project and wants his standard set of AI coding skills available. He runs `lore init`, which detects `.claude/` in the project and creates a `lore.yml`. He opens it, adds his skill repo and the skills he needs — `commit-message`, `frontend-design`, `code-review`. He runs `lore sync`. Loremaster clones his skill repo to the cache, symlinks the three skills into `.claude/skills/`, and adds them to `.gitignore`. He's ready to code.

Alternatively, he copies a `lore.yml` from another project, tweaks the `include` list, and runs `lore sync`. Same result, different starting point.

**Capabilities revealed:** `lore init`, `lore.yml` parsing, git clone, symlink creation, auto-gitignore, provider auto-detection.

### Journey 2: Daily Development (Steady State)

Domagoj opens a project and starts working. The symlinks point to the cache, which points to his skill repo clone — skills are always current with whatever's in the cache. He doesn't run `lore sync` unless he knows something changed upstream in his skill repo. When he does, `lore sync` does a `git pull` on the cached repo and the symlinks are already pointing at the right place. Done in seconds.

**Capabilities revealed:** Warm cache detection, `git pull` on existing clones, idempotent sync, fast no-op when nothing changed.

### Journey 3: Editing a Skill In Place

Domagoj is working on a project and notices his `commit-message` skill could be better. He edits it directly — the file he's editing is symlinked to the cache clone of his skill repo. He navigates to the cache directory, commits and pushes the improvement. Every other project using that skill via symlinks instantly has the update. No sync needed.

**Capabilities revealed:** Symlink-first design enabling in-place editing, cache as a regular git working tree, zero friction for skill iteration.

### Journey 4: Adding a Skill to an Existing Project

Mid-project, Domagoj realizes he needs the `code-review` skill. He opens `lore.yml`, adds `code-review` to the `include` list for his skill repo, and runs `lore sync`. Loremaster symlinks the new skill into `.claude/skills/` and adds it to `.gitignore`. One edit, one command.

**Capabilities revealed:** Incremental sync, automatic gitignore updates on every sync, no disruption to existing skills.

### Journey 5: Something Goes Wrong

Domagoj runs `lore sync` but his skill repo has been moved or his SSH key isn't loaded. Loremaster fails loudly — tells him exactly which repo couldn't be reached and why. No partial state, no broken symlinks from the failed source. Skills that were already synced from other sources remain untouched.

**Capabilities revealed:** Clear error reporting, no silent failures, partial failure isolation (one bad source doesn't break others), no broken symlinks from failed operations.

### Journey Requirements Summary

| Capability | Journeys |
|---|---|
| `lore init` with provider auto-detection | 1 |
| `lore.yml` parsing (repo-grouped format) | 1, 4 |
| Git clone to central cache | 1 |
| Git pull on existing cache | 2 |
| Symlink creation to target skill dirs | 1, 4 |
| Auto-gitignore on every sync | 1, 4 |
| Idempotent sync (no-op when current) | 2 |
| Clear error reporting with context | 5 |
| Partial failure isolation | 5 |
| Cache as editable git working tree | 3 |

## CLI-Specific Requirements

### Project-Type Overview

Loremaster is a non-interactive, scriptable CLI built in Go using cobra. All commands run without prompts or confirmation dialogs — the only exception is `lore init`, which may ask the user to select a target tool if auto-detection is ambiguous.

### Technical Architecture Considerations

- **Language:** Go — single binary, zero runtime dependencies, cross-platform
- **CLI framework:** Cobra — mature, built-in shell completion, subcommand structure
- **Git operations:** go-git or shelling out to git — TBD during implementation
- **Config parsing:** YAML via `gopkg.in/yaml.v3` or similar

### Command Structure

| Command | Behavior | Interactive |
|---|---|---|
| `lore init` | Bootstrap `lore.yml`, auto-detect target tool | Minimal (tool selection if ambiguous) |
| `lore sync` | Clone/pull repos, symlink skills, update gitignore | Non-interactive |

### Configuration

- Single config file: `lore.yml` — no environment variables, no CLI flag overrides for MVP
- Config location: project root, `.claude/`, `.opencode/`, or `~/` for global

### Output & Scripting

- Human-readable plain text output for all commands
- Clear error messages with actionable context (which repo failed, why)
- Exit codes: 0 for success, non-zero for failure

### Shell Completion

- Built-in shell completion for bash, zsh, and fish via cobra's completion generation
- Included in MVP

### Implementation Considerations

- Stateless design — no daemon, no background processes, no database
- Central cache at `~/.local/share/loremaster/` — one clone per unique repo URL
- Symlink-first with hard copy as opt-in per skill
- Auto-gitignore management — append-only, idempotent, respects existing entries

## Project Scoping & Phased Development

### MVP Strategy & Philosophy

**MVP Approach:** Problem-solving MVP — the minimum that eliminates manual skill copying and makes `lore sync` the only command needed to set up any project's skills.

**Resource Requirements:** Solo developer, Go experience, familiar with git internals and symlink mechanics.

### MVP Feature Set (Phase 1)

**Core User Journeys Supported:**
- Journey 1: First-time project setup (`lore init` + `lore sync`)
- Journey 2: Daily development (warm cache sync)
- Journey 3: Editing skills in place (symlink-first)
- Journey 4: Adding a skill to existing project
- Journey 5: Error handling (clear failures)

**Must-Have Capabilities:**
- `lore init` — bootstrap `lore.yml` with provider auto-detection
- `lore sync` — clone/pull repos, symlink or copy skills, update gitignore
- `lore.yml` parsing — repo-grouped format with `source`, `ref`, `include`, `type`
- Symlink-first with hard copy opt-in per skill
- Auto-gitignore on every sync
- Provider detection for Claude Code and OpenCode
- Shell completion (bash, zsh, fish) via cobra
- Linux and macOS support

### Post-MVP Features

**Phase 2 (Growth):**
- `lore status` — show current sync state
- `.lore.lock` with pinned commit SHAs for reproducibility
- `lore verify` — CI-friendly sync state validation
- Additional provider support (Cursor, Windsurf, etc.)

**Phase 3 (Expansion):**
- Community skill discovery and sharing
- Windows support
- `lore.yml` as a recognized ecosystem convention

### Risk Mitigation Strategy

**Technical Risks:** Symlink fragility on exotic filesystems and CI/Docker environments. Mitigation: hard copy mode exists as fallback; document known limitations clearly.

**Market Risks:** AI coding tool vendors ship native skill sync. Mitigation: loremaster is tool-agnostic — its value is multi-tool support, not competing with any single vendor.

**Resource Risks:** Solo developer project. Mitigation: MVP scope is intentionally minimal — three commands, one config file, one mechanism. Achievable in days, not months.

## Functional Requirements

### Project Initialization

- FR1: User can initialize a new `lore.yml` in the current project via `lore init`
- FR2: System can auto-detect the target AI coding tool by scanning for `.claude/` or `.opencode/` directories
- FR3: User can select a target tool manually when auto-detection is ambiguous
- FR4: System can generate a valid `lore.yml` skeleton with the detected target

### Configuration & Parsing

- FR5: System can parse `lore.yml` files using the repo-grouped format (`source`, `ref`, `include`, `type`)
- FR6: User can specify multiple skill sources (git repos or local paths) in a single `lore.yml`
- FR7: User can group multiple skills under a single source repo using `include` lists
- FR8: User can specify a git ref (tag or commit SHA) per source for version pinning
- FR9: User can specify link type per skill (`soft` for symlink, `hard` for copy) defaulting to `soft`
- FR10: System can locate `lore.yml` in project root, `.claude/`, `.opencode/`, or `~/` for global scope
- FR33: User can specify a `name` field for single-skill local path sources to set the skill directory name

### Sync Operations

- FR11: User can sync all declared skills with a single `lore sync` command
- FR12: System can clone a git repo to the central cache when not already present
- FR13: System can pull latest changes for a cached repo that already exists
- FR14: System can check out a specific git ref when one is specified in config
- FR15: System can create symlinks from the target skill directory to the cached skill source
- FR16: System can copy skill directories when hard copy type is specified
- FR17: System can warn and skip hard-copied skills that have local modifications
- FR18: System can resolve local paths as skill sources without git operations

### Git Exclusion

- FR19: System can add synced skill directories to the project's `.gitignore` on every sync
- FR20: System can manage `.gitignore` entries idempotently without duplicating entries
- FR21: System can preserve existing `.gitignore` content when adding entries

### Provider Management

- FR22: System can resolve target skill directories for Claude Code (`.claude/skills/<name>/`)
- FR23: System can resolve target skill directories for OpenCode (`.opencode/skills/<name>/`)
- FR24: System can resolve global skill directories for each provider (`~/.claude/skills/<name>/`, `~/.config/opencode/skills/<name>/`)

### Cache Management

- FR25: System can store all cloned repos in a central cache at `~/.local/share/loremaster/`
- FR26: System can maintain one cache entry per unique repo URL
- FR27: User can edit cached skill files directly (cache is a regular git working tree)

### Error Handling

- FR28: System can report which specific repo or skill failed during sync with actionable context
- FR29: System can isolate failures per source — one failed repo does not affect other sources
- FR30: System can fail loudly when a symlink cannot be created instead of continuing silently
- FR31: System can exit with non-zero exit code on any failure

### Shell Integration

- FR32: User can generate shell completions for bash, zsh, and fish

## Non-Functional Requirements

### Performance

- Warm cache sync (`lore sync` with all repos already cloned) completes in < 2 seconds for a typical project (5-10 skills)
- Cold cache sync is bounded by git clone speed — loremaster adds negligible overhead per repo
- Symlink creation is near-instantaneous — no file copying unless hard copy type is specified
- No network calls when all cached repos are at the expected ref

### Reliability

- `lore sync` is idempotent — running it twice produces identical results with no side effects
- Partial failures are isolated — one failed source does not corrupt or remove skills from other sources
- `.gitignore` modifications are append-only and idempotent — never corrupt existing content
- Hard-copied skills with local modifications are never overwritten without explicit warning

### Integration

- Relies on system git installation or go-git for all repository operations
- Relies on user's existing SSH keys or git credential helpers for authentication — no custom auth
- Respects `XDG_DATA_HOME` for cache location (falls back to `~/.local/share/loremaster/`)
- Interoperates with any AI coding tool that uses a directory-based skill structure
