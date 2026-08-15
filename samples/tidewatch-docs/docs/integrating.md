---
title: Integrating
description: Reading alerts from your own systems, and the field names the wire contract keeps.
sidebar_label: Integrating
---

Tidewatch exposes alerts over a read API and, for terminals that ask for it, a
webhook. Both carry the same object.

## Reading alerts

```
GET /v1/alerts?terminal=NL-RTM-04&state=raised
```

The response is a list of alerts, most recent first, each carrying the berth, the
vessel, the constraint that failed and the time the forecast crosses it.

## Field names

The wire contract keeps `mooring_id` as the identifier of a berth. The vocabulary
retired *mooring* in favour of *berth* on 20 January 2026; the field name did
not change, because it is published and integrators depend on it. Prose says
berth, the wire says `mooring_id`, and both are correct in their own place.

## Webhook delivery

A webhook is delivered at most once per alert transition, and is retried for one
hour. A terminal that requires exactly-once handling reconciles against the read
API rather than relying on delivery.
