# Wiki Index

Durable current-state codebase knowledge. Read this file first when answering codebase questions.

## Start here

- [Project overview](./overview.md): Purpose, audience, deliverables, stack, and repository boundaries.
- [Repository map](./map.md): Entry points, package responsibilities, tests, and common change paths.
- [Architecture](./architecture.md): Runtime flow, data ownership, integrations, invariants, and tradeoffs.
- [Development](./development.md): Documented workflows, verification strategy, and initial-ingest check results.
- [Configuration contract](./configuration.md): Config discovery, YAML schema, provider destinations, and validation rules.

## Conventions

- [Conventions index](./conventions/index.md): Repository-specific coding, testing, and sync-safety rules.
- [Go code conventions](./conventions/go-code.md): Package boundaries, formatting, errors, and deterministic persistence.
- [Testing conventions](./conventions/testing.md): Standard-library test structure, isolation, and expected coverage.
- [Sync safety conventions](./conventions/sync-safety.md): Ownership, containment, integrity checks, and rollback rules.

## Dreams

- [Memory pointers](./dreams/MEMORY.md): Conditional routes to high-value repository learnings.
- [2026-07-23 sync reporting and wiki integration](./dreams/2026-07-23-2017-sync-reporting-and-wiki-integration.md): Implemented, reviewed, verified, and committed sync change reporting, then merged the repository wiki branch; local work is complete and unpushed.

## Maintenance

- [Wiki instructions](./AGENTS.md): Scoped rules for durable memory maintenance.
- [Wiki log](./log.md): Chronological maintenance log.
- [Wiki state](./state.md): Ingest and maintenance state.
- [Raw sources](./raw/README.md): Rules for untrusted raw inputs.

## Decisions and intent

Canonical ADRs belong in `../spec/decisions/` when needed. Current implementation rationale is also recorded in tracked planning artifacts under `../spec/`; verify conclusions against source and tests.

## Stale or needs review

- Needs review: `../spec/index.md` does not currently route the existing tracked planning artifacts.
- Needs review: `../README.md` identifies GPL v2, but no license file is tracked.
