package main

// The registry of every eval, and the questions they answer.
//
// This file is authored. Its claims about coverage are checked against reality
// by the companion test. A registry that can say whatever it likes about how
// well kapi is measured would be the opposite of evidence.
//
// The organising unit on the cover page is the QUESTION, not the eval. A reader
// arrives wanting to know whether something is true, and a question can be
// answered badly: "is authoring governed too" reads honestly when the answer is
// "not measured yet", where a heading asserting that it is would not.
//
// Every eval carries a card. The fields are the ones the EvalEval Coalition's
// evaluation-card work found missing almost everywhere: across 50,000+ published
// evaluation records, 96.5% lacked at least one field needed to re-run them.
// An eval nobody can reproduce is not evidence, it is an assertion with a table.

// Question is one thing a reader might want to know, and the evals that answer
// it. Questions with no eval are listed too, and are the most useful rows here.
type Question struct {
	ID string `json:"id"`
	// Ask is the question in the form a reader would put it.
	Ask string `json:"ask"`
	// Why says what turns on the answer, for a reader deciding whether to care.
	Why string `json:"why"`
	// Evals are the ids answering it, in reading order.
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
)

// Status is where an eval actually is, which is the honest half of this page.
type Status string

const (
	// StatusMeasured: it runs, its data is committed, and its numbers can be
	// read as they stand.
	StatusMeasured Status = "measured"
	// StatusPartial: it runs, and covers less than the question needs. The card
	// says what it misses.
	StatusPartial Status = "partial"
	// StatusUnvalidated: it runs and produces numbers that should not yet be
	// relied on, such as a judged eval whose agreement with a person is unmeasured.
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

	// Data is the committed result this eval publishes, relative to the repo
	// root. Empty for an absent eval. The test asserts a non-empty path exists.
	Data string `json:"data,omitempty"`

	// Page is where its results are rendered. Empty for an absent eval.
	Page string `json:"page,omitempty"`

	// Validation is required when Method is judged: what agreement with a human
	// has been measured, and whether it clears the bar. Empty means unmeasured,
	// and the status must then be unvalidated.
	Validation string `json:"validation,omitempty"`
}

// questions is the cover page, in order.
var questions = []Question{
	{
		ID:  "round-trip",
		Ask: "Does content survive being read and written back?",
		Why: "Everything else is downstream of this. A format kapi reads imperfectly is a format " +
			"whose translations land in a document that no longer matches the original.",
		Evals: []string{"parity", "format-maturity", "kbf-conformance"},
	},
	{
		ID:  "reuse",
		Ask: "Is approved wording reused when the rules still hold, and refused when they do not?",
		Why: "The reason to keep a content memory at all. Reusing too little wastes reviewed work; " +
			"reusing too much writes wording that was approved under rules that have since moved.",
		Evals: []string{"reuse-rules", "reuse-effect"},
	},
	{
		ID:  "governance-reaches-the-model",
		Ask: "Does the governance actually reach the model, rather than only checking its output?",
		Why: "A voice profile that is enforced only after generation is a correction loop. One that " +
			"is in the prompt is a steer, and the difference shows up in how much there is to correct.",
		Evals: []string{"context-value", "prompt-contents"},
	},
	{
		ID:  "violations-caught",
		Ask: "Do the checks find the violations they claim to find?",
		Why: "A check that misses is worse than no check: it reports a clean run over content nobody " +
			"has looked at.",
		Evals: []string{"check-accuracy"},
	},
	{
		ID:  "authoring-governed",
		Ask: "Is source authoring governed the same way translation is?",
		Why: "kapi governs content, not only translations. If the English is off-voice and off-terms, " +
			"every language downstream inherits it, and nothing here measures whether kapi helps.",
		Evals: []string{"authoring-checks", "authoring-effect", "voice-infer-quality"},
	},
	{
		ID:  "cost",
		Ask: "What does a run cost, and is that what we said it costs?",
		Why: "A quality improvement that triples the token bill is a trade, not an improvement, and " +
			"it should be visible as one.",
		Evals: []string{"batching-cost", "model-cost", "engine-speed"},
	},
	{
		ID:  "where-we-stand",
		Ask: "Where does kapi stand against the alternatives?",
		Why: "The question a reader has and we cannot answer from the inside. Measuring it needs " +
			"independent ground truth, and publishing it means publishing the losses too.",
		Evals: []string{"conversion-comparison"},
	},
}
