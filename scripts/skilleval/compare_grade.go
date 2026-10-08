package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
)

// Grading, separate from running.
//
// The deterministic graders read only what an attempt left behind (its final
// content files) against the project as it started, so a grading change can
// be applied to sessions already run. A house check counts a rule broken in
// the prose the attempt added: the lines a Markdown file gained and the values
// a JSON catalogue gained or changed, with code spans, code blocks and link
// targets set aside. A task's own checks read the final file whole (a rename
// update has to leave none of the old name behind) or its structure.

// CompareCheckResult is one grader's verdict on one attempt.
type CompareCheckResult struct {
	ID       string `json:"id"`
	Category string `json:"category"`
	File     string `json:"file,omitempty"`
	Pass     bool   `json:"pass"`
	Evidence string `json:"evidence,omitempty"`
}

// CompareKapiCheck is what `kapi check` reported over the attempt's work, run
// in the grader cell, where the rules are held, whatever arm the attempt ran in.
type CompareKapiCheck struct {
	Ran     bool     `json:"ran"`
	Failing int      `json:"failing"`
	Rules   []string `json:"rules"`
	Error   string   `json:"error,omitempty"`
}

// CompareGrade is one attempt's deterministic score.
type CompareGrade struct {
	Attempt CompareAttempt       `json:"attempt"`
	Checks  []CompareCheckResult `json:"checks"`
	// Violations counts the rule checks that failed; task checks are apart.
	Violations int `json:"violations"`
	// Broken lists the rule categories with at least one failed check.
	Broken []string `json:"broken"`
	// TaskDone reports that every output the task names was written and every
	// task check passed.
	TaskDone  bool             `json:"task_done"`
	KapiCheck CompareKapiCheck `json:"kapi_check"`
	// Text is what the attempt wrote, file by file, as the judges read it.
	Text string `json:"text"`
}

var (
	compareFence      = regexp.MustCompile("^\\s*(```|~~~)")
	compareInlineCode = regexp.MustCompile("`[^`]*`")
	compareLinkTarget = regexp.MustCompile(`\]\([^)]*\)`)
	compareHTMLNote   = regexp.MustCompile(`<!--.*?-->`)
)

// compareProse returns the prose of a file's text: code blocks, code spans,
// link targets and comments removed. A JSON catalogue's prose is its values.
func compareProse(name, text string) string {
	if strings.HasSuffix(name, ".json") {
		values, err := compareJSONStrings([]byte(text))
		if err == nil {
			out := []string{}
			for _, key := range compareSortedKeys(values) {
				out = append(out, values[key])
			}
			return strings.Join(out, "\n")
		}
	}
	text = compareHTMLNote.ReplaceAllString(text, " ")
	out := []string{}
	inFence := false
	for line := range strings.SplitSeq(text, "\n") {
		if compareFence.MatchString(line) {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		line = compareInlineCode.ReplaceAllString(line, " ")
		line = compareLinkTarget.ReplaceAllString(line, "]")
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

// compareAdded returns what a file gained, raw (for the judges) and as prose
// (for the graders). A new file gained all of it. A Markdown or text file
// gained the lines its baseline did not hold, counted as a multiset so a
// repeated line is counted as often as it was added. A JSON catalogue gained
// the values of keys that are new or whose value changed.
func compareAdded(name string, baseline, final []byte, isNew bool) (raw, prose string) {
	if isNew {
		return string(final), compareProse(name, string(final))
	}
	if strings.HasSuffix(name, ".json") {
		before, errBefore := compareJSONStrings(baseline)
		after, errAfter := compareJSONStrings(final)
		if errBefore == nil && errAfter == nil {
			rawLines, proseLines := []string{}, []string{}
			for _, key := range compareSortedKeys(after) {
				if value, ok := before[key]; ok && value == after[key] {
					continue
				}
				rawLines = append(rawLines, fmt.Sprintf("%q: %q", key, after[key]))
				proseLines = append(proseLines, after[key])
			}
			return strings.Join(rawLines, "\n"), strings.Join(proseLines, "\n")
		}
	}
	// Prose is taken from the final file as a whole and then narrowed to the
	// added lines, so a code fence that opened above an added line still
	// hides it.
	finalProse := strings.Split(compareProse(name, string(final)), "\n")
	finalLines := strings.Split(string(final), "\n")
	counts := map[string]int{}
	for line := range strings.SplitSeq(string(baseline), "\n") {
		counts[line]++
	}
	rawLines, proseLines := []string{}, []string{}
	proseIndex := compareProseLineMap(string(final))
	for i, line := range finalLines {
		if counts[line] > 0 {
			counts[line]--
			continue
		}
		if strings.TrimSpace(line) == "" {
			continue
		}
		rawLines = append(rawLines, line)
		if j, ok := proseIndex[i]; ok && j < len(finalProse) {
			proseLines = append(proseLines, finalProse[j])
		}
	}
	return strings.Join(rawLines, "\n"), strings.Join(proseLines, "\n")
}

// compareProseLineMap maps each line of a Markdown file to its line in the
// file's prose, leaving out fence lines and the lines inside a fence, which
// compareProse drops.
func compareProseLineMap(text string) map[int]int {
	index := map[int]int{}
	inFence := false
	prose := 0
	// compareProse replaces each comment with a space, which can join lines,
	// so the map is built on the same text it reads.
	text = compareHTMLNote.ReplaceAllString(text, " ")
	for i, line := range strings.Split(text, "\n") {
		if compareFence.MatchString(line) {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		index[i] = prose
		prose++
	}
	return index
}

func compareJSONStrings(data []byte) (map[string]string, error) {
	var doc any
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	out := map[string]string{}
	var walk func(prefix string, value any)
	walk = func(prefix string, value any) {
		switch v := value.(type) {
		case map[string]any:
			for key, child := range v {
				name := key
				if prefix != "" {
					name = prefix + "." + key
				}
				walk(name, child)
			}
		case []any:
			for i, child := range v {
				walk(fmt.Sprintf("%s[%d]", prefix, i), child)
			}
		case string:
			out[prefix] = v
		}
	}
	walk("", doc)
	return out, nil
}

func compareSortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// compareExempt reports whether a file sits under one of a check's exempt
// patterns: "dir/**" covers everything under dir, anything else is a
// path.Match pattern.
func compareExempt(name string, exempt []string) bool {
	for _, pattern := range exempt {
		if prefix, ok := strings.CutSuffix(pattern, "/**"); ok {
			if strings.HasPrefix(name, prefix+"/") {
				return true
			}
			continue
		}
		if ok, _ := filepath.Match(pattern, name); ok {
			return true
		}
	}
	return false
}

// gradeCompareAttempt scores one attempt's final files against the project
// as it started. It is pure.
func gradeCompareAttempt(project CompareProject, task CompareTask, attempt CompareAttempt, final map[string][]byte) CompareGrade {
	grade := CompareGrade{Attempt: attempt, Checks: []CompareCheckResult{}, Broken: []string{}}
	names := []string{}
	for name := range final {
		names = append(names, name)
	}
	for name := range project.Files {
		if _, ok := final[name]; !ok {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	added := map[string]string{}
	var text strings.Builder
	for _, name := range names {
		after, present := final[name]
		before, existed := project.Files[name]
		if !present || (existed && string(before) == string(after)) {
			continue
		}
		raw, prose := compareAdded(name, before, after, !existed)
		added[name] = prose
		label := "changed"
		if !existed {
			label = "new"
		}
		fmt.Fprintf(&text, "### %s (%s)\n\n%s\n\n", name, label, strings.TrimSpace(raw))
	}
	grade.Text = strings.TrimSpace(text.String())
	// House checks over every file's added prose.
	for _, check := range project.Checks {
		pattern := regexp.MustCompile(check.Pattern)
		result := CompareCheckResult{ID: check.ID, Category: check.Category, Pass: true}
		for _, name := range compareSortedKeys(added) {
			if compareExempt(name, check.Exempt) {
				continue
			}
			if match := pattern.FindString(added[name]); match != "" {
				result.Pass = false
				result.File = name
				result.Evidence = match
				break
			}
		}
		grade.Checks = append(grade.Checks, result)
	}
	// The task's own checks.
	for _, check := range task.Checks {
		grade.Checks = append(grade.Checks, compareTaskCheck(check, project, final))
	}
	grade.TaskDone = true
	for _, output := range task.Outputs {
		after, present := final[output]
		if !present || string(after) == string(project.Files[output]) {
			grade.TaskDone = false
		}
	}
	for _, result := range grade.Checks {
		if result.Pass {
			continue
		}
		if result.Category == compareCategoryTask {
			grade.TaskDone = false
			continue
		}
		grade.Violations++
		grade.Broken = pairedUnique(grade.Broken, result.Category)
	}
	slices.Sort(grade.Broken)
	return grade
}

func compareTaskCheck(check CompareCheck, project CompareProject, final map[string][]byte) CompareCheckResult {
	result := CompareCheckResult{ID: check.ID, Category: check.Category, File: check.File, Pass: true}
	data, present := final[check.File]
	if !present {
		result.Pass, result.Evidence = false, "file missing"
		return result
	}
	text := string(data)
	if !check.Raw {
		text = compareProse(check.File, text)
	}
	switch check.Kind {
	case compareCheckForbidFinal:
		if match := regexp.MustCompile(check.Pattern).FindString(text); match != "" {
			result.Pass, result.Evidence = false, match
		}
	case compareCheckRequireFinal:
		if !regexp.MustCompile(check.Pattern).MatchString(text) {
			result.Pass, result.Evidence = false, "not found: "+check.Pattern
		}
	case compareCheckJSONValid:
		var doc any
		if err := json.Unmarshal(data, &doc); err != nil {
			result.Pass, result.Evidence = false, err.Error()
		}
	case compareCheckJSONKeysKept:
		before, _ := compareJSONStrings(project.Files[check.File])
		after, err := compareJSONStrings(data)
		if err != nil {
			result.Pass, result.Evidence = false, err.Error()
			break
		}
		for _, key := range compareSortedKeys(before) {
			if _, ok := after[key]; !ok {
				result.Pass, result.Evidence = false, "key removed or renamed: "+key
				break
			}
		}
	case compareCheckJSONNewKeys:
		before, _ := compareJSONStrings(project.Files[check.File])
		after, err := compareJSONStrings(data)
		if err != nil {
			result.Pass, result.Evidence = false, err.Error()
			break
		}
		pattern := regexp.MustCompile(check.Pattern)
		count := 0
		for key := range after {
			if _, existed := before[key]; !existed && pattern.MatchString(key) {
				count++
			}
		}
		if count < check.Min {
			result.Pass, result.Evidence = false, fmt.Sprintf("%d new keys match, want %d", count, check.Min)
		}
	default:
		result.Pass, result.Evidence = false, "unknown check kind "+check.Kind
	}
	return result
}

// readCompareFinal reads an attempt's captured content files.
func readCompareFinal(dir string) (map[string][]byte, error) {
	final := map[string][]byte{}
	root := filepath.Join(dir, "final")
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() {
			return walkErr
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		final[filepath.ToSlash(rel)] = data
		return nil
	})
	return final, err
}

// gradeCompare grades every completed attempt of the pilot and the full run,
// writing grade.json beside each, and runs `kapi check` over each attempt's
// work in a grader cell per project.
func gradeCompare(ctx context.Context, opts CompareOptions, m CompareManifest) error {
	graders := map[string]*compareGrader{}
	graded := 0
	for _, dir := range comparePhaseDirs(opts) {
		entries, err := os.ReadDir(dir)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			attemptDir := filepath.Join(dir, entry.Name())
			var result CompareResult
			if readPairedJSON(filepath.Join(attemptDir, "result.json"), &result) != nil || !result.completed() {
				continue
			}
			task, err := findCompareTask(result.Attempt.Task)
			if err != nil {
				return err
			}
			project, err := loadCompareProject(task.Project)
			if err != nil {
				return err
			}
			final, err := readCompareFinal(attemptDir)
			if err != nil {
				return err
			}
			grade := gradeCompareAttempt(project, task, result.Attempt, final)
			grader := graders[project.Name]
			if grader == nil {
				grader, err = newCompareGrader(ctx, opts, project)
				if err != nil {
					return err
				}
				graders[project.Name] = grader
			}
			grade.KapiCheck = grader.check(ctx, final)
			if err := writeCompareJSON(filepath.Join(attemptDir, "grade.json"), grade); err != nil {
				return err
			}
			graded++
		}
	}
	fmt.Fprintf(os.Stderr, "graded %d attempts\n", graded)
	return nil
}

// compareGrader is a kapi-arm cell no agent runs in, holding the project's
// rules, where any attempt's files are laid over the baseline and checked.
type compareGrader struct {
	project CompareProject
	paths   ComparePaths
	env     []string
	base    string
	kapi    string
}

func newCompareGrader(ctx context.Context, opts CompareOptions, project CompareProject) (*compareGrader, error) {
	paths, wiring, err := prepareCompareCell(ctx, filepath.Join(opts.CellsDir, "grader", project.Name), project, compareArmKapi, opts.KapiBin)
	if err != nil {
		return nil, err
	}
	if _, err := compareGateProbe(ctx, paths, project, wiring.Baseline); err != nil {
		return nil, err
	}
	return &compareGrader{project: project, paths: paths, env: compareEnv(paths, project.Name), base: wiring.Baseline, kapi: filepath.Join(paths.Bin, "kapi")}, nil
}

func (g *compareGrader) check(ctx context.Context, final map[string][]byte) CompareKapiCheck {
	result := CompareKapiCheck{Rules: []string{}}
	restore := func() {
		_, _ = compareRunIn(ctx, g.paths.Repo, g.env, "", "git", "checkout", "--quiet", "--", ".")
		_, _ = compareRunIn(ctx, g.paths.Repo, g.env, "", "git", "clean", "-fdq")
	}
	defer restore()
	for name := range g.project.Files {
		if _, ok := final[name]; !ok {
			_ = os.Remove(filepath.Join(g.paths.Repo, filepath.FromSlash(name)))
		}
	}
	for name, data := range final {
		target := filepath.Join(g.paths.Repo, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			result.Error = err.Error()
			return result
		}
		if err := os.WriteFile(target, data, 0o600); err != nil {
			result.Error = err.Error()
			return result
		}
	}
	out, _ := compareRunIn(ctx, g.paths.Repo, g.env, "", g.kapi, "check", "--diff-against", g.base, "--json")
	check := evalParseCheck([]byte(out))
	result.Ran, result.Error = check.Ran, check.Error
	for _, finding := range check.failing() {
		result.Failing++
		label := finding.Rule
		if finding.Term != "" {
			label += " " + finding.Term
		}
		result.Rules = pairedUnique(result.Rules, label)
	}
	return result
}
