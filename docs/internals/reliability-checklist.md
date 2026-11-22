# Reliability checklist

The reliability checklist lists checks that past incidents showed to be
necessary, each selected by what a diff touches. Before you open or merge a
change, run it and act on what it prints. You do not need to remember the
outage behind each item: the item carries it.

The items live in [reliability-checklist.yaml](reliability-checklist.yaml).
Each one has:

- a **trigger** that can be read from a diff: changed paths, added lines, or
  both;
- a **check** to perform, with the exact command where one exists;
- the **incident** behind it, with pull request numbers and dates;
- an **owner surface** (framework, cli, server, stores, desktop, web, mobile,
  formats, ci, release or docs).

The checklist covers what needs judgement. A lesson a guard enforces belongs
in that guard (`make lint`, `scripts/pre-push-check.sh` or a CI job that runs
on pull requests), not here.

## Running it

```bash
make reliability-checklist                        # your branch and working tree against origin/main
make reliability-checklist BASE=<ref>             # against another base
make reliability-checklist BASE=<sha>^ HEAD=<sha> # one merged commit
scripts/reliability-checklist.py --all            # every item
scripts/reliability-checklist.py --ids            # matching ids only
```

Without `HEAD`, the diff runs from the merge base of `BASE` (default
`origin/main`) and `HEAD` to the working tree, untracked files included. With
`HEAD`, it is `git diff BASE...HEAD`.

For each matching item the script prints its id, title, the path or added line
that matched, the check and the incident. Run each check, or state in the pull
request why it does not apply; an item's `note` says when a match is a false
alarm. A typical pull request selects 3 to 10 items, and a docs-only one
selects none.

The script exits 0 whenever it prints a selection: it informs and never gates.
It exits 2 when the checklist is malformed, 1 when the self-test fails and 3
when git cannot produce the diff. `make lint` runs
`make check-reliability-checklist`, the self-test: the file parses, every item
validates, and each trigger selects its fixture.

## Trigger fields

| Field | Meaning |
| --- | --- |
| `paths` | Globs. A changed file that matches selects the item. |
| `added` | Regular expressions (Python `re`) searched in each added line. |
| `added_in` | Globs. When present, `added` reads only lines of matching files. |
| `exclude` | Globs. Files never considered, for paths or added lines. |
| `note` | When a match is a false alarm. |

In a glob, `**` spans directories, `*` and `?` stay inside one segment, and
`{a,b}` is a choice. Quote every glob and regular expression in single quotes;
`''` is a literal quote.

## Adding an item

An item earns a place when the same failure happened twice, or happened once
and cost a day or more, and its trigger can be read from a diff. A person
approves each new item.

1. Add the item to the YAML file under its surface, with the trigger fields
   above.
2. Add a fixture for it to `FIXTURES` in `scripts/reliability-checklist.py`;
   the self-test fails for an item that no fixture selects.
3. Run the script over recent merged pull requests with
   `--base <sha>^ --head <sha>`, and over the diff that introduced the
   incident. Narrow any trigger that fires on most pull requests.

## Retiring an item

Delete an item when a guard now enforces it (name the guard in the commit), or
when it has not led to a real finding in six months. Remove its fixture in the
same commit.
