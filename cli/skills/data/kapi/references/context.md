# Ask what applies here

kapi holds this project's content context in a form you can read: the voice in
force at a location, the terms bound there, and wording the project has already
approved. Communication is contextual. A legal notice is not a help article, and
the same fact is written differently in a changelog, a migration guide and a
support reply. Retrieve first and write second; a check that fails afterwards is
the expensive way to learn the same fact.

## Two questions, never a store

Which store holds the answer is not something you should have to know.

```bash
kapi context docs/guide.md          # what applies HERE, at this location
kapi context --profile marketing    # the same, for a profile, with no file
kapi context search widget          # what do we know about THIS word
kapi context search "sign in" --json
```

`kapi context <path>` answers for the place a file sits: the point it resolved
to, the voice in force with its full guidance, the terms bound there, the
candidates nobody has decided on, and the governance windows around them. Read
that one document before you touch the file.

`kapi context search` answers for a word or a phrase, across every store the
project binds. The usage count on each term is as of the last `kapi up`: a term
you just added shows no uses until the next run.

Narrow a by-location answer with `--locale` (one language) and `--limit` (how
many terms to render; the answer reports the total either way).

Over MCP the same two are `context://<project-relative-path>`, a resource you
**read** rather than a tool you call, and the `context_search` tool.
`context://profile/<name>` is the by-name form, and `?format=json` gives the
structured shape instead of markdown.

## Every call names its project

Every project-scoped MCP tool takes an optional `project`, and the resource
takes `?project=<path>`. Pass it whenever you work outside the project the
server started in: the value is that project's `kapi.yaml`, its root directory,
or any path inside it, so the file you are editing will do. Omit it and the call
acts on the project the server started in. A path that holds no project is
refused, and names the path back to you.

## What an answer says about itself

Both answers carry `coverage` and `provenance`, and both say plainly when a
store was unreachable rather than returning a confident empty result.

- `coverage` is `empty`, `thin` or `covered`. An **empty** answer means the
  project has recorded nothing at that point, and its notes say what is worth
  noticing while you work. That is a fact about the project, not an absence of
  an answer: it is the moment to record what you notice
  ([growing-context.md](growing-context.md)).
- `provenance` names the project that answered, the workspace revision it was
  read at, and whether the content kapi holds still matches the files on disk.
- `scope` says how much could have been read, so you can tell "this project
  holds no answer" from "nothing that could hold one was consulted".

## Candidates are not rules

An answer lists `candidates` apart from the voice and the terms in force. Each
is a rule or a fact somebody proposed and nobody has decided on, carrying the
evidence behind it, who recorded it and in which session.

A check reports every candidate and no check fails on one. Build on them, and
report none of them to the user as a rule in force. `kapi context confirm <id>`
is a person's decision, and the id is on the candidate.

## A retrieved answer goes stale

The project's context moves while you work: a colleague approves a decision, a
`kapi up` run brings one down. Two surfaces tell you, and both stop at telling
you:

```bash
kapi status                       # the governance line: in sync, or what moved
```

A `context_search` answer carries a note when the context, the terms or the
decisions moved since you last read them here. **Read the notes, and when one
says the context moved, ask again before you continue.** The wording you had
settled on was chosen against a context that has since changed, and everything
you write from here on inherits the stale answer.

Neither surface resolves anything. What a moved context means for work already
done is a judgement: re-read, reconsider what you have written, and say what
changed rather than silently rewriting.

`kapi check` is the enforcing half. Content produced under a context that has
since been superseded fails the staleness gate, naming what moved, and
re-running `kapi up` reproduces it under the context now in force.
