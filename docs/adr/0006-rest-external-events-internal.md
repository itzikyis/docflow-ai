# ADR-0006: REST externally, events internally, no gRPC

- **Status:** Accepted
- **Date:** 2026-09-27

## Context

The original technology list includes "gRPC where appropriate". We need to decide how clients talk to the system and how services talk to each other.

- **External clients** (browsers, scripts, curl, other systems) upload files and poll for results.
- **Internal communication** is entirely asynchronous (ADR-0002). No service calls another service and waits for an answer.

gRPC's strengths are efficient, strongly typed, **synchronous** service-to-service calls and streaming. This design has no synchronous internal calls to apply them to.

## Decision

- **External API: REST over HTTP with JSON**, described by an OpenAPI contract in `api/openapi.yaml`. File uploads use `multipart/form-data`.
- **Internal communication: events over Pub/Sub**, JSON-encoded, using the versioned envelope from ADR-0002.
- **No gRPC** for now. We will adopt it only when a synchronous, latency-sensitive internal dependency appears, for example a dedicated model-serving service that the API must call inline.

## Alternatives considered

- **gRPC between internal services.** It would need synchronous call paths that the architecture intentionally avoids. Adding them only to use gRPC would weaken failure isolation.
- **gRPC externally, with a REST gateway.** Browser clients cannot call gRPC directly, file uploads over gRPC are awkward, and the gateway adds a component with no benefit for the clients we have.
- **GraphQL.** Useful for rich, client-driven queries across many entities. Our API has a handful of resource-oriented endpoints.
- **Protobuf for event payloads** (without gRPC). Gives schema enforcement and compact encoding, and Pub/Sub supports schemas. We choose JSON for readability during development and debugging, and handle evolution through the explicit envelope version and contract tests. This is worth revisiting if event schemas multiply.

## Trade-offs

- **Gained:** universally accessible API; no protobuf toolchain; human-readable messages in logs and dead-letter topics; fewer components.
- **Given up:** no compile-time schema enforcement across services, beyond shared Go types in the same module. JSON is larger and slower to encode than protobuf, which is negligible at our message sizes.

## Consequences

- The OpenAPI document is the external contract. Handlers and tests must stay consistent with it.
- Event schemas are defined as Go types in `internal/events`, with contract tests that pin the JSON shape of each version.
- Being able to explain *why gRPC is not used* is part of the design. A future ADR will supersede this one if a synchronous internal dependency is introduced.
