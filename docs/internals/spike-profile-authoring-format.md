# SPIKE: the voice profile authoring format

**This is a spike, not a proposal to migrate.** It exists to answer one question
with running code: should a voice profile be authored as YAML, as it is today,
or as markdown with frontmatter in the shape of an agent skill file?

Nothing here is wired into a command. The code lives in `core/profile/mdspike/`
and is imported by nothing outside its own tests. Both committed profiles still
load through `profile.LoadProfileYAML` and still pass `kapi voice validate`.

Branch: `spike/profile-authoring-format`.

## Recommendation

**Adopt with changes, and the changes are not the format.**

Split into three decisions, because they were bundled in the question and they
have very different costs:

| Decision | Verdict | Why |
| --- | --- | --- |
| Composition (`extends:`) | **Adopt now, in the existing YAML loader** | Highest value, lowest cost, no format change. The duplication is already drifting. |
| Vocabulary from the terms store | **Adopt for the ban list only, opt-in per profile, as a governance decision** | It changes whether content ships. Sourcing the *preferred* list is a mistake. |
| Markdown with frontmatter | **Do not adopt yet. Keep the spike.** | It costs a writer, and the write path is where the difficulty is. The read side is not the problem. |

The direct answer to the format question: **YAML is the right shape for the
rules and markdown is the right shape for the prose, and today's file is mostly
rules.** The prose trapped in folded scalars is real: it is what a
non-engineer reviews and it diffs as a whole block rather than per line. That
is a review-ergonomics complaint, and it does not on its own justify a
second decoder, a second writer, and a second thing `kapi voice new` can emit.

The two defects that actually bite are composition and duplication. Neither
needs markdown. `extends:` and `from_terms:` are YAML keys; the spike bundled
them with the format because that is how the question was posed, but they are
separable, and separating them is the recommendation.

## What was built

`core/profile/mdspike/` holds a reader producing the same `*profile.VoiceProfile`
the YAML loader produces, so `RenderVoiceGuide`, `MatchVocabulary`,
`CalculateScore` and `ValidateProfile` all work on it unchanged.

The document splits along the line the profile already draws:

- **Frontmatter** carries what governs pass/fail: tone enums, style booleans,
  prohibited patterns with their severities, vocabulary rules, locale enums.
- **Body** carries the prose: description, `## Guidelines`, `## Example: <category>`
  sections with labelled before/after paragraphs, `## Locale: <id>` cultural notes.

The split is mechanical, not conventional. The frontmatter struct has no
`guidelines` and no `cultural_notes` field and is decoded with `KnownFields`,
so prose in the frontmatter fails to load rather than quietly bypassing review
of the body. An unrecognized `##` heading is also an error. The standing
objection to markdown as a config format is that prose degrades silently, and
this form does not let it.

Worked example in `core/profile/mdspike/testdata/`:

- `house.md`: the four prohibitions and three bans that both committed profiles
  carry verbatim today, declared once.
- `bowrain-voice.md`: `.kapi/profiles/bowrain/voice.yaml` re-expressed, inheriting the
  house rules instead of copying them.
- `bowrain-voice-from-terms.md`: the same profile with vocabulary sourced from
  `.kapi/terms.json` instead of restated.

## What the spike proves

Each claim below is a test in `core/profile/mdspike/`, not an assertion.

**The markdown form loses nothing.** `TestMarkdownFormEqualsYAMLForm` asserts
deep equality of the whole `VoiceProfile` struct against the one
`.kapi/profiles/bowrain/voice.yaml` decodes to, rather than a chosen subset. The
paragraph normalization (hard-wrapped lines joined with single spaces) is what
makes a markdown paragraph and a YAML `>-` scalar compare equal. The assertion
was verified to fail on a one-word change to the body.

**Nothing reaching the model changes.** `TestRenderedGuideIsByteIdentical`:
`RenderVoiceGuide` and `RenderVoiceGuideCompact` produce byte-identical output
from both forms. Those four consumers (the AI translate prompt, the voice check
tool, `kapi voice guide`, the cloud MCP `get_voice_guide`) see no difference.

**The duplication is real and has already drifted.**
`TestHouseRulesAreDuplicatedToday` reads the two committed profiles and finds
four prohibitions shared verbatim by regex, of which one has diverged in
description:

```
drifted prohibition [\x{1F300}-\x{1FAFF}\x{2600}-\x{27BF}]:
  brand-voice.yaml:   "Emoji in documentation or committed prose"
  bowrain-voice.yaml: "Emoji in committed prose"
```

That is not cosmetic. `patternHints` sends the *description*, not the regex, to
the model. The two profiles already instruct the model differently for the same
prohibition.

**Inheritance removes the duplication without changing the resolved rules.**
`TestInheritanceDeclaresHouseRulesOnce`: the four inherited prohibitions keep
their positions and the child's own rule lands last, matching the committed
YAML's order exactly.

**Deduplicating a restated rule is load-bearing.**
`TestRestatingAnInheritedRuleDoesNotDoubleIt`: a child that repeats an inherited
prohibition to tighten its severity must replace it, not append. Two copies raise
two findings for one violation and double the score penalty. A composition
feature that silently changes whether content ships is exactly what the question
was worried about, and it lives in the merge, not in the format.

**Terms-store sourcing changes check behaviour. Measured, not guessed.**
`TestTermsStoreSourcingChangesCheckBehaviour`:

```
forbidden terms hand-authored (4): [easily magic simply solution]
forbidden terms terms-sourced (8): [easily glossary magic sievepen simply solution termbase translation memory]
```

The four added bans are exactly the retired spellings the project already
forbids in prose. Text that scores 100 under the committed profile scores less
under the sourced one. Every derived ban carries its concept ID and the
concept's preferred term as the replacement, so a finding can pivot back to the
concept, a link that is populated today only when the platform promotes a rule
from a correction.

**Status mapping is something the profile cannot express at all.**
`TestAdmittedTermsAreNotBanned`: `terms` is an *admitted* alternative for the
concept whose preferred form is `terms store`. A ban list derived from status
leaves it alone. A profile has no way to say "acceptable but not preferred".

**Sourcing the preferred list is a mistake.** Two tests, two independent reasons:

- `TestTermsStoreSourcingBloatsTheGuide`: preferred terms go 5 → 18 and the
  rendered guide grows 2596 → 3633 bytes (+39%) for three domains out of ten.
  A profile's preferred list is a short editorial selection; the store is the
  whole governed terminology. Two of the five (`workspace`, `context graph`) have
  no concept at all and still have to be hand-authored.
- `TestTermsStoreDefinitionsAreNotPromptText`: a concept definition is written
  for a translator choosing a word; a profile note is written to be pasted into
  a model prompt. Two dogfood definitions (`leverage`, `pre-translate`) still
  spell the retired abbreviation, and sourcing carries it straight into the
  brand guide the model reads.

**The committed profile is already machine-written.**
`TestCommittedProfileIsMachineRewritten`: `host.applyBrandEntry` does not patch
the file. It loads it, upserts the rule, and writes the whole struct back with
`yaml.Marshal` (`host/apply_assets.go:723`). Struct marshalling emits no
comments, so the header on `.kapi/profiles/bowrain/voice.yaml` (the one recording
that the house rules are duplicated because composition does not exist) does not
survive the first applied rule. This is the finding that decides the
recommendation.

## What stayed theoretical

- **No non-engineer read either form.** The review-ergonomics argument is the
  main case for markdown and it is entirely untested with a real reviewer. That
  is the experiment worth running before spending anything further.
- **One profile was converted, and the same person authored both sides.** A
  conversion done by someone who was not holding the target struct in mind would
  be a different measurement.
- **`channels:` and `personas:` were not exercised.** No committed profile uses
  either. They are keyed override maps carrying nested `*ToneProfile` and
  `*StyleRules`, and the body/frontmatter split for a nested override is
  unanswered.
- **There is no writer, so there is no round-trip.** Everything above is the
  read side.
- **Composition is only two levels deep** (house → brand). House → brand →
  channel was not built.
- **Composition does not survive the store.** `Store.CreateProfile` takes a
  resolved `*VoiceProfile`; `compileBrandProfile` would flatten the inheritance
  on import. The file layer and the store layer would disagree about where a
  rule came from.
- **A child cannot remove an inherited rule.** The merge deliberately forbids it,
  which suits house rules. Nobody has yet needed a locale- or surface-specific
  exemption, so it is untested whether that holds.

## Migration cost if adopted

**Profiles that exist: nine files.** Two committed dogfood profiles
(`.kapi/voice.yaml`, `.kapi/profiles/bowrain/voice.yaml`), five embedded starter
packs (`core/profile/packs/*.yaml`, `go:embed`ed), two retired harness fixtures.
Plus an unbounded number of store rows, which no file-format change reaches.

**Code that would move.** `LoadProfileYAML` has eight non-test call sites
(`core/profile/packs/embed.go`, `host/apply_assets.go` ×2, `host/mcp_voice.go`,
`host/voice.go` ×2, `cli/voice.go` ×2) plus `DecodeProfileStrict` in
`cli/voice.go`. Adding a second decoder in front of the same
`ValidateProfile` is cheap; `TestMarkdownFormValidates` shows the semantic
validator needs no new rules. The expensive pieces:

- `host.writeProfileYAML` (`apply_assets.go:723`) is the write-back. A markdown
  writer must *patch the frontmatter in place*. Marshalling the resolved profile
  back would inline every inherited house rule into the child and silently undo
  the composition, and would flatten the body prose the author wrote.
- `host.VoiceProfileTemplate` is the commented YAML `kapi voice new` emits.
- Two authoring surfaces would then disagree. The Bowrain web editor
  (`bowrain/packages/ui/src/brand/BrandProfileEditor.tsx`, `BrandProfileWizard.tsx`)
  edits store rows through the JSON shape and never sees a file. A file-format
  change reaches neither it nor the profiles it edits.

**Does terms-store sourcing change check behaviour? Yes, and say so plainly.**
Four new prohibitions on the dogfood corpus: `sievepen`, `glossary`, `termbase`,
`translation memory`. At `forbid_severity: minor` each costs 1 point; left unset
they default to major and cost 5, so a paragraph carrying four of them lands
exactly on the default on-brand bar of 80. Prose that ships today would stop
shipping. The direction is the one the project wants, which makes it tempting to
land quietly. It should be landed as a decision with the diff stated, not as a
side effect of a refactoring.

## What I would NOT do

1. **Not migrate the committed profiles or the starter packs.** Nine files moving
   to a format with no writer, for a review benefit nobody has measured.
2. **Not write a markdown writer that marshals the resolved profile.** If markdown
   is ever adopted, `apply` must patch frontmatter in place. Anything else undoes
   inheritance and destroys the body prose, and the current YAML writer already
   destroys the file's comments, so this failure mode has a precedent here.
3. **Not source `preferred_terms` from the terms store.** It triples the list,
   grows the prompt 39%, drops the brand coinages the store has no concept for,
   and injects retired vocabulary into the guide.
4. **Not make terms sourcing implicit.** It must be a declared block in a
   specific profile, reviewed like any other rule change.
5. **Not move a pattern's `description` into the body.** It reads like prose, but
   it is what `patternHints` sends to the model for that prohibition, and it
   belongs next to the regex it explains.
6. **Not let a generator own the frontmatter, though not for the stated reason.**
   See below.

## Where I disagree with the evaluation I was given

**"AI must not generate the structured part" is directionally right, stated too
strongly, and the codebase already has the better version of it.**
`core/ai/tools/brandinfer.go` already has a model generate a complete
`VoiceProfile`, severities included, and it is safe, because the output is a
*draft* carrying `DraftEvidence` (per-field confidence and corpus rationale),
and because `core/profile/promote.go`, `blastradius.go` and
`Store.RecordRuleDecision` make promotion a recorded human decision. The
invariant is not "AI must not generate structure". It is **structure enters
through a recorded decision, never through a regeneration**, because
regeneration is not idempotent-safe and a recorded decision is durable. That invariant is
already implemented, and the authoring format does not touch it either way.

**"The prose parts are what actually reach the model" is not right.**
`RenderVoiceGuideCompact`, the one inlined into the translation system prompt,
sends pattern descriptions and term swaps and bans, which are structured fields.
`RenderVoiceGuide` renders every structured field. The model sees both halves;
the structure reaches it *as* prose. The frontmatter/body line is a split in who
decides, not in what the model reads. That is why the pattern descriptions
stayed in the frontmatter.

**"`PreferredTerms`/`ForbiddenTerms` say the same thing as `.kapi/terms.json`"
is half right, and the halves point opposite ways.** `ForbiddenTerms` genuinely
duplicates the store's deprecated and forbidden statuses, and the store is
strictly richer: it has the replacement, the concept, the locale and the
validity window. `PreferredTerms` does not duplicate anything: 57 governed
concepts against a five-item editorial selection, with two of the five absent
from the store entirely. Merging those two is not deduplication, it is
conflation.

**"Frontmatter carries the rules (authored, never generated)" does not describe
the current system.** A machine already writes this file, comments and all
(`host/apply_assets.go:723`). The read side of a new format is easy; the write
side is the whole cost, and the evaluation did not account for it.

## Verify

```
export PKG_CONFIG_PATH="$(brew --prefix icu4c)/lib/pkgconfig:$PKG_CONFIG_PATH"
go test -tags fts5 -v ./core/profile/mdspike/     # the spike's own claims, with the measurements logged
go test -tags fts5 ./core/... ./host/... ./cli/... # unchanged: 124 packages
kapi voice validate .kapi/voice.yaml       # VALID
kapi voice validate .kapi/profiles/bowrain/voice.yaml     # VALID
```
