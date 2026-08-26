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
		ID:     "engine-speed",
		Title:  "Engine throughput",
		Method: MethodBenchmark,
		// Not "measured". The committed dataset records one engine over 85
		// files, of which zero succeeded, so the published millisecond figure
		// is how long it takes to process nothing. See #2221.
		Status: StatusUnvalidated,
		Corpus: "85 fixtures on the pseudo-translation path, which exercises read and write without a model.",
		Covers: "How fast the engine moves content when no provider is in the way, per format and per engine.",
		Misses: "The number on the page should not be read. Its dataset records 0 of 85 files succeeding, so it times a run that produced no output, and the harness published it as a result rather than failing. " +
			"kapi pseudo-translates the same corpus correctly by hand, so this is the harness rather than the engine. " +
			"Three of the four engines it is meant to compare did not run at all.",
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
		ID:     "conversion-comparison",
		Title:  "kapi against other converters",
		Method: MethodComparative,
		Status: StatusAbsent,
		Corpus: "Needs a document corpus and independent ground truth, meaning rendered pages rather than another tool's output.",
		Covers: "Would measure conversion speed and fidelity against markitdown, pandoc, docling, unstructured and LibreOffice.",
		Misses: "Not built here. A harness of this shape exists in the anydoc repository and already includes kapi in its speed tables; neokapi publishes nothing of its own.",
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
		Settings:  "Not recorded, and not pinned. Anthropic's default sampling applies, so a re-run samples afresh.",
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
		Settings:  "Not recorded, and not pinned. Each swept model runs at its own API default.",
		Reproduce: "make context-eval",
		Data:      "web/src/pages/context-eval/_contexteval.json",
		Page:      "/context-eval",
	},
	{
		ID:     "authoring-effect",
		Title:  "Whether governed authoring changes the output",
		Method: MethodJudged,
		Status: StatusAbsent,
		Spends: true,
		Corpus: "Needs source content and a voice profile with something to say about it.",
		Covers: "Would be the authoring counterpart of reuse-effect: whether content written or rewritten under a voice profile differs, and differs in the direction the profile asks for.",
		Misses: "Not built. kapi's authoring-side claim currently rests on no measurement.",
	},
	{
		ID:     "voice-infer-quality",
		Title:  "Whether an inferred voice profile is usable",
		Method: MethodJudged,
		Status: StatusAbsent,
		Spends: true,
		Corpus: "Needs corpora with hand-authored reference profiles to compare a draft against.",
		Covers: "Would measure whether voice-infer's draft resembles what a person would have written from the same corpus.",
		Misses: "Not built, and the hardest of the three: it needs reference profiles that do not exist yet.",
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
		Corpus:    "A fixed corpus swept across batch sizes and models.",
		Covers:    "Throughput and structural integrity as blocks per call rises, priced per model.",
		Misses:    "Concurrency is recorded because throughput depends on it, so two models swept at different concurrencies cannot be raced on speed. Segmentation (M-02) is not measured at all, and it decides what a block is before any of this runs.",
		Settings:  "Not recorded, and not pinned. Concurrency is recorded; the sampling settings are not.",
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
		Misses:    "Three of the five checks are not measured at all: term-check, voice-check and voice-vocab-check. Ten cases is too few to separate a real precision difference from noise.",
		Reproduce: "make check-eval",
		Data:      "web/src/pages/check-eval/_eval.json",
		Page:      "/check-eval",
	},
	{
		ID:     "authoring-checks",
		Title:  "Whether source checks find real violations",
		Method: MethodLabelled,
		Status: StatusAbsent,
		Corpus: "Needs a labelled corpus of source content carrying known voice and terminology violations.",
		Covers: "Would measure term-check, voice-check and voice-vocab-check the way check-accuracy measures the other two.",
		Misses: "Not built. These three checks ship and nothing measures whether they are right.",
	},

	// ==========================================================================
	// Band: Agent skills
	// ==========================================================================

	// --- S, Surfaces ----------------------------------------------------------
	{
		ID:     "skill-triggering",
		Title:  "Whether the skill fires on the right tasks",
		Method: MethodScenario,
		Status: StatusMeasured,
		Spends: true,
		Local:  true,
		Corpus: "17 prompts that must load the skill and 5 that must not, three passes each, every one in its own workspace holding the files the prompt names.",
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
		ID:     "skill-completion",
		Title:  "Whether the agent finishes the job",
		Method: MethodScenario,
		Status: StatusPartial,
		Spends: true,
		Local:  true,
		Corpus: "15 of the positive scenarios driven to the end in a sandboxed kapi with isolated config, data, cache and plugins.",
		Covers: "That the agent does not merely start: it drives the loop to a green gate, with kapi check --ship or kapi check passing.",
		Misses: "Scored at catalog-gate depth by decision, so in-locale rendering is never verified. " +
			"Two scenarios are blocked on a private npm registry absent from the sandbox rather than on any skill defect.",
		Settings:  "claude -p in a sandboxed kapi, Gemini via env. Turn caps vary by scenario; sampling is not recorded.",
		Reproduce: "make skill-eval-completion",
		Data:      "web/src/pages/skill-eval/_skilleval.json",
		Page:      "/skill-eval",
	},
	{
		ID:     "mcp-surface",
		Title:  "Whether an agent picks the right MCP tool",
		Method: MethodScenario,
		Status: StatusPartial,
		Spends: true,
		Local:  true,
		Corpus: "Seven tasks with one right answer each among the nineteen tools the server advertises, plus one task no tool should answer. Three passes each.",
		Covers: "The other door into kapi. An MCP client already holds the tool list, so it cannot fail to notice kapi; it fails by reaching for the wrong tool, and a near miss names two descriptions that are not telling each other apart.",
		Misses: "Seven of nineteen tools have a scenario. The review and approval tools, redaction and translate are unmeasured. " +
			"A wrong pick is recorded rather than diagnosed, and the negative is one prompt.",
		Settings:  "claude -p with --strict-mcp-config, so the run sees this checkout's server and nothing the developer has configured. Each scenario keeps its own turn budget, because picking a tool can take a step or two. Sampling is not pinned.",
		Reproduce: "make mcp-eval",
		Data:      "web/src/pages/skill-eval/_skilleval.json",
		Page:      "/skill-eval",
	},
}
