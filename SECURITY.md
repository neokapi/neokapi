# Security Policy

## Supported versions

Security fixes are provided for the latest released `1.0.x` line. Older
pre-1.0 releases are not maintained.

| Version | Supported |
| ------- | --------- |
| 1.0.x   | Yes       |
| < 1.0   | No        |

## Reporting a vulnerability

Please do not open a public issue for security vulnerabilities.

Report privately through GitHub's coordinated disclosure: open the repository's
**Security** tab and choose **Report a vulnerability** (GitHub private
vulnerability reporting). Include a description, affected versions, and a
reproduction where possible.

You can expect an initial acknowledgement within a few business days. Once a
fix is available we will coordinate a disclosure timeline with you and credit
the report unless you prefer to remain anonymous.

## Trust model

kapi reads files, and some of those files can ask kapi to do things. This
section states which inputs are treated as instructions and which are treated
as data, so that a report can be judged against a stated contract rather than
against an assumption.

The short form: **content is data, configuration is instruction, and anything
that runs code needs a person to have said yes.**

### The person running kapi is the trust root

kapi acts with the privileges and the environment of whoever invokes it, and it
does not attempt to sandbox itself from that user. Anything that user could do
directly, kapi may do on their behalf. What the model governs is whether a
*file* can make that happen without the user choosing it.

Arguments typed on the command line are the user's own intent. `kapi exec
external-command --command …` runs what it was told to run, and that is not a
vulnerability; the question is only ever whether a file obtained the same
effect unasked.

### Content is data

Documents kapi parses — every input format, and the block store built from them
— are untrusted data. A parser is expected to reject malformed input rather
than misbehave on it, and no document may cause code execution, escape the
paths it was asked to write, or alter the configuration that governs its own
processing. Parser crashes, resource exhaustion on hostile input, and path
traversal on write-back are all in scope.

Content is also allowed to be *hostile prose*. Text extracted from a document
is passed to language models as data, and a document that attempts to redirect
a model is a known and expected class of input rather than a vulnerability in
kapi. Guarding a specific model's behaviour is a product concern; guarding
kapi's own actions is what this section covers.

### Recipes are configuration, and configuration is instruction

A `kapi.yaml` recipe is discovered by a git-style upward walk from the working
directory, so entering a directory is enough to bind a project. A recipe
therefore has to be treated the way a build file is treated: it declares what
kapi should do, and most of what it declares is harmless — languages, content
globs, gates, per-tool settings.

Two built-in tools are not harmless, because running code the recipe names is
their entire purpose: `external-command` spawns a subprocess of its choosing,
and `script` evaluates recipe-supplied JavaScript. A recipe naming either is
gated behind a decision a person takes once, per project, and which is
remembered only for as long as what it approved stays the same:

- kapi shows what would run and asks, on a terminal, before the project is used.
- The answer is stored under the kapi config directory — not in the project's
  own `.kapi/`, which is disposable and would carry the answer to the next
  person — and it is keyed to a fingerprint of the commands approved. Editing
  the recipe to run something else asks again.
- With no terminal attached, kapi refuses rather than assuming consent. The
  general-purpose `--yes` flag does not grant this: unattended pipelines
  already pass it for other prompts, so it is not evidence that anyone
  considered execution. `KAPI_TRUST_EXEC=1` is the deliberate, separately named
  opt-in for automation whose operator has made that judgement.

Surfaces where no person is present to ask do not ask. A recipe carried inside
a `.kpz` package has these steps stripped on ingest, and the engine's gRPC API
and the MCP agent surface refuse them outright — including under the MCP
switch that otherwise exposes every tool, because "show me everything" and "run
arbitrary commands" are different requests.

**Recipes are not secret storage.** Provider API keys belong in the OS keychain
or in the conventional per-provider environment variable, never in a committed
recipe, and the endpoint a provider is called at is host configuration rather
than a recipe field — so that a recipe cannot redirect where a key is sent.

### Packages carry a project, and are treated as foreign

A `.kpz` is a whole project in a file, including its recipe, which is precisely
why the recipe cannot be trusted on the way back in. On ingest kapi strips the
exec-class steps and the per-tool configuration that would arm them, along with
side-effecting extension blocks and any output layout naming a destination
outside the directory the package is merged into. Paths inside a package may
only name locations within that project. Sanitising happens on read regardless
of what the writer claims to have done.

### Plugins are code, and are verified before they are code

Plugins are separate binaries kapi launches as subprocesses; installing one is
installing software. Distribution is the trust boundary: plugins are resolved
through a registry index and their artefacts are signature-verified before use.
A recipe may *declare* that it needs a plugin, and kapi will offer to install
it, but the declaration is a request the user confirms, not an instruction that
executes.

Once installed, a plugin runs with the user's privileges. kapi does not
sandbox plugin subprocesses; it limits what they are handed.

### Out of scope

- A user running commands they typed, or a project they wrote and approved.
- Compromise of the machine, its keychain, or the user account kapi runs as.
- Behaviour of third-party services and models kapi calls, including a model's
  response to hostile text in a document.
- Vulnerabilities in a plugin's own code, which belong to that plugin's
  project. Report those there; report the *host* mishandling a plugin here.
