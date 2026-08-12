---
title: Northsea operations documentation
description: How to run a port day with Compass and Tidewatch.
---

# Northsea operations documentation

Northsea builds two things: **Compass**, the fleet view your duty officer keeps
open all day, and **Tidewatch**, the service that warns you before conditions
change. This documentation covers both, and the `tidectl` command line that
drives them from a terminal.

## Who this is for

You are a duty officer, a berth planner, or an integrator wiring Northsea into a
port management system. The pages assume you know how a port day runs; they do
not assume you have used Northsea before.

## Start here

- [Berths](berths.md) — how a vessel is assigned a place alongside, and what
  happens when the assignment changes.
- [Alerts](alerts.md) — how Tidewatch decides that conditions have changed, and
  who hears about it.
- [API reference](api/reference.md) — the HTTP surface, for integrators.
- [Changelog](changelog.md) — what changed, and when.

## Conventions in this documentation

A command you type is written in a code block. A value you replace is written in
angle brackets, like `<vessel-id>`. Where a page refers to a field of the HTTP
API, it uses the field name exactly as the API spells it, which is not always
the word this documentation uses in prose.

## Getting help

Every page ends with the support address for that area. If you are unsure which
area a question belongs to, write to operations support and it will be routed.
