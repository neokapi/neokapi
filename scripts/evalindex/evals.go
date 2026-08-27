package main

// The cards. One per eval, including the ones that do not exist.
//
// Grouped by the layer that lists them. An absent eval carries the same card as
// a measured one minus its data, because the useful thing about a gap is the
// shape of what is missing rather than the fact of it.
//
// Every Reproduce, Page and Data here is checked against the repository by the
// companion tests. Authoring this file the first time named four commands that
// do not exist and one page that was retired, which is the argument for the
// tests rather than against the registry.

var evals = []Eval{
	// ==========================================================================
	// Band: Engine and formats
	// ==========================================================================

	// --- F, Foundations -------------------------------------------------------
	{
		ID:        "kbf-conformance",
		Title:     "KBF conformance across engines",
		Method:    MethodDeterministic,
		Status:    StatusPartial,
		Corpus:    "The KBF conformance suite, run against the Go engine compiled to WebAssembly and the TypeScript mirror.",
		Covers:    "That two independent implementations of the bundle format agree, operation by operation, in your browser.",
		Misses:    "Runs only when someone opens the page. Nothing in CI executes it, so it reports rather than gates. The structural and envelope checks run on the Go engine alone, and identity (F-03) is not measured at all.",
		Reproduce: "open /kbf-tests",
		Page:      "/kbf-tests",
	},

	// --- E, Engine ------------------------------------------------------------
	{
		ID:        "parity",
		Title:     "Filter parity against upstream Okapi",
		Method:    MethodDeterministic,
		Status:    StatusPartial,
		Corpus:    "The Okapi Framework's own filter test suite, mapped onto neokapi's readers.",
		Covers:    "Whether a document read by neokapi produces the same content model as the Java implementation it descends from, per format.",
		Misses:    "Publishes nothing. The report lands in a gitignored sandbox and the dashboard that used to render it was retired, so the result is visible only to whoever ran it. Formats with no upstream counterpart are unmeasurable this way, and a known-failure list is carried rather than fixed.",
		Reproduce: "make parity-test",
	},
	{
		ID:        "format-maturity",
		Title:     "Format maturity, L0 to L4",
		Method:    MethodDeterministic,
		Status:    StatusMeasured,
		Corpus:    "Every registered format, scored against a fixed rubric.",
		Covers:    "How far each format has been taken: read, write, round-trip, inline codes, faithful write-back.",
		Misses:    "A rubric level is a claim about capability, not about quality on your documents.",
		Reproduce: "/format-ops triage-score",
		Data:      "web/static/data/format-maturity.json",
		Page:      "/format-maturity",
	},
	{
		ID:        "engine-speed",
		Title:     "Engine throughput",
		Method:    MethodBenchmark,
		Status:    StatusMeasured,
		Corpus:    "844 fixtures on the pseudo-translation path, which exercises read and write without a model, across four engines.",
		Covers:    "How fast each engine moves content when no provider is in the way, with the count of files whose output actually contains pseudo-translated text beside the timing, so an engine that is fast because it read less is visible as such.",
		Misses:    "Isolates the engine deliberately, so it says nothing about a run that calls a model. One machine, one platform.",
		Reproduce: "make bench-stress",
		Data:      "web/static/data/pseudobench.json",
		Page:      "/pseudobench",
	},
	{
		ID:        "model-cost",
		Title:     "What a local model costs to run",
		Method:    MethodBenchmark,
		Status:    StatusMeasured,
		Corpus:    "The bundled ML models, on one machine's runtime.",
		Covers:    "What a user pays in download size, cold start, per-sentence inference and resident memory.",
		Misses:    "Measured on one platform. A different runtime or accelerator moves every number.",
		Reproduce: "python3 scripts/ml-benchmark.py",
		Data:      "web/src/pages/ml-benchmark/_benchmark.json",
		Page:      "/ml-benchmark",
	},
	{
		ID:        "conversion-comparison",
		Title:     "How much of a document each converter keeps",
		Method:    MethodComparative,
		Status:    StatusPartial,
		Corpus:    "The Okapi Framework's own integration-test resources — real .docx, .pptx and .xlsx collected by another project for another purpose, which is what makes them a fair test rather than a demonstration.",
		Covers:    "Text-extraction completeness against ground truth read from each document's own XML parts, so no converter's output stands in for the answer, plus how long each conversion took. Compared per format, because the tools accept different ones and a single column would rank them by what they declined.",
		Misses:    "Text only. Structure and ordering are not compared, so a converter that flattens every heading scores the same as one that keeps the outline. Headers, footnotes, comments and speaker notes are excluded because converters disagree about whether they belong in the output. Only the converters installed on the machine that ran it appear.",
		Reproduce: "make conversion-eval",
		Data:      "web/src/pages/conversion-eval/_conversioneval.json",
		Page:      "/conversion-eval",
	},

	// ==========================================================================
	// Band: AI and context
	// ==========================================================================

	// --- C, Context -----------------------------------------------------------
	{
		ID:        "reuse-rules",
		Title:     "What an author's edit does to reuse",
		Method:    MethodDeterministic,
		Status:    StatusMeasured,
		Corpus:    "One approved sentence pair, edited eleven ways; a three-sentence paragraph; five governance scenarios.",
		Covers:    "Which rules apply where, whether an edited source still gets its approved translation, whether a prior version is offered or withheld, and what reaches the prompt.",
		Misses:    "One language pair. The edit ladder is one sentence, chosen for length rather than sampled.",
		Reproduce: "make coordinate-report",
		Data:      "web/src/pages/coordinate/_coordinate.json",
		Page:      "/coordinate",
	},
	{
		ID:        "reuse-effect",
		Title:     "Whether the prior version changes what the model writes",
		Method:    MethodJudged,
		Status:    StatusUnvalidated,
		Spends:    true,
		Corpus:    "Six wording cases with a control, plus a 20-block document in three batching arms.",
		Covers:    "Whether showing the model a block's previously approved translation keeps the approved wording, and what it costs in calls and tokens.",
		Misses:    "One model, one language pair, one direction. The judged half has no measured agreement with a person; only the deterministic consistency half should be relied on.",
		Settings:  "Temperature 0, pinned in the harness and recorded in the dataset. Max tokens follow the model's ceiling. No seed: the API exposes none.",
		Reproduce: "make prior-ab-eval",
		Data:      "web/src/pages/coordinate/_abeval.json",
		Page:      "/coordinate",
		// Validation is deliberately empty: see Status.
	},
	{
		ID:        "context-value",
		Title:     "What context injection is worth",
		Method:    MethodJudged,
		Status:    StatusUnvalidated,
		Spends:    true,
		Corpus:    "A synthetic brand corpus with a fixed rubric, swept across models.",
		Covers:    "Whether a model given voice guidance and terminology produces output that needs less correction than one that is not.",
		Misses:    "Judge-to-human agreement is gated at 30 labelled items; the literature puts the useful floor at 100 to 200, and no sweep has cleared even the lower bar.",
		Settings:  "Temperature 0 on every swept model, pinned in the harness. The committed sweep predates the field that records it, so the dataset carries no temperature of its own until the next one. Max tokens follow each model's ceiling. No seed: the APIs expose none.",
		Reproduce: "make context-eval",
		Data:      "web/src/pages/context-eval/_contexteval.json",
		Page:      "/context-eval",
	},
	{
		ID:        "authoring-effect",
		Title:     "Whether the voice guide steers writing or just improves it",
		Method:    MethodBenchmark,
		Status:    StatusPartial,
		Spends:    true,
		Corpus:    "Six briefs about a synthesized product, each written twice by one model: once bare, once with `kapi voice guide` as the system turn. Both versions scored against the profile the guide came from and against a contrast profile that wants the opposite.",
		Covers:    "Whether the guide moves writing toward its own profile specifically. Any competent writing guidance raises any reasonable score, so the reference gain alone proves nothing; the measurement is the difference between the two gains. Guided writing gained 5.8 points against the reference profile and lost 6.7 against the contrast, an effect of 12.5. The mechanism is visible in the findings: passive constructions fell from 11 to 4 across the six documents, and the writing moved to second person.",
		Misses:    "Six documents on one model, and each arm turns on a single rule. Every point the reference arm gained is passive-voice reduction: the model never reached for a forbidden term in either condition, so those rules contributed nothing. Every point the contrast arm lost is the pronoun shift, since that profile declares three forbidden terms and no patterns. The two arms moving in opposite directions is real, and it is two mechanisms rather than a broad effect. The declared fields (person, sentence length) are in the guide the model read and not in the score, because nothing offline evaluates them.",
		Settings:  "Temperature 0, and the user turn is byte-identical across arms. The bare arm has no system turn at all rather than a placebo one.",
		Reproduce: "make authoring-eval",
		Data:      "web/src/pages/authoring-eval/_authoringeval.json",
		Page:      "/authoring-eval",
	},
	{
		ID:        "voice-infer-quality",
		Title:     "Whether an inferred voice profile is usable",
		Method:    MethodDeterministic,
		Status:    StatusBlocked,
		Spends:    true,
		Corpus:    "Built and ready: the corpus's on-profile half was written from a reference profile, so recovery is checkable field by field rather than by asking a judge. The comparison runs the moment there is a draft.",
		Settings:  "Whatever provider the run names, at temperature 0. Nothing is recorded because the tool returns nothing; the field will carry the real settings with the first draft it produces.",
		Covers:    "Would measure how much of the reference profile a draft recovers — each tone and style field, and how many of the forbidden terms.",
		Misses:    "`kapi exec voice-infer` writes nothing to stdout and exits 0 (#2225), and it has no other surface: there is no `kapi voice infer` subcommand. There is no draft to compare.",
		Reproduce: "make authoring-eval",
		Data:      "web/src/pages/authoring-eval/_authoringeval.json",
		Page:      "/authoring-eval",
	},

	// --- M, Multilingual ------------------------------------------------------
	{
		ID:        "prompt-contents",
		Title:     "What each call actually sends",
		Method:    MethodDeterministic,
		Status:    StatusMeasured,
		Corpus:    "The prompts built for the reuse evals, captured as they went to the model.",
		Covers:    "That the voice guide, the terminology a call can use, and a block's prior version are in the prompt, and that the cache key moves with them.",
		Misses:    "Reads the prompt kapi built, not the reply it got. It shows the governance arrived; whether the model followed it is context-value's question.",
		Reproduce: "make prior-ab-eval",
		Data:      "web/src/pages/coordinate/_abeval.json",
		Page:      "/coordinate",
	},
	{
		ID:        "batching-cost",
		Title:     "What batching costs and saves",
		Method:    MethodBenchmark,
		Status:    StatusMeasured,
		Spends:    true,
		Corpus:    "600 blocks swept across four batch sizes on two models, plus the earlier sweeps kept in the same history.",
		Covers:    "Throughput and structural integrity as blocks per call rises, priced per model. Both models hold every block to 128 per call and neither can answer at 600.",
		Misses:    "Concurrency is recorded because throughput depends on it, so two models swept at different concurrencies cannot be raced on speed. Segmentation (M-02) is not measured at all, and it decides what a block is before any of this runs.",
		Settings:  "Temperature 0, pinned in the harness. The committed sweep predates the field that records it, so the dataset carries concurrency but not temperature until the next one. No seed: the APIs expose none.",
		Reproduce: "make batch-eval",
		Data:      "web/src/pages/batch-eval/_batcheval.json",
		Page:      "/batch-eval",
	},

	// --- A, Assurance ---------------------------------------------------------
	{
		ID:        "check-accuracy",
		Title:     "Precision and recall of the content checks",
		Method:    MethodLabelled,
		Status:    StatusPartial,
		Corpus:    "10 hand-labelled cases.",
		Covers:    "do-not-translate and placeholder checks.",
		Misses:    "Ten cases is too few to separate a real precision difference from noise. The voice checks are measured separately by authoring-checks; term-check is not measured by either, and belongs to neither — its examples are XLIFF with a target, so it scores translations against approved terms rather than source prose.",
		Reproduce: "make check-eval",
		Data:      "web/src/pages/check-eval/_eval.json",
		Page:      "/check-eval",
	},
	{
		ID:        "authoring-checks",
		Title:     "Whether the voice checks find real violations",
		Method:    MethodLabelled,
		Status:    StatusPartial,
		Corpus:    "Twelve synthesized documents about one product: six written to a voice profile, six written against it with 18 violations marked. The clean half is what makes false positives measurable.",
		Covers:    "Recall over the marked violations, split by how the profile expresses each rule. The offline check found 89% overall — every prohibited pattern, 12 of 13 forbidden terms, and none of the declared fields, which nothing offline evaluates. It also raised a finding on 3 of the 6 clean documents.",
		Misses:    "The LLM check cannot be scored: `kapi exec voice-check` returns nothing under every provider and profile tried (#2225). The one term miss is `utilizes` against a profile forbidding `utilize` (#2226). The corpus is synthesized, which is disclosed in the data itself.",
		Reproduce: "make authoring-eval",
		Data:      "web/src/pages/authoring-eval/_authoringeval.json",
		Page:      "/authoring-eval",
	},

	// ==========================================================================
	// Band: Agent skills
	// ==========================================================================

	// --- S, Surfaces ----------------------------------------------------------
	{
		ID:      "skill-triggering",
		FreshAt: "skill:trigger",
		Title:   "Whether the skill fires on the right tasks",
		Method:  MethodScenario,
		Status:  StatusMeasured,
		Spends:  true,
		Local:   true,
		Corpus:  "17 prompts that must load the skill and 5 that must not, three passes each, every one in its own workspace holding the files the prompt names.",
		Covers: "The only lever on triggering, which is the skill's description: does an assistant reach for kapi on a content task, and leave it alone on a code task. " +
			"Activation counts a Skill call or a kapi command, so an agent that reads the skill once and then works from it is not scored as a miss.",
		Misses: "Triggering only. A scenario that fires and then does the job badly passes here; whether the work is right is skill-completion's question. " +
			"One model on one day, and the negatives are five prompts rather than a sample of the code tasks a real assistant sees.",
		Settings:  "claude -p with a 4-turn cap, bypassPermissions, 3 repeats. Sampling follows the local CLI's defaults and is not pinned.",
		Reproduce: "make skill-eval",
		Data:      "web/src/pages/skill-eval/_skilleval.json",
		Page:      "/skill-eval",
	},
	{
		ID:      "skill-completion",
		FreshAt: "skill:completion",
		Title:   "Whether the agent finishes the job",
		Method:  MethodScenario,
		Status:  StatusPartial,
		Spends:  true,
		Local:   true,
		Corpus:  "15 of the positive scenarios driven to the end in a sandboxed kapi with isolated config, data, cache and plugins.",
		Covers:  "That the agent does not merely start: it drives the loop to a green gate, with kapi check --ship or kapi check passing.",
		Misses: "Scored at catalog-gate depth by decision, so in-locale rendering is never verified. " +
			"Two scenarios are blocked on a private npm registry absent from the sandbox rather than on any skill defect.",
		Settings:  "claude -p in a sandboxed kapi, Gemini via env. Turn caps vary by scenario; sampling is not recorded.",
		Reproduce: "make skill-eval-completion",
		Data:      "web/src/pages/skill-eval/_skilleval.json",
		Page:      "/skill-eval",
	},
	{
		ID:      "mcp-surface",
		FreshAt: "mcp:trigger",
		Title:   "Whether an agent picks the right MCP tool",
		Method:  MethodScenario,
		Status:  StatusPartial,
		Spends:  true,
		Local:   true,
		Corpus:  "Seven tasks with one right answer each among the nineteen tools the server advertises, plus one task no tool should answer. Three passes each.",
		Covers:  "The other door into kapi. An MCP client already holds the tool list, so it cannot fail to notice kapi; it fails by reaching for the wrong tool, and a near miss names two descriptions that are not telling each other apart.",
		Misses: "Seven of nineteen tools have a scenario. The review and approval tools, redaction and translate are unmeasured. " +
			"A wrong pick is recorded rather than diagnosed, and the negative is one prompt.",
		Settings:  "claude -p with --strict-mcp-config, so the run sees this checkout's server and nothing the developer has configured. Each scenario keeps its own turn budget, because picking a tool can take a step or two. Sampling is not pinned.",
		Reproduce: "make mcp-eval",
		Data:      "web/src/pages/skill-eval/_skilleval.json",
		Page:      "/skill-eval",
	},
}
