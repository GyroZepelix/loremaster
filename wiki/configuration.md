# Configuration Contract

## Discovery

`internal/config/locate.go` searches exactly one filename in this order:

1. Current directory.
2. `.claude/`.
3. `.opencode/`.
4. `.pi/`.
5. `.pi/agent/`.
6. `.agents/`.
7. `.codex/`.

The first existing file wins. No parent-directory search or config merge occurs. The default profile uses `lore.yml`; a named profile uses `lore-<profile>.yml`. Profile names match `[a-z0-9][a-z0-9_-]*`, at most 64 characters; empty and `default` both select `lore.yml` (`internal/config/locate.go`).

## YAML model

`provider` is the only reserved top-level key. It accepts one provider or a list. Every other top-level key is an ordered, literal resource path whose value is a non-empty list of source objects (`internal/config/config.go`).

Allowed source fields are:

- `source`: required Git URL or local directory.
- `ref`: optional Git branch, tag, or commit.
- `include`: required exact source paths, optionally mapped as `src:dst`.
- `type`: `soft` or `hard`; default is `soft`.

Source object fields are strict, but dynamic resource names are intentional. A misspelled top-level resource can therefore be valid configuration (`README.md`, `internal/config/config.go`).

The exact resource `skills` requires included sources to resolve to directories. Other resources accept regular files or directories (`internal/sync/sync.go`). Globs are not expanded.

## Provider destinations

| Provider | Project config root | Root when sync root is `HOME` |
| --- | --- | --- |
| Claude | `.claude/` | `.claude/` |
| OpenCode | `.opencode/` | `.opencode/` |
| Pi | `.pi/` | `.pi/agent/` |
| Codex | `.agents/` | `.agents/` |

Destination formula:

```text
<provider-config-root>/<resource>/<mapped-destination>
```

`.codex/` is a Codex marker and config discovery location, but Codex output remains under `.agents/` (`internal/provider/codex.go`). Pi compares real paths as well as cleaned strings when deciding whether the sync root is home (`internal/provider/pi.go`).

## Validation and collision rules

Resource, include source, and include destination paths must be relative and cannot contain control characters, backslashes, colons, or glob metacharacters. They cannot escape through `..` (`internal/config/include.go`). Exact and ancestor/descendant destination overlap is rejected across the full `<resource>/<destination>` path (`internal/config/config.go`).

A single Git source cannot be declared with different refs in one config. Distinct declarations of the same source are fetched once (`internal/config/config.go`, `internal/sync/sync.go`).

## Provider consumption caveat

Loremaster guarantees literal filesystem transport, not provider discovery. For example, Claude commands belong under a `commands:` resource, while Pi prompt discovery has its own flat-path rules. See `README.md` before adding user-facing examples.
