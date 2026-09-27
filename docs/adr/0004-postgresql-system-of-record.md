# ADR-0004: PostgreSQL as the system of record

- **Status:** Accepted
- **Date:** 2026-09-27

## Context

We need to store document metadata, per-stage processing jobs, extraction results and (from Phase 4) outbox records. The access patterns are:

- transactional writes that must update several rows together (a status change plus a job record plus an outbox event);
- relational queries (documents by status, jobs by document, stuck jobs older than N minutes);
- extraction results whose shape varies by document type (invoice fields differ from contract fields).

## Decision

We will use **PostgreSQL** as the single system of record: **Cloud SQL for PostgreSQL** in GCP, and the official PostgreSQL image in Docker locally.

- Schema changes are versioned SQL migrations in `migrations/`.
- Extraction results that vary by document type are stored as **`JSONB`**, alongside typed columns for the fields we query and index.
- Applications use a connection pool sized explicitly per service.

## Alternatives considered

- **Firestore.** Serverless and easy to scale, but it has limited multi-document transactions, no joins and no ad-hoc SQL. The transactional outbox and operational queries would be awkward.
- **Cloud Spanner.** Horizontally scalable with strong consistency, but its cost and complexity are far beyond what this workload needs.
- **MongoDB.** Flexible schemas, but `JSONB` gives us that flexibility for the one place we need it, while keeping relational integrity everywhere else.

## Trade-offs

- **Gained:** ACID transactions across related rows; the outbox can live in the same transaction as the state change; mature migration and testing tooling; SQL for operational queries; `JSONB` for semi-structured data.
- **Given up:** a single primary sets the ceiling for write scaling (well above this project's needs). Cloud SQL is billed while it runs, even when idle. Each Postgres connection is relatively expensive, so many pods with large pools can exhaust `max_connections`.

## Consequences

- Pool sizes must be set per service, and the total (replicas × pool size) must stay below the database's connection limit. This becomes a real constraint once autoscaling is added in Phase 11.
- Integration tests run against a real PostgreSQL in Docker, not an in-memory substitute.
- Migrations must be backward compatible with the currently deployed code, because the API and workers are rolled out independently.
