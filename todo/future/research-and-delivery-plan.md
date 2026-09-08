# Kaftar Open-Source Redesign: Research and Delivery Plan

## Plan Status

- **Status:** Future research input; not approved for execution
- **Prepared:** September 5, 2026
- **Audience:** Kaftar owners, maintainers, engineering reviewers, and future open-source contributors
- **Current evidence:** completed migration records and maintained runtime contracts; the temporary reference checkout was retired on September 8, 2026
- **Working directory:** repository root

This document contains unresolved options and requirements. Promote an accepted research lane or design decision into a numbered task under `todo/` before implementation.

## 1. Objective

Determine and document the recommended product strategy and target system architecture for turning the internal Arcaptcha Kaftar service into a secure, extensible, observable, and maintainable open-source message-delivery system.

The research must answer this primary decision question:

> What architecture, public contracts, security baseline, repository structure, and phased delivery plan should Kaftar adopt so it can provide reliable scheduled and retried message delivery through HTTP and queue interfaces without remaining coupled to RabbitMQ or internal Arcaptcha components?

## 2. Required Deliverables

### 2.1 Strategy Memorandum

Use the retained **Strategy Memorandum** DOCX template.

Primary audience:

- Project owners
- Technical leadership
- Maintainers deciding scope, sequencing, and investment

The memorandum will contain:

1. Purpose and direct recommendation.
2. Strategic context and open-source opportunity.
3. Options considered.
4. Recommended product and architecture position.
5. Build-versus-adopt decisions.
6. Open-source scope and repository boundaries.
7. Risks and mitigations.
8. Phased roadmap, milestones, and decision gates.
9. Resourcing and ownership assumptions.
10. Explicit decisions requested from project leadership.

Target length: 8–12 pages, excluding references.

### 2.2 System Design

Use the retained **System Design** DOCX template.

Primary audience:

- Backend engineers
- Platform and SRE engineers
- Security reviewers
- Client and console developers
- External contributors

The design will contain:

1. Abstract.
2. Goals and non-goals.
3. Current-state assessment.
4. Functional and quality requirements.
5. Architecture options and tradeoffs.
6. Recommended architecture and system boundaries.
7. Component responsibilities and interfaces.
8. Submission, scheduling, retry, and delivery lifecycles.
9. State machine and durability guarantees.
10. Provider extension model.
11. Queue and embedded-mode architecture.
12. Payload and attachment storage.
13. Authentication, authorization, TLS, and secret handling.
14. HTTP and queue contracts.
15. Health, metrics, logs, tracing, and error handling.
16. Console UI architecture.
17. Deployment profiles and configuration.
18. Data model and migrations.
19. Failure scenarios and recovery behavior.
20. Testing, compatibility, rollout, and migration plan.
21. Alternatives rejected and unresolved questions.

Target length: 20–35 pages, excluding references.

### 2.3 Supporting Research Files

Keep these working files together under `todo/future/redesign/` during research:

- `todo/future/redesign/report-source.md`: canonical internal research synthesis.
- `todo/future/redesign/claim-source-ledger.md`: consequential claims mapped to sources.
- `todo/future/redesign/gap-matrix.md`: evidence confidence, contradictions, and unresolved gaps.
- `todo/future/redesign/adr/`: proposed architecture decision records.

The working files support review but are not substitutes for the two template-based deliverables. Promote accepted outputs into maintained documentation only after the owner approves them.

## 3. Scope

### Included

- Current Kaftar implementation and contract assessment.
- Reliable asynchronous message-delivery semantics.
- Scheduling, retries, dead messages, cancellation, and status history.
- HTTP submission and queue submission.
- Provider extension and configuration architecture.
- RabbitMQ adapter design and broker independence.
- Feasibility of a no-external-queue embedded mode.
- PostgreSQL and embedded persistence options.
- Payload and attachment storage interfaces.
- Local filesystem and S3-compatible storage profiles.
- Authentication, authorization, TLS, API tokens, sessions, and secret storage.
- SSRF and outbound HTTP-provider security.
- Health checks, Prometheus metrics, structured logs, error handling, and tracing.
- Console UI requirements and architecture.
- OpenAPI, AsyncAPI, and shared schemas.
- Separate Go and JavaScript/TypeScript client repositories.
- Open-source publication blockers, governance, release process, and compatibility policy.
- Incremental migration from the current implementation.

### Excluded

- Writing the complete production implementation during the research phase.
- Developing the final console UI.
- Developing the public Go or JavaScript/TypeScript clients.
- Selecting branding, visual identity, or a hosted commercial offering.
- Committing to multi-tenancy, billing, quotas, or SaaS control-plane features.
- Supporting Kafka, NATS, Redis, or other brokers in the first release unless research identifies a compelling requirement.
- Runtime installation of untrusted provider code in the first release unless evidence shows compile-time extensions are insufficient.
- Exactly-once delivery claims across third-party providers that do not support idempotency.

## 4. Assumptions

- Kaftar remains implemented primarily in Go.
- The first public version can make breaking changes to internal APIs and queue payloads.
- RabbitMQ remains supported, but its role may change.
- PostgreSQL is acceptable for production deployments.
- An embedded single-node mode is desirable but optional.
- A bootstrap administrator supplied through environment configuration is acceptable for the first public release.
- Programmatic clients need a non-cookie authentication mechanism.
- Provider configuration will eventually be manageable through the console.
- Reliable scheduling and retry require durable payload availability until terminal completion.
- The project prefers incremental migration over rewriting every feature before producing usable releases.
- The public repository location, final Go module path, and license are not yet confirmed.

## 5. Success Criteria

The research is complete when:

1. Every material architectural recommendation is supported by primary documentation or clearly labeled engineering inference.
2. The current implementation's critical reliability and security risks are tied to specific code paths.
3. At least two credible alternatives are evaluated for each high-impact architecture decision.
4. The recommended durability model states precise guarantees and limitations.
5. Provider extension does not require editing central channel switches or transport routes for every provider.
6. RabbitMQ-specific behavior is isolated behind narrow adapter boundaries.
7. The embedded-mode recommendation is feasible, bounded, and does not imply unsupported clustering guarantees.
8. Payload retention and “do not store” behavior are logically compatible with scheduling and retry guarantees.
9. Authentication and TLS recommendations provide a safe default deployment.
10. The health and observability design is actionable for operators.
11. Public HTTP and queue contracts can independently support Go and TypeScript clients.
12. The migration roadmap has independently releasable milestones and rollback points.
13. Both final DOCX artifacts pass structural and visual verification.

## 6. Primary Decision Areas

| Decision | Options to Evaluate | Required Output |
| --- | --- | --- |
| Durable scheduling source | Database-owned scheduling; RabbitMQ-owned delay queues; dedicated embedded queue | Recommended source of truth and failure guarantees |
| RabbitMQ role | Required internal work queue; optional notifier; public ingress only; combined ingress/notifier | Narrow adapter contract and outage behavior |
| Embedded mode | SQLite scheduler; append-only file queue; embedded KV store; no embedded mode | Recommendation and operational limits |
| Provider extensibility | Compile-time registry; Go plugin; RPC sidecar; WASM extension | First-release model and future extension path |
| Message model | Closed channel types; canonical kind schemas; fully provider-defined payloads | Versioning and validation strategy |
| Provider routing | Explicit provider ID; default provider per kind; weighted/failover routing | Initial routing semantics |
| Payload storage | Database inline; local filesystem; S3-compatible store; client-hosted references | Storage interface and size/retention policy |
| Sensitive payload handling | Retain; delete on terminal; time-based retention; encrypted storage; synchronous-only no-retention mode | Honest reliability and privacy guarantees |
| Authentication | Basic auth; bootstrap admin session; bearer API tokens; external OIDC | Minimum secure release and extension path |
| TLS | Reverse-proxy only; native TLS; both | Deployment defaults and configuration |
| Provider secrets | Environment only; encrypted database configuration; external secret manager | Console-compatible secret lifecycle |
| API design | Channel-specific endpoints; resource-oriented message API; provider-specific endpoints | Stable HTTP v1 contract |
| Queue contract | Internal outbox serialization; transport-neutral submission envelope; CloudEvents-style envelope | Versioned public contract |
| Observability | Logs only; Prometheus plus logs; OpenTelemetry-first | Minimum viable and target stack |
| Console delivery | Separate deployment; embedded static SPA; server-rendered UI | Build and operational model |
| Client repositories | Handwritten clients; generated clients; generated core plus ergonomic wrapper | Go and TypeScript strategy |
| Compatibility | Immediate break; compatibility adapter; long deprecation period | Public beta migration policy |

## 7. Research Method

### Source Priority

Use sources in this order:

1. Official specifications, standards, security guidance, and product documentation.
2. Maintainer-authored design documents and original engineering publications.
3. High-quality independent analysis with transparent methodology.
4. Community discussions only for discovering operational problems or adoption signals.

### Expected Primary Sources

- Go language and standard-library documentation.
- RabbitMQ reliability, confirms, acknowledgements, quorum/classic queues, dead lettering, TTL, and TLS documentation.
- PostgreSQL transaction, locking, skip-locked, advisory-lock, JSON, and operational documentation.
- SQLite transaction, WAL, locking, durability, and concurrency documentation.
- Prometheus metric and instrumentation guidance.
- OpenTelemetry semantic convention and propagation documentation.
- OpenAPI and JSON Schema specifications.
- AsyncAPI specification and RabbitMQ bindings.
- OWASP authentication, session, TLS, secrets, SSRF, API, and logging guidance.
- IETF HTTP, TLS, bearer-token, idempotency, and trace-context standards where applicable.
- S3 API and selected S3-compatible storage documentation.
- OSI/SPDX license material and official open-source project-security guidance.

### Evidence Rules

- Consequential claims require primary evidence whenever available.
- Rapidly changing product features must be rechecked as of September 5, 2026.
- RabbitMQ, PostgreSQL, SQLite, Go, OpenAPI, AsyncAPI, Prometheus, and OpenTelemetry behavior must be sourced from version-relevant official documentation.
- Security recommendations must distinguish mandatory baseline controls from optional hardening.
- Engineering inferences must be labeled and tied to the implementation constraints that support them.
- Contradictory evidence must be preserved until definitions, versions, or deployment assumptions resolve it.

## 8. Research Lanes

### Lane A: Current-State Code and Contract Audit

Questions:

- What are the current submission, persistence, publication, consumption, retry, and status flows?
- Which reliability guarantees are claimed, and which are actually implemented?
- Where do RabbitMQ, provider, persistence, DTO, and client types cross architectural boundaries?
- Which credentials, private dependencies, internal URLs, generated artifacts, and machine-specific tests block publication?
- Which public contracts are accidental representations of internal database or queue models?

Outputs:

- Component and dependency map.
- Request and retry sequence diagrams.
- Reliability and security finding list with code references.
- Open-source publication-blocker inventory.

### Lane B: Durability, Scheduling, and Queue Architecture

Questions:

- Should the database or RabbitMQ be the scheduling source of truth?
- What does Kaftar guarantee after accepting an HTTP request or acknowledging a queue message?
- How should workers claim work and recover abandoned processing?
- How are duplicate broker notifications and ambiguous provider responses handled?
- Is SQLite sufficient for a supported embedded deployment?
- Is a custom filesystem queue justified compared with SQLite or an existing embedded library?

Outputs:

- Durability invariant.
- State machine.
- Claim/lease design.
- Retry and dead-message policy.
- Embedded and clustered deployment profiles.
- Queue adapter capability boundaries.

### Lane C: Provider and Message Extension Model

Questions:

- Is a provider an implementation of a message kind, a transport, or both?
- How can a new SMTP/SMS/chat provider be added without editing the core?
- How should message validation and provider configuration schemas be registered?
- Which capabilities need discovery: attachments, HTML, batching, delivery receipts, idempotency, rate limits, and health?
- Should external providers run in-process, out-of-process, or both?

Outputs:

- Provider SDK interfaces.
- Message codec/schema registry.
- Provider factory and instance lifecycle.
- Typed delivery-result and failure model.
- Routing and fallback recommendation.

### Lane D: Storage, Security, and Privacy

Questions:

- Which data must be durable for scheduled/retried delivery?
- When can bodies and attachments be deleted?
- What should remain inline versus in blob storage?
- How should local filesystem storage prevent traversal, corruption, and partial writes?
- How should provider secrets be stored if configured through the console?
- What authentication and TLS behavior is safe enough for the first release?
- How should the outbound HTTP provider prevent SSRF, DNS rebinding, unsafe redirects, and credential leakage?

Outputs:

- Blob-store interface and lifecycle.
- Retention-policy model.
- Encryption and secret-management approach.
- Initial authentication and authorization model.
- TLS and trusted-proxy requirements.
- HTTP-provider security policy.

### Lane E: Operations, Contracts, UI, and Ecosystem

Questions:

- Which dependencies determine liveness and readiness?
- Which metrics reveal stalled delivery, retry storms, provider degradation, and storage failure?
- Which log fields and trace propagation are required across HTTP and queues?
- What is the stable HTTP resource model?
- What is the stable queue-submission envelope?
- Should the console be embedded or separately deployed?
- Which code and contracts belong in the server, Go client, and TypeScript client repositories?
- What release, compatibility, contribution, and security processes are needed before publication?

Outputs:

- Health and observability specification.
- OpenAPI and AsyncAPI contract outline.
- Console information architecture.
- Repository and release topology.
- Public beta readiness checklist.

## 9. Execution Phases

### Phase 0: Workspace and Template Preflight — Complete

- Confirm the repository root and use retained migration evidence instead of restoring the retired checkout.
- Read both template configurations.
- Inspect retained template layouts and section structures.
- Check available document tooling.

Finding:

- Both selected templates are DOCX documents.
- No connected Codex document session is currently available.
- Research and Markdown synthesis can proceed, but final template cloning, rendering, and visual verification require a connected document capability.

### Phase 1: Current-State Discovery

- Inventory code, configuration, tests, generated documentation, dependencies, and repository history.
- Trace HTTP and queue submission paths.
- Trace immediate, scheduled, retry, dead, and status flows.
- Identify accidental public contracts and publication blockers.
- Populate the initial gap matrix.

Exit condition:

- Every major current component and critical flow has a code-backed description.

### Phase 2: First-Wave Primary Research

- Research official reliability semantics for RabbitMQ, PostgreSQL, and SQLite.
- Research provider extension constraints in Go.
- Research storage, auth, TLS, SSRF, observability, OpenAPI, and AsyncAPI requirements.
- Map evidence to each decision area.
- Record version and date applicability.

Exit condition:

- Every high-impact decision has at least one primary source and at least two viable options.

### Phase 3: Gap-Driven Follow-Up

- Reconcile database-versus-broker scheduling tradeoffs.
- Validate embedded queue feasibility and boundaries.
- Challenge the recommended provider model with runtime-extension alternatives.
- Resolve payload no-retention versus durable retry requirements.
- Verify security and operational recommendations against failure scenarios.
- Independently recheck the most consequential claims.

Exit condition:

- Material contradictions are resolved or explicitly bounded, and remaining gaps would not change the recommendation.

### Phase 4: Architecture Synthesis

- Write `todo/future/redesign/report-source.md` once as the canonical synthesis.
- Define product boundary, invariants, guarantees, and non-goals.
- Select the target architecture.
- Define the data model, interfaces, state machine, and API/queue contracts.
- Define embedded and production deployment profiles.
- Produce diagrams and sequence flows.
- Draft ADRs for major decisions.

Exit condition:

- Engineering reviewers can evaluate the complete architecture without relying on unstated assumptions.

### Phase 5: Strategy Synthesis

- Translate technical decisions into product and execution choices.
- Compare incremental refactor, side-by-side V2, and full rewrite strategies.
- Define release phases, investment sequence, compatibility policy, and public beta criteria.
- Identify decisions that require owner approval.

Exit condition:

- Leadership can approve a direction, scope, and phased investment plan.

### Phase 6: Template Artifact Production

- Clone the retained Strategy Memorandum reference DOCX.
- Populate it from the completed strategy synthesis.
- Clone the retained System Design reference DOCX.
- Populate it from the completed architecture synthesis.
- Preserve page setup, styles, tables, headers, footers, and recurring elements.
- Add descriptive source hyperlinks and endnotes.

Dependency:

- A connected document-capable Codex session must be available.

### Phase 7: Verification and Delivery

- Audit all headings, tables, lists, figures, links, citations, and document metadata.
- Verify that strategic recommendations and system-design decisions do not conflict.
- Render both DOCX artifacts.
- Inspect title pages, dense tables, architecture diagrams, transition pages, references, and final pages at 100% zoom.
- Correct only observed defects, then render once more for confirmation.
- Deliver the System Design and Strategy Memorandum artifacts first, followed by a concise summary.

Exit condition:

- Both artifacts are structurally valid, visually reviewed, evidence-backed, and internally consistent.

## 10. Gap Matrix Format

Maintain one row per consequential claim or decision:

| Claim or Decision | Current Evidence | Confidence | Contradiction | Missing Evidence | Next Query or Action |
| --- | --- | --- | --- | --- | --- |
| Example: database should own retry schedule | Current code audit plus PostgreSQL/RabbitMQ docs | Medium | Broker-owned delay queues may reduce polling | Operational scaling evidence | Compare failure recovery and latency profiles |

Confidence levels:

- **High:** primary evidence plus implementation fit; no material unresolved contradiction.
- **Medium:** credible evidence, but an assumption or deployment boundary materially affects the result.
- **Low:** incomplete, indirect, outdated, or contradictory evidence.

## 11. Claim-to-Source Ledger Format

| Claim ID | Supported Claim | Source Title | Publisher | Published/Updated | URL | Access Notes |
| --- | --- | --- | --- | --- | --- | --- |

The final documents will use descriptive hyperlinks and notes rather than exposing internal research identifiers.

## 12. Expected Architecture Options

The research will test, not assume, these three top-level designs.

### Option A: RabbitMQ-Centric Refactor

- RabbitMQ remains mandatory for work dispatch and scheduling.
- Core depends on a generic queue abstraction.
- An embedded queue becomes a second implementation of that abstraction.

Questions to challenge:

- Can one abstraction safely cover acknowledgement, delay, ordering, redelivery, topology, and health differences?
- Does broker unavailability prevent accepting otherwise durable work?
- Does the embedded implementation accidentally recreate a message broker?

### Option B: Database-Centric Delivery Core

- Database is the source of truth for due work and retry schedule.
- Workers atomically claim records using leases.
- RabbitMQ is optional for ingress and low-latency notifications.
- SQLite plus local filesystem supports a bounded embedded profile.

Questions to challenge:

- Is polling and row claiming sufficient for expected scale and latency?
- How are PostgreSQL and SQLite semantics kept compatible?
- When does a notification broker become operationally worthwhile?

### Option C: Pluggable Durable Queue Core

- A queue engine owns scheduling, leases, and durable payload references.
- RabbitMQ and an embedded queue implement a richer queue contract.
- Database stores tracking and configuration rather than controlling execution.

Questions to challenge:

- Can transactions between tracking storage and queue state remain consistent?
- How much broker-specific behavior leaks into the interface?
- Is the implementation and test burden justified for the first public release?

The Strategy Memorandum will compare these options by time to value, execution risk, reliability, operational complexity, contributor accessibility, and long-term strategic fit.

## 13. Provisional Implementation Roadmap

This roadmap is a hypothesis to validate during research.

### Milestone 0: Publication Safety

- Rotate and remove credentials from source and history.
- Remove private infrastructure assumptions and machine-specific tests.
- Select license and public module/repository names.
- Replace private dependencies.
- Add security and contribution policies.

### Milestone 1: Contracts and Reliability Kernel

- Define message states, attempts, retry policy, and idempotency.
- Define provider, message-codec, blob-store, and message-store ports.
- Implement state-machine and conformance tests.
- Add versioned migrations.

### Milestone 2: Embedded Vertical Slice

- Implement SQLite message scheduling and claims.
- Implement local filesystem payload storage.
- Port one provider.
- Implement message submission and status APIs.
- Demonstrate restart-safe scheduled delivery and retries.

### Milestone 3: Production Adapters

- Implement PostgreSQL store.
- Implement RabbitMQ ingress and notification adapters.
- Implement S3-compatible storage.
- Port remaining providers.
- Add legacy compatibility adapters.

### Milestone 4: Security and Operations

- Add bootstrap administrator, API tokens, secure sessions, and CSRF protection.
- Add reverse-proxy and native TLS support.
- Encrypt provider secrets.
- Add readiness, metrics, structured logs, tracing, and centralized errors.
- Harden the outbound HTTP provider.

### Milestone 5: Console and Public Contracts

- Finalize OpenAPI, AsyncAPI, and JSON Schemas.
- Build and embed the console UI.
- Add provider management and message tracking workflows.
- Publish migration and operations documentation.

### Milestone 6: Client Repositories and Public Beta

- Publish Go client repository.
- Publish JavaScript/TypeScript client repository.
- Run compatibility, load, failure-injection, security, and upgrade tests.
- Complete public beta readiness review.

## 14. Review Gates

### Gate 1: Product Boundary

Approve:

- Durable delivery system rather than general-purpose broker.
- Initial single-tenant scope.
- Supported message kinds and providers for public beta.

### Gate 2: Durability Architecture

Approve:

- Source of truth for scheduling and retries.
- Embedded-mode constraints.
- RabbitMQ role.
- Public reliability guarantee wording.

### Gate 3: Extension and Contract Model

Approve:

- Provider SDK model.
- Message schema ownership.
- HTTP and queue resource/envelope models.
- Compatibility policy.

### Gate 4: Security and Data Handling

Approve:

- Bootstrap authentication.
- API token model.
- TLS defaults.
- Provider-secret encryption.
- Payload retention and outbound HTTP security.

### Gate 5: Release Plan

Approve:

- Milestone sequencing.
- Public repository and module names.
- License.
- Beta support policy and release criteria.

## 15. Risks to Research Quality

| Risk | Mitigation |
| --- | --- |
| Designing only around current RabbitMQ behavior | Compare broker-centric and database-centric models from first principles |
| Treating an interface as proof of portability | Define semantics and conformance tests before interface shapes |
| Overengineering runtime plugins | Separate provider extensibility from runtime installation requirements |
| Claiming “no storage” with reliable retries | Model durability and retention as separate concerns |
| Using optional-provider failure as global unavailability | Separate critical readiness from provider-specific health |
| High-cardinality monitoring design | Review every proposed metric label against bounded-cardinality rules |
| Public contracts importing server internals | Require independent schema artifacts and client types |
| Template formatting driving weak content | Finish research and canonical synthesis before populating templates |
| Outdated technical assumptions | Record versions and recheck consequential product behavior as of September 5, 2026 |
| Incomplete visual verification | Require render-and-inspect before claiming artifact completion |

## 16. Inputs That Improve the Final Recommendation

These inputs are useful but do not block beginning the research:

- Expected messages per second and peak burst size.
- Maximum scheduled-message horizon.
- Typical and maximum payload/attachment sizes.
- Required deployment topology: one node, active-active, or both.
- Whether Kaftar must support internal/private webhook targets.
- Required backward-compatibility period.
- Intended public repository organization and module path.
- Preferred open-source license.
- Whether multi-tenancy is a near-term requirement.
- Whether provider plugins must be installable without rebuilding Kaftar.

Unknown values will be documented as assumptions and converted into explicit design limits.

## 17. Stop Condition

Research stops when:

- All report sections have sufficient evidence.
- Every high-impact recommendation has primary support or a disclosed limitation.
- Contradictions are resolved or bounded by deployment assumptions.
- The gap matrix contains no unresolved issue likely to change the selected architecture.
- Additional searching is producing only redundant or weaker evidence.

At that point, effort moves to synthesis and verified artifact production rather than continued broad discovery.
