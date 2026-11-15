---
sidebar_position: 5
title: Review on your phone
description: The Bowrain mobile app shows the translations waiting for review in one language as a deck of cards. You approve, reject, fix or put off each card with a swipe or a button.
keywords: [review, mobile app, phone, iOS, review deck, swipe, fix, bowrain]
---

# Review on your phone

The Bowrain mobile app shows the translations waiting for your review as a
deck of cards, one language at a time. You read a card and decide with a swipe
or a button. A decision made on the phone needs the same review permission as
one made in the [review session](/server/review), and Bowrain records it the
same way.

The app is in beta on iOS.

## Sign in

The app signs in to Bowrain Cloud. If your workspace runs on its own server,
tap the cog on the sign-in screen to open **Server settings**, choose
**Specify a different server** and enter its web address. Tap **Sign in** to finish in the browser
with the account you use for the web app.

## Find the work

The app has four tabs: **Home**, **Review**, **Activity** and **Account**.

**Review** lists each language you can review that has translations waiting,
across all your projects and workspaces. The language with the most waiting
comes first. Tap a row to open that language's deck.

**Home** lists your workspaces, their projects and each project's target
languages. Each language shows how many cards are quick and how many need a
computer. Tap a language to open its deck.

## The deck

Bowrain sorts the cards by how much attention each needs. The deck starts with
the cards you can judge at a glance:

- **Quick**: no check findings and a high pre-review score.
- **Look closer**: a low or missing pre-review score, or findings that do not
  fail a check.

Some translations need a closer look than a phone allows. These are listed
below the deck as cards for the desk, and you review them in Bowrain on a
computer. A translation goes to the desk when it fails a check, when its source
changed after it was approved, when it is a new draft after a rejection, or
when its source is long.

Each card shows the source and the translation, with placeholders and inline
codes drawn as labels. When they exist, the card also shows the pre-review
score, the number of failing checks, and notes on checks, terms and the voice
profile. The key and the file appear at the bottom of the card. A card with
none of these notes says that it has no context, so you know you cannot see
where the text appears.

## Decide on a card

| Action | Swipe | Button | Result |
| --- | --- | --- | --- |
| Approve | Right | **Approve** | The translation is approved. |
| Reject | Left | **Reject** | The fix sheet opens on the rejection. You choose a reason, add a note if you want, and Bowrain asks for a new draft. |
| Later | Up | **Later** | Nothing is recorded. The card moves to the end of the deck. |
| Fix | None | **Fix** | The fix sheet opens on the fixes. |

**Undo** takes back your last decision, and the card returns to the deck.

**Approve** is turned off on a translation you wrote. Another reviewer
approves it. You can still fix or reject it.

## Fix a translation

Tap **Fix** to open the fix sheet. Under **Why**, choose what is wrong: wrong
term, meaning, tone, placeholder or grammar. Bowrain offers ready-made fixes
for that reason. A fix can be a term fix from the terms store or a match from
the content memory. Tap **Suggest a rewrite** for an AI rewrite, which uses AI
credits.

Tap **Use this fix** to take a suggestion. To write the correction yourself,
tap **Edit the translation**. The editor locks placeholders and inline codes,
so you change only the words around them. A translation with plural or select
forms can only be edited on a computer.

**Approve the correction** saves your wording and approves it in one step. If
your workspace requires another reviewer for it, the app tells you that the
correction is saved and waiting for them.

A correction settles the text now. A rejection waits for a new draft. Choose a
correction when you can see the right wording.

## What happens after a decision

An approval from the phone writes the same records as one from the review
session: a decision in the project's ledger and an entry in the workspace audit
log. The approved wording goes into the workspace
[content memory](/server/translation-memory). Later runs reuse it before they
spend credits on a new draft. A correction you approve adds your wording the
same way. See [What an approval leaves behind](/server/review#what-an-approval-leaves-behind).

A decision applies to the wording you read. If someone changed the translation
after you opened the card, Bowrain records nothing. The app tells you, and the
new version comes back into the deck.

## Without a connection

You can keep deciding when the phone is offline. Your decisions wait on the
phone, the deck shows how many are queued, and the app sends them in order
when the connection returns. Ready-made fixes need a connection, but you can
still edit or reject.

If you sign out with decisions still queued, the app asks first, because
signing out discards them.

## Activity

The **Activity** tab shows what changed in a project: pushes, runs, review
sessions, context changes, change sets and coverage milestones. It shows the
project you opened last. Tap the project name to choose another. From a review
entry, **Open the review deck** takes you to the deck for that language.

## Account

The **Account** tab shows the server you use, the appearance setting and the
app version. **Manage account** opens your account settings in
the browser. **Sign out** is at the bottom.

## Related

- [Review](/server/review): the review session, bulk approval and ship states
- [Content memory](/server/translation-memory): how approved wording is reused
- [Members and roles](/server/members-and-roles): who can review which language
