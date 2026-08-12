---
title: Changelog
description: What changed in Compass, Tidewatch, and the API, and when.
---

# Changelog

Entries are kept as they were written. A past entry is a record of what was
announced at the time, so it keeps the wording that was in use then, including
names that have since been retired.

## 2026-03-01 — Compass 4.2

- The berth plan accepts a movement reason on every reassignment, and the
  reason is carried into the movement record the port authority reads.
- Waiting arrivals are ordered by scheduled arrival rather than entry order.

## 2026-01-20 — Vocabulary change

Compass now calls a place alongside a **berth** everywhere in the interface and
in this documentation. It was previously called a *mooring*.

The API is unchanged. The field that identifies the place stays exactly as it is,
because it is part of the published contract and renaming it would break every
integration that reads it. Entries in this changelog written before today keep
the old word.

## 2025-11-04 — Compass 4.1

- Mooring attributes gained a services column, so shore power, fresh water, and
  waste reception are visible in the plan.
- The list now hides any place alongside whose draught cannot accept the
  selected vessel, rather than showing it as unavailable.

## 2025-09-16 — Tidewatch 2.0

- Alerts carry the reading and the threshold that raised them.
- Escalation to the terminal supervisor is configurable per terminal, and
  defaults to fifteen minutes.

---

Questions about a released change go to operations@northsea.example.
