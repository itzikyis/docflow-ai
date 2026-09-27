# ADR-0005: Decompose workers by pipeline stage

- **Status:** Accepted
- **Date:** 2026-09-27

## Context

Processing consists of OCR, classification, structured extraction, validation and (later) notification. The original design proposed a separate worker service for each step. Each additional service brings a deployment, a topic, a subscription, a dead-letter topic, dashboards, alerts and a network hop.

Services are worth separating when they **scale, fail or deploy differently**, not simply because they are different steps.

- **OCR** has its own latency profile, cost model and failure modes, depending on the provider or on CPU.
- **Classification and extraction** are both calls to the same AI model family. A multimodal model can classify and extract in one structured-output request. Their resource profile is identical.
- **Notification** is a side effect: its failures must never block or retry document processing.

## Decision

We will run three workers:

| Worker | Consumes | Produces | Responsibility |
|---|---|---|---|
| `ocr-worker` | `document.uploaded` | `document.ocr_completed` | Extract text; store it in Cloud Storage |
| `ai-worker` | `document.ocr_completed` | `document.processed` / `document.failed` | Classify, extract fields with confidence scores, validate, set the final status |
| `notification-worker` (later) | `document.processed`, `document.failed` | — | Notify interested parties |

Intermediate artifacts (OCR text) are stored in Cloud Storage and passed by reference in events.

## Alternatives considered

- **One worker per step (four or five services).** More hops, queues and dashboards with no scaling benefit: classification and extraction would always scale together. More latency and more ways to fail.
- **One "pipeline" worker running every step in-process.** The simplest option, but OCR and AI failures and retries become coupled (an AI timeout would re-run OCR), and the two cannot scale independently even though their capacity limits differ.

## Trade-offs

- **Gained:** independent scaling and retries where resource profiles actually differ; fewer moving parts than one service per step; notification is isolated from the processing path.
- **Given up:** `ai-worker` does several things (classification, extraction, validation). It must be kept well structured inside, with classifier, extractor and validator behind separate small interfaces, so it could be split later if needed.

## Consequences

- Each `(document_id, stage)` pair has one processing job record, which makes redelivered messages detectable and safe to acknowledge.
- A shared worker framework in `internal/worker` (Phase 5) provides concurrency, graceful shutdown, retries and metrics, so each worker only implements its handler.
- Splitting `ai-worker` later would mean adding one topic and one deployment, with no change to the API or to `ocr-worker`.
