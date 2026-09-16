# ADR-0001: Use a Modular Monolith for the Core POC

- Status: Accepted

## Context

The main risk is domain-model correctness, not horizontal scalability. Splitting the system into many services would add RPC, distributed transaction, deployment and versioning complexity before the boundaries are validated.

## Decision

Build the first Core Platform as a Modular Monolith with clear domain modules and a separate worker process.

Initial modules include resource, dataset, entity, workflow, rights, quality, compliance, contract, product, cost and evidence.

## Consequences

- Faster vertical-slice development.
- Easier transactional consistency.
- Domain boundaries remain explicit so modules can be extracted later if needed.
- No microservice split is allowed solely for architectural aesthetics during the POC.