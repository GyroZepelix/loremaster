# Loremaster

![Go](https://img.shields.io/badge/Go-1.24-00ADD8?logo=go&logoColor=white)
![License](https://img.shields.io/badge/License-GPL--2.0-blue)
![Version](https://img.shields.io/badge/version-0.4.0-green)

Declarative resource sync for AI coding tools. Define skills, prompts, commands, or arbitrary provider-relative resource directories in `lore.yml`, then fetch and link them with one command.

Loremaster supports Claude Code, OpenCode, Pi, and Codex. Sources may be Git repositories or local directories, and synchronized paths are automatically excluded from the project's Git index.

## Installation

Requirements: Go 1.24+

```bash
go install github.com/GyroZepelix/loremaster/cmd/lore@latest
```

Or build from source:

```bash
git clone https://github.com/GyroZepelix/loremaster.git
cd loremaster
go build -o lore ./cmd/lore
```

## Quick Start

```bash
cd ~/my-project
lore init
# Edit lore.yml, then:
lore sync
```

A configuration can synchronize several resource types to several providers:

```yaml
provider: [pi, claude]

skills:
  - source: ssh://git@github.com/example/agent-resources.git
    ref: main
    include:
      - example-skill
      - nested/skill
      - source-dir:renamed-skill

prompts:
  - source: ssh://git@github.com/example/agent-resources.git
    ref: main
    include:
      - review.md

hooks/tools:
  - source: ssh://git@github.com/example/agent-resources.git
    include:
      - validate.sh:check.sh
```

For a project sync, this produces paths such as:

```text
.pi/skills/example-skill
.claude/skills/example-skill
.pi/prompts/review.md
.claude/prompts/review.md
.pi/hooks/tools/check.sh
.claude/hooks/tools/check.sh
```

When the sync root is `$HOME`, Pi uses `~/.pi/agent/` instead of `~/.pi/`.

## Configuration

Loremaster searches for `lore.yml` or `lore-<profile>.yml` in the current directory and these provider directories:

```text
.claude/
.opencode/
.pi/
.pi/agent/
.agents/
.codex/
```

Exactly one config is used. Project and global configs are not merged, and Loremaster does not search parent directories.

### Schema

`provider` is the only reserved top-level key. Every other top-level key is a literal resource directory relative to each selected provider's configuration root.

Each resource contains one or more source objects:

```yaml
provider: claude

commands:
  - source: git@github.com:user/resources.git  # Git URL or local directory
    ref: v1.2.0                                # Optional branch, tag, or commit
    include:
      - deploy.md                              # Exact source path
      - source.md:renamed.md                   # Exact src:dst mapping
      - templates                              # Exact directory, synchronized recursively
    type: soft                                 # soft or hard, default: soft
```

Allowed source fields are strictly validated:

- `source`: required Git URL or local directory
- `ref`: optional Git branch, tag, or commit
- `include`: required list of exact relative paths
- `type`: optional `soft` or `hard`

A misspelled source field is an error. Because arbitrary top-level keys are intentional, a correctly shaped top-level typo such as `skils:` is treated as a literal resource named `skils`.

### Skills Versus Other Resources

The exact resource name `skills` retains its original contract: every include must resolve to a directory.

All other resources may include regular files or directories. Included directories are synchronized recursively as one managed item. There is no extension filter or format validation.

### Exact Paths Only

Glob expansion is not supported. `*`, `?`, `[` and `]` are rejected in resource and include paths.

```yaml
# Invalid
prompts:
  - source: ./resources
    include: ["*.md"]
```

List each file explicitly or include its containing directory.

Resource names, include sources, and mapped destinations must be clean relative paths. Loremaster rejects:

- Absolute paths
- Paths that escape through `..`
- Backslashes
- Colons outside the `src:dst` separator
- Control characters
- Glob metacharacters
- Exact or parent-child destination overlaps

Collision checks use the complete `<resource>/<destination>` path. For example, `skills/foo` plus destination `bar` conflicts with destination `foo/bar` under `skills`.

### Literal Provider Paths

Loremaster transports files to literal paths. It does not translate resource names or validate whether a provider consumes them.

| Provider | Project configuration root | Root when syncing from `$HOME` |
|---|---|---|
| Claude Code | `.claude/` | `~/.claude/` |
| OpenCode | `.opencode/` | `~/.opencode/` |
| Pi | `.pi/` | `~/.pi/agent/` |
| Codex | `.agents/` | `~/.agents/` |

The destination formula is:

```text
<provider-config-root>/<resource-name>/<mapped-include-destination>
```

Provider consumption rules still matter:

- Pi loads project prompt templates from `.pi/prompts/*.md` and global templates from `~/.pi/agent/prompts/*.md`. Pi's default prompt discovery is non-recursive.
- Claude Code uses `.claude/commands/*.md` for flat custom commands. A `prompts:` resource creates `.claude/prompts/...` literally, but Claude Code does not treat that directory as its command location.
- Use `commands:` when the intended Claude destination is `.claude/commands/`.

### Multi-Provider Sync

A scalar or list is accepted:

```yaml
provider: claude
```

```yaml
provider: [claude, opencode, pi, codex]
```

Each distinct source is fetched once and reused across resources and providers. A failure in one source or item does not stop unrelated items, but the command exits with an error after recording safe partial successes.

### Profiles

Profiles use `lore-<profile>.yml`:

```bash
lore init -p dev
lore sync -p dev
```

- No profile flag, or `-p default`, uses `lore.yml`.
- Profiles own their synchronized destination paths.
- A profile cannot overwrite a path owned by another profile.
- `lore sync --prune` removes items belonging to profiles whose config files no longer exist.
- Modified or unverifiable hard copies are preserved during sync, provider removal, resource removal, and prune.

### Linking Modes and Ownership Safety

`soft` is the default. It creates a symlink to the selected source file or directory.

`hard` copies the selected file or directory. Loremaster records a versioned checksum in `.lore-manifest.yml`; new hard copies do not add checksum files to provider resource directories. Directory checksums include file contents, permissions, empty directories, and symlink targets.

Loremaster refuses to overwrite an existing destination unless the active profile owns it. It also refuses to replace an owned destination when:

- A managed symlink was replaced with another filesystem type
- A hard-copied file or directory has local modifications
- Ownership or checksum metadata cannot be verified

Remove or relocate an unmanaged conflict explicitly, then run `lore sync` again.

### Manifest and Gitignore

Loremaster always writes `.lore-manifest.yml` in the sync root and adds it to `.gitignore`.

Manifest version 2 records each item's:

- Project-relative path
- Provider
- Resource name
- Soft or hard mode
- File or directory kind
- Hard-copy checksum and checksum version
- Soft-link target

Version 1 manifests containing path strings are migrated automatically only when every existing item can be verified as a managed symlink or legacy checksum directory. Migration stops safely when legacy checksum limitations make local symlinks or empty directories unverifiable.

If the manifest is missing or corrupt, existing destinations are treated as unmanaged and are not overwritten. Gitignore entries remain while any profile owns the corresponding path.

## Commands

```text
lore init                        Create a lore.yml skeleton
lore init -p <profile>           Create lore-<profile>.yml
lore sync                        Fetch and synchronize resources
lore sync -p <profile>           Synchronize a named profile
lore sync --prune                Remove safely verified orphan-profile items
lore completion <shell>          Generate bash, zsh, or fish completion
lore --version                   Print the version
lore help [command]              Show help
```

Concurrent syncs targeting the same project are unsupported and may corrupt `.gitignore` or `.lore-manifest.yml`.

### Sync Output

A sync reports repository activity and changed managed destinations:

```text
Repositories:
  cloned git@github.com:example/resources.git @ 8f16d7b59e0bf1b27b09ad7e57f42ab95a16f0c8
  fast-forwarded ssh://git@github.com/example/tools.git 0f5d9a6f7496b20ec4e03b50b702aa2a48e3a73f -> 38ea2230eeac8b700f14773079df9f653fdb5c77
Synced items:
  added .pi/skills/example-skill
  updated .claude/commands/review.md
  deleted .pi/prompts/old.md
```

Directory resources are reported as one managed destination item, not as every nested file. Repositories that did not change are omitted. If no repository or managed item changed, the command prints:

```text
No repository or synced item changes.
```

Warnings and item errors continue to use stderr. A partial sync reports confirmed successful changes before returning a nonzero exit status.

## How It Works

1. Parse and validate providers, dynamic resources, sources, includes, and destination collisions.
2. Fetch each distinct Git or local source once.
3. Resolve literal resource destinations for each provider.
4. Verify manifest ownership before linking or copying each file or directory.
5. Reconcile removed resources and providers without deleting modified or unmanaged content.
6. Save the structured manifest and reconcile the managed `.gitignore` section.

Filesystem replacements and removals retain same-directory backups until the manifest is saved. If manifest persistence fails, Loremaster rolls those changes back.

Git repositories are cached under `$XDG_DATA_HOME/loremaster/` when `XDG_DATA_HOME` is set, otherwise under `~/.local/share/loremaster/`.

## Shell Completions

```bash
lore completion bash > ~/.bashrc.d/lore.bash
lore completion zsh > "${fpath[1]}/_lore"
lore completion fish > ~/.config/fish/completions/lore.fish
```

## Development

```bash
gofmt -w $(find cmd internal -type f -name '*.go')
go test ./... -count=1
go vet ./...
go test -race ./...
go build ./cmd/lore
```

## License

[GNU General Public License v2.0](https://www.gnu.org/licenses/old-licenses/gpl-2.0.html)
