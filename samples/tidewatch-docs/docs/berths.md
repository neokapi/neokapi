---
title: Berths
description: How a berth's constraints are declared, and what Tidewatch does when they change inside an open window.
sidebar_label: Berths
---

A berth is the place a vessel occupies alongside. Tidewatch holds two figures
for each one: the declared depth alongside, and the clearance required under the
keel. Both are the port authority's figures, not Tidewatch's.

## Declaring a berth

1. Open the berth register in Compass.
2. Enter the declared depth alongside, in metres, as the last survey recorded it.
3. Enter the required clearance for the terminal.
4. Record the survey date. Tidewatch marks a berth whose survey is older than
   twelve months, and keeps checking it.

The register is the record the port authority reads, so a figure is entered as
surveyed rather than as estimated.

## When a constraint changes inside an open window

A survey lands, or the authority revises a clearance, while a movement is already
planned. Tidewatch re-evaluates every open window against the new figure
immediately and raises an alert for each movement that no longer clears it. The
previous evaluation is kept: an alert states which figure it was raised against,
so the record shows what was known when.

Nothing is withdrawn silently. An alert that no longer applies is closed with the
reason and the time, and stays in the record.

## Releasing a berth

Release a berth when the vessel has cleared it, not when it is scheduled to. The
next arrival cannot be booked into a berth that is still occupied, and Tidewatch
evaluates the plan it is given.
