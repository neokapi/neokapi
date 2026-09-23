package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Measure 4: review is worth it.
//
// After the grow runs, a person reads one sheet: what each run recorded,
// grouped by the convention it matches, with the evidence each record carries.
// It is short enough to read in a few minutes, and it ends with three
// questions. The person answers them in a small YAML file beside the evidence,
// and the report records the answers. No model reads the sheet or answers for
// the person.

// evalReviewQuestions are the sheet's three questions, in order.
var evalReviewQuestions = []string{
	"How many minutes did reading this sheet take?",
	"Which records would you keep? List them by run and id.",
	"Did the records teach you anything about the fixture that was not planted?",
}

// EvalReviewAnswers is a person's answers, as they write them.
type EvalReviewAnswers struct {
	// Minutes is how long reading the sheet took.
	Minutes float64 `yaml:"minutes" json:"minutes"`
	// Keep lists, per run, the ids of the records worth keeping.
	Keep map[string][]string `yaml:"keep" json:"keep"`
	// Learned is anything the records showed about the fixture that the key
	// does not plant. Empty means nothing.
	Learned string `yaml:"learned" json:"learned"`
}

// evalReviewAnswersTemplate is what a person fills in.
const evalReviewAnswersTemplate = `# Answers to the review sheet. Save as review-answers.yaml beside the sheet.
minutes: 0          # how long reading the sheet took
keep:               # the records worth keeping, by run and id
  # grow-feature-page-claude: ["4", "7"]
learned: ""         # anything the records showed about the fixture that was not planted
`

// readEvalReviewAnswers reads a person's answers. A missing file is no answer
// yet, which the report says.
func readEvalReviewAnswers(path string) (*EvalReviewAnswers, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var answers EvalReviewAnswers
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&answers); err != nil {
		return nil, fmt.Errorf("read the review answers %s: %w", path, err)
	}
	if answers.Minutes < 0 {
		return nil, fmt.Errorf("read the review answers %s: minutes cannot be negative", path)
	}
	if answers.Keep == nil {
		answers.Keep = map[string][]string{}
	}
	return &answers, nil
}

// evalReviewGroup is one heading of the sheet.
type evalReviewGroup struct {
	Title   string
	Records []evalReviewLine
}

type evalReviewLine struct {
	Run    string
	Record EvalRecordScore
}

// evalReviewGroups arranges every grow run's records under the convention they
// match, in the key's order, then the decoy, then records outside the key.
func evalReviewGroups(key EvalKey, rows []EvalGrowRow) []evalReviewGroup {
	byID := map[string]*evalReviewGroup{}
	order := []string{}
	add := func(id, title string) {
		byID[id] = &evalReviewGroup{Title: title}
		order = append(order, id)
	}
	for _, c := range key.planted() {
		add(c.ID, c.Summary)
	}
	add(evalVerdictDecoy, "The decoy, "+key.decoy().Summary+", proposed as a rule")
	add(evalVerdictHarmless, "Outside the key, and harmless")
	add(evalVerdictNoise, "Outside the key, and noise")
	for _, row := range rows {
		for _, record := range row.Score.Records {
			line := evalReviewLine{Run: row.Session, Record: record}
			switch record.Verdict {
			case evalVerdictKey:
				for _, id := range record.Conventions {
					byID[id].Records = append(byID[id].Records, line)
				}
			default:
				byID[record.Verdict].Records = append(byID[record.Verdict].Records, line)
			}
		}
	}
	groups := []evalReviewGroup{}
	for _, id := range order {
		group := byID[id]
		sort.SliceStable(group.Records, func(i, j int) bool { return group.Records[i].Run < group.Records[j].Run })
		groups = append(groups, *group)
	}
	return groups
}

// renderEvalReviewSheet writes the sheet a person reads.
func renderEvalReviewSheet(report EvalReport) string {
	var out strings.Builder
	fmt.Fprintf(&out, "# Review sheet: %s\n\n", report.Study)
	rows := report.Grow
	total := 0
	for _, row := range rows {
		total += len(row.Score.Records)
	}
	fmt.Fprintf(&out, "%d runs recorded %d entries between them. Each entry is listed under the convention\n"+
		"it matches, with the evidence the agent gave. Read it as the person who owns these docs\n"+
		"would, then answer the three questions at the end.\n\n", len(rows), total)
	if total == 0 {
		out.WriteString("No run recorded anything, so there is nothing to review.\n\n")
	}
	for _, group := range evalReviewGroups(report.Key, rows) {
		fmt.Fprintf(&out, "## %s\n\n", group.Title)
		if len(group.Records) == 0 {
			out.WriteString("Nothing recorded.\n\n")
			continue
		}
		out.WriteString("| run | id | kind | what was recorded | evidence |\n|---|---|---|---|---|\n")
		for _, line := range group.Records {
			op := line.Record.Op
			evidence := []string{}
			for _, e := range op.Evidence {
				evidence = append(evidence, e.String())
			}
			fmt.Fprintf(&out, "| %s | %s | %s | %s | %s |\n", line.Run, op.ID, op.Kind,
				evalCell(op.Subject), evalCell(evalOr(strings.Join(evidence, "; "), "none")))
		}
		out.WriteString("\n")
	}
	out.WriteString("## Questions\n\n")
	for index, question := range evalReviewQuestions {
		fmt.Fprintf(&out, "%d. %s\n", index+1, question)
	}
	out.WriteString("\nWrite the answers to `review-answers.yaml` in this directory, in this shape, and run\n" +
		"`make eval-report` again:\n\n```yaml\n" + evalReviewAnswersTemplate + "```\n")
	return out.String()
}

// evalCell keeps a table cell on one row.
func evalCell(value string) string {
	value = strings.ReplaceAll(strings.TrimSpace(value), "|", "/")
	value = strings.Join(strings.Fields(value), " ")
	if runes := []rune(value); len(runes) > 120 {
		return string(runes[:117]) + "..."
	}
	return value
}

func evalOr(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
