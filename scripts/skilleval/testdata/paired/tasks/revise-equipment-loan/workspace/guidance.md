# Alder Library equipment lending

This is a fictional library service. Apply decision EQ-2026-09 to the current
camera loan pages and reminder template. The decision is already approved.

## Current decision

Camera loans last 14 days. Use the exact approved label `14-day loan` in
`loan_label` and the string `14` in `loan_days`. The borrowing page's
`return_instruction` must read `Return the camera within 14 days of collection.`
The reminder's `body` must read `Your camera loan lasts 14 days. Return it by {due_date}.`
The reminder is sent two days before the loan ends; store its day after
collection in `send_after_days`, as a decimal string without units. Collection
is day 0. Update every current camera surface, including the desk card.

The change does not add renewals, delivery, a grace period or late fees. Other
wording and metadata stay as written. Keep `{due_date}` intact. These are
approved display strings and scheduling values, not a request for a rewrite.

## Protected exceptions

- Telescope loans still last 7 days. Their page, including the `7-day loan`
  label, is unchanged. This exception is approved by the equipment service owner
  under EQ-2026-09-T.
- The archive is a record of the previous camera policy. Preserve it exactly.
- The integration contract's `camera_7_day` identifier is a stable external ID.
  Neither the integration contract nor any `policy_id` changes with display copy.
- Edit only the camera borrowing page, camera reminder and camera desk card.
  Guidance, the recipe, profiles and all other files remain unchanged.

The recipe binds each file to its content channel. The voice profile expresses
checks for obsolete loan wording and the telescope exception. It also carries
this decision as guidance. The schedule calculation and preserved identifiers
remain separate acceptance requirements even when wording checks pass.
