# Verify a change to the kapi CLI or the engine under it

For a diff under `cli/`, `host/`, `kapi/`, `core/` (not `core/formats/`),
`memory/`, `terms/` or `providers/`. Build the checkout's own `bin/kapi` and
drive the changed command on the sample project.

## Build

```bash
make build                     # bin/kapi from this checkout: 18 s cold, 9 s warm
```

`make` finds ICU on macOS. When you call `go build` or `go test` yourself and
pkg-config cannot find `icu-uc`, run
`export PKG_CONFIG_PATH="$(brew --prefix icu4c)/lib/pkgconfig"` first.

## The sandbox

The `kapi` on PATH is a Homebrew release, not your change. A bare `bin/kapi`
inside the repo binds to the dogfood `kapi.yaml` and to your real config and
workspace. Use the sandbox script, from the checkout root:

```bash
S=.claude/skills/verify/scripts/kapi-sandbox.sh
$S init                              # fresh sandbox, a copy of samples/compass
$S run store import ./context -p .   # read the sample's terms, voice and memory in
$S run status -p . --jq .project     # "compass": the sandbox, not the repo
```

`run` refuses a missing `bin/kapi`, or one older than any Go source, and says
how to rebuild. It sets `KAPI_NO_PROJECT=1`, so a project command needs `-p .`.
It drops every `KAPI_*` variable you exported, moves config, data, cache,
plugins and `HOME` into the sandbox, and turns telemetry off. It also clears
the agent host markers, so kapi treats the run as a person's; set
`KAPI_SANDBOX_AS_AGENT=1` to verify what kapi refuses an agent. It passes
kapi's exit code through.

Three shell traps: `PIPESTATUS` is empty in zsh, so use `set -o pipefail`
before a pipe. Use `/bin/cp` and `/bin/mv`, because the interactive aliases
prompt and do nothing. Give files outside the sandbox project as absolute paths.

## Drive the command families

Run the family your diff touches, and one neighbour it should leave alone.
Each command takes under a second on the sample. For a read-only command
(`ls`, `look`, `context`, `inspect`, `status`), run it with `-p .` and read
the output, as with `status` above.

**`kapi up`, twice.** The second run must change no file:

```bash
$S run up -p .                                 # "materialized 2 target file(s)"
SB=$($S path); rm -rf "$SB/before"; /bin/cp -R "$SB/project" "$SB/before"
$S run up -p .
diff -r -x work "$SB/before" "$SB/project" && echo unchanged
```

Both runs exit 0 and end `Not yet up to date`, because the sample's Dutch waits
for review. Target-language drift never fails a run.

**`kapi check`**, a pass and then a failure:

```bash
$S run check -p . site/locales/en-GB.json; echo "exit=$?"   # PASS, exit=0
P=$($S path)/project
sed -i.orig 's/No vessel is alongside\./No vessel is seamless./' "$P/site/locales/en-GB.json"
$S run check -p . site/locales/en-GB.json; echo "exit=$?"   # FAILS terms.vocabulary, exit=3
/bin/mv -f "$P/site/locales/en-GB.json.orig" "$P/site/locales/en-GB.json"
```

**A store command:**

```bash
$S run terms stats -p .                                # concepts and terms by locale
$S run terms lookup alert -s en-GB -t de -p .          # alert -> Warnung
$S run memory lookup "Berth plan" -s en-GB -t nb -p .  # Kaiplan, exact
```

Pass `-s en-GB`: the lookups default to `en` and find nothing in this project.

## The edges of a changed command

```bash
$S run <cmd> --help            # exit 0, the help you wrote
$S run <cmd> --bogus           # "Error: unknown flag: --bogus", exit 1
$S run <cmd> -p /nonexistent   # an error naming the path, exit 1
```

Add the failure your change introduces or guards, with its exit code. Where the
command takes `--json` (or `--jq`, which implies it), check that a new field
appears and that field names are snake_case:

```bash
$S run status -p . --json
$S run check -p . site/locales/en-GB.json --jq '.findings[] | {rule, fails, message}'
```

## Model providers and the network

Nothing above needs a key or the network. The sample's `converge` flow drafts
with `provider: demo`, an offline stub that marks its output `⟦nl⟧`.
`kapi translate` defaults to a real provider, so pass `--provider demo`:

```bash
$S run translate site/locales/en-GB.json --target-lang nl --provider demo -o "$($S path)/out"
```

Without it the sandbox has no credentials and translate exits 1 with
`no saved credentials found`. A change to a real provider adapter needs a key
and a live call, which the sandbox cannot make. Say so in the report and rely
on the adapter's package tests.

## The e2e suite

`make test-e2e-kapi` builds its own binary and runs `kapi/e2e` in its own
isolated home, in 27 s. Pull request CI runs it only when the diff touches
`cli/`, `host/` or `kapi/`. Run it yourself for a change under `core/`,
`memory/`, `terms/` or `providers/`, and before you push a change to a
command's flags, output, help or exit codes. Read its exit code.

## A second `kapi up` changes nothing

When the change reaches `kapi up`, or anything it writes:

```bash
go test -tags "fts5,e2e" ./kapi/e2e/ -run '^TestUpTwiceIsANoOp$' -count=1 -v   # 15 s
```

The suite sits behind the `e2e` build tag: without it Go reports that build
constraints exclude every file, which is not a result.

The test runs `kapi up` on a sample with the offline `demo` provider, requires
translations in every target, runs it again, and compares every file in the
project and in the data root. Pull request CI runs it only when `cli/`,
`host/` or `kapi/` changed.

## Evidence

Record each command, the output lines that show the behaviour, and the exit
code, for example the `FAILS terms.vocabulary` line that names the forbidden
term, and `exit=3`. "It works" is not evidence. Finish with `$S clean`.
