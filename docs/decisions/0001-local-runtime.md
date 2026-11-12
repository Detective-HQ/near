# ADR 0001: Runnable local runtime

- Status: Accepted for prototype
- Date: 2026-11-10

## Context

The handoff architecture recommends a Go API, PostgreSQL, Redis, and workers. The available environment has Node.js 22 and npm but does not have the Go toolchain. A Go build cannot be verified here, and installing a toolchain would add a dependency and execution step outside the requested project changes.

## Decision

Use Node.js built-in HTTP, filesystem, crypto, and test modules for a dependency-free local prototype. Persist demo records in one local JSON file and keep all data-source and execution capabilities disabled by default in production.

## Consequences

- Collaborators can run and test the prototype without a package install.
- The local store is single-process and not suitable for concurrent production service.
- The current architecture is not the production target from the product brief.
- Before production, replace this API/store with the reviewed service boundaries, PostgreSQL schema, background worker, provider adapters, and wallet clients.
