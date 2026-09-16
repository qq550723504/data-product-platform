# ADR-0009: National Data Infrastructure integration boundary

- Status: Accepted
- Date: 2026-09-16

## Context

The Data Product Platform will eventually publish released data products into external data-infrastructure ecosystems, including China's National Data Infrastructure (NDI), trusted data spaces, data exchanges, and other distribution channels.

NDI provides external interoperability capabilities such as subject identity, identifiers, catalog registration, connector-based delivery, and cross-node interoperability. These capabilities are important external integration targets, but they must not become the Core Platform's system of record.

The Core Platform remains responsible for the business truth of data-product production and governance:

- DataResource and DatasetVersion
- Entity and EntityMapping
- Authorization / Rights
- Workflow / Execution
- Quality and Compliance decisions
- DataContract
- DataProduct / ProductVersion / ProductRelease
- Cost / Evidence / Audit

## Decision

NDI is treated as an **external interoperability layer**.

The Core Platform remains the **System of Record** for data-product production and lifecycle state.

NDI-specific schemas, identifiers, credentials, connector states, and API contracts must not leak into Core Domain entities. They are accessed through adapters and generic bindings.

The preferred integration model is:

```text
Data Product Platform
        │
        │ ProductRelease / Domain Events
        ▼
External Integration Layer
        │
        ├── NDI Adapter
        ├── Trusted Data Space Adapter
        ├── Data Exchange Adapter
        └── other future adapters
```

## Generic external models

Core should use provider-neutral concepts instead of NDI-specific fields.

### External identity binding

Maps an internal Subject / Organization to an external infrastructure identity.

Suggested shape:

```text
provider
subject_id
external_subject_id
credential_ref
registration_node
status
metadata
```

### External identifier

Maps internal business objects to external identifiers.

Suggested shape:

```text
provider
object_type
object_id
external_id
external_uri
status
registered_at
metadata
```

Supported object types may include:

- SUBJECT
- CONNECTOR
- DATA_RESOURCE
- DATA_PRODUCT
- PRODUCT_RELEASE

### External publication

Tracks the publication of a ProductRelease to an external infrastructure.

Suggested lifecycle:

```text
DRAFT
  ↓
SUBMITTING
  ↓
REGISTERED
  ↓
PUBLISHED
  ├── SUSPENDED
  └── WITHDRAWN
```

## Policy and enforcement boundary

The platform owns the business policy:

- who may use a product;
- for which purpose;
- which fields / assets are exposed;
- which ProductRelease is valid;
- usage constraints and expiry.

External connectors/infrastructure may enforce transport and access controls:

- authentication;
- network access;
- delivery;
- connector-side access control;
- runtime / usage logs.

Connector logs are imported as downstream Evidence but do not replace the platform's production Evidence Graph.

## Adapter interfaces

The integration layer may evolve into provider-neutral ports such as:

```text
ExternalIdentityProvider
ExternalIdentifierProvider
CatalogPublisher
DeliveryConnector
UsageEventSource
```

NDI-specific implementations belong under adapters, for example:

```text
adapters/ndi/
```

## POC impact

NDI integration does **not** block the Core POC.

Sprint 1-3 continue to focus on:

```text
DataResource
→ DatasetVersion
→ Entity Resolution
→ Workflow
→ Quality / Compliance / Rights
→ DataContract
→ ProductRelease
→ Evidence / Cost
```

An NDI integration spike should begin only after ProductRelease is stable.

## Consequences

- Core business models remain independent from NDI protocol evolution.
- The same ProductRelease can be published to multiple external ecosystems.
- External IDs never replace internal stable IDs.
- NDI integration can be tested or replaced independently.
- Formal NDI specifications and testbed requirements can be adopted later without redesigning Core Domain.
