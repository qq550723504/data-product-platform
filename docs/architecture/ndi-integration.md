# NDI Integration Architecture

## 1. Positioning

The Data Product Platform is positioned as a **data-product production and governance capability platform** that can participate in external data-infrastructure ecosystems as a business-node capability, rather than implementing a regional/full-domain infrastructure node itself.

The platform should produce a stable `ProductRelease` first, then publish it through external adapters.

```text
Raw / Source Data
      ↓
Data Product Platform
      ↓
ProductRelease
      ↓
External Integration Layer
      ├── NDI
      ├── Trusted Data Space
      ├── Data Exchange
      └── Other channels
```

## 2. Layered architecture

```text
┌──────────────────────────────────────────────┐
│ External data infrastructure                │
│                                              │
│ NDI / trusted data space / exchange          │
│ identity · identifier · catalog · connector  │
└──────────────────────▲───────────────────────┘
                       │ adapters
┌──────────────────────┴───────────────────────┐
│ Data Product Platform                       │
│                                              │
│ UseCase · Rights · Dataset · Entity          │
│ Workflow · Quality · Compliance · Contract   │
│ Product · Release · Cost · Evidence          │
└──────────────────────▲───────────────────────┘
                       │ governance projection
┌──────────────────────┴───────────────────────┐
│ Metadata / governance engines               │
│ OpenMetadata and future alternatives         │
└──────────────────────────────────────────────┘
```

The layers answer different questions:

- Metadata engine: **what data exists and how is it governed technically?**
- Core Platform: **how is data produced into a governed Data Product?**
- External infrastructure: **how is a released product identified, discovered, delivered, and used across organizations?**

## 3. Core-to-external mapping

| Core object | External concern | Mapping rule |
| --- | --- | --- |
| Subject / Organization | external identity | bind, never replace internal ID |
| DataResource | external resource identifier / catalog entry | publish through adapter |
| DataProduct | product catalog description | project metadata only |
| ProductRelease | concrete publishable version | publication unit |
| Authorization / Rights | usage eligibility | source for external usage policy |
| DataContract | schema / SLA / delivery contract | map selected contract fields |
| Evidence | runtime / publication evidence | import external logs as evidence |

## 4. Publication flow

```text
ProductRelease = PUBLISHED
        ↓
ExternalPublicationRequested
        ↓
Provider Adapter
        ↓
Identity / identifier checks
        ↓
Catalog registration
        ↓
Connector / delivery configuration
        ↓
ExternalPublication = PUBLISHED
        ↓
Usage events / connector logs
        ↓
Evidence Graph
```

The external publication process must not mutate the original ProductRelease snapshot.

## 5. Identity and identifier model

Internal IDs remain canonical inside the platform.

Examples:

```text
Internal Product ID
DP-ACTIVITY-001

Internal Release ID
REL-2026-001
```

External identifiers are stored separately:

```text
provider = NDI
object_type = DATA_PRODUCT
object_id = <internal UUID>
external_id = <provider identifier>
```

This allows one object to have multiple external identifiers at the same time.

## 6. Policy vs enforcement

Core Platform owns policy definition:

```text
Subject
+ ProductRelease
+ Purpose
+ Action
+ Scope
+ Constraints
= Decision
```

External connectors may enforce the resulting decision during delivery.

The connector is therefore an enforcement/runtime component, not the source of truth for business authorization.

## 7. Evidence integration

External infrastructure evidence is appended to the existing Evidence Graph.

```text
Rights Evidence
      ↓
Dataset / Processing Evidence
      ↓
Quality / Compliance Evidence
      ↓
ProductRelease Evidence
      ↓
External Registration Evidence
      ↓
Connector Usage Evidence
```

External runtime logs do not replace upstream production evidence.

## 8. Implementation phases

### Core POC — no dependency on NDI

Continue Sprint 1-3 without blocking on external integration.

### Integration Spike

After ProductRelease stabilizes, implement provider-neutral models:

- `external_identity_binding`
- `external_identifier`
- `external_publication`
- adapter ports

Then implement an `adapters/ndi` proof of concept against the formal interface specification/test environment available at implementation time.

### Production integration

Only after formal interface documentation and testbed validation should NDI-specific production behavior be finalized.

## 9. Non-goals

The project does not currently aim to implement:

- an NDI full-domain node;
- a regional/industry functional node;
- a replacement for a standard access connector;
- an NDI-specific Core Domain model.
