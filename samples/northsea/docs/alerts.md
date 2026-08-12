---
title: Alerts
description: How Tidewatch decides that conditions have changed, and who hears about it.
---

# Alerts

Tidewatch watches tide, wind, and swell against the constraints of each berth in
the plan, and raises an alert when a scheduled movement stops being safe. An
alert is a statement about one arrival or one departure, at one berth, in one
window. It is never a general weather warning.

## What raises an alert

Tidewatch raises an alert when any of the following becomes true for a movement
already on the plan:

- The forecast depth at the berth falls below the vessel's draught plus the
  under-keel clearance the terminal requires.
- The forecast crosswind at the berth exceeds the limit recorded for the vessel
  class.
- The tidal window that the movement was planned into narrows to less than the
  manoeuvre time the pilot has recorded.

Each condition carries the reading that triggered it and the threshold it
crossed, so a duty officer can judge the alert without opening the forecast.

## Who hears about it

An alert goes to the duty officer on watch for the terminal, and to any user who
has subscribed to the vessel. Subscriptions follow the vessel, not the berth: a
vessel that is reassigned keeps its subscribers.

Tidewatch does not send an alert to the vessel's agent. Telling an agent that a
movement is at risk is an operational decision, so Compass gives you a prepared
message to send rather than sending one for you.

## Acknowledging an alert

Acknowledge an alert to record that a person has seen it. Acknowledgement does
not close the alert; the alert closes when the condition that raised it clears,
or when the movement it refers to is cancelled.

An alert that nobody acknowledges within the escalation window is repeated to
the terminal supervisor. The escalation window is set per terminal and defaults
to fifteen minutes.

## Alert history

Every alert is kept with its readings for two years. The history is what you
read when you are asked why a movement was delayed, so the readings are stored
as they were at the time and are never restated against a later forecast.

---

Questions about alerting go to tidewatch@northsea.example.
