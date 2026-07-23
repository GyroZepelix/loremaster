# Wiki State

Last full ingest commit: 22c4d4e2f53292a8b640274380a82707cd86c3d7
Last incremental ingest commit: c442d9c657ca67e9cef6f6ddddd45c6bb04bc214
Last lint date: 2026-07-23

## Processed inputs

| Input | Type | Source | Processed date | Output pages | Notes |
| --- | --- | --- | --- | --- | --- |
| `https://git.dgjalic.com/dgjalic/repo-wiki-template` | Template installer | Forgejo repository | 2026-07-23 | `AGENTS.md`, `spec/`, `wiki/` | Installed lean repo wiki template. |
| `22c4d4e2f53292a8b640274380a82707cd86c3d7` | Full codebase ingest | Git tracked working tree | 2026-07-23 | `wiki/overview.md`, `wiki/map.md`, `wiki/architecture.md`, `wiki/configuration.md`, `wiki/development.md`, `wiki/conventions/`, `wiki/index.md` | Complete initial ingest from a clean baseline. |
| `95614b5`, `923f579`, `c442d9c` | Incremental code and wiki ingest | Git commits and completed implementation session | 2026-07-23 | `wiki/architecture.md`, `wiki/conventions/sync-safety.md`, `wiki/conventions/testing.md`, `wiki/development.md`, `wiki/dreams/`, `wiki/index.md` | Sync reporting, review fixes, verification evidence, and wiki merge processed into durable memory. |

## Full ingest record

- Status: Complete.
- Analysis commit: `22c4d4e2f53292a8b640274380a82707cd86c3d7`.
- Ingest date: 2026-07-23.
- Working-tree snapshot: baseline status was empty; no modified tracked or untracked evidence was included.
- Output pages: `wiki/overview.md`, `wiki/map.md`, `wiki/architecture.md`, `wiki/configuration.md`, `wiki/development.md`, `wiki/conventions/index.md`, `wiki/conventions/go-code.md`, `wiki/conventions/testing.md`, `wiki/conventions/sync-safety.md`, and `wiki/index.md`.
- Allowed checks: `gofmt -d $(git ls-files '*.go')` passed with no diff; wiki indexing, routing, links, citations, ASCII, fences, whitespace, scope, and secret-pattern checks passed.
- Unverified: tests, vet, race checks, builds, application commands, and Git/network flows were not run under the ingest safety constraints.
- Needs review: `spec/index.md` does not route existing tracked specs; `README.md` identifies GPL v2 but no license file is tracked.

## Maintenance policy

This file is compact processing state for resume, dedupe, lint, and ingest checkpoints. It is not append-only history.

Update this file when:

- Full or incremental ingest checkpoints change.
- Lint date or lint checkpoint state changes.
- Raw files, source batches, URL batches, specs, ADRs, or commit ranges are processed into durable wiki knowledge and need dedupe/resume tracking.

Do not add rows for every session, chat turn, verification command, routine implementation plan, routine wiki edit, log-only event, or codebase change that does not affect durable wiki knowledge.

For wiki maintenance rows, `Output pages` should name wiki pages. Install/bootstrap rows may summarize the broader installed payload.

- Update checkpoints only after affected wiki pages are processed.
- Keep raw sources separate from trusted wiki synthesis.
- Mark uncertain or unverified pages as stale in `wiki/index.md`.
