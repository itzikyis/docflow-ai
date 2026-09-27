# ADR-0001: Go, as a single module with multiple binaries

- **Status:** Accepted
- **Date:** 2026-09-27

## Context

The system consists of an HTTP API and several background workers (OCR, AI processing, later notifications). All of them are I/O-bound: they spend their time waiting on HTTP clients, PostgreSQL, Cloud Storage, Pub/Sub and AI model calls. They run as containers on Kubernetes and scale horizontally, so start-up time, memory footprint and image size directly affect scaling speed and cost.

The services share a domain model (documents, processing states) and a set of event contracts. One team develops and deploys all of them.

## Decision

We will write all services in **Go** and keep them in **one repository and one Go module** (`github.com/itzikyis/docflow-ai`):

- each deployable binary lives in `cmd/<service>/`;
- shared code lives in `internal/`, organised by responsibility (`document`, `postgres`, `events`, ...) rather than by technical layer;
- one multi-stage `Dockerfile` builds any service, selected with the `SERVICE` build argument.

## Alternatives considered

- **Java / Spring Boot.** A mature ecosystem, but JVM start-up time and memory use make fast horizontal scaling slower and more expensive, and it brings more framework than this system needs.
- **Python.** It has the richest AI ecosystem, but we call AI models over HTTP APIs, so that advantage mostly disappears. Its concurrency story (the GIL, async runtimes) is weaker for a high-throughput message consumer.
- **TypeScript / Node.js.** A reasonable choice for I/O-bound work, but it has weaker static guarantees and a heavier runtime than a single static binary.
- **One repository or module per service.** This allows independent versioning, but changing an event contract then means coordinated releases across repositories. That overhead buys nothing for a single team.

## Trade-offs

- **Gained:** static binaries of about 8 MB in distroless images; fast start-up; goroutines and `context.Context` fit concurrent message processing naturally; one `go test ./...` covers everything; an event contract change is a single atomic commit.
- **Given up:** services cannot pin different versions of shared code. A change in `internal/` rebuilds every service. Go's AI SDK ecosystem is thinner than Python's.

## Consequences

- Shared event types give compile-time consistency, but **not** deployment-time consistency. Services are deployed independently, so producers and consumers will run different versions at the same time. Events must therefore stay backward compatible and carry an explicit schema version (see ADR-0002).
- Container images are still versioned and deployed per service.
- The `internal/` package boundary must be kept healthy: a worker must not import HTTP handler code, for example.
