# Check what you changed

`kapi check` holds content to the context in force at its own location: the
voice profile bound there, the terms bound there, and the rule-based checks the
project declares. Run it on what you changed, read the report, repair, run it
again.

```bash
kapi check content/en/page.json --json
```

Over MCP the same two questions are `check_file` (a file on disk) and
`check_text` (a draft, with `context_path` naming the destination it is written
for, so the draft is checked as that destination's content).

## Check the change, not the project

After editing files in a repository:

```bash
kapi check --diff-against HEAD --json
```

Only the content blocks the change touched are checked, each block whole.
`scope.files` lists every changed file and what became of it, and a finding's
`location.lines` gives the lines of its block.

Three more ways to name the change:

```bash
kapi check --diff-file patch.diff --json   # a diff you already hold; `-` reads stdin
kapi check --staged --json                 # exactly what the next commit records
kapi check --diff-range origin/main...HEAD --json
```

`--staged` reads each file from the index, so unstaged edits and untracked files
do not count. `--diff-range A...B` reads each file from B and nothing from the
working tree, and checks the change B made since its merge base with A. Finding
lines are the lines of the version checked.

## Read the report

Read `execution.contexts` to confirm the effective voice selection, profile and
channel, then the findings and `execution.analyzers`. Fix the relevant findings
within the requested scope and re-check.

Unsupported semantic guidance still needs review against the retrieved context:
a passing score covers only the checks that ran. If the same finding persists or
contradicts the governing guidance, report the unresolved issue rather than
rewriting unrelated text around it.

A rule can be scoped, so the same word can be permitted in one place and retired
in another: the old name may be allowed in the migration guide and nowhere else.
When a check flags something that reads as correct in context, surface it to the
user instead of silently rewriting.

## Say what you checked against

`evaluation` names the state the run was evaluated against. Quote it when you
report a result, so the verdict can be read again later:
`evaluation.context.project` and `evaluation.context.revision` (the workspace
position the terms, voice rules and decisions were read at),
`evaluation.tool.version`, and `evaluation.analyzers`, which says which
analyzers covered the content and which did not.

`evaluation.context.stale` means the blocks kapi holds were read from files that
have since changed, so anything counted over content is out of date. Run
`kapi up` to read the files again, then check again. Report the finding as
measured against files as they were if you cannot.

A missing `evaluation.context` means the check ran on files outside any project:
no voice profile, terms or recorded decisions applied. Run the check from inside
the project, or with `-p`, before treating a pass as governed.

## Exit 4 is never a pass

Exit 4 means the check did not run. Read `did_not_run_cause` before you
continue:

- `checker_invalid`: a kapi checker is broken, and nothing the run reported can
  be trusted. Stop and report it.
- `nothing_to_check`: the change touched no content.
- `content_not_checked`: content was left unchecked, and `did_not_run` names it.

## Warnings

The report's `warnings` list names configuration problems. A warning never
changes the verdict, the score or the exit code, and it clears only when the
configuration changes.

- `voice.unknown_key`: an unknown key in a voice profile, with the file in
  `source` and the key's dotted path in `key`.
- `format.no_reader`: a declared file, in `source`, that the check did not read
  because the plugin supplying its format is not installed. Nothing in that file
  was checked. Install the plugin the message names before relying on the result
  for it.

Fix the named configuration when it is in scope, or report it.

## Release gates

```bash
kapi check --ship --json
```

The ship gate adds the project's release policy: the voice score bar
(`--min-score`, default 80) and, where the recipe declares one, the
translation-coverage bar in `ship_gate:`. See [project.md](project.md) and
[translate.md](translate.md).

The optional Claude Code Stop hook runs these project gates when installed.
