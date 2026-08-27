package main

// The MCP scenarios: the other door into kapi.
//
// A skill and an MCP server fail differently, which is why they are scored
// separately rather than averaged into one number.
//
//   - A skill is a description an agent must decide to read. The question is
//     whether it notices kapi is relevant at all, and the whole lever is the
//     `description` field.
//   - An MCP server is a tool list the agent already holds. It cannot fail to
//     notice kapi. It fails by picking the wrong one of nineteen, or by
//     reaching for a shell when a tool would have answered.
//
// So these scenarios name the tool the task should lead to, and score whether
// the agent found it. A run that solved the task some other way is recorded as
// solved-but-missed rather than as a pass: it means the tool's description is
// not carrying its own weight, which is the thing this eval exists to see.

// The catalogue the server advertises, so a scenario cannot name a tool that
// does not exist. Kept beside the scenarios rather than fetched at run time
// because a missing tool must fail a test, not a metered sweep.
var mcpToolCatalogue = []string{
	"apply_edits", "approve_unit", "check_file", "check_text", "context_search",
	"detect_format", "extract_content", "redact", "reject_unit", "review_queue",
	"review_unit", "sign_off_unit", "stats", "term-check", "translate", "up",
	"up_plan", "voice_check", "voice_rewrite",
}

// A project with a voice profile, terms and content, so the context tools have
// something real to answer from.
//
// Uses the same recipe the skill scenarios do, because both were invented with
// the same wrong keys and both silently loaded as a project with no source
// language and no targets. See loopRecipe.
const mcpRecipe = loopRecipe

func init() {
	scenarios = append(scenarios, mcpScenarios...)
}

var mcpScenarios = []Scenario{
	{
		ID:         "m01-voice-check",
		Kind:       positive,
		Surface:    surfaceMCP,
		ExpectTool: "voice_check",
		Prompt:     "Score the text in README.md against the voice profile in voice.yaml and tell me the number.",
		Path:       "mcp (voice)",
		Why:        "The plainest tool match there is. If an agent cannot find voice_check from this, the description is not doing its job.",
		Fixture: []FixtureFile{
			{As: "voice.yaml", Body: voiceProfile, Note: "forbids utilize, leverage, cutting-edge"},
			{As: "README.md", Body: "# Northwind\n\nNorthwind is a cutting-edge platform that lets teams " +
				"utilize their content across every channel.\n", Note: "two violations to score"},
		},
		Turns: 6,
	},
	{
		ID:         "m02-voice-rewrite",
		Kind:       positive,
		Surface:    surfaceMCP,
		ExpectTool: "voice_rewrite",
		Prompt:     "Rewrite the text in README.md so it complies with voice.yaml.",
		Path:       "mcp (voice)",
		Why: "Scoring and rewriting are adjacent tools with adjacent names. Reaching for voice_check here " +
			"and then editing by hand is the near miss this scenario is shaped to catch.",
		Fixture: []FixtureFile{
			{As: "voice.yaml", Body: voiceProfile},
			{As: "README.md", Body: "# Northwind\n\nLeverage our cutting-edge workspace.\n"},
		},
		// Two steps, not one: read the file, then rewrite. At six turns a pass
		// ran out of budget mid-task and scored as a wrong pick, which measured
		// the cap rather than the tool descriptions.
		Turns: 10,
	},
	{
		ID:         "m03-check-file",
		Kind:       positive,
		Surface:    surfaceMCP,
		ExpectTool: "check_file",
		Prompt:     "Check the content inside app.json for problems.",
		Path:       "mcp (checks)",
		Why: "check_text and check_file differ only in what they take. Naming a file in the prompt is the " +
			"whole signal, so an agent that reads the file itself and calls check_text has misread the pair.",
		Fixture: []FixtureFile{
			{As: "app.json", From: "harness/demos/05-ai-checks-guardrail/fixtures/app.json",
				Note: "a real catalog with placeholders to check"},
		},
		Turns: 6,
	},
	{
		ID:         "m04-detect-format",
		Kind:       positive,
		Surface:    surfaceMCP,
		ExpectTool: "detect_format",
		Prompt:     "What format is mystery.dat, and can kapi read it?",
		Path:       "mcp (formats)",
		Why: "A file with a meaningless extension and JSON inside. `file` answers 'ASCII text', which is true " +
			"and useless; the tool answers which reader would handle it. An agent could also just open it and " +
			"see, so this scores tool choice rather than capability.",
		Fixture: []FixtureFile{
			{As: "mystery.dat", Body: "{\n  \"greeting\": \"Sign in to your workspace\",\n  \"cta\": \"Get started\"\n}\n",
				Note: "JSON wearing the wrong extension"},
		},
		Turns: 5,
	},
	{
		ID:         "m05-extract-content",
		Kind:       positive,
		Surface:    surfaceMCP,
		ExpectTool: "extract_content",
		Prompt:     "Pull the readable text out of proposal.docx so I can look at it.",
		Path:       "mcp (read)",
		Why: "A binary, and one the agent can in fact read another way: unzipping a .docx and pulling the XML " +
			"works. What is scored is whether it reaches the tool that does it in one step.",
		Fixture: []FixtureFile{
			{As: "proposal.docx", From: fxDocx},
		},
		Turns: 5,
	},
	{
		ID:         "m06-stats",
		Kind:       positive,
		Surface:    surfaceMCP,
		ExpectTool: "stats",
		Prompt:     "How much translatable content is in this project?",
		Path:       "mcp (project)",
		Why:        "A question about the project rather than a file. It needs the recipe read, which is what stats does.",
		Fixture: []FixtureFile{
			{As: "kapi.yaml", Body: mcpRecipe},
			{As: "src/locales/en.json", From: fxLocales},
		},
		Turns: 6,
	},
	{
		ID:         "m07-up-plan",
		Kind:       positive,
		Surface:    surfaceMCP,
		ExpectTool: "up_plan",
		Prompt:     "What would kapi do if I ran it here? Don't actually change anything.",
		Path:       "mcp (convergence)",
		Why: "up and up_plan are one word apart and one of them writes. The prompt says not to change " +
			"anything, so calling up here is the failure the pair exists to prevent.",
		Fixture: []FixtureFile{
			{As: "kapi.yaml", Body: mcpRecipe},
			{As: "src/locales/en.json", From: fxLocales},
		},
		Turns: 6,
	},
	{
		ID:      "m08-no-tool-needed",
		Kind:    negative,
		Surface: surfaceMCP,
		Prompt:  "Rename the variable `x` to `total` in sum.go.",
		Path:    "mcp (negative)",
		Why: "Nineteen tools in the context and none of them apply. An agent that reaches for one anyway " +
			"is being pulled by the tool list rather than the task, which is the MCP-side equivalent of a false trigger.",
		Fixture: []FixtureFile{
			{As: "sum.go", Body: "package main\n\nfunc Sum(xs []int) int {\n\tx := 0\n\tfor _, v := range xs {\n\t\tx += v\n\t}\n\treturn x\n}\n"},
		},
		Turns: 5,
	},
}
