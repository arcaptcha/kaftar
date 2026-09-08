# Task Rules

Root `AGENTS.md` rules always apply.

## Naming and Ordering

- Name tasks `<prefix>-<short-kebab-case-title>.md`.
- Use non-negative integer segments without leading zeros: `0.1`, `1`, `1.1`, `1.1.1`, `1.2`, `1.15`, `2`.
- Compare segments numerically from left to right. A shorter shared prefix sorts first.
- Insert work between `1.1` and `1.2` as a child such as `1.1.1`.
- Treat every assigned prefix as immutable. Never renumber active or completed tasks.
- Prefixes express intended order, not hard dependencies. State dependencies explicitly in the task context or scope.

## Required Format

```markdown
# <Prefix>: <Title>

## Context

## Objective

## Scope

## Acceptance Criteria

## Validation

## Completion Notes
```

- Keep scope and acceptance criteria concrete and testable.
- Record commands run and their outcomes under `Validation`.
- Record important implementation decisions, skipped validation, and follow-up work under `Completion Notes`.
- Do not create placeholder task files.

## Lifecycle

- Keep active tasks directly under `todo/`.
- Move completed tasks to `todo/done/` without changing their filename or prefix.
- Create `todo/done/` only when the first task is completed.
- Update repository links in the same change when moving a completed task.

## Future Work

- Keep unapproved research, exploratory designs, and uncertain requirements under `todo/future/`.
- Files under `todo/future/` are inputs for decisions, not active tasks or implementation commitments.
- When work is approved, create a numbered task directly under `todo/` and link its source material.
