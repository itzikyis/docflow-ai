# ADR-0002: Asynchronous, event-driven processing pipeline

- **Status:** Accepted
- **Date:** 2026-09-27

## Context

Processing a document involves OCR, classification, structured extraction and validation. These steps:

- take seconds to minutes, far longer than an HTTP request should stay open;
- depend on external services (OCR, AI models) that have rate limits, quotas and transient failures;
- fail independently: extraction can fail after OCR has already succeeded and been paid for;
- receive bursty load, since users upload in batches.

## Decision

We will process documents **asynchronously**, as a pipeline of stages connected by events.

1. `POST /documents` stores the file, records the document in PostgreSQL and returns **`202 Accepted`** with the document ID and a status URL. It does not wait for processing.
2. Each stage consumes an event, does its work, persists its result and emits the next event (`document.uploaded` → `document.ocr_completed` → `document.processed` / `document.failed`).
3. Stages are coordinated by **choreography**: each stage reacts to the previous stage's event. No central orchestrator.
4. The document's status in PostgreSQL is the source of truth for where a document is in the pipeline. Clients poll `GET /documents/{id}/status`.
5. All events share a versioned envelope:

   ```json
   {
     "eventId": "uuid",
     "eventType": "document.uploaded",
     "version": 1,
     "occurredAt": "RFC 3339 timestamp",
     "correlationId": "id of the originating request",
     "data": { "...": "event-specific payload" }
   }
   ```

## Alternatives considered

- **Synchronous processing in the upload request.** The client would hold a connection for up to minutes, hitting load-balancer timeouts. A retry repeats all the work. API capacity would be tied to AI provider capacity, so a slow model would exhaust API pods.
- **Background goroutines inside the API.** Work is lost when a pod restarts or is scaled down. There is no backpressure or retry across replicas, and the API and processing could not scale independently.
- **Orchestration (Cloud Workflows, Temporal).** This gives explicit workflow state, visibility and compensation logic, but adds a significant piece of infrastructure. With two or three linear stages and no branching or compensation, choreography is enough. We will revisit this if the pipeline grows branches, human-in-the-loop steps or multi-step rollbacks.

## Trade-offs

- **Gained:** fast, predictable upload latency; each stage scales and retries independently; failures are isolated to a stage; bursts are absorbed by the queue instead of overloading downstream services.
- **Given up:** the system is eventually consistent (a document's result is not available immediately); debugging spans several processes; messages can be delivered more than once or out of order.

## Consequences

- Every consumer must be **idempotent**, because delivery is at-least-once.
- Updating the database and publishing an event must happen atomically, or a crash between the two strands documents. A transactional outbox will be decided in Phase 4.
- A correlation ID, and later the W3C trace context, must travel with every event so one document's journey can be followed across services.
- Document status transitions must be enforced by an explicit state machine in the domain layer.
- Clients poll for results. Webhooks or push notifications can be added later with a notification worker.
