---
sidebar_position: 8
title: Telemetry
description: The anonymous usage telemetry the kapi CLI collects, the exact event list and payload shape, the privacy invariants, and every way to turn it off.
keywords: [telemetry, privacy, analytics, opt out, DO_NOT_TRACK, KAPI_TELEMETRY, anonymous usage data]
---

# Telemetry

Release builds of the kapi CLI collect a small amount of anonymous usage
telemetry (which built-in commands are run, how long they take, in coarse
buckets, and whether they succeed) to guide development priorities. This
page is the complete inventory: every event, every property, the privacy
invariants, and every mechanism for turning it off.

Telemetry is **opt-out**: it is enabled by default in official release
binaries, disclosed by a one-time notice on first use, and disabled by any
of the switches below. Builds you compile yourself carry no reporting key
and send nothing.

## What is collected

Two events exist. There are no others.

| Event | When | Properties |
| --- | --- | --- |
| `cli_first_run` | Once, at the moment the first-run notice is printed | common properties only |
| `cli_command_run` | After a top-level command completes | `command`, `duration_bucket`, `exit_class`, plus the common properties |

Common properties on every event: `app_version` (the kapi version),
`os` (e.g. `darwin`), `arch` (e.g. `arm64`), and `surface` (`cli`).

Property values are restricted to fixed vocabularies:

- **`command`**: the name of the command that ran, but only for kapi's
  built-in verbs (for subcommands, the dotted path, e.g. `memory.import`).
  A command contributed by a plugin is reported as the literal string
  `plugin`; a verb that matched nothing is reported as `other`; a bare
  `kapi` invocation is reported as `root`. Plugin and unknown verb names
  are never transmitted.
- **`duration_bucket`**: one of `<100ms`, `<1s`, `<10s`, `<60s`, `>=60s`.
  The exact duration is not reported.
- **`exit_class`**: `ok`, `error`, or `usage`.

Events are keyed by a single identifier: a random UUID generated on the
machine the first time telemetry is eligible to run, stored under the
`telemetry.machine_id` key in the app configuration file (`kapi.yaml` in the
user config directory (`~/.config/kapi` on Linux,
`~/Library/Application Support/kapi` on macOS). `kapi config path` prints the
resolved location. It encodes nothing (neither the hostname, the username, nor
any hardware identifier), and deleting it causes a new random one to be
generated.

`kapi telemetry status` prints the current state, the reason when disabled,
and this payload shape.

## What is never collected

Under no circumstances do events contain:

- command **arguments** or **flag values**,
- **file paths**, **file contents**, or any extracted text,
- **project names** or recipe contents,
- hostnames, usernames, IP-derived geolocation beyond what any HTTP request
  reveals, or any other personal information.

Help and shell-completion invocations (`--help`, `kapi help`,
`kapi completion`, and the hidden completion machinery) emit nothing, and
the `kapi telemetry` command group itself is never reported.

## Turning it off

Telemetry is disabled when **any** of the following holds; the switches
compose as a lattice where any single one wins:

| Switch | Scope |
| --- | --- |
| `kapi telemetry off` | Persisted in the kapi configuration file; `kapi telemetry on` reverses it |
| `KAPI_TELEMETRY=0` (or `false`) | Environment, per invocation or exported |
| `DO_NOT_TRACK=1` (any non-empty value other than `0`) | The cross-tool console convention |
| `CI` set in the environment | Automatic; CI runs never report |
| Built with `-tags notelemetry` | Compile-time: the entire client is excluded from the binary |
| No reporting key in the build | Automatic; source builds and development builds send nothing |

Disabled means disabled: no events are sent, no first-run notice is shown,
and no identifier is generated.

## Delivery

Events are queued in memory and posted in the background to a European
Union PostHog ingestion endpoint. Delivery is best-effort and fire-and-forget:
a failed or slow request is silently dropped, the flush at process exit waits
at most 100 milliseconds, and telemetry can never block, slow down
(beyond that bound), or fail a command.

## The first-run notice

The first time an eligible command runs in an enabled build, kapi prints a
short notice to stderr stating what is collected, what never is, the
opt-out switches, and a link to this page. It is printed exactly once and
recorded in the configuration file (`telemetry.notice_shown`). Ineligible
runs (help, completion, or any run with telemetry disabled) neither show
the notice nor consume it.
