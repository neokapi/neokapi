# Ledgerly writing guide

Follow this guide for everything you write in this repository: the app's
interface strings (`locales/en.json`), help articles (`help/`) and support
replies (`support/`).

## Voice

Ledgerly writes for freelancers who are good at their work and short on time
for admin. Be friendly and exact: say what happens to their money, when, and
what it costs.

- Friendly, exact and reassuring. Neutral register, calm tone, no humor.
- Address the reader as "you". Active voice, short sentences, contractions
  where natural.
- Give real numbers and dates instead of adjectives like "fast" or "low".
- Never imply that money moves instantly or for free.
- Never promise anything about taxes: Ledgerly does not give tax advice, so
  point the reader to their accountant.
- Do not mention other products by name.
- Interface strings use sentence case. Buttons start with a verb and have no
  full stop.

## Avoid

- "instant" or "instantly": payouts and payments are never instant. Give the
  real timing.
- Claims that a payment costs nothing: "no fees", "zero fees", "fee-free",
  "free of charge". Give the real fee.
- Promises about taxes or outcomes: "tax-compliant", "tax-ready",
  "guarantee", "accountant-approved".
- Exclamation marks.

## Words

| Write | Not |
| --- | --- |
| other invoicing tools | Invoicr (a competitor; never name it) |
| customer, customers | client, clients |
| payout | pay-out |

## The Autopay rename

Autopay was renamed **Scheduled payments**. Every string and help article uses
the new name. String keys in `locales/en.json` (such as `autopay.title`) stay
as they are.
