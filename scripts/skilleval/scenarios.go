package main

import "strings"

// The scenarios, ported from cli/skills/EVALS.md.
//
// Each one is a prompt, a workspace, and an expectation. The workspace is the
// half that is easy to get wrong: a prompt about pitch.pptx in an empty
// directory tests nothing, and a "find every X across docs/" scenario whose
// docs/ holds only Markdown correctly does NOT trigger, because native grep is
// the better tool there. The fixture is what makes the scenario mean what it
// says, so every file a prompt names exists before the agent starts.
//
// Fixtures come from the repository wherever a real binary is needed, so no
// .docx or .pptx is duplicated into this directory. Text fixtures are written
// inline, where being able to read the scenario and its material together is
// worth more than the reuse.

// FixtureFile is one file placed in a scenario's workspace.
type FixtureFile struct {
	// As is the path inside the workspace.
	As string `json:"as"`
	// From is a repo-relative path to copy. Mutually exclusive with Body.
	From string `json:"from,omitempty"`
	// Body is inline content. Mutually exclusive with From.
	Body string `json:"-"`
	// Note says what this file contributes to the scenario, for a reader of the
	// dashboard who wants to know why it is there.
	Note string `json:"note,omitempty"`
	// Bytes is filled in at run time so the dashboard can show the workspace
	// the agent actually saw.
	Bytes int `json:"bytes,omitempty"`
}

// Scenario is one prompt and what should happen when an agent reads it.
type Scenario struct {
	ID string `json:"id"`
	// Kind is positive (must trigger) or negative (must not).
	Kind string `json:"kind"`
	// Surface is how kapi is offered to the agent: through the shipped Agent
	// Skill and the CLI, or through the MCP server. Empty means skill.
	//
	// These are the two doors an assistant can come through and they fail
	// differently. A skill is a description an agent has to decide to read; an
	// MCP server is a tool list it already has, where the question is not
	// whether it notices kapi but whether it picks the right one of nineteen.
	Surface string `json:"surface,omitempty"`
	// ExpectTool is the MCP tool this scenario should lead to, without the
	// mcp__kapi__ prefix. Only meaningful when Surface is mcp.
	ExpectTool string `json:"expectTool,omitempty"`
	// Prompt is what the agent is given, verbatim.
	Prompt string `json:"prompt"`
	// Path is the route through the skill this exercises.
	Path string `json:"path"`
	// Why explains what the scenario is for, and for a negative, why firing
	// would be wrong.
	Why string `json:"why"`
	// Fixture is the workspace.
	Fixture []FixtureFile `json:"fixture"`
	// Turns caps the agent. Triggering shows up in the first turn or two; the
	// cap is what stops a positive running away into metered translation.
	Turns int `json:"turns"`
	// CompletionGate is the command that decides whether the agent finished,
	// run in the workspace after the agent stops. Empty means this scenario is
	// scored on triggering alone.
	CompletionGate string `json:"completionGate,omitempty"`
	// KnownLimit records a reason this scenario cannot reach a green gate that
	// is not a defect in the skill. It is printed beside the result rather than
	// quietly lowering the score.
	KnownLimit string `json:"knownLimit,omitempty"`
}

const (
	positive = "positive"
	negative = "negative"

	surfaceSkill = "skill"
	surfaceMCP   = "mcp"
)

// surfaceOf defaults an unset Surface to the skill, so the seventeen scenarios
// written before MCP existed need no edit.
func surfaceOf(sc Scenario) string {
	if sc.Surface == "" {
		return surfaceSkill
	}
	return sc.Surface
}

// Repo-relative fixture sources. Named rather than inlined at each use so a
// moved fixture is one edit and the companion test can resolve them all.
const (
	fxDocx     = "harness/demos/09-toolbox-find-replace/fixtures/proposal.docx"
	fxDocxAnn  = "harness/demos/03-translate-docx/fixtures/announcement.docx"
	fxXlsx     = "harness/demos/09-toolbox-find-replace/fixtures/pricing.xlsx"
	fxPptx     = "apps/kapi-desktop/backend/sample/kapimart/marketing/en/onboarding-deck.pptx"
	fxLanding  = "harness/demos/01-localize-landing-page/fixtures/index.html"
	fxLocales  = "harness/demos/kapi-bilingual-workflow/fixtures/src/locales/en/messages.json"
	fxNextPage = "harness/demos/02-nextjs-zero-to-i18n/fixtures/src/app/page.tsx"
	fxNextPkg  = "harness/demos/02-nextjs-zero-to-i18n/fixtures/package.json"
)

// A voice profile with something to say, so a "check against our voice" prompt
// has a rule to check against rather than an empty file.
const voiceProfile = `name: Northwind
description: Plain, direct product voice
tone:
    personality:
        - plain
        - direct
    formality: neutral
    emotion: neutral
    humor: none
    guidelines: Write plainly. Address the reader as you. No marketing superlatives.
style:
    active_voice: true
    sentence_length: short
    person_pov: second
    contractions: sometimes
vocabulary:
    preferred_terms:
        - term: use
          note: Preferred over 'utilize' and 'leverage'
    forbidden_terms:
        - term: utilize
          severity: major
        - term: leverage
          severity: major
        - term: cutting-edge
          severity: major
`

// A term seed with real entries. EVALS.md records that scenario 7 shipped with
// no terms and therefore tested nothing it claimed to; this is the fix.
const glossaryCSV = `source,target_fr,target_de,note
Northwind,Northwind,Northwind,product name, never translated
workspace,espace de travail,Arbeitsbereich,
dashboard,tableau de bord,Übersicht,
sign in,se connecter,anmelden,
`

// Gate expressions, shared where more than one scenario needs the same check.
//
// Every one was run by hand in both directions before being written down: a
// gate that cannot fail proves nothing, and one that cannot pass wastes a
// metered sweep discovering its own bug. The `-p .` on anything reading a
// project is not optional — the isolation contract sets KAPI_NO_PROJECT=1, so
// discovery is off and a bare `kapi status` cannot find a recipe the agent
// just wrote.

// A profile is usable when `kapi voice validate` accepts it. Which file the
// agent chose is its business, so the gate tries each candidate.
//
// Three things this had wrong, all found by running it rather than reading it:
//
// It asked `kapi voice check`, which is green on any YAML in the directory:
// every profile field is optional, so an empty file and `hello: world` both
// load as a profile and score 100/100 "on brand". validate is the command with
// an opinion, and it rejects both for a missing name. See issue #2224.
//
// It globbed `*.yaml` in the working directory only, while kapi's own
// convention puts the profile at `.kapi/voice.yaml`. The agent that did exactly
// the right thing was scored a failure for it.
//
// It ended in `exit 1`, so composing it with `&&` produced a gate whose second
// half could never run: `…; exit 1 && test -n "$(find …)"` terminates at the
// exit. p16 asked for a voice profile and terms, found both, and failed.
//
// It is one `test` now, which composes.
var gateUsableVoiceProfile = `test -n "$(find . -path ./node_modules -prune -o ` +
	`\( -name '*.yaml' -o -name '*.yml' \) -print | while read -r f; do ` +
	`kapi voice validate "$f" >/dev/null 2>&1 && echo "$f"; done)"`

// A project exists when the recipe is there AND kapi can read it. The first
// half alone passes on a file the agent hand-wrote and kapi rejects.
const gateReadableProject = `test -f kapi.yaml && kapi status -p . >/dev/null 2>&1`

// A locale has moved when status reports translated coverage above zero.
//
// This is what the loop changes. Looking for a target file instead would ask
// about delivery: Defaults.Materialize is `manual` unless the recipe opts in,
// so a fully converged project writes no nb.json and the state lives in
// .kapi/. Checked in both directions on the p11 fixture — 1 before `kapi up`,
// 0 after, with the run served locally by ollama.
const gateLocaleTranslated = `kapi status -p . --json 2>/dev/null | ` +
	`python3 -c 'import json,sys;d=json.load(sys.stdin);` +
	`sys.exit(0 if any(l["pct"].get("translated",0)>0 for l in d.get("locales",[])) else 1)'`

// The app is translatable when the hardcoded string has left the component and
// the strings it held live in a catalog.
//
// The second half used to be `-name '*.json' -path '*locale*'`, which is
// react-i18next's convention rather than a definition of done. The agent
// working with kapi extracted to `i18n/src/App.klf`, a catalog in kapi's own
// exchange format, and the gate scored that a failure while an unaided agent
// reaching for react-i18next passed. The gate was measuring which library was
// chosen, and it made kapi look worse than no kapi on two scenarios.
//
// Any format a reader would recognise as a catalog counts now. The first half
// is unchanged and still does the real work: a catalog beside an untouched
// App.jsx is the likelier half-finished outcome.
var gateStringsExtracted = `! grep -q "Welcome back, Alex" src/App.jsx && ` +
	gateHasFileMatching(`\( -name '*.klf' -o -name '*.po' -o -name '*.pot' `+
		`-o -name '*.xlf' -o -name '*.xliff' `+
		`-o \( -name '*.json' -a \( -path '*locale*' -o -path '*i18n*' -o -path '*translation*' \) \) \)`)

// A translated .docx exists when some document other than the source carries
// Japanese, and kapi can still read it. Existence alone would pass on an empty
// file, and a text search alone would pass on a corrupt one. Which file the
// agent wrote is its business.
//
// One `test`, not a loop ending in `exit 0`. A gate that ends the shell cannot
// be composed, and that same shape in gateUsableVoiceProfile silently ate the
// second half of p16's gate.
var gateDocxContainsJapanese = `test -n "$(find . -name '*.docx' ! -name 'announcement.docx' | ` +
	`while read -r f; do kapi kcat "$f" 2>/dev/null | python3 -c ` +
	`"import sys; t=sys.stdin.read(); ` +
	`sys.exit(0 if any(chr(0x3040)<=c<=chr(0x9fff) for c in t) else 1)" ` +
	`&& echo "$f"; done)"`

// gateAnswerMentions checks the agent's closing message, for the scenarios
// whose deliverable is an answer rather than a file.
func gateAnswerMentions(pattern string) string {
	return `grep -qiE ` + shellQuote(pattern) + ` .skilleval/answer.txt`
}

// gateHasFileMatching is existence, for a deliverable whose name the agent
// chooses.
func gateHasFileMatching(findArgs string) string {
	return `test -n "$(find . -path ./node_modules -prune -o ` + findArgs + ` -print -quit)"`
}

// shellQuote wraps a pattern for sh, doubling any single quote.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// hardcodedReactApp is a component with strings in the markup and no i18n
// anywhere: no import, no t(), no catalog.
//
// The repo's React fixture could not be used. It already imports
// useTranslation and every string is already t("nav.dashboard"), so "add i18n
// to this app" was being asked of an app that had it, and the fixture note
// calling them "hardcoded strings to extract" described a file that contained
// none. Written out here so the scenario and its material can be read together
// and the mismatch cannot recur silently.
const hardcodedReactApp = `export default function App() {
  const count = 3;
  return (
    <main>
      <nav>
        <a href="/">Dashboard</a>
        <a href="/tasks">Tasks</a>
      </nav>
      <h1>Welcome back, Alex</h1>
      <p>You have {count} tasks due today.</p>
      <button>Add a task</button>
      <label>
        <input type="checkbox" /> Email me about updates
      </label>
    </main>
  );
}
`

// catalogWithGovernedTerms is the catalog p07 translates.
//
// It exists because the repo fixture governed nothing: the glossary names
// Northwind as do-not-translate and the fixture never said Northwind, so the
// entry could not be violated and could not be honoured. That is the same
// fixture bug EVALS.md records this scenario shipping with once already, in
// its other form. Both governed terms appear here.
const catalogWithGovernedTerms = `{
  "greeting": "Welcome back to Northwind",
  "intro": "This is your dashboard. Everything you need is here.",
  "nav.dashboard": "Dashboard",
  "cta": "Sign in to Northwind",
  "save": "Save",
  "cancel": "Cancel"
}
`

// The project fixtures, in the schema kapi actually reads.
//
// Both of these were invented, and every key was wrong: `version: "1"` where
// the loader wants `v1`, `source:`/`targets:` where the fields are
// `source_language`/`target_languages`, and `include:` where a collection
// carries `content: - path:`. Only the last of those was ever reported —
// Defaults and Collection both end in `Extras map[string]yaml.Node` with
// `yaml:",inline"`, so an unknown key is preserved for platform layers to
// decode rather than rejected. A recipe full of near-miss keys therefore loads
// as a project with no source language and no targets, and says so only as
// "Monolingual project". See issue #2223.
const loopRecipe = `version: v1
name: northwind
defaults:
  source_language: en
  target_languages: [nb]
collections:
  - name: app
    base: src/locales
    content:
      - path: en.json
        target: "{lang}.json"
`

// refreshRecipe binds the voice profile p17 must not let the agent rewrite.
const refreshRecipe = `version: v1
name: tidewatch
defaults:
  source_language: en
  voice: voice.yaml
collections:
  - name: docs
    content:
      - path: "docs/**/*.md"
`

// tidewatchProfile names the OLD product, and kapi can parse it.
//
// It carried the same invented shape the recipes did: a `version` the profile
// schema has no field for, and `tone` as a list where it is an object. An
// unparseable profile inverts this scenario, whose point is that the agent must
// not rewrite the profile before asking. Checked: `kapi voice check` scores it
// 95 and reports the forbidden term.
const tidewatchProfile = `name: Tidewatch
description: Plain, direct product voice
tone:
    personality:
        - plain
        - direct
    formality: neutral
    emotion: neutral
    humor: none
    guidelines: Write plainly. Address the reader as you.
style:
    active_voice: true
    sentence_length: short
    person_pov: second
vocabulary:
    forbidden_terms:
        - term: Tideguard
          severity: major
`

var scenarios = []Scenario{
	// ---- Positive: must trigger ---------------------------------------------
	{
		ID:     "p01-read-binary",
		Kind:   positive,
		Prompt: "What does slide 3 of pitch.pptx say?",
		Path:   "read (binary)",
		Why: "The plainest case the skill exists for. It does NOT test whether kapi is necessary: a .pptx is a " +
			"zip of XML, and an unaided agent read this deck with `unzip -p` in three calls and answered " +
			"correctly. What is measured is whether the skill fires; what kapi adds is the control arm's job.",
		Fixture: []FixtureFile{
			{As: "pitch.pptx", From: fxPptx, Note: "a real three-slide deck; slide 3 is Next Steps"},
		},
		Turns: 5,
		// The deliverable is an answer, not a file. Slide 3 is titled "Next
		// Steps" and lists the Partner Quick Start Guide and the Seller
		// Community; any of those means the agent actually reached slide 3
		// rather than summarising the deck.
		CompletionGate: gateAnswerMentions("next steps|quick start guide|seller community"),
	},
	{
		ID:     "p02-edit-binary",
		Kind:   positive,
		Prompt: "Make the intro of report.docx more concise, and keep the formatting.",
		Path:   "edit (round-trip)",
		Why: "Editing inside a binary format is the round-trip claim. Whether the formatting actually survived " +
			"is not checked here: no gate reads the result back, so this scenario shows the route taken and " +
			"not the fidelity of what came out.",
		Fixture: []FixtureFile{
			{As: "report.docx", From: fxDocx, Note: "a formatted document, 62 words, to edit in place"},
		},
		Turns: 6,
		// Two halves. Still readable catches the round-trip risk, which is a
		// .docx that came back corrupt; fewer words than the 62 it started
		// with catches an agent that reworded without shortening. Neither
		// checks that the FORMATTING survived, and the card says so.
		CompletionGate: `kapi kcat report.docx >/dev/null 2>&1 && ` +
			`[ "$(kapi kcat report.docx 2>/dev/null | wc -w)" -lt 62 ]`,
	},
	{
		ID:     "p03-voice-check",
		Kind:   positive,
		Prompt: "Check README.md against our voice profile and fix what's off.",
		Path:   "voice",
		Why:    "A voice profile exists and the content violates it, so there is something to find.",
		Fixture: []FixtureFile{
			{As: "voice.yaml", Body: voiceProfile, Note: "forbids utilize, leverage, cutting-edge"},
			{As: "README.md", Body: "# Northwind\n\nNorthwind is a cutting-edge platform that lets teams " +
				"utilize their content across every channel. Leverage our workspace to ship faster.\n",
				Note: "three violations planted"},
		},
		Turns: 8,
		// Not --min-score: three major violations still scored 85, which passes
		// any threshold a reader would think of setting.
		//
		// Zero findings alone is not enough either. An empty profile has no
		// rules, so it reports zero findings on anything, and an agent that
		// emptied voice.yaml and left the README alone would pass. The gate
		// asks for both: a profile kapi still considers valid, and prose that
		// no longer uses the three planted words.
		CompletionGate: `kapi voice validate voice.yaml >/dev/null 2>&1 && ` +
			`! grep -qiE 'cutting-edge|utiliz|leverag' README.md`,
	},
	{
		ID:     "p04-cross-format-sweep",
		Kind:   positive,
		Prompt: "We renamed SalesPilot to Northwind. Update every mention across docs/.",
		Path:   "toolbox (kgrep/ksed)",
		Why: "SalesPilot appears nine times inside the .docx and the .xlsx and once in the Markdown. Plain grep " +
			"reads only the Markdown, so a grep-only sweep finishes with nine misses and no error, which is " +
			"what the gate catches. An unaided agent can still get there by unzipping and editing the XML, so " +
			"the control arm is what says whether kapi enabled this or merely shortened it.",
		Fixture: []FixtureFile{
			{As: "docs/proposal.docx", From: fxDocx, Note: "5 mentions, opaque to grep"},
			{As: "docs/pricing.xlsx", From: fxXlsx, Note: "4 mentions, opaque to grep"},
			{As: "docs/notes.md", Body: "SalesPilot is the workspace we use daily.\n",
				Note: "1 mention in plain text, so a grep-only sweep looks like it worked"},
		},
		Turns: 10,
		// kgrep exits 1 when nothing matches, so the negation is the gate: no
		// SalesPilot left anywhere kapi can read.
		CompletionGate: "! kgrep -ri salespilot docs/",
	},
	{
		ID:     "p05-voice-infer",
		Kind:   positive,
		Prompt: "Set up a voice profile for us from our landing page.",
		Path:   "context discovery (create)",
		Why:    "Inferring a profile from existing material rather than authoring one from nothing.",
		Fixture: []FixtureFile{
			{As: "index.html", From: fxLanding, Note: "the corpus the profile is inferred from"},
		},
		Turns:          10,
		CompletionGate: gateUsableVoiceProfile,
	},
	{
		ID:     "p06-translate-docx",
		Kind:   positive,
		Prompt: "Translate announcement.docx into Japanese.",
		Path:   "translate",
		Why: "Translation through a round-trip, which is the format claim and the language claim at once. " +
			"Neither is verified here: no gate opens the result, so a .docx that came back corrupt would " +
			"still read as a pass.",
		Fixture: []FixtureFile{
			{As: "announcement.docx", From: fxDocxAnn, Note: "the document to translate; English throughout"},
		},
		Turns: 10,
		// A second .docx that kapi can still read and that contains kana or
		// kanji. Existence alone would pass on an empty file, and a text search
		// alone would pass on a corrupt one.
		CompletionGate: gateDocxContainsJapanese,
		KnownLimit: "`apply` rejects every edit to a block with paired inline codes (#2227), and " +
			"announcement.docx has one, so a faithful ten-block change-set lands nine and leaves the tenth. " +
			"Whether the run finishes then depends on what the agent does next: one spent its whole budget " +
			"on the rejection and shipped an untranslated copy, another routed around it and produced good " +
			"Japanese. The unaided control finished with textutil in 26 messages both times.",
	},
	{
		ID:     "p07-translate-json-terms",
		Kind:   positive,
		Prompt: "Localize src/locales/en.json into fr and de using our glossary.",
		Path:   "translate (terminology)",
		Why: "Terminology enforcement. EVALS.md records that this scenario once shipped with an empty glossary " +
			"and so tested nothing it claimed to; the seed below is that fix.",
		Fixture: []FixtureFile{
			{As: "src/locales/en.json", Body: catalogWithGovernedTerms,
				Note: "the catalog to translate; carries both terms the glossary governs"},
			{As: "glossary.csv", Body: glossaryCSV,
				Note: "four entries, and two of them appear in the catalog: an ordinary term and a do-not-translate product name"},
		},
		Turns: 12,
		// Terminology, not merely translation. The glossary says dashboard is
		// Übersicht in German, and that Northwind is never translated. A
		// catalog that came back French-shaped but ignored both would pass any
		// existence check, which is why EVALS.md records this scenario once
		// shipping with an empty glossary and testing nothing it claimed to.
		CompletionGate: `de=$(find . -name '*.json' -path '*de*' -print -quit); ` +
			`test -n "$de" && grep -q 'Übersicht' "$de" && grep -q 'Northwind' "$de"`,
	},
	{
		ID:     "p08-interchange",
		Kind:   positive,
		Prompt: "Get report.docx ready for a translation vendor in French.",
		Path:   "translate (interchange)",
		Why:    "The hand-off path: extract to an interchange format, not translate in place.",
		Fixture: []FixtureFile{
			{As: "report.docx", From: fxDocx, Note: "source for the vendor package"},
		},
		Turns: 10,
		// The hand-off is a file in a format a vendor's tools open. Which one
		// is the agent's call, so the gate names the family rather than a path.
		CompletionGate: gateHasFileMatching(
			`\( -name '*.xlf' -o -name '*.xliff' -o -name '*.kpz' -o -name '*.po' -o -name '*.tmx' \)`),
	},
	{
		ID:     "p09-react-i18n",
		Kind:   positive,
		Prompt: "Add i18n to this React app.",
		Path:   "i18n (adoption)",
		Why:    "The i18n route, entered from a bare React app with hardcoded strings.",
		Fixture: []FixtureFile{
			{As: "src/App.jsx", Body: hardcodedReactApp, Note: "six strings in the markup, no i18n anywhere"},
			{As: "package.json", Body: `{"name":"demo","private":true,"dependencies":{"react":"^19.0.0"}}` + "\n"},
		},
		Turns: 12,
		// The strings must leave the component and land in a catalog. Checking
		// only that a catalog appeared would pass on a scaffold beside an
		// untouched App.jsx, which is the likelier half-finished outcome.
		CompletionGate: gateStringsExtracted,
		KnownLimit:     "The neokapi-i18n path installs @neokapi/i18n-react from a private registry that a sandbox does not have. A catalog-library route completes; that one cannot.",
	},
	{
		ID:     "p10-bootstrap",
		Kind:   positive,
		Prompt: "Set kapi up for this project.",
		Path:   "bootstrap",
		Why:    "The plainest setup ask. EVALS.md notes it needs room: below ~25 turns it reads as full i18n adoption.",
		Fixture: []FixtureFile{
			{As: "README.md", Body: "# Northwind\n\nA workspace for teams.\n"},
			{As: "src/locales/en.json", From: fxLocales, Note: "gives kapi init something to find"},
		},
		Turns:          12,
		CompletionGate: gateReadableProject,
	},
	{
		ID:     "p11-kapi-loop",
		Kind:   positive,
		Prompt: "Bring our project's Norwegian translations up to date and flag what still needs review.",
		Path:   "translate (the kapi loop)",
		Why: "The loop end to end: read state, catch up, surface the review queue. Completing means driving the gate, " +
			"not translating one file.",
		Fixture: []FixtureFile{
			{As: "kapi.yaml", Body: loopRecipe, Note: "a project with nb as its only target"},
			{As: "src/locales/en.json", From: fxLocales, Note: "the source catalog the loop converges"},
		},
		Turns: 14,
		// Not gateReadableProject: the fixture provides the recipe, so that
		// gate is green before the agent starts. Nor a target file — Defaults
		// .Materialize is `manual` unless a recipe says otherwise, so a
		// converged project writes no nb.json at all. What the loop moves is
		// coverage, which status reports either way.
		CompletionGate: gateLocaleTranslated,
	},
	{
		ID:     "p12-i18n-advice",
		Kind:   positive,
		Prompt: "Which i18n library should we use for our Next.js app?",
		Path:   "i18n (advice)",
		Why: "Advice rather than action. EVALS.md sets a bar above a bare recommendation, that the answer quote " +
			"the toil grades, and records the last manual run failing it. Nothing here checks that: the " +
			"scenario scores whether the skill fired, so a confident wrong answer passes.",
		Fixture: []FixtureFile{
			{As: "src/app/page.tsx", From: fxNextPage},
			{As: "package.json", From: fxNextPkg, Note: "identifies the framework"},
		},
		Turns: 8,
		// EVALS.md sets the bar above a bare recommendation: the answer must
		// quote the toil grade. The grades are T0 to T3, so the token is the
		// check, and a confident recommendation without one now fails rather
		// than passing on having fired.
		CompletionGate: gateAnswerMentions(`\bT[0-3]\b`),
	},
	{
		ID:     "p13-flutter",
		Kind:   positive,
		Prompt: "Internationalize this Flutter app and translate it to German.",
		Path:   "i18n (flutter)",
		Why: "A framework with its own codegen, which the skill should reach through detection rather than a " +
			"flag. Which route it took is visible in the recorded commands rather than scored.",
		Fixture: []FixtureFile{
			{As: "pubspec.yaml", Body: "name: northwind\nenvironment:\n  sdk: '>=3.0.0 <4.0.0'\ndependencies:\n  flutter:\n    sdk: flutter\n  flutter_localizations:\n    sdk: flutter\n"},
			{As: "lib/main.dart", Body: "import 'package:flutter/material.dart';\n\nvoid main() => runApp(const App());\n\nclass App extends StatelessWidget {\n  const App({super.key});\n  @override\n  Widget build(BuildContext c) =>\n      const MaterialApp(home: Scaffold(body: Center(child: Text('Sign in to your workspace'))));\n}\n",
				Note: "one hardcoded string to find"},
		},
		Turns: 12,
		// gen_l10n's convention: catalogs are .arb, and a German one means the
		// translation half happened rather than only the scaffolding.
		CompletionGate: gateHasFileMatching(`-name '*.arb'`),
	},
	{
		ID:     "p14-retrofit",
		Kind:   positive,
		Prompt: "Our app has hardcoded strings everywhere, make it translatable.",
		Path:   "i18n (retrofit)",
		Why:    "The same route as p09, entered by the symptom rather than the name of the technique.",
		Fixture: []FixtureFile{
			{As: "src/App.jsx", Body: hardcodedReactApp, Note: "the same six strings, reached by the symptom rather than the name"},
			{As: "package.json", Body: `{"name":"demo","private":true,"dependencies":{"react":"^19.0.0"}}` + "\n"},
		},
		Turns:          12,
		CompletionGate: gateStringsExtracted,
		KnownLimit:     "Same private-registry limit as p09.",
	},
	{
		ID:     "p15-android",
		Kind:   positive,
		Prompt: "Localize this Android app into French.",
		Path:   "i18n (androidxml)",
		Why:    "A format with a directory convention: the target lands in values-fr/, not beside the source.",
		Fixture: []FixtureFile{
			{As: "app/src/main/res/values/strings.xml", Body: "<?xml version=\"1.0\" encoding=\"utf-8\"?>\n<resources>\n    <string name=\"app_name\">Northwind</string>\n    <string name=\"sign_in\">Sign in to your workspace</string>\n    <string name=\"dashboard\">Dashboard</string>\n</resources>\n"},
			{As: "app/build.gradle", Body: "plugins { id 'com.android.application' }\nandroid { namespace 'com.northwind' compileSdk 34 }\n"},
		},
		Turns: 10,
		// The androidxml convention: a French target lands in values-fr/,
		// beside the source rather than inside it.
		CompletionGate: "test -f app/src/main/res/values-fr/strings.xml",
	},
	{
		ID:     "p16-onboard",
		Kind:   positive,
		Prompt: "Set up our content context from this repo: the voice we write in and the terms we use.",
		Path:   "context discovery (onboard)",
		Why: "The first visit, with nothing in place. Completing means the context files exist and a voice check " +
			"passes over a sample of the repo's own material, rather than a profile being invented from nothing.",
		Fixture: []FixtureFile{
			{As: "README.md", Body: "# Northwind\n\nNorthwind gives teams one workspace for their content.\nWe write plainly and we do not shout.\n"},
			{As: "marketing/launch.docx", From: fxDocx, Note: "material for the profile to be inferred from"},
			{As: "marketing/tone.md", Body: "We use plain words. We avoid marketing superlatives. We address the reader as you.\n"},
		},
		Turns: 14,
		// The prompt asks for two things, so the gate asks for two: a profile
		// kapi validates, and terms recorded somewhere kapi or a reader can
		// find them. `kapi terms` has no validate verb, so the terms half is
		// existence rather than structure — the weaker half, and named as such.
		CompletionGate: gateUsableVoiceProfile + ` && ` +
			gateHasFileMatching(`( -name '*term*' -o -name '*glossary*' )`),
	},
	{
		ID:     "p17-refresh",
		Kind:   positive,
		Prompt: "We renamed Tidewatch to Tideguard and launched a support site, refresh our context.",
		Path:   "context discovery (refresh)",
		Why: "The second visit, and it fails in two directions. Completing means the agent read the drift BEFORE writing: " +
			"a rewritten profile looks like success in a transcript, so the gate is that nothing changed before approval.",
		Fixture: []FixtureFile{
			{As: "kapi.yaml", Body: refreshRecipe, Note: "binds the profile below"},
			{As: "voice.yaml", Body: tidewatchProfile,
				Note: "names the OLD product, and kapi can parse it; must not be rewritten before approval"},
			{As: "docs/intro.md", Body: "# Tidewatch\n\nTidewatch keeps an eye on your tides.\n"},
			{As: "support/faq.md", Body: "# Tideguard support\n\nTideguard is the new name for Tidewatch.\n",
				Note: "the undeclared surface, using the new name: the drift to be found"},
		},
		Turns: 14,
		// The scenario is that the agent must NOT rewrite the profile before
		// asking, so gateUsableVoiceProfile is the wrong gate twice over: the
		// fixture ships a valid profile, so it is green before the agent
		// starts, and it stays green if the agent renames everything to
		// Tideguard. Completing means the profile still names Tidewatch and the
		// closing message surfaced the drift.
		CompletionGate: `grep -q '^name: Tidewatch' voice.yaml && ` +
			gateAnswerMentions("tideguard"),
	},

	// ---- Negative: must NOT trigger -----------------------------------------
	{
		ID:     "n01-go-refactor",
		Kind:   negative,
		Prompt: "Refactor this Go function for readability.",
		Path:   "code",
		Why:    "A code task with no content or format work. Native editing is the right tool.",
		Fixture: []FixtureFile{
			{As: "sum.go", Body: "package main\n\nfunc Sum(xs []int) int {\n\tt := 0\n\tfor i := 0; i < len(xs); i++ {\n\t\tt = t + xs[i]\n\t}\n\treturn t\n}\n"},
		},
		Turns: 4,
	},
	{
		ID:     "n02-write-script",
		Kind:   negative,
		Prompt: "Write a Python script to parse these log files.",
		Path:   "code authoring",
		Why:    "Authoring code. Nothing here is document content.",
		Fixture: []FixtureFile{
			{As: "app.log", Body: "2026-08-01 12:00:00 INFO started\n2026-08-01 12:00:01 WARN slow query 431ms\n"},
		},
		Turns: 4,
	},
	{
		ID:     "n03-fix-test",
		Kind:   negative,
		Prompt: "Fix the failing unit test in auth_test.go.",
		Path:   "code/test",
		Why:    "A test task. Firing here would mean the description has drifted toward any file work.",
		Fixture: []FixtureFile{
			{As: "auth_test.go", Body: "package auth\n\nimport \"testing\"\n\nfunc TestLogin(t *testing.T) {\n\tif Login(\"a\", \"b\") != true {\n\t\tt.Fatal(\"expected success\")\n\t}\n}\n"},
		},
		Turns: 4,
	},
	{
		ID:      "n04-general-knowledge",
		Kind:    negative,
		Prompt:  "What's the capital of France?",
		Path:    "general knowledge",
		Why:     "The control. A skill that fires on this fires on everything.",
		Fixture: []FixtureFile{},
		Turns:   3,
	},
	{
		ID:     "n05-locale-code",
		Kind:   negative,
		Prompt: "Format this date according to the user's locale.",
		Path:   "locale-aware code",
		Why: "The sharpest negative: it is about locales, and it is still a code task. " +
			"A description broad enough to catch this catches every Intl call in every codebase.",
		Fixture: []FixtureFile{
			{As: "date.ts", Body: "export function show(d: Date, locale: string) {\n  return d.toISOString();\n}\n"},
		},
		Turns: 4,
	},
}
