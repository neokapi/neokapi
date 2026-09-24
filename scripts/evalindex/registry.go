package main

// Evaluation registry, grouped by subject and architecture layer.
// Engine checks assert deterministic results. Model evaluations estimate
// quality over a corpus. Agent evaluations score repeated task scenarios.
// Tests validate coverage declarations and links to supporting material.

// Band is what a group of evals has under test.
type Band string

const (
	// BandEngine is kapi's own code: formats, the content model, the flows.
	// Deterministic, so these evals gate.
	BandEngine Band = "engine"
	// BandAI is what a model produces under kapi's governance. Sampled, so
	// these evals are tracked rather than enforced.
	BandAI Band = "ai"
	// BandSkills is an agent driving kapi: the shipped Agent Skill and the MCP
	// surface. Scenario-scored, run on a maintainer's machine.
	BandSkills Band = "skills"
)

// BandInfo introduces a band: what it has under test, what kind of number comes
// out, and whether that number can gate anything.
type BandInfo struct {
	ID    Band   `json:"id"`
	Title string `json:"title"`
	// Subject is what these evals are pointed at.
	Subject string `json:"subject"`
	// Evidence is what the numbers are, and it is the reason for the split.
	Evidence string `json:"evidence"`
	// Gates says plainly whether a failure here can stop a build.
	Gates string `json:"gates"`
	// Layers are the layer ids in this band, in dependency order.
	Layers []string `json:"layers"`
}

// Layer is one architecture series, and the evals that measure it.
type Layer struct {
	ID   string `json:"id"`
	Band Band   `json:"band"`
	// Series is the AD letter: F, E, C, M, S, A.
	Series string `json:"series"`
	Title  string `json:"title"`
	// Scope is what sits in this layer, in a sentence.
	Scope string `json:"scope"`
	// Rests names what this layer depends on being right. Empty at the bottom
	// of a band.
	Rests string `json:"rests,omitempty"`
	// AD links the first architecture decision of the series. The series
	// directories carry no generated index, so a category link would 404; the
	// companion test resolves this path to a file.
	AD string `json:"ad"`
	// Evals are the ids measuring this layer, in reading order.
	Evals []string `json:"evals"`
}

// Method is how an eval reaches its numbers. It decides what the numbers can be
// trusted to mean, so it is stated on every card rather than inferred.
type Method string

const (
	// MethodDeterministic: runs kapi's own code and compares against an
	// expected result. Exact, repeatable, and cheap enough to gate a build.
	MethodDeterministic Method = "deterministic"
	// MethodLabelled: scored against a corpus someone labelled by hand.
	// Precision and recall mean what they usually mean; the corpus size is the
	// limit on what can be concluded.
	MethodLabelled Method = "labelled"
	// MethodJudged: a model scores the output. Cannot be trusted above the
	// judge's measured agreement with a person, which is why Validation is a
	// required field for this method rather than an optional one.
	MethodJudged Method = "judged"
	// MethodBenchmark: cost, speed, size. No opinion involved, but the
	// methodology decides comparability: what is included in a timing, whether
	// cached tokens are counted. Named for the practice rather than the act,
	// because "measured" is already a Status and a card carrying both drew the
	// pills "MEASURED MEASURED".
	MethodBenchmark Method = "benchmark"
	// MethodComparative: kapi against named alternatives. Needs independent
	// ground truth, or it measures agreement with whoever we compared to.
	MethodComparative Method = "comparative"
	// MethodScenario: an agent is given a prompt and a workspace, and what it
	// did is scored. Distinct from judged because no model grades the output:
	// the score is which tool the agent reached for, and whether the run ended
	// at a green gate. Stochastic on the agent's side, so a scenario means
	// little until it is repeated.
	MethodScenario Method = "scenario"
)

// Status describes the available evidence and any validation gaps.
type Status string

const (
	// StatusMeasured: it runs, its data is committed, and its numbers can be
	// read as they stand.
	StatusMeasured Status = "measured"
	// StatusPartial: it runs, and covers less than the layer needs. The card
	// says what it misses.
	StatusPartial Status = "partial"
	// StatusUnvalidated: it runs and produces numbers that should not yet be
	// relied on, such as a judged eval whose agreement with a person is
	// unmeasured, or a scenario suite whose last run has gone stale.
	StatusUnvalidated Status = "unvalidated"
	// StatusBlocked: the harness is written and runs, and the surface it
	// measures returns nothing to measure. Distinct from absent, because the
	// work left is a fix rather than a build, and the card can name the issue
	// number and link to a page that shows what is ready to run.
	StatusBlocked Status = "blocked"
	// StatusAbsent: nothing measures this. Listed because a gap a reader cannot
	// see is a gap they will assume is covered.
	StatusAbsent Status = "absent"
)

// AllStatuses defines the complete set used to initialize coverage totals.
var AllStatuses = []Status{
	StatusMeasured, StatusPartial, StatusUnvalidated, StatusBlocked, StatusAbsent,
}

// Eval is one measurement and everything needed to judge or repeat it.
type Eval struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Method Method `json:"method"`
	Status Status `json:"status"`

	// Spends says the eval calls a model. A spending eval's data is committed
	// and refreshed on demand; a free one can be regenerated on every build and
	// drift-tested, which is a stronger guarantee.
	Spends bool `json:"spends"`

	// Local says the eval runs on a maintainer's machine and never in CI,
	// because it drives an interactive agent or costs too much per run to sit
	// in a build. Its committed dataset is the only thing CI ever sees, so the
	// date on that dataset is the real currency of the numbers.
	Local bool `json:"local,omitempty"`

	// Corpus describes the evaluation inputs and sample size.
	Corpus string `json:"corpus"`

	// Covers and Misses describe the measured scope and its limitations.
	Covers string `json:"covers"`
	Misses string `json:"misses,omitempty"`

	// Reproduce is the command. Anyone reading a number should be able to run
	// the thing that produced it.
	Reproduce string `json:"reproduce"`

	// Settings records sampling parameters such as temperature, token limits and
	// seed. Required for evaluations that call a model. Use "not recorded" when
	// the dataset omits a parameter rather than assuming the provider's default.
	Settings string `json:"settings,omitempty"`

	// Data is the committed result this eval publishes, relative to the repo
	// root. Empty for an absent eval. The test asserts a non-empty path exists.
	Data string `json:"data,omitempty"`

	// Page is where its results are rendered. Empty for an absent eval.
	Page string `json:"page,omitempty"`

	// FreshAt is the key inside Data holding this card's own numbers, for the
	// datasets that carry several cards' results in one file. Empty means the
	// whole file is this card's.
	FreshAt string `json:"-"`

	// Fresh contains measurement dates read from the committed dataset.
	Fresh Freshness `json:"fresh,omitzero"`

	// Headline is the summary result extracted from the dataset. See headline.go.
	Headline *Headline `json:"headline,omitempty"`

	// Validation is required when Method is judged: what agreement with a human
	// has been measured, and whether it clears the bar. Empty means unmeasured,
	// and the status must then be unvalidated.
	Validation string `json:"validation,omitempty"`
}

// bands is the page, top to bottom.
var bands = []BandInfo{
	{
		ID:       BandEngine,
		Title:    "Engine and formats",
		Subject:  "kapi's own code: what it does to a document it reads, holds and writes back.",
		Evidence: "Deterministic checks compare the content model and output with expected results. Repeated runs use the same inputs.",
		Gates:    "Yes. A failure here stops a build.",
		Layers:   []string{"foundations", "engine"},
	},
	{
		ID:      BandAI,
		Title:   "AI and context",
		Subject: "What a model writes under kapi's governance, and whether the governance reached it.",
		Evidence: "Scores estimate performance on the stated corpus. Model-judged scores also depend on the judge's validated agreement with people. " +
			"Each card reports corpus size to make the sample limitations visible.",
		Gates:  "No. Results are tracked over time; regressions require investigation.",
		Layers: []string{"context", "multilingual", "assurance"},
	},
	{
		ID:      BandSkills,
		Title:   "Agent skills",
		Subject: "An agent driving kapi: the shipped Agent Skill, and the MCP tools an assistant calls.",
		Evidence: "Repeated scenarios measure tool selection and whether the agent completes the task with a passing gate. " +
			"Scores use observed actions and gate results rather than an LLM judge.",
		Gates:  "No. Runs use an interactive agent on a maintainer's machine. CI reads the committed dataset; its date indicates when the results were measured.",
		Layers: []string{"surfaces"},
	},
}

// layers is the architecture, in dependency order within each band.
var layers = []Layer{
	// --- Engine and formats ---------------------------------------------------
	{
		ID:     "foundations",
		Band:   BandEngine,
		Series: "F",
		Title:  "Foundations",
		Scope:  "The content model every format is read into, how a piece of content is identified, and the wire schema that carries it.",
		AD:     "/contribute/architecture/foundations/f-01-framework-and-modules",
		Evals:  []string{"kbf-conformance"},
	},
	{
		ID:     "engine",
		Band:   BandEngine,
		Series: "E",
		Title:  "Engine",
		Scope:  "Reading a format, running tools over the content, binding flows, and writing the document back unchanged where nothing was edited.",
		Rests:  "the content model being able to hold what the format carried",
		AD:     "/contribute/architecture/engine/e-01-processing-engine",
		Evals:  []string{"parity", "format-maturity", "engine-speed", "model-cost", "conversion-comparison"},
	},

	// --- AI and context -------------------------------------------------------
	{
		ID:     "context",
		Band:   BandAI,
		Series: "C",
		Title:  "Context",
		Scope:  "What governs content at the point it sits: the coordinates, the voice profile, the terms, and the content memory holding what was already approved.",
		Rests:  "the engine putting the right content in front of the governance",
		AD:     "/contribute/architecture/context/c-01-project-model",
		Evals:  []string{"reuse-rules", "reuse-effect", "context-value", "authoring-effect", "voice-infer-quality"},
	},
	{
		ID:     "multilingual",
		Band:   BandAI,
		Series: "M",
		Title:  "Multilingual",
		Scope:  "The call itself: what goes into a prompt, how many blocks travel together, and how the text was split before any of that.",
		Rests:  "the context layer having resolved what applies before a prompt is built",
		AD:     "/contribute/architecture/multilingual/m-01-bilingual-interop",
		Evals:  []string{"prompt-contents", "batching-cost"},
	},
	{
		ID:     "assurance",
		Band:   BandAI,
		Series: "A",
		Title:  "Assurance",
		Scope:  "The checks that read finished content and report what breaks the rules, and whether they are right about it.",
		Rests:  "every layer below, since a check reports on what they produced",
		AD:     "/contribute/architecture/assurance/a-01-testing-and-documentation",
		Evals:  []string{"check-accuracy", "authoring-checks"},
	},

	// --- Agent skills ---------------------------------------------------------
	{
		ID:     "surfaces",
		Band:   BandSkills,
		Series: "S",
		Title:  "Surfaces",
		Scope:  "How kapi is reached: the CLI, the desktop app, the shipped Agent Skill, and the MCP tools an assistant calls.",
		Rests:  "every layer below, since a surface is how a person or an agent gets at them",
		AD:     "/contribute/architecture/surfaces/s-01-kapi-cli",
		Evals:  []string{"skill-triggering", "skill-completion", "mcp-surface"},
	},
}
