package main

// The registry of every eval, the layer of the system it measures, and the band
// that layer belongs to.
//
// This file is authored. Its claims about coverage are checked against reality
// by the companion tests. A registry that can say whatever it likes about how
// well kapi is measured would be the opposite of evidence.
//
// # Why bands, and why these three
//
// The first version of this page was a flat list of questions, and it read as
// chaos: "does content survive a round trip" and "does the prior version change
// what the model writes" are not the same kind of claim, and putting them in one
// column invites a reader to weigh them the same way.
//
// The top-level split is therefore by SUBJECT, what the eval has under test,
// because the subject decides what its numbers can mean:
//
//   - kapi's own code is deterministic. Same input, same output. An eval here
//     can assert correctness and can gate a build.
//   - a model's output is sampled. An eval here estimates, over a corpus, with
//     a judge whose agreement with a person is itself a measurement. It can be
//     tracked, and it cannot gate anything.
//   - an agent's behaviour is stochastic in a third way. Whether a skill fires
//     is a property of a description and a model's reading of it, scored per
//     scenario over repeats, and it drifts when either side moves.
//
// Within a band the structure is the architecture: the same six AD series the
// contributor docs are organised by, so an eval sits beside the document that
// describes what it measures, and a series with no eval is visible as a hole in
// the architecture rather than as an absence from a list.

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

// Status is where an eval actually is, which is the honest half of this page.
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
	// StatusAbsent: nothing measures this. Listed because a gap a reader cannot
	// see is a gap they will assume is covered.
	StatusAbsent Status = "absent"
)

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

	// Corpus is what it runs over, in a sentence, with its size. The size is
	// the honest ceiling on what the eval can conclude.
	Corpus string `json:"corpus"`

	// Covers and Misses are the scope, stated in both directions. Misses is the
	// field that makes a card evidence rather than advertising.
	Covers string `json:"covers"`
	Misses string `json:"misses,omitempty"`

	// Reproduce is the command. Anyone reading a number should be able to run
	// the thing that produced it.
	Reproduce string `json:"reproduce"`

	// Settings is what a model call was made with: temperature, max tokens,
	// seed. Required of any eval that spends, because a command alone does not
	// reproduce a number that was sampled.
	//
	// "not recorded" is a valid answer, and it was the true one for every eval
	// here until providers/ai learned to send the field: Config.Temperature was
	// honoured by Ollama, honoured-except-zero by Bedrock, and dropped on the
	// floor by Anthropic, OpenAI, Azure and Gemini. Every cloud eval ran at
	// whatever the API defaulted to, and none of them wrote it down.
	//
	// The field existed to make that visible rather than let it stay implied.
	// It now carries the pinned value instead.
	Settings string `json:"settings,omitempty"`

	// Data is the committed result this eval publishes, relative to the repo
	// root. Empty for an absent eval. The test asserts a non-empty path exists.
	Data string `json:"data,omitempty"`

	// Page is where its results are rendered. Empty for an absent eval.
	Page string `json:"page,omitempty"`

	// Fresh is how old the committed numbers are, read out of the dataset
	// rather than typed here. A hand-written date is a date nobody updates;
	// /pseudobench spent three months showing results measured on 2026-05-20
	// with nothing on the page saying so.
	Fresh Freshness `json:"fresh,omitzero"`

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
		Evidence: "Deterministic. The same document produces the same content model every run, so these evals assert rather than estimate, and a regression is a fact rather than a shift in a distribution.",
		Gates:    "Yes. A failure here stops a build.",
		Layers:   []string{"foundations", "engine"},
	},
	{
		ID:      BandAI,
		Title:   "AI and context",
		Subject: "What a model writes under kapi's governance, and whether the governance reached it.",
		Evidence: "Sampled. A score is an estimate over a corpus, and where a model does the scoring it is an estimate of an estimate. " +
			"Corpus size is the ceiling on what any of it can conclude, so every card states it.",
		Gates:  "No. These are tracked over time, and a drop is a signal to look rather than a build failure.",
		Layers: []string{"context", "multilingual", "assurance"},
	},
	{
		ID:      BandSkills,
		Title:   "Agent skills",
		Subject: "An agent driving kapi: the shipped Agent Skill, and the MCP tools an assistant calls.",
		Evidence: "Scenario-scored. A prompt and a workspace go in, and what the agent did comes out: which tool it reached for, whether it finished at a green gate. " +
			"No model grades anything. Triggering is stochastic, so a scenario means little until it is repeated.",
		Gates:  "No, and these never run in CI. They drive an interactive agent on a maintainer's machine, so the committed dataset is all CI sees and the date on it is the real currency of the numbers.",
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
