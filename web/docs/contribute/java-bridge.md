---
sidebar_position: 6
title: Okapi Bridge
description: The okapi-bridge plugin lives in its own repository and serves as the reference implementation of a third-party kapi plugin in a non-Go language.
keywords: [Okapi bridge, plugin, gRPC, JVM]
---

# Okapi Bridge

The Okapi bridge — a JVM plugin daemon that exposes the Okapi Framework's Java
filters over the kapi plugin protocol — is developed in its own repository:
[neokapi/okapi-bridge](https://github.com/neokapi/okapi-bridge).

It is no longer part of the product surface. Its role today is keeping the
plugin protocol honest: it is the reference implementation of a third-party
kapi plugin in a non-Go language, and its CI runs the protocol conformance
suite against released kapi versions.

The plugin *protocol* (manifest model, subprocess lifecycle, gRPC services) is
documented in [Plugin system](./architecture/007-plugin-system.md) and the
[plugin bridge protocol notes](./notes-internal/plugin-bridge-protocol.md).
