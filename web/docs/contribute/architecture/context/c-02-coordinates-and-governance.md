---
id: c-02-coordinates-and-governance
sidebar_position: 2
title: "C-02: Coordinates and governance"
description: "Architecture decision: content sits at a point in the context space. Two structural axes, the product it belongs to and the channel it ships on, are derived from the recipe; declared axes such as brand and mode are stated once and inherited. One resolver answers what governs that point, at an instant, for every caller."
keywords: [coordinates, context space, governance, profiles, channels, brand, mode, validity, voice profile, neokapi, architecture decision]
---

# C-02: Coordinates and governance

## Summary

Content is written for a **point** in the context space. Two axes are
**structural**: the **product** the content belongs to and the **channel** it
ships on. Neither is a taxonomy a recipe invents. A key under `profiles:` *is*
the product-axis value, and the channels that profile declares *are* the
channel-axis values it can carry. The remaining axes are **declared**: a project
names the dimensions its content actually varies along (a brand, a document
mode, a market) once under `defaults.coordinates`, and a collection that sits
elsewhere overrides the one axis it differs on.

Governance (a voice profile, a vocabulary) binds to a point. One function,
`KapiProject.ResolveGovernanceFor`, answers *what governs here, at this instant*,
and every caller goes through it: a run, a check, a push, and the by-location
retrieval answer. Resolution walks the declared bindings from the finest point to
the coarsest, and a binding whose validity window excludes the instant is skipped
exactly as if the recipe had not declared it, with the transition reported
rather than applied silently.

The resolved point is what travels: as coordinates on the wire, and as the
coordinate a collection is `governed_by` in the context graph
([C-03](c-03-context-store-and-graph.md)).

## Context

A project is not always one voice, and a voice does not read the same
everywhere. One repository can ship more than one product, and a project-wide
binding then governs the wrong one half the time. Which product a page belongs to
and which channel it appears on are the two things that decide how it should
read.

A free-form taxonomy declared alongside the collections that use it has to be
kept in step with them by hand, and drifts. Deriving the structural axes from
what the recipe already says removes both the taxonomy and the drift: the
profile that governs a product and the channels that product ships on are the
same declaration. The axes a project has to name for itself are stated in one
place and inherited, for the same reason `defaults.voice` is a default with a
per-collection override rather than a field repeated on every entry.

## Decision

### The space is structural

```yaml
profiles:
  northsea:
    channels: [docs, app]
    voice: .kapi/voice.yaml
  acme:
    channels: [docs, landing]
    # no `voice:`; .kapi/profiles/acme/voice.yaml answers by convention

collections:
  - name: northsea-docs
    channel: northsea/docs    # both products ship `docs`, so qualify it
  - name: acme-landing
    channel: acme/landing
```

A `channel:` is always written `profile/channel`: a channel is a surface *of* a
product, and the binding reads as one. A bare channel name is a load error that
spells out the qualified form(s), so the fix is a copy-paste. A collection that
binds no channel is governed by the project's default point: `defaults.voice`
and the project's own terms.

The channel is what the framework interprets. Once a profile is selected, the
channel selects the matching override *inside* that profile's voice
([C-07](c-07-voice-profiles.md)). A landing-page register is therefore authored
once, in the voice it varies; a channel the profile says nothing about leaves the
base voice in place, which is the right answer for a voice that reads the same
everywhere.

Profile names and channels are **slugs** (lowercase letters, digits and
hyphens), and they are machine identifiers: stable, never translated, compared
byte for byte. Each may *carry* a concept reference for display, and resolution
never looks at it. Concepts are designed to be renamed and deprecated as
vocabulary is revised, and governance that moved when someone edited a term
would be governance nobody could rely on.

The two structural axes are named by the framework. They travel as `product`
and `channel` (`project.ProductAxis`, `project.ChannelAxis`), and
`ChannelRef.Coordinates()` renders a resolved point as that map, omitting the
axes it does not set.

### Declared axes

The coordinate map is open. The wire carries it as `map<string,string>`, the
context entry's hash folds whatever it finds in sorted order, and the graph
writes each axis as a property, so a project names the dimensions its content
varies along:

```yaml
defaults:
  coordinates:
    brand: northsea        # every collection sits here unless it says otherwise
    mode: reference

collections:
  - name: northsea-tutorials
    channel: northsea/docs
    coordinates:
      mode: tutorial       # moves on one axis; inherits the brand
```

A collection's point is `project.MergeCoordinates(defaults, derived, declared)`:
the project's `defaults.coordinates`, overlaid with what its `channel:` derives,
overlaid with its own `coordinates:`. Most specific wins, per axis. An empty
value never erases what a broader layer said: blanking a value is an incomplete
edit, and reading it as an erasure would let a typo move content off its point.

The structural axes are never written under `coordinates:`. They are derived
from `channel:`, and a declared value that could shadow them would let a recipe
contradict its own point; `project.DeclarableAxis` refuses `product` and
`channel` wherever a coordinate is edited.

Two declared axes are spelled by the framework so the spelling is one thing
rather than each recipe's own. `project.BrandAxis` (`brand`) is coarser than
product: a workspace has brands, a brand has products, a product ships channels.
Brand is an axis rather than a subsystem. Content sits *at* a brand the way it
sits at a product, and what governs it there (a voice profile, terms, gates) is
bound at the point rather than being part of it. `project.ModeAxis` (`mode`)
carries what kind of document sits at a point in the Diátaxis sense, with
`tutorial`, `how-to`, `reference` and `explanation` as conventional values rather
than an enum. Correct style is a function of mode: hedging is wrong in a tutorial
and right in an explanation, so one profile applied flatly across all four is
wrong for at least one of them.

### A profile's files mirror the recipe

`.kapi/profiles/<name>/` holds what that profile overrides: `voice.yaml`, and
`terms.json` where the vocabulary differs too, with `<name>` the profile's key
under `profiles:`. The project default has no directory: its files are the flat
ones in `.kapi/` itself. A profile that binds no `voice:`/`termstore:` of its
own is answered by its directory before `defaults.voice` is, so a profile
declaring only its channels is a complete profile.

This is the filesystem mirroring the recipe. A recipe states its default
governance under `defaults:` and its per-product governance under `profiles:`;
the default's files sit flat and each profile's sit in a directory of its own, so
"which voice governs this product's docs" is answerable by looking. Governance is
the only thing that splits this way. The content memory and the unit-state
record stay top-level ([C-01](c-01-project-model.md)), because a recycled
translation and an approval are facts about a unit, true wherever it is governed
from.

### One resolver, one ladder

`GovernancePoint` names the place to resolve for and the instant to resolve at:

```go
type GovernancePoint struct {
    Profile    string    // a profile named directly, with no location under it
    Collection string    // a content collection, by name
    Path       string    // a project-relative, slash-separated file path
    At         time.Time // the run's wall clock; the zero value is the as-declared view
}

func (p *KapiProject) ResolveGovernanceFor(pt GovernancePoint) (*ResolvedGovernance, error)
```

Resolution walks the declared bindings **from the finest to the coarsest**:

1. **A content item's own `channel:`**. A file is the finest declared point, so
   one file in a collection can ship on a different channel, or under a different
   profile, than its neighbours. A path is matched against every item with the
   same first-match-wins glob walk that assigns a file to its collection.
2. **The collection's `channel:`**, for a path no item claims and for a caller
   that names a collection rather than a file.
3. **The project's default point**: `defaults.voice` and the project's own
   terms.

`Profile` outranks both location forms and does not fall through: a caller that
asked about a specific product is not served by the project default, so a name
the recipe does not declare is an error rather than a silent substitution. That
is the point an ad-hoc question occupies, where there is no content location to
refine the answer with.

The result is resolved already, so a caller applies it without knowing where it
came from:

```go
type ResolvedGovernance struct {
    Channel    string
    Voice      *VoiceBinding      // the matched profile's, else defaults.voice
    TermStore  string             // a standalone terms store bound by the profile
    VoiceField string             // the recipe key Voice came from, for error messages
    Profile    string             // the directory under .kapi/profiles/ to look in
    Validity   *graph.Validity
    Fallback   *GovernanceFallback
}
```

`VoiceField` names the recipe line to fix when a profile cannot be loaded: the
difference between "your project has a broken voice" and "line 14 of your
recipe".

### Governance expires

A profile bounds its governance in time with `valid_from` / `valid_to`, authored
as a bare date or an RFC3339 instant:

```yaml
profiles:
  northsea-2025:
    channels: [landing]
    voice: .kapi/profiles/northsea-2025/voice.yaml
    valid_from: 2025-09-01
    valid_to: 2026-03-01
```

The window is the **same half-open model** terms and graph edges carry
(`valid_from` inclusive, `valid_to` exclusive), parsed into the shared
`graph.Validity` vocabulary, so a profile's "from when until when" reads and
matches identically to a term's. A bare date is read as midnight UTC, which makes
a `valid_to` date exclude that whole day, the reading "until" invites.

At resolution, a binding whose profile is outside its window at `pt.At` is
skipped **exactly as if the recipe did not declare it**, and the ladder continues
to the next rung. The skip is not silent: the result carries a
`GovernanceFallback` naming the profile that stopped governing, which boundary
excluded the instant, and what governs in its place. `kapi run` and `kapi check`
print it (`host/governance.go`) and the by-location retrieval answer carries it
as a note (`host/contextpoint.go`). Governance changing on a date has to be
visible, because a governance change nobody is told about is indistinguishable
from a bug.

Two views of the same recipe follow:

| View | Call | Windows |
| --- | --- | --- |
| **As declared** | `ResolveGovernance(collection)`; `At` is the zero value | carried on the result, not applied |
| **As of** | `ResolveGovernanceAt(collection, at)`, or `ResolveGovernanceFor` with a non-zero `At` | applied; an out-of-window binding does not govern |

A surface reporting what the recipe *says* wants the first. Every run takes the
second. `ProfileWindows()` lists the profiles that bound anything, for surfaces
that report which governance is in force and until when.

### The resolved point is what travels

`ResolvedGovernance.Ref()` renders the resolution back as the point it names, so
a caller carrying coordinates (a push, a graph write) carries what actually
governed rather than what the recipe declared. A resolution that fell through to
the default point renders as the zero ref, which puts no structural coordinates
on the wire. The declared axes still travel: the entry a push carries for a
collection is the merged point, so a collection binding no channel in a project
that declares a brand sits at that brand.

That is what makes governance and retrieval agree by construction rather than by
convention: the coordinate a collection is `governed_by` in the graph, the
coordinates on the wire, and the answer `kapi context <path>` gives are the same
resolution.

### Validation happens at load

Every collection and every item is resolved once when the recipe loads, so a
project that loaded cleanly has no channel reference left to fail on later. The
errors are written to be fixed without reading this page: an unqualified channel
lists the qualified forms that would work, a channel naming an undeclared profile
lists the declared profiles, and a channel a profile does not declare lists the
channels it does.

### The point is edited through the one write verb

A `kapi apply` change-set with `kind: "recipe"` sets one recipe field per
entry, and the coordinate surface is part of what it may set:
`defaults.coordinates.<axis>` declares or withdraws one axis of the default point
(an empty value withdraws it, so the operation stays total), and
`collections.<name>.channel` places a named collection. Each goes through
`core/project.SetField`, which refuses `product` and `channel` under
`coordinates:` and preserves the recipe's formatting. A collection is addressed
by name because a name is what a decision is about; an index moves when an
unrelated entry is added above it.

### One run, one resolution per collection

The tool chain is assembled before any content is read and bakes the resolved
profile into the steps that need it, so a run does not switch voice per file: it
resolves governance per collection and executes once per distinct resolution. A
recipe where no collection binds a channel runs unsplit, exactly as one that has
never heard of the context space.

A gate is the exception: `kapi check` reads rather than writes, so it resolves
per file and holds each one to the voice **and the vocabulary** in force where
it sits. Both halves resolve through the same point, so a profile that binds its
own `termstore:` governs exactly the files its channels carry, which is how a
surface keeps a name the vocabulary retired.

### Not yet built: a point beneath the file

A content item's own `channel:` is the finest declared point, which means the
finest governed unit is a file, for voice and for vocabulary alike. The case
that remains open is a passage (*the retired name is permitted in these two
paragraphs of the migration guide*), which needs a point beneath the file that
nothing declares yet.

## Consequences

- A repository holding several products carries several profiles, and that is
  what tells their content apart.
- Adding a channel to a product is one line in the profile; nothing else has to
  learn about it.
- A project states its brand once and every collection inherits it; a
  collection that sits elsewhere moves on that one axis.
- Governance with an end date is expressible, and the day it ends is announced by
  every surface that resolves it rather than discovered by a reader.
- Because the structural axes are resolved from the recipe rather than declared
  beside it, a collection cannot claim a point that does not exist.

## See also

- [C-01: The project model](c-01-project-model.md): the recipe and the layout
  these bindings live in.
- [C-03: The context store and graph](c-03-context-store-and-graph.md):
  coordinate nodes and the `governed_by` edge.
- [C-06: Context retrieval](c-06-retrieval.md): the by-location answer, which is
  this resolution rendered.
- [C-07: Voice profiles](c-07-voice-profiles.md): what a profile binds, and how
  a channel refines it.
- [kapi.yaml project file](../../implementation/context/kapi-project-file.md): the
  schema reference.
