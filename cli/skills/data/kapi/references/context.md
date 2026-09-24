# Ask what applies here

Retrieve the project's content context before writing: the voice profile and
terms that apply to the file, together with previously approved wording.
Guidance can vary by location, so retrieve it for the file you plan to edit.

## Retrieve context by location or search

These commands resolve the project's configured stores for you.

```bash
kapi context docs/guide.md          # what applies HERE, at this location
kapi context --profile marketing    # the same, for a profile, with no file
kapi context search widget          # what do we know about THIS word
kapi context search "sign in" --json
```

`kapi context <path>` returns the file's resolved point, full voice guidance,
applicable terms, pending suggestions and governance windows. Read the result
before editing the file.

`kapi context search` searches for a word or phrase across the project's
configured stores. The usage count on each term is as of the last `kapi up`: a term
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

## Coverage and provenance

Both results include `coverage` and `provenance`. They report unreachable
stores so you can distinguish missing context from a failed lookup.

- `coverage` is `empty`, `thin` or `covered`. An **empty** result means no
  context is recorded at that point. Its notes suggest observations you can
  record while working; see [growing-context.md](growing-context.md).
- `provenance` names the project that answered, the workspace revision it was
  read at, and whether the content kapi holds still matches the files on disk.
- `scope` says how much could have been read, so you can tell "this project
  holds no answer" from "nothing that could hold one was consulted".

## Suggestions are not rules

Results list pending `suggestions` separately from established voice guidance
and terms. Each includes evidence, its author and the session that recorded it.
A `contested` suggestion conflicts with the rule named by `contested_by`.

Checks report suggestions without failing on them. Describe them as pending
suggestions until a person establishes them with `kapi context keep <id>`.

## A retrieved answer goes stale

Context can change during a task, for example when `kapi up` retrieves a
colleague's approved decision. Check the governance status:

```bash
kapi status                       # the governance line: in sync, or what moved
```

A `context_search` answer carries a note when the context, the terms or the
decisions moved since you last read them here. **Read the notes, and when one
says the context moved, retrieve it again before continuing.** Review your
existing work against the updated guidance and explain any resulting changes.

`kapi check` is the enforcing half. Content produced under a context that has
since been superseded fails the staleness gate, naming what moved, and
re-running `kapi up` reproduces it under the context now in force.
