---
title: 'Note: CLI conventions'
sidebar_label: CLI conventions
---

# CLI conventions

Every kapi command answers the same four questions the same way: how it takes
input, how it emits output, what its exit code means, and what it does inside a
project. These are contracts enforced by tests
(`cli/conventions_test.go`, `host/inputs_test.go`), not per-command choices — a
new command that breaks one fails the build.

## 1. Input

One resolver, `host.App.ResolveInputs` (`host/inputs.go`), backs every command
that takes file inputs. Three rules:

**Globs expand in-process.** `kapi stats 'src/**'` behaves identically on zsh,
bash and PowerShell, and whether or not the shell expanded the pattern first — a
shell-expanded list of concrete paths is just the resolved form of the same
input. Expansion uses doublestar, so `**` is recursive (which `filepath.Glob`
cannot express) and `{a,b}` alternates. A wildcard never descends into a
dot-directory; naming one (`.github/**/*.yml`) opts back in.

**Directories are content, not errors.** A directory argument expands to the
regular files beneath it, skipping hidden directories and editor junk (`~$…`
Office lock files, `._…` AppleDouble stubs). The toolbox utilities that emulate
a Unix filter (`kgrep`, `kcat`, `ksed`) keep the POSIX `-r` requirement, because
they are deliberate emulations of tools whose muscle memory includes it.

**No input never means "block on the terminal".** Given nothing, a content verb
uses the project's tracked content set when a recipe is in scope; otherwise it
reads standard input only when stdin is actually redirected. On an interactive
terminal it reports how to give it input and exits `2`. Standard input is always
available explicitly as `-`.

## 2. Output

One axis, `--output-format`, with the values `text` (alias `table`), `json` and
`yaml`; `--json` and `--text` are shorthands, and `--jq <expr>` implies JSON.
Resolution lives in `host/output.ResolveFormat`, and every structured result
type implements `FormatText` for the human rendering. `text` is a table for
listings and a labeled summary for single records.

`-o` is **not** the format flag. Across the tree `-o` means an output *path* or
path template (`kapi extract -o work.kpz`, `kapi tm export -o tm.tmx`, the
`{dir}{name}{ext}{lang}` template on `kapi exec <tool>`), and that meaning is
consistent, so the format axis keeps the unambiguous long spelling.

Progress for anything that can exceed a second goes to **stderr**
(`host/progressstep.go`), so stdout stays exactly the machine-readable result:
`kapi stats --output-format json 'src/**' > out.json` writes only JSON. It stays
silent below `ProgressDelay`, is suppressed by `--quiet`, and degrades to plain
append-only lines on a pipe.

## 3. Exit codes

`host/exitcode.go` defines the whole vocabulary; no command invents another.

| Code | Meaning |
| --- | --- |
| 0 | success |
| 1 | operational error |
| 2 | usage error — bad flags, unreadable input, no input |
| 3 | quality gate unmet (`ErrQualityGate`) |
| 130 | interrupted (SIGINT) |

The grep-family utilities additionally use `1` for "no match", which is their
namesakes' contract.

## 4. Project vs ad-hoc

`ResolveProjectPath` is the single discovery path: `-p/--project` flag →
`KAPI_NO_PROJECT` opt-out → `KAPI_PROJECT` env → git-style upward walk for
`kapi.yaml`. A command is one of:

- **project-required** (`up`, `status`, `ls`, `add`, `rm`, `extract`, `merge`,
  `check --ship`) — errors without a recipe;
- **project-preferred** (`check`, `stats`, `inspect`, `translate`,
  `pseudo-translate`, `run`) — uses the project's content when given nothing,
  works ad-hoc on named files;
- **ad-hoc** (`formats`, `tools`, `flows`, `plugin`, `models`, `config`
  subcommands, the toolbox) — no project involved.

## Command surface

| Command | Input | Glob / dir | Format axis | Project | Non-zero exits |
| --- | --- | --- | --- | --- | --- |
| `up` | — | — | text·json·yaml | required | 1 |
| `status` | — | — | text·json·yaml | required | — (always 0) |
| `check [files…]` | positional | yes | text·json·yaml | preferred | 3 gate, 1 op |
| `check --ship` | positional | yes | text·json·yaml | required | 3 gate |
| `stats [files…]` | positional, stdin | yes | text·json·yaml | preferred | 2 per-file |
| `inspect [files…]` | positional, stdin | yes | text·json·yaml (+`--jsonl` stream) | preferred | 2 per-file |
| `translate [files…]` | positional, `-i` | yes | text·json·yaml | preferred | 1 |
| `pseudo-translate [files…]` | positional, `-i` | yes | text·json·yaml | preferred | 1 |
| `run [flow]` | `-i` | yes | text·json·yaml | preferred | 1 |
| `extract` | project content, or positional + `-o <kpz>` | yes | text·json·yaml | required | 1 |
| `merge` | `-i` (file, glob, dir) | yes | text·json·yaml | required | 1 |
| `apply [changeset]` | positional or stdin | — | text·json·yaml | preferred | 3 drift |
| `pack` / `unpack` / `info` | positional archive | — | text·json·yaml | preferred / — | 1 |
| `add` / `ls` / `rm` | positional patterns | yes | text·json·yaml | required | 1 |
| `exec <tool> <files…>` | positional | yes | text·json·yaml | tm/terms bound from project | 1 |
| `kcat` / `kgrep` / `ksed` / `kconv` | positional, stdin | globs yes, dirs need `-r` | text·json·yaml | — | 2; kgrep 1 = no match |
| `kdiff a [b]` | 1–2 positional | — | text·json·yaml | — | 1 differ |
| `flows` / `tools` / `formats` | — | — | text·json·yaml | — | 1 |
| `plugin …` | positional name | — | text·json·yaml | — | 1; doctor 1 unhealthy |
| `models …` | positional model | — | text·json·yaml | — | 1 |
| `tm …` / `termbase …` | positional / `--name`,`--local`,`--file` | `import-dir` walks | text·json·yaml | resource flags | 1 |
| `brand …` | positional / `--input-text` / stdin | — | text·json·yaml | profile flags | 3 min-score |
| `credentials …` | positional name | — | text·json·yaml | — | 1 |
| `config …` | positional key/value | — | text·json·yaml | positional form: required | 1 |
| `version` / `update` / `telemetry` / `completion` | — | — | text·json·yaml | — | 1 |
| `mcp` / `engine serve` | protocol streams | — | protocol | preferred | 1 |

### Deliberate exceptions

Three departures are intentional and documented rather than fixed:

- **`kgrep`/`kcat`/`ksed` require `-r` for directories.** They are explicit
  emulations of POSIX tools; matching their contract is the point.
- **`kgrep` exits 1 on no-match.** Same reason.
- **`kapi status` always exits 0.** Target-language drift is pending work, not
  failure, so the reporting command never fails on it. `kapi check --ship` is
  the explicit enforcement point — see
  [the convergence model](/kapi/convergence).

## One config verb

`kapi config` is the only configuration command. A plugin does not ship its own:
it declares a key namespace in its manifest
(`capabilities.config_namespaces`), and kapi routes `kapi config set
bowrain.server.url …` to that plugin's own global config file with the prefix
stripped — the same file and key the plugin already reads. `kapi config list`
spans kapi's own keys and every installed plugin's namespace.

`KAPI_CONFIG_DIR` contains plugin config files too
(`$KAPI_CONFIG_DIR/<plugin>/<plugin>.yaml`), so the in-repo isolation contract
covers namespaced writes.
