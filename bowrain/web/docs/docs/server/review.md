---
sidebar_position: 4
title: Review
---

# Review

The Review surface is where translations are checked block by block. It is a
sibling of the [Translation Editor](/server/translation-editor), scoped to the
same file and reached from the surface switcher in the header.

Review is deliberately separate from editing: the Translate editor stays focused
on producing translations, while Review is focused on accepting them.

## Block list

Review lists every translatable block with its source, target, and current
status. Filter the list by status — **Not Started**, **Draft**, **Translated**,
or **Reviewed** — to work through one stage at a time. Each filter shows a count
so you can see how much of the file remains.

## Approve and reject

Each block has **Approve** and **Reject** actions. Approving marks the block
**Reviewed**; rejecting sends it back to **Draft** for further work. The status
badge updates in place.

## Bulk actions

Select blocks individually or use **Select all** to act on the whole filtered
list at once:

- **Mark reviewed** — approve every selected block.
- **Apply exact TM** — fill selected blocks from exact (100%) translation-memory
  matches.

## QA findings

Click **Run QA** to check the file. Findings appear both inline under each
affected block and in a problems panel; clicking a finding scrolls to its block.
Each finding carries a severity (error or warning), a type, and a message.

## Brand review

Block-level translation review is separate from brand-rule review. Promoting or
rejecting suggested brand rules happens on the brand review page — see
[Brand voice](/server/brand-voice).
