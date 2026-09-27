# ADR-0000: Record architecture decisions

- **Status:** Accepted
- **Date:** 2026-09-27

## Context

DocFlow AI is built incrementally over many phases. Decisions made early (message broker, database, service boundaries) constrain everything after them, and the reasoning behind them is easy to lose. Code shows *what* was built, never *why* the obvious alternative was rejected.

## Decision

We will record every significant architectural decision as a numbered Markdown file in `docs/adr/`, using [template.md](template.md). Each ADR has five sections: Context, Decision, Alternatives considered, Trade-offs and Consequences.

- An ADR is written in the phase where the decision is made, not in advance.
- Once accepted, an ADR is not rewritten. A changed decision gets a new ADR, and the old one is marked *Superseded by ADR-NNNN*.
- A decision is "significant" if reversing it would take more than a day of work or would change a public contract (API, event schema, database schema).

## Alternatives considered

- **Design docs in a wiki.** They drift away from the code and are not reviewed in pull requests.
- **Comments in code.** Good for local constraints, but architectural decisions span many files and have no natural home in any one of them.
- **No records.** The reasoning then lives only in people's heads.

## Trade-offs

Writing an ADR costs a small amount of time per decision. In return, reviewers and future maintainers can see the reasoning, and the decision is reviewed in the same pull request as the code it affects.

## Consequences

Architectural changes in pull requests should come with an ADR. The ADR index in [README.md](README.md) must be kept up to date.
