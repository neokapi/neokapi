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

The plugin *protocol* — the manifest model, the three transports, and the gRPC
services a daemon serves — is specified in
[Plugin protocol v1](./notes-internal/plugin-protocol-v1.md), with its rationale
in [Plugin system](./architecture/007-plugin-system.md). Any plugin repository
can verify itself against that contract with the conformance suite the spec
describes.
