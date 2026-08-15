---
title: Alerts
description: The alert lifecycle, from raised to acknowledged, and what each state means to the record.
sidebar_label: Alerts
---

An alert is one statement about one movement, at one berth, in one window. It is
not a weather warning, and it is not advice.

## The lifecycle

| State | What it means |
| --- | --- |
| Raised | The forecast crosses a declared constraint inside the window. |
| Acknowledged | A named duty officer has read it. The movement is unchanged. |
| Closed | The condition no longer holds, or the movement was withdrawn. |

An alert moves forward only. A condition that returns raises a new alert rather
than reopening the previous one, so the record shows two events where two things
happened.

## Acknowledging

Acknowledgement records who read the alert and when. It does not record agreement,
and it does not clear the alert: a movement that goes ahead against an open alert
goes ahead with the alert on the record.

Acknowledge from Compass, or from your own system through the API.

## What an alert never does

Tidewatch does not hold a movement, does not notify a pilot, and does not claim
that a movement is safe. It states the constraint it checked and the figure it
checked against. The decision belongs to the people who hold the rest of the
picture.
