# Architecture Decision Records

An ADR captures one significant architectural decision: the context that forced it, what was decided, which alternatives were rejected and why, and what we now have to live with. See [ADR-0000](0000-record-architecture-decisions.md) for the process.

New ADRs start from [template.md](template.md).

| ADR | Title | Status |
|---|---|---|
| [0000](0000-record-architecture-decisions.md) | Record architecture decisions | Accepted |
| [0001](0001-go-modular-monorepo.md) | Go, as a single module with multiple binaries | Accepted |
| [0002](0002-asynchronous-event-driven-processing.md) | Asynchronous, event-driven processing pipeline | Accepted |
| [0003](0003-pubsub-message-broker.md) | Google Cloud Pub/Sub as the message broker | Accepted |
| [0004](0004-postgresql-system-of-record.md) | PostgreSQL as the system of record | Accepted |
| [0005](0005-worker-decomposition-by-stage.md) | Decompose workers by pipeline stage | Accepted |
| [0006](0006-rest-external-events-internal.md) | REST externally, events internally, no gRPC | Accepted |

## Planned

These decisions are deliberately deferred to the phase where there is enough context to make them well.

| Topic | Phase |
|---|---|
| Upload path: through the API vs. signed URLs | 3 — Cloud Storage |
| Transactional outbox for reliable event publishing | 4 — Pub/Sub |
| Vertex AI / Gemini, and the OCR approach | 6 — AI processing |
| GKE vs. Cloud Run; Autopilot vs. Standard | 7–8 — Terraform and GKE |
| HPA vs. event-driven autoscaling (KEDA) | 11 — Scaling |
