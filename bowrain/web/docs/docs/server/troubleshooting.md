---
title: Troubleshooting
sidebar_position: 21
description: Practical fixes for the problems that actually come up. Sync conflicts, exhausted AI credits, connector failures, the desktop offline queue, sign-in, and automation logs.
---

# Troubleshooting

This page covers the situations that come up in normal use and how the platform
behaves in each. Each entry describes verified behavior, not a workaround.

## Push and pull are out of sync

kapi keeps a project in step with the server the way git keeps a checkout in step
with a remote. It compares content by hash, so a push or pull transfers only what
actually changed.

Start with `kapi status`, which reports the delta between your local project and
the server: how many items have changed locally (to push) and how many have
changed on the server (to pull). Use `kapi diff` to inspect the specific changes
before you move them.

When both sides have changed, pull before you push:

1. `kapi status`: see what diverged.
2. `kapi pull`: bring the server's changes into your local project.
3. Review, then `kapi push`: send your local changes up.

Because the server versions every change and rollback is non-destructive, a push
never silently discards prior content: a superseded value stays in history and can
be restored. If you need to force a full transfer, for example after a local
cache is rebuilt, `kapi push --force` re-uploads every block and
`kapi pull --force` re-downloads everything, unchanged blocks included. Reach for
these only when you specifically want to bypass the content-hash shortcut; they do
more work and are not the normal path.

See [the developer route](/cli/overview) for the full command set.

## AI operations are blocked ("out of credits")

Hosted workspaces meter AI work in credits: paid plans have a monthly
allowance, and every workspace receives a one-time grant of trial credits at
creation. When the spendable balance is used up, only **AI operations** are
blocked: AI drafting and AI quality checks return an error indicating the
quota is exhausted. Everything that does not call an AI provider keeps working:
browsing content, editing by hand, running content memory lookups, pushing,
and pulling.

You have three ways forward:

- **Wait for the monthly reset** (paid plans). Allowances reset on the 1st of
  each month at 00:00 UTC. The Free plan has no recurring allowance; its
  one-time trial credits do not renew.
- **Buy a credit pack.** Purchased credits never expire; they persist and are
  spent after the plan allowance and trial credits are used up. There is no
  automatic purchase; a workspace is only ever charged because a person chose to
  buy.
- **Upgrade the plan** for a monthly allowance (or a larger one).

Two things are never metered against credits: a **bring-your-own AI key** (those
runs go to your own provider account), and a **self-hosted deployment** (which
runs without the billing pipeline and imposes no credit limit). See
[Security and privacy](/server/security-and-privacy#bring-your-own-ai-keys).

## A connector sync failed

A connector reports its standing through its status: the time of the last sync,
the count of items pending pull (changed upstream) and pending push (changed
locally), and a list of errors from the most recent attempt. When a sync fails,
the error text on the connector status is the first place to look; it usually
points at the upstream cause, such as an expired credential or an unreachable
source.

Connector syncs are **not retried automatically**. After you have addressed the
cause (refreshed a credential, corrected the configuration, restored access to
the source), trigger the sync again. Re-running a sync is safe: like push and
pull, it reconciles by comparing content, so it moves only what changed.

See [Connectors](/server/connectors).

## The desktop app has pending or failed changes

The Bowrain desktop app is a working copy of the server. When the server is
unreachable, edits you make are queued locally instead of being lost, and the app
shows a **pending** count. When the connection returns, the queue is replayed in
order.

Two counts matter, and they mean different things:

- **Pending**: changes waiting to be sent. These drain automatically once the
  server is reachable again.
- **Failed**: changes that will not be retried and need attention.

A change becomes failed in one of two ways:

- **Transient failure** (the server was unreachable, a request timed out, or the
  server was rate-limiting). The app retries the change on later replay passes, up
  to five attempts, before giving up on it.
- **Permanent rejection.** If the server rejects the request itself with a client
  error (a `4xx` status other than request-timeout (`408`) or rate-limited
  (`429`)), retrying the identical request can never succeed, so the change is
  marked failed immediately rather than retried. This is what "failed-permanent"
  means: the server declined the operation (for example, a validation error or a
  permission the account no longer holds), not a network hiccup.

A failed count that will not clear indicates permanently rejected changes. Inspect
those changes, correct the underlying cause (permissions, or the edit itself), and
redo the work; they will not replay on their own.

## Sign-in or session problems

Access tokens are deliberately short-lived and the clients refresh them silently
in the background, so a session that has been idle usually recovers on its own. If
sign-in fails outright:

- **Refresh-token reuse.** If a refresh token is presented twice, the whole
  session family is invalidated as a theft-protection measure. Sign in again to
  start a fresh session.
- **Self-hosted redirect mismatch.** A sign-in that returns to an error after the
  identity provider usually means the OIDC client's redirect URI or public URL
  does not match the address the browser used. Confirm `BOWRAIN_OIDC_PUBLIC_URL`
  and the provider's registered redirect URI against your public HTTPS URL; see
  [Self-hosting](/server/self-hosting).
- **Clock skew.** Because sessions use short-lived, time-bounded tokens, a client
  whose clock is far off can fail validation. Correct the system clock.

## Where to see automation run logs

Automation runs are recorded per project. Each run captures its trigger, its
steps, and the outcome of each step; each step keeps its own log lines. Open a
project's automation runs to see the history: which runs fired, which steps
succeeded or failed, and the per-step logs for a failed step. This is the record
to consult when an event-driven automation (for example, drafting on push)
did not do what you expected. See [Automation](/server/automation).
