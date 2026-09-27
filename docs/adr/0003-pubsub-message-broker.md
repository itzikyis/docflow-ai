# ADR-0003: Google Cloud Pub/Sub as the message broker

- **Status:** Accepted
- **Date:** 2026-09-27

## Context

ADR-0002 requires a durable message broker between pipeline stages. The platform runs on Google Cloud. Requirements:

- durable delivery with retries and dead-lettering;
- fan-out: one event consumed independently by several services (for example, a notification worker alongside the next pipeline stage);
- no broker cluster for us to operate;
- throughput in the range of tens to thousands of documents per minute, not millions of events per second.

## Decision

We will use **Google Cloud Pub/Sub**:

- one **topic per event type** (`document.uploaded`, `document.ocr_completed`, ...);
- one **subscription per consuming service**, so each service keeps its own delivery position and retry state;
- **pull subscriptions with streaming pull** from long-running worker pods;
- an exponential-backoff **retry policy** and a **dead-letter topic** on every subscription;
- messages carry **references** (document ID, storage path), never document contents. Pub/Sub messages are limited to 10 MB, and payloads belong in Cloud Storage.

Locally, we use the official Pub/Sub emulator.

## Alternatives considered

- **Apache Kafka (self-managed or Confluent).** Its strengths are log retention, replay and per-partition ordering at very high throughput. We don't need replay or strict ordering, and running Kafka means real operational work or significant managed cost.
- **RabbitMQ on Kubernetes.** Flexible routing, but we would have to operate a stateful cluster ourselves, including upgrades, disk and high availability.
- **Cloud Tasks.** Good at dispatching individual tasks to HTTP handlers with rate limiting, but each task has one target. It has no fan-out, which the notification use case needs.
- **PostgreSQL as a queue (`SELECT ... FOR UPDATE SKIP LOCKED`).** Genuinely viable at this scale and needs no extra infrastructure. However, it ties queue load to the primary database and gives up managed dead-lettering and fan-out. We will use this pattern for the outbox relay (Phase 4), not as the main broker.

## Trade-offs

- **Gained:** fully managed, scales automatically, has native dead-letter topics and retry policies, integrates with IAM and Cloud Monitoring (backlog metrics drive autoscaling in Phase 11), and costs little at low volume.
- **Given up:** delivery is **at-least-once**, so duplicates happen. Ordering is not guaranteed unless ordering keys are used. We are locked in to GCP at this layer, and the emulator does not implement every feature, so retry and dead-letter behaviour must also be verified against real Pub/Sub.

## Consequences

- Consumers are idempotent. Every message is processed under a unique `(document_id, stage)` job record, and redeliveries of completed work are acknowledged without being reprocessed.
- Ordering is not needed. A document's stages are causally chained: the next event is only published after the previous stage completes.
- Pub/Sub access is hidden behind small publisher and subscriber interfaces in `internal/events`, so business logic does not depend on the SDK and a different broker could be swapped in if needed.
