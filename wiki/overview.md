# Project Overview

## Purpose

Loremaster is a Go CLI for declaratively synchronizing AI coding resources. A user declares Git or local sources in `lore.yml`; `lore sync` fetches each source and installs selected files or directories into one or more provider configuration roots (`README.md`, `cmd/root.go`).

The primary audience is developers who want reproducible skills, prompts, commands, and other provider-relative resources across projects and home-directory configurations (`README.md`).

## Main deliverables

- The `lore` executable, entered through `cmd/lore/main.go` and Cobra commands in `cmd/`.
- `lore init`, which detects a provider and creates `lore.yml` or `lore-<profile>.yml` (`cmd/init.go`).
- `lore sync`, which fetches sources, installs resources, records ownership in `.lore-manifest.yml`, and reconciles a managed `.gitignore` section (`cmd/sync.go`).
- Shell completion output for bash, zsh, and fish (`cmd/root.go`).

## Stack

- Go 1.24 module `github.com/GyroZepelix/loremaster` (`go.mod`).
- Cobra for the CLI and YAML v3 for configuration and manifest encoding (`go.mod`).
- The system `git` executable for production fetches (`cmd/sync.go`, `internal/git/exec.go`). A go-git implementation remains behind the same interface (`internal/git/git.go`).
- Filesystem symlinks, copies, checksums, atomic renames, and YAML state. There is no database or service process (`internal/sync/linker.go`, `internal/manifest/manifest.go`).

## Repository boundaries

- `cmd/` wires CLI behavior and owns cross-package orchestration.
- `internal/` contains all reusable domain logic; it is not a public Go library API.
- `spec/` stores plans and product intent. Current behavior must be verified against source and tests.
- `wiki/` stores durable navigation and codebase memory.

Loremaster transports paths literally. It does not translate resource names or guarantee that a provider consumes a synchronized path (`README.md`, `internal/provider/provider.go`). Config discovery is local to the invocation directory and known provider subdirectories; parent directories are not searched (`internal/config/locate.go`). Concurrent syncs against one project are unsupported (`README.md`).

## Needs review

- `README.md` identifies GPL v2, but no license file is tracked. Confirm packaging and distribution expectations before a release-oriented change.
