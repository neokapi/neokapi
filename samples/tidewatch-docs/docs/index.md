---
title: Tidewatch operator handbook
description: What Tidewatch checks, when it raises an alert, and what you do about it.
sidebar_label: Overview
---

Tidewatch compares the tidal forecast with the constraints of each berth and
raises an alert when a scheduled movement stops being safe. It does not decide
anything. It states what it checked, against which figure, and at what time, and
the duty officer decides.

## What it reads

Three inputs, in this order:

1. The berth register — declared depth alongside, and the clearance each berth
   requires under the vessel's keel.
2. The tidal forecast for the terminal, refreshed every ten minutes.
3. The arrivals plan, as Compass holds it.

Where two of these disagree, Tidewatch reports both figures and raises an alert.
It never silently prefers one source over another.

## What it produces

One alert per movement, per window. An alert names the berth, the vessel, the
constraint that failed, and the time the forecast crosses it. It carries no
recommendation, because the pilot and the master hold information Tidewatch does
not.

## Where to go next

- [Berths](berths) — how a berth's constraints are declared, and what happens
  when they change mid-window.
- [Alerts](alerts) — the alert lifecycle, from raised to acknowledged.
- [Integrating](integrating) — reading alerts from your own systems.
