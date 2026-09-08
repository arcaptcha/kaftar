# Documentation

This directory contains detailed project documentation. It starts flat and gains semantic subdirectories only when enough real documents exist to justify them.

## Contents

- [Server Development](development.md) — build, test, local environment, and configuration.
- [HTTP API Contract](openapi.yaml) — human-maintained OpenAPI 3.1 specification.
- [Delivery and Queue Contract](queue-contract.md) — durable acceptance, idempotency, scheduling, recovery, and direct submission encoding.
- [Provider Contracts](provider-contracts.md) — acceptance checks, attachment/cancellation behavior, and outbound destination policy.
- [Reference Migration](migration.md) — completed source migration, compatibility, and separate retirement gates.
- [Hardening Design](hardening-design.md) — approved reliability/provider design, original failure evidence, and validation requirements.

## Planned Topics

Architecture, development, operations, API, and decision records will be added as approved tasks produce verified content. Empty category directories and placeholder documents are intentionally avoided.

The root [README](../README.md) remains the onboarding entry point. Update this index whenever documentation is added, moved, renamed, or removed.

Unapproved research, designs, and uncertain requirements live under [`todo/future/`](../todo/future/) until an owner promotes them to a numbered task.
