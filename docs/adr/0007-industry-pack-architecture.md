# ADR-0007: Industry-Specific Semantics Live in Industry Packs

- Status: Accepted

## Context

The first implementation uses a park scenario, but the Core Platform must remain reusable across manufacturing, government, finance, healthcare and other domains.

## Decision

Industry-specific semantics are packaged under `industry-packs/<industry>/` as rules, templates and configuration.

Industry Packs may define:

- Entity Types
- Glossary / Domains
- Standardization Rules
- Matching Policies
- Indicators
- Quality Rules
- Compliance Rules
- Product Templates

Core domain code must not contain scattered industry switches such as `if industry == PARK`.

## Consequences

- Park is a Reference Implementation, not the boundary of the product.
- New industries can reuse Core lifecycle, version, rights, cost and evidence models.
- Pack contracts and validation need to become explicit as the ecosystem grows.