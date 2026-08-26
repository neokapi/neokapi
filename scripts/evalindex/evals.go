package main

// The cards. One per eval, including the ones that do not exist.
//
// Written in the order the questions ask them. An absent eval carries the same
// card as a measured one minus its data, because the useful thing about a gap
// is the shape of what is missing rather than the fact of it.
//
// Every Reproduce, Page and Data here is checked against the repository by the
// companion test. Authoring this file the first time named four commands that
// do not exist and one page that was retired, which is the argument for the
// test rather than against the registry.

var evals = []Eval{
	// --- Does content survive being read and written back? --------------------
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
		ID:        "kbf-conformance",
		Title:     "KBF conformance across engines",
		Method:    MethodDeterministic,
		Status:    StatusPartial,
		Corpus:    "The KBF conformance suite, run against the Go engine compiled to WebAssembly and the TypeScript mirror.",
		Covers:    "That two independent implementations of the bundle format agree, operation by operation, in your browser.",
		Misses:    "Runs only when someone opens the page. Nothing in CI executes it, so it reports rather than gates. The structural and envelope checks run on the Go engine alone.",
		Reproduce: "open /kbf-tests",
		Page:      "/kbf-tests",
	},

	// --- Is approved wording reused correctly? --------------------------------
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
		Reproduce: "make prior-ab-eval",
		Data:      "web/src/pages/coordinate/_abeval.json",
		Page:      "/coordinate",
		// Validation is deliberately empty: see Status.
	},

	// --- Does governance reach the model? -------------------------------------
	{
		ID:        "context-value",
		Title:     "What context injection is worth",
		Method:    MethodJudged,
		Status:    StatusUnvalidated,
		Spends:    true,
		Corpus:    "A synthetic brand corpus with a fixed rubric, swept across models.",
		Covers:    "Whether a model given voice guidance and terminology produces output that needs less correction than one that is not.",
		Misses:    "Judge-to-human agreement is gated at 30 labelled items; the literature puts the useful floor at 100 to 200, and no sweep has cleared even the lower bar.",
		Reproduce: "make context-eval",
		Data:      "web/src/pages/context-eval/_contexteval.json",
		Page:      "/context-eval",
	},
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

	// --- Do the checks find violations? ---------------------------------------
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

	// --- Is authoring governed? -----------------------------------------------
	{
		ID:     "authoring-checks",
		Title:  "Whether source checks find real violations",
		Method: MethodLabelled,
		Status: StatusAbsent,
		Corpus: "Needs a labelled corpus of source content carrying known voice and terminology violations.",
		Covers: "Would measure term-check, voice-check and voice-vocab-check the way check-accuracy measures the other two.",
		Misses: "Not built. These three checks ship and nothing measures whether they are right.",
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

	// --- What does it cost? ---------------------------------------------------
	{
		ID:        "batching-cost",
		Title:     "What batching costs and saves",
		Method:    MethodBenchmark,
		Status:    StatusMeasured,
		Spends:    true,
		Corpus:    "A fixed corpus swept across batch sizes and models.",
		Covers:    "Throughput and structural integrity as blocks per call rises, priced per model.",
		Misses:    "Concurrency is recorded because throughput depends on it, so two models swept at different concurrencies cannot be raced on speed.",
		Reproduce: "make batch-eval",
		Data:      "web/src/pages/batch-eval/_batcheval.json",
		Page:      "/batch-eval",
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
		ID:        "engine-speed",
		Title:     "Engine throughput",
		Method:    MethodBenchmark,
		Status:    StatusMeasured,
		Corpus:    "844 fixtures on the pseudo-translation path, which exercises read and write without a model.",
		Covers:    "How fast the engine moves content when no provider is in the way, per format and per engine.",
		Misses:    "Isolates the engine deliberately, so it says nothing about a run that calls a model.",
		Reproduce: "make bench-stress",
		Data:      "web/static/data/pseudobench.json",
		Page:      "/pseudobench",
	},

	// --- Where do we stand? ---------------------------------------------------
	{
		ID:     "conversion-comparison",
		Title:  "kapi against other converters",
		Method: MethodComparative,
		Status: StatusAbsent,
		Corpus: "Needs a document corpus and independent ground truth, meaning rendered pages rather than another tool's output.",
		Covers: "Would measure conversion speed and fidelity against markitdown, pandoc, docling, unstructured and LibreOffice.",
		Misses: "Not built here. A harness of this shape exists in the anydoc repository and already includes kapi in its speed tables; neokapi publishes nothing of its own.",
	},
}
