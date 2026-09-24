package main

// Evaluation descriptions, grouped by the architecture layer they measure.
// Entries include unmeasured areas so readers can identify coverage gaps.
// Tests verify reproduction commands, page links and dataset paths.

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
		ID:     "parity",
		Title:  "Filter parity against upstream Okapi",
		Method: MethodDeterministic,
		Status: StatusPartial,
		Corpus: "The Okapi Framework's own filter test suite, mapped onto neokapi's readers.",
		Covers: "Whether a document read by neokapi produces the same content model as the Java implementation it descends from, per format.",
		Misses: "The report is written to a gitignored sandbox and has no published dashboard. Formats without an " +
			"upstream counterpart cannot be compared this way. Known failures are listed separately.",
		Reproduce: "make parity-test",
	},
	{
		ID:     "format-maturity",
		Title:  "Format maturity, L0 to L4",
		Method: MethodDeterministic,
		Status: StatusMeasured,
		Corpus: "Every registered format, scored against a fixed rubric.",
		Covers: "How far each format has been taken: read, write, round-trip, inline codes, faithful write-back.",
		Misses: "Rubric levels summarize tested capabilities. Results on your documents require separate " +
			"verification.",
		Reproduce: "/format-ops triage-score",
		Data:      "web/static/data/format-maturity.json",
		Page:      "/format-maturity",
	},
	{
		ID:     "engine-speed",
		Title:  "Engine throughput",
		Method: MethodBenchmark,
		Status: StatusMeasured,
		Corpus: "844 fixtures on the pseudo-translation path, which exercises read and write without a model, across four engines.",
		Covers: "Engine throughput without model calls. Each timing includes the count of outputs containing " +
			"pseudo-translated text, so incomplete processing is visible alongside speed.",
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
		Covers:    "Model download size, cold-start time, per-sentence inference time and resident memory.",
		Misses:    "Measured on one platform. A different runtime or accelerator moves every number.",
		Reproduce: "python3 scripts/ml-benchmark.py",
		Data:      "web/src/pages/ml-benchmark/_benchmark.json",
		Page:      "/ml-benchmark",
	},
	{
		ID:     "conversion-comparison",
		Title:  "How much of a document each converter keeps",
		Method: MethodComparative,
		Status: StatusPartial,
		Corpus: "DOCX, PPTX and XLSX documents from the Okapi Framework's integration-test resources, collected " +
			"independently of this comparison.",
		Covers: "Text-extraction completeness against reference text read directly from each document's XML " +
			"parts, together with conversion time. Results are grouped by format because the converters " +
			"support different inputs.",
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
		ID:     "context-value",
		Title:  "What context injection is worth",
		Method: MethodJudged,
		Status: StatusUnvalidated,
		Spends: true,
		Corpus: "A synthetic brand corpus with a fixed rubric, swept across models.",
		Covers: "Whether a model given voice guidance and terminology produces output that needs less correction than one that is not.",
		Misses: "Publication requires Cohen's kappa of at least 0.6 on at least 100 labelled items. " +
			"The committed judge validations have not passed that gate.",
		Settings: "The current harness sets temperature to 0, but the committed runs contain no temperature field. " +
			"Max tokens follow each model's ceiling. No seed is available from the APIs.",
		Reproduce: "make context-eval",
		Data:      "web/src/pages/context-eval/_contexteval.json",
		Page:      "/context-eval",
	},
	{
		ID:     "authoring-effect",
		Title:  "Effect of voice guidance on profile adherence",
		Method: MethodBenchmark,
		Status: StatusPartial,
		Spends: true,
		Corpus: "Six briefs about a synthetic product, each written with and without `kapi voice guide` as the " +
			"system turn. Both versions are scored against the reference profile and a contrasting profile " +
			"with opposing requirements.",
		Covers: "Whether guidance improves adherence specifically to its own profile, measured as the difference " +
			"between score changes for the reference and contrasting profiles. Guided writing gained 5.8 " +
			"points against the reference profile and lost 6.7 against the contrast, a difference of 12.5. " +
			"Passive constructions fell from 11 to 4 across the six documents, and the writing shifted to " +
			"second person.",
		Misses: "Six documents on one model. The reference gain comes entirely from reduced passive voice; " +
			"neither condition used forbidden terms. The contrast loss comes from the pronoun shift under a " +
			"profile with three forbidden terms and no patterns. These results demonstrate two specific " +
			"effects. Declared person and sentence-length fields are supplied in the guide but have no " +
			"offline scoring implementation.",
		Settings:  "Temperature 0, and the user turn is byte-identical across arms. The bare arm has no system turn at all rather than a placebo one.",
		Reproduce: "make authoring-eval",
		Data:      "web/src/pages/authoring-eval/_authoringeval.json",
		Page:      "/authoring-eval",
	},
	{
		ID:     "voice-infer-quality",
		Title:  "Whether an inferred voice profile is usable",
		Method: MethodDeterministic,
		Status: StatusPartial,
		Spends: true,
		Corpus: "Documents written to a known reference profile. The inferred profile is compared with that " +
			"reference field by field.",
		Settings:  "claude-code:sonnet, one inference over the whole corpus rather than one per file. Six minutes is the budget: a local model could not finish inside kapi's own HTTP timeout to it.",
		Covers:    "How much of the reference profile a draft recovers, field by field. 5 of 9 on claude-code:sonnet from six documents: formality, humor, sentence length, person and active voice all correct; personality a third right.",
		Misses:    "One run on one model over six documents. Two of the four misses say something about the tool rather than the model: it recovered no forbidden terms at all, which a corpus written TO a profile cannot teach since it contains no violations, and it answered `emotion` with \"calm and reassuring\" where the schema takes an enum, so the draft did not validate. Inferring what to forbid needs a corpus that breaks the rules.",
		Reproduce: "make authoring-eval",
		Data:      "web/src/pages/authoring-eval/_authoringeval.json",
		Page:      "/authoring-eval",
	},

	// --- M, Multilingual ------------------------------------------------------
	{
		ID:        "prompt-contents",
		Title:     "Prompt contents sent to the model",
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
		ID:     "batching-cost",
		Title:  "What batching costs and saves",
		Method: MethodBenchmark,
		Status: StatusMeasured,
		Spends: true,
		Corpus: "600 blocks swept across four batch sizes on two models, plus the earlier sweeps kept in the same history.",
		Covers: "Throughput and structural integrity as blocks per call rises, priced per model. Both models hold every block to 128 per call and neither can answer at 600.",
		Misses: "Throughput comparisons require matching concurrency settings, which are recorded in the dataset. " +
			"Segmentation (M-02) is not measured by this evaluation.",
		Settings: "The current harness sets temperature to 0. The committed runs record concurrency but omit " +
			"temperature. No seed is available from the APIs.",
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
		Misses:    "Ten cases is too few to separate a real precision difference from noise. The voice checks are measured separately by authoring-checks; term-check is not measured by either, and belongs to neither: its examples are XLIFF with a target, so it scores translations against approved terms rather than source prose.",
		Reproduce: "make check-eval",
		Data:      "web/src/pages/check-eval/_eval.json",
		Page:      "/check-eval",
	},
	{
		ID:     "authoring-checks",
		Title:  "Whether the voice checks find real violations",
		Method: MethodLabelled,
		Status: StatusPartial,
		Corpus: "Twelve synthetic documents about one product. Six comply with the voice profile and are used to " +
			"measure false positives. Six intentionally violate it, with 18 marked violations for measuring " +
			"recall.",
		Covers:    "Recall over the marked violations, split by how the profile expresses each rule, for the offline check and the LLM one side by side. Offline finds 94%: every prohibited pattern, every forbidden term, and none of the declared fields, which nothing offline evaluates. It raises a finding on 3 of the 6 clean documents, all from one over-broad passive regex.",
		Misses:    "The corpus is synthesized, which is disclosed in the data itself. The declared fields (active_voice, person_pov, sentence_length) have no offline implementation at all, so their row is a property of the design rather than a defect to fix. And a rule stated as a regex is only as good as the regex: every false positive here is one passive-voice pattern firing on a predicate adjective. The term row reads 13 of 13 because the reference profile declares each term's other forms, which is what `kapi voice expand` writes; a profile whose terms were typed by hand and never expanded matches the bare string only, and scores lower here than it would in a language with less inflection.",
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
		Covers:  "Whether the agent completes the task with a passing kapi check --ship or kapi check result.",
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
		Covers: "Whether an agent selects the appropriate MCP tool from the advertised list. Incorrect selections " +
			"can identify descriptions that need clearer distinctions.",
		Misses: "Seven of nineteen tools have a scenario. The review and approval tools, redaction and translate are unmeasured. " +
			"A wrong pick is recorded rather than diagnosed, and the negative is one prompt.",
		Settings:  "claude -p with --strict-mcp-config, so the run sees this checkout's server and nothing the developer has configured. Each scenario keeps its own turn budget, because picking a tool can take a step or two. Sampling is not pinned.",
		Reproduce: "make mcp-eval",
		Data:      "web/src/pages/skill-eval/_skilleval.json",
		Page:      "/skill-eval",
	},
}
