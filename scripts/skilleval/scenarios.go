package main

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
	fxReactApp = "harness/demos/04-i18n-react-catalogs/fixtures/src/App.jsx"
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
Northwind,Northwind,Northwind,product name — never translated
workspace,espace de travail,Arbeitsbereich,
dashboard,tableau de bord,Übersicht,
sign in,se connecter,anmelden,
`

// A profile is usable when kapi can parse it and score against it. Which file
// the agent chose is its business, so the gate tries each candidate rather than
// pinning a name.
const gateUsableVoiceProfile = `for f in *.yaml *.yml; do [ -e "$f" ] || continue; ` +
	`kapi voice check --profile-file "$f" --input-text probe --json >/dev/null 2>&1 && exit 0; done; exit 1`

// A project exists when the recipe is there AND kapi can read it. The first
// half alone passes on a file the agent hand-wrote and kapi rejects.
const gateReadableProject = `test -f kapi.yaml && kapi status -p . >/dev/null 2>&1`

var scenarios = []Scenario{
	// ---- Positive: must trigger ---------------------------------------------
	{
		ID:     "p01-read-binary",
		Kind:   positive,
		Prompt: "What does slide 3 of pitch.pptx say?",
		Path:   "read (binary)",
		Why:    "An editor cannot open a .pptx. Reading it is the plainest case the skill exists for.",
		Fixture: []FixtureFile{
			{As: "pitch.pptx", From: fxPptx, Note: "a real deck; the agent has no other way to read it"},
		},
		Turns: 5,
	},
	{
		ID:     "p02-edit-binary",
		Kind:   positive,
		Prompt: "Make the intro of report.docx more concise, and keep the formatting.",
		Path:   "edit (round-trip)",
		Why:    "Editing inside a binary format while preserving structure is the round-trip claim.",
		Fixture: []FixtureFile{
			{As: "report.docx", From: fxDocx, Note: "formatting must survive the edit"},
		},
		Turns: 6,
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
		// any threshold a reader would think of setting. Zero findings is the
		// only gate that means what "fix what's off" means.
		CompletionGate: "kapi voice check --profile-file voice.yaml --json < README.md | jq -e '.findings | length == 0'",
	},
	{
		ID:     "p04-cross-format-sweep",
		Kind:   positive,
		Prompt: "We renamed SalesPilot to Northwind. Update every mention across docs/.",
		Path:   "toolbox (kgrep/ksed)",
		Why: "The fixture is the whole point. SalesPilot appears nine times inside the .docx and the " +
			".xlsx, where grep cannot see it, and once in the Markdown, where it can. An agent that " +
			"sweeps with grep alone finishes with nine misses and no error, so the gate counts what is left.",
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
		Why:    "Translation through a faithful round-trip, which is the format claim and the language claim at once.",
		Fixture: []FixtureFile{
			{As: "announcement.docx", From: fxDocxAnn, Note: "must come back as a valid .docx"},
		},
		Turns: 10,
	},
	{
		ID:     "p07-translate-json-terms",
		Kind:   positive,
		Prompt: "Localize src/locales/en.json into fr and de using our glossary.",
		Path:   "translate (terminology)",
		Why: "Terminology enforcement. EVALS.md records that this scenario once shipped with an empty glossary " +
			"and so tested nothing it claimed to; the seed below is that fix.",
		Fixture: []FixtureFile{
			{As: "src/locales/en.json", From: fxLocales, Note: "the catalog to translate"},
			{As: "glossary.csv", Body: glossaryCSV, Note: "four real entries, one a do-not-translate product name"},
		},
		Turns: 12,
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
	},
	{
		ID:     "p09-react-i18n",
		Kind:   positive,
		Prompt: "Add i18n to this React app.",
		Path:   "i18n (adoption)",
		Why:    "The i18n route, entered from a bare React app with hardcoded strings.",
		Fixture: []FixtureFile{
			{As: "src/App.jsx", From: fxReactApp, Note: "hardcoded strings to extract"},
			{As: "package.json", Body: `{"name":"demo","private":true,"dependencies":{"react":"^19.0.0"}}` + "\n"},
		},
		Turns:      12,
		KnownLimit: "The neokapi-i18n path installs @neokapi/i18n-react from a private registry that a sandbox does not have, so this scenario can trigger but cannot complete.",
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
			{As: "kapi.yaml", Body: "version: \"1\"\nname: northwind\ndefaults:\n  source: en\n  targets: [nb]\ncollections:\n  - name: app\n    include: [\"src/locales/en.json\"]\n    format: json\n"},
			{As: "src/locales/en.json", From: fxLocales, Note: "the source catalog the loop converges"},
		},
		Turns:          14,
		CompletionGate: gateReadableProject,
	},
	{
		ID:     "p12-i18n-advice",
		Kind:   positive,
		Prompt: "Which i18n library should we use for our Next.js app?",
		Path:   "i18n (advice)",
		Why: "Advice rather than action. The bar is higher than a recommendation: it must quote the toil grades, " +
			"which is what EVALS.md records it failing to do last run.",
		Fixture: []FixtureFile{
			{As: "src/app/page.tsx", From: fxNextPage},
			{As: "package.json", From: fxNextPkg, Note: "identifies the framework"},
		},
		Turns: 8,
	},
	{
		ID:     "p13-flutter",
		Kind:   positive,
		Prompt: "Internationalize this Flutter app and translate it to German.",
		Path:   "i18n (flutter)",
		Why:    "A framework with its own codegen, reached through detection rather than a flag.",
		Fixture: []FixtureFile{
			{As: "pubspec.yaml", Body: "name: northwind\nenvironment:\n  sdk: '>=3.0.0 <4.0.0'\ndependencies:\n  flutter:\n    sdk: flutter\n  flutter_localizations:\n    sdk: flutter\n"},
			{As: "lib/main.dart", Body: "import 'package:flutter/material.dart';\n\nvoid main() => runApp(const App());\n\nclass App extends StatelessWidget {\n  const App({super.key});\n  @override\n  Widget build(BuildContext c) =>\n      const MaterialApp(home: Scaffold(body: Center(child: Text('Sign in to your workspace'))));\n}\n",
				Note: "one hardcoded string to find"},
		},
		Turns: 12,
	},
	{
		ID:     "p14-retrofit",
		Kind:   positive,
		Prompt: "Our app has hardcoded strings everywhere, make it translatable.",
		Path:   "i18n (retrofit)",
		Why:    "The same route as p09, entered by the symptom rather than the name of the technique.",
		Fixture: []FixtureFile{
			{As: "src/App.jsx", From: fxReactApp, Note: "the hardcoded strings"},
			{As: "package.json", Body: `{"name":"demo","private":true,"dependencies":{"react":"^19.0.0"}}` + "\n"},
		},
		Turns:      12,
		KnownLimit: "Same private-registry limit as p09.",
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
		Turns:          14,
		CompletionGate: gateUsableVoiceProfile,
	},
	{
		ID:     "p17-refresh",
		Kind:   positive,
		Prompt: "We renamed Tidewatch to Tideguard and launched a support site, refresh our context.",
		Path:   "context discovery (refresh)",
		Why: "The second visit, and it fails in two directions. Completing means the agent read the drift BEFORE writing: " +
			"a rewritten profile looks like success in a transcript, so the gate is that nothing changed before approval.",
		Fixture: []FixtureFile{
			{As: "kapi.yaml", Body: "version: \"1\"\nname: tidewatch\ndefaults:\n  source: en\n  voice: brand\nprofiles:\n  brand:\n    voice: voice.yaml\ncollections:\n  - name: docs\n    include: [\"docs/**/*.md\"]\n    format: markdown\n"},
			{As: "voice.yaml", Body: "version: \"1\"\nname: Tidewatch\ntone:\n  - plain and direct\nvocabulary:\n  preferred_terms:\n    - term: Tidewatch\n      replacement: \"\"\n      severity: major\n",
				Note: "names the OLD product; must not be rewritten before approval"},
			{As: "docs/intro.md", Body: "# Tidewatch\n\nTidewatch keeps an eye on your tides.\n"},
			{As: "support/faq.md", Body: "# Tideguard support\n\nTideguard is the new name for Tidewatch.\n",
				Note: "the undeclared surface, using the new name — the drift to be found"},
		},
		Turns:          14,
		CompletionGate: gateUsableVoiceProfile,
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
