---
title: API reference
description: The Northsea HTTP surface, for integrators.
---

# API reference

The Northsea API is HTTP and JSON. Every request carries a bearer token issued
for one terminal. Times are RFC 3339 in UTC.

The field names below are the wire contract. Several of them predate the current
vocabulary and are kept exactly as they are, because renaming a field breaks
every integration that reads it. Where a field name and the prose name differ,
the difference is noted.

## `GET /v1/arrivals`

Lists arrivals on the berth plan for one terminal and one day.

| Parameter | Type | Notes |
| --- | --- | --- |
| `terminal_id` | string | Required. |
| `date` | string | Required. `YYYY-MM-DD`, terminal local time. |
| `state` | string | One of `waiting`, `assigned`, `alongside`, `departed`. |

Each arrival in the response carries:

```json
{
  "arrival_id": "arr_8f21",
  "vessel_id": "ves_0c44",
  "mooring_id": "mrg_17",
  "state": "assigned",
  "eta": "2026-03-14T06:40:00Z",
  "etd": "2026-03-14T18:10:00Z"
}
```

`mooring_id` identifies the place alongside. The prose name for that place is
**berth**; the field is `mooring_id` because it was named before the vocabulary
was settled, and it is part of the published contract.

## `PUT /v1/arrivals/{arrival_id}/assignment`

Assigns an arrival to a place alongside, or moves it.

```json
{
  "mooring_id": "mrg_23",
  "reason": "Draught restriction at mrg_17"
}
```

The request fails with `409` if the target cannot accept the vessel. The
response body names the constraint that refused it.

## `GET /v1/alerts`

Lists open alerts for a terminal.

| Parameter | Type | Notes |
| --- | --- | --- |
| `terminal_id` | string | Required. |
| `since` | string | RFC 3339. Alerts raised at or after this instant. |
| `include_acknowledged` | boolean | Defaults to `false`. |

An alert carries the reading and the threshold that raised it:

```json
{
  "alert_id": "alt_44b1",
  "arrival_id": "arr_8f21",
  "mooring_id": "mrg_17",
  "kind": "under_keel_clearance",
  "reading_m": 1.2,
  "threshold_m": 1.5,
  "raised_at": "2026-03-14T04:05:00Z"
}
```

## Errors

| Status | Meaning |
| --- | --- |
| `400` | The request was malformed. The body names the field. |
| `401` | The token is missing, expired, or issued for another terminal. |
| `409` | The request was well formed but conflicts with the current plan. |
| `429` | Too many requests. Retry after the interval in `Retry-After`. |

---

Questions about the API go to integrations@northsea.example.
