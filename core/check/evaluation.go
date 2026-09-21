package check

import "sort"

// What a check result says it was evaluated against.
//
// A check reads its governance from a per-user workspace outside the checkout,
// where terms, voice rules and recorded decisions accumulate as people work. A
// verdict is read long after the run that produced it, from a CI log or an
// agent transcript, so the result names the state it was reached from.
//
// Nothing in the record reaches the verdict. [Report.Decide] never reads it, so
// an absent workspace, an empty context and a projection that has drifted all
// leave the verdict, the score and the gate exactly as they would be.

// Evaluation is what one check run was evaluated against: the project and the
// state of its context, the build that ran the check and the plugins that
// served it, what each analyzer covered, and the instant it all held.
type Evaluation struct {
	// At is when the run was evaluated, RFC 3339 in UTC.
	At string `json:"at"`
	// Context is the project the run read its governance from and the state
	// that project's context was in. A check of files outside any project
	// carries none.
	Context *ContextProvenance `json:"context,omitempty"`
	// Tool is the build that ran the check.
	Tool EvaluationTool `json:"tool"`
	// Plugins are the plugins that served the run, sorted by name.
	Plugins []EvaluationPlugin `json:"plugins,omitempty"`
	// Analyzers is what each analyzer covered, sorted by id. It projects
	// Execution.Analyzers, which holds one entry per analyzer and input.
	Analyzers []AnalyzerCoverage `json:"analyzers"`
}

// ContextProvenance says which project an answer came from and what state its
// context was in when the answer was read.
//
// A retrieval answer is quoted, acted on and sometimes committed, often an
// hour after it was read and by a process that has since asked the same
// question of another project. A check verdict is read the same way. Three
// facts make one answer distinguishable from another: which project answered,
// where the workspace's record of that project had got to, and whether the
// blocks kapi holds still match the files on disk.
type ContextProvenance struct {
	// Project is the project's stable identity: the recipe's `id:`, or its
	// `name:` where the recipe carries no id. It is the key everything kapi
	// records about the project is filed under.
	Project string `json:"project,omitempty"`
	// Name is the recipe's `name:`, the label a person recognises. It is left
	// out when it is the identity as well.
	Name string `json:"name,omitempty"`
	// Revision is the position the workspace's operation log had reached when
	// this answer was read. Two answers carrying one revision were read from
	// one state of the context.
	Revision int64 `json:"revision"`
	// Stale reports that the blocks this project holds were read from files
	// that have since changed, so anything counted over content (a term's use
	// count, a coverage figure) describes the files as they were.
	Stale bool `json:"stale"`
	// StaleReason says what moved, in the wording an answer's note carries.
	StaleReason string `json:"stale_reason,omitempty"`
}

// EvaluationTool is the build that ran a check.
type EvaluationTool struct {
	// Name is the program, such as kapi.
	Name string `json:"name"`
	// Version is the build's version, "dev" for one built from a working tree.
	Version string `json:"version"`
	// Commit is the commit it was built from, when the build stamped one.
	Commit string `json:"commit,omitempty"`
}

// EvaluationPlugin is one plugin that served part of a check.
type EvaluationPlugin struct {
	// Name is the plugin's declared name, such as pdfium.
	Name string `json:"name"`
	// Version is the version its manifest declares. It is empty when the run
	// reached the plugin through a discovery of its own and kapi holds no
	// manifest for it.
	Version string `json:"version,omitempty"`
	// Serves names what the plugin did for this run, sorted, each entry a kind
	// and a name: `format:pdf`, `comments:python`, `analyzer:voice.similarity`.
	Serves []string `json:"serves,omitempty"`
}

// AnalyzerCoverage rolls one analyzer's recorded executions up over the inputs
// it ran against, so a reader learns what a run covered without walking the
// per-input list.
type AnalyzerCoverage struct {
	// ID is the analyzer, such as hygiene, voice.rules or reader.validation.
	ID string `json:"id"`
	// Ran counts the inputs the analyzer evaluated, whether it reported
	// findings or a clean result.
	Ran int `json:"ran"`
	// NotRun groups the inputs the analyzer produced no usable result for, by
	// the status that says why. An analyzer that missed its canary is grouped
	// here as well: what it reported on the content cannot be read as a result.
	NotRun []AnalyzerNotRun `json:"not_run,omitempty"`
}

// AnalyzerNotRun is one reason an analyzer took some of its inputs no further,
// and the number of inputs it accounts for.
type AnalyzerNotRun struct {
	Status AnalyzerStatus `json:"status"`
	Reason string         `json:"reason,omitempty"`
	Inputs int            `json:"inputs"`
}

// AnalyzerCoverageOf projects one run's analyzer executions into the coverage
// an [Evaluation] carries. It reads the bookkeeping the run already did and
// keeps none of its own: an analyzer counts as having run exactly when its
// execution reports AnalyzerPassed or AnalyzerFindings, which is the same
// grouping the human summary counts as completed.
//
// The result is sorted by analyzer id, and each analyzer's reasons by status
// then reason, so two runs over the same content produce the same document.
func AnalyzerCoverageOf(runs []AnalyzerExecution) []AnalyzerCoverage {
	type reason struct {
		status AnalyzerStatus
		text   string
	}
	ids := make([]string, 0, len(runs))
	ran := map[string]int{}
	notRun := map[string]map[reason]int{}
	for _, r := range runs {
		if _, seen := notRun[r.ID]; !seen {
			ids = append(ids, r.ID)
			notRun[r.ID] = map[reason]int{}
		}
		switch r.Status {
		case AnalyzerPassed, AnalyzerFindings:
			ran[r.ID]++
		default:
			notRun[r.ID][reason{r.Status, r.Reason}]++
		}
	}
	sort.Strings(ids)
	out := make([]AnalyzerCoverage, 0, len(ids))
	for _, id := range ids {
		c := AnalyzerCoverage{ID: id, Ran: ran[id]}
		for r, inputs := range notRun[id] {
			c.NotRun = append(c.NotRun, AnalyzerNotRun{Status: r.status, Reason: r.text, Inputs: inputs})
		}
		sort.Slice(c.NotRun, func(i, j int) bool {
			if c.NotRun[i].Status != c.NotRun[j].Status {
				return c.NotRun[i].Status < c.NotRun[j].Status
			}
			return c.NotRun[i].Reason < c.NotRun[j].Reason
		})
		out = append(out, c)
	}
	return out
}
