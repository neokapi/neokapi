---
sidebar_position: 0
title: Overview
slug: index
---
# Implementation Notes

Implementation notes contain tactical details -- some extracted from
[Architecture Decisions](/docs/ad/index), others standalone. These include
SQL schemas, API routes, algorithm pseudocode, Go interface definitions,
and other reference material.

| Note | Parent AD | Content |
|------|-----------|---------|
| [Content Store Schema](content-store-schema.md) | [AD-003](/docs/ad/003-content-store) | SQL schemas, migrations, API routes |
| [Connector Interfaces](connector-interfaces.md) | [AD-005](/docs/ad/005-connector-system) | Go structs, method signatures |
| [Plugin Bridge Protocol](plugin-bridge-protocol.md) | [AD-007](/docs/ad/007-plugin-system) | gRPC protocol, bridge descriptor |
| [TM Matching Algorithm](tm-matching-algorithm.md) | [AD-009](/docs/ad/009-translation-memory) | Tiered matching, TMX mapping |
| [Terminology Data Model](terminology-data-model.md) | [AD-010](/docs/ad/010-terminology) | Go structs, TermBase interface |
| [Bowrain UI Components](bowrain-ui-components.md) | [AD-012](/docs/ad/012-bowrain) | Editor modes, component library |
| [CLI Commands Reference](cli-commands-reference.md) | [AD-013](/docs/ad/013-cli-and-server) | Command tree, REST routes, gRPC |
| [Kapi Sync Protocol](kapi-sync-protocol.md) | [AD-016](/docs/ad/016-kapi-project-model) | Config schema, sync algorithms |
| [Glass UI Theme](glass-ui-theme.md) | -- | shadcn-glass-ui, OKLCH tokens, 3 themes |
| [Keycloak Theming](keycloak-theming.md) | -- | Keycloakify v11, custom login pages |
| [NPM Workspaces](npm-workspaces.md) | -- | Workspace config, build order, lock files |
| [Docker Compose](docker-compose.md) | -- | Dev deps, Keycloak + Mailpit, e2e support |
| [MCP Tools Reference](mcp-tools-reference.md) | [AD-021](/docs/ad/021-mcp-integration) | Tool specs, input/output schemas, testing |
| [Skeleton Store](skeleton-store.md) | [AD-005](/docs/ad/005-connector-system) | SkeletonStore binary format, streaming HTML reader/writer |
| [Implementing Formats](implementing-formats.md) | [AD-005](/docs/ad/005-connector-system) | Step-by-step guide for new format readers/writers |
