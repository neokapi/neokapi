---
title: Berths
description: Assign a vessel a place alongside, and change the assignment when the day moves.
---

# Berths

A **berth** is the place a vessel occupies alongside. Compass holds one berth
plan per terminal per day, and every arrival on that plan is either assigned a
berth or waiting for one.

## Assign a berth

1. Open the berth plan for the terminal and the day you are planning.
2. Select the arrival you want to place.
3. Choose a free berth from the list. Compass shows only the berths whose
   length and draught accept the vessel.
4. Confirm the assignment.

Compass records who made the assignment and when. The vessel's agent sees the
change on the next refresh of the arrivals board.

## Change an assignment

An assignment holds until you change it or the vessel departs. To move a vessel,
select the arrival and choose a different berth. Compass checks the new berth
against the vessel's dimensions and refuses an assignment the berth cannot
accept.

Moving a vessel that has already arrived writes a movement record. The movement
record is what the port authority reads when it reconciles the day, so give it a
reason.

## When no berth is free

If no berth accepts the vessel, Compass leaves the arrival in the waiting list
and raises an alert. The waiting list is ordered by scheduled arrival, not by
the order in which arrivals were entered.

You have three ways to clear a waiting arrival:

- Release a mooring earlier than planned, by shortening the departure window of
  the vessel that holds it.
- Accept a shorter stay for the waiting vessel, which may open a berth that was
  otherwise too tightly scheduled.
- Move the arrival to the next tidal window, which Tidewatch will already have
  proposed if the tide is the constraint.

## Berth attributes

| Attribute | What it means |
| --- | --- |
| Length overall | The longest vessel the berth accepts, in metres. |
| Draught | The deepest vessel the berth accepts at mean low water, in metres. |
| Services | Shore power, fresh water, and waste reception, where fitted. |
| Restrictions | Any standing restriction, such as a night movement ban. |

Berth attributes are maintained by the terminal, not by the duty officer. If an
attribute is wrong, raise it with the terminal rather than working around it in
the plan.

---

Questions about berth planning go to berths@northsea.example.
