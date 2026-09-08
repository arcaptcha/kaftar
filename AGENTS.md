# Kaftar Project Rules

## Core Rules

- Keep changes focused and solve the root problem without unrelated refactors.
- Avoid boilerplate and scripts that duplicate standard Go, Docker, or Git commands.
- Test behavioral changes and run the affected component's full test suite before completion.
- Update affected documentation in the same change; keep `README.md` accurate and concise.
- Never commit secrets, local environment files, build output, or machine-specific configuration.
- Use English for repository files, documentation, identifiers, and commit messages.
- Use imperative Conventional Commit messages such as `feat:`, `fix:`, `docs:`, and `refactor:`.
- Record skipped validation and its reason in the related task completion notes.
- Keep handwritten code lines at or below 100 columns. Break long expressions and declarations into readable multiline forms; generated files are exempt.
- Write comments only when they explain intent, invariants, constraints, or non-obvious tradeoffs. Do not narrate obvious code or preserve dead code in comments.

## Repository Direction

- Keep unapproved research, designs, and uncertain requirements under `todo/future/`. Promote accepted work to a numbered active task before implementation.
- Treat the completed migration records as historical evidence; do not restore the retired `arcaptcha-kaftar/` checkout inside this repository.
- Introduce `server/`, `console/`, `deploy/`, and other directories only when an approved task needs them.
- Implement client SDKs in independent repositories, one per language; do not migrate them into this repository.
- Keep server and console images independently buildable. A future combined image belongs in `deploy/combined.Dockerfile` and must use proper process supervision rather than ad hoc background shell commands.

Scoped `AGENTS.md` files may add stricter rules. They may override a root rule only when they state the exception explicitly.
