# Documentation Rules

- Root `AGENTS.md` rules always apply.
- Keep documentation concise, task-oriented, and accurate to verified behavior.
- Update `docs/README.md` whenever a document is added, moved, renamed, or removed.
- Link to detailed documents from the root `README.md` when they are important to onboarding, operation, or API use.
- Start with a flat directory. Add semantic subdirectories only when they provide real organization; do not create empty documentation scaffolding.
- Use repository-relative links and commands that have been verified from the repository root.
- State assumptions, limitations, and project status explicitly; do not document planned behavior as implemented behavior.

## API Contracts

- Maintain the authoritative HTTP contract as a human-edited OpenAPI 3.1 YAML file at `docs/openapi.yaml` when the API is introduced.
- Update the contract and implementation together.
- Do not generate or commit duplicate Swagger JSON, YAML, Go documentation files, or rendered HTML.
- Validate the specification with a standard OpenAPI tool; do not add a wrapper script only for validation.
