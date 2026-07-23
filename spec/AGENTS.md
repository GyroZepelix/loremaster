# Spec Instructions

## Purpose

`/spec` stores intended change: plans, tasks, verification notes, outcomes, and architecture decision records. Specs explain what should happen and how completion is verified.

## When to create or update a spec

Create or update a spec for user-requested work that is non-trivial, multi-step, risky, or likely to matter later.

Use specs to capture:

- Goals and scope.
- Assumptions and constraints.
- Step-by-step implementation work.
- Verification commands and results.
- Outcomes and decisions.

Do not use specs as scratchpads for transient chat.

## Structure

- Use `spec/templates/plan.md` by default.
- Use `spec/templates/adr.md` for decisions with long-lived consequences.
- Put active work in `spec/active/<YYMMDD-HHMM>-short-name/`.
- Move completed work to `spec/completed/<YYMMDD-HHMM>-short-name/`.
- Keep durable decisions in `spec/decisions/` when needed.

## Indexing

Update `spec/index.md` when adding, completing, cancelling, or moving a spec.

A completed spec should normally include:

- `plan.md`
- `verification.md` when validation details are useful
- `outcome.md` when summarizing completed work is useful

## Authority

Accepted specs and ADRs are more authoritative than wiki summaries, but less authoritative than explicit user instructions and the current `AGENTS.md` files.

When specs conflict with source files at `HEAD`, inspect both and record the conflict instead of guessing.
