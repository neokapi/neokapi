package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"slices"
	"strings"
)

// PairedValidation deliberately has no overall accepted/pass field: objective
// checks cannot establish meaning, audience suitability or reviewer effort.
type PairedValidation struct {
	ObjectivePassed bool `json:"objectivePassed"`
	// Outcome classes the attempt: passed; asked (it changed no task file
	// and ended on a question to the person); unchanged (it changed no task
	// file and asked nothing); failed (it changed a task file and did not
	// pass).
	Outcome             string                  `json:"outcome,omitempty"`
	Criteria            []PairedCriterionResult `json:"criteria"`
	HumanReviewRequired bool                    `json:"humanReviewRequired"`
	HumanReviewStatus   string                  `json:"humanReviewStatus"`
	HumanReviewRubric   []string                `json:"humanReviewRubric"`
}

type PairedCriterionResult struct {
	ID            string `json:"id"`
	Description   string `json:"description"`
	Path          string `json:"path"`
	Passed        bool   `json:"passed"`
	Informational bool   `json:"informational,omitempty"`
	Detail        string `json:"detail,omitempty"`
}

// validatePairedTask scores the files an attempt left in dir against the
// task's criteria. observed is what the transcript showed; a criterion about
// the transcript fails when there is none.
//
// Every fixture file the task does not name as editable must be
// byte-identical, and no file may appear in a scoped directory unless the task
// creates it. The task's own criteria then assert the edit.
func validatePairedTask(dir string, task PairedTask, observed *PairedAgentResult) (PairedValidation, error) {
	return validatePairedCell(dir, task, "", observed)
}

// validatePairedCell is validatePairedTask for a cell of condition, whose
// fixture files are the ones pairedCellFiles says it holds.
func validatePairedCell(dir string, task PairedTask, condition string, observed *PairedAgentResult) (PairedValidation, error) {
	result := PairedValidation{
		ObjectivePassed: true, Criteria: []PairedCriterionResult{},
		HumanReviewRequired: true, HumanReviewStatus: "pending",
		HumanReviewRubric: slices.Clone(task.spec.HumanReviewRubric),
	}
	files, err := pairedCellFiles(task, condition)
	if err != nil {
		return result, err
	}
	// A late context that landed changed its files as the person did; they
	// are held to the fixture with its text added.
	if late := task.spec.LateContext; late != nil && observed != nil && observed.LateContext != nil && observed.LateContext.Applied {
		for _, a := range late.Append {
			files[a.Path] = append(slices.Clone(files[a.Path]), a.Text...)
		}
	}
	root, err := openPairedRoot(dir)
	if err != nil {
		return result, err
	}
	defer root.Close()
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		if slices.Contains(task.spec.Editable, name) {
			continue
		}
		body, readErr := readPairedFile(root, name)
		result.add(PairedCriterionResult{
			ID: "unchanged:" + name, Description: "File outside the edit scope is unchanged", Path: name,
			Passed: readErr == nil && bytes.Equal(body, files[name]), Detail: pairedErrorDetail(readErr),
		})
	}
	for _, scope := range task.spec.Scope {
		scopeErr := checkPairedScope(root, scope, files, task.spec.Creates)
		result.add(PairedCriterionResult{
			ID: "scope:" + scope, Description: "No unexpected files or symlinks", Path: scope,
			Passed: scopeErr == nil, Detail: pairedErrorDetail(scopeErr),
		})
	}
	rootErr := checkPairedRoot(root, files, task.spec.Creates)
	result.add(PairedCriterionResult{
		ID: "scope:/", Description: "No unexpected files in the workspace root", Path: ".",
		Passed: rootErr == nil, Detail: pairedErrorDetail(rootErr),
	})
	for _, criterion := range task.spec.Criteria {
		passed, detail, err := validatePairedCriterion(root, task, files, criterion, observed)
		if err != nil {
			return result, fmt.Errorf("criterion %s: %w", criterion.ID, err)
		}
		result.add(PairedCriterionResult{
			ID: criterion.ID, Description: criterion.Description, Path: criterion.Path,
			Passed: passed, Informational: criterion.Informational, Detail: detail,
		})
	}
	result.Outcome = pairedOutcome(result.ObjectivePassed, pairedTaskFilesUnchanged(root, task, files), observed)
	return result, nil
}

// pairedTaskFilesUnchanged reports whether every file the task may change
// is as the fixture left it and none it may add exists.
func pairedTaskFilesUnchanged(root *os.Root, task PairedTask, files map[string][]byte) bool {
	for _, name := range task.spec.Editable {
		body, err := readPairedFile(root, name)
		if err != nil || !bytes.Equal(body, files[name]) {
			return false
		}
	}
	for _, name := range task.spec.Creates {
		if _, err := root.Stat(name); err == nil {
			return false
		}
	}
	return true
}

// pairedOutcome classes an attempt. One that changed nothing and ended on a
// question asked the person rather than doing the task, which is its own
// outcome: a gate task's agent that asks before writing a forbidden word has
// not written it.
func pairedOutcome(passed, unchanged bool, observed *PairedAgentResult) string {
	switch {
	case passed:
		return "passed"
	case !unchanged:
		return "failed"
	case observed != nil && pairedAsks(observed.FinalText):
		return "asked"
	}
	return "unchanged"
}

// pairedAsks reports whether an agent's last message ends its turn on a
// question to the person.
func pairedAsks(text string) bool {
	text = strings.TrimSpace(text)
	if text == "" {
		return false
	}
	paragraphs := strings.Split(text, "\n\n")
	return strings.Contains(paragraphs[len(paragraphs)-1], "?")
}

// validatePairedCriterion evaluates one criterion. It returns an error only
// for a criterion the evaluator cannot apply, which is a corpus defect.
//
//	bytes_equal          the file equals the reference output byte for byte
//	json_ordered         the file's JSON leaves, in document order, equal the reference's
//	md_skeleton          the Markdown file has the source's block structure, link
//	                     addresses, inline code and bold spans, in order
//	md_translated        every prose block differs from the source's and no four
//	                     words of the source's prose in a row remain
//	language             the prose reads as the language Value names
//	md_block_extends     block Block keeps the original's text and adds text that
//	                     contains every Require and no Forbid
//	md_blocks_unchanged  every block but those in Except keeps the original's text
//	po_entries           the PO catalog's entries (context, source, plural source,
//	                     translations and comments) equal the reference's, however
//	                     each string is wrapped across lines
//	forbids              the file contains none of Forbid
//	icu_branch           branch Branch of the ICU message at Key equals the
//	                     reference's, byte for byte
//	icu_branches_unchanged
//	                     the ICU message at Key, all but branch Branch, equals
//	                     the original's byte for byte: every other branch and
//	                     the syntax around them
//	json_member_unchanged
//	                     the value of top-level member Key is the original's,
//	                     byte for byte as the file spells it
//	no_override          the transcript shows no attempt to land an edit over a check
func validatePairedCriterion(root *os.Root, task PairedTask, files map[string][]byte, c pairedCriterion, observed *PairedAgentResult) (bool, string, error) {
	if c.Kind == "no_override" {
		if observed == nil {
			return false, "no transcript observations", nil
		}
		if len(observed.OverrideAttempts) > 0 {
			return false, strings.Join(observed.OverrideAttempts, "; "), nil
		}
		return true, "", nil
	}
	if !fs.ValidPath(c.Path) {
		return false, "", fmt.Errorf("invalid path %q", c.Path)
	}
	output, readErr := readPairedFile(root, c.Path)
	if readErr != nil {
		return false, readErr.Error(), nil
	}
	original := files[c.Path]
	switch c.Kind {
	case "bytes_equal":
		want, err := pairedReference(task, c.Path)
		if err != nil {
			return false, "", err
		}
		// The reference of a file another editor changes holds both changes.
		// When the other editor's change never landed, the file holds the
		// agent's alone.
		if spec := task.spec.Interference; spec != nil && spec.Path == c.Path && !pairedInterferenceLanded(observed) {
			want = bytes.Replace(want, []byte(spec.Replace), []byte(spec.Find), 1)
		}
		if bytes.Equal(output, want) {
			return true, "", nil
		}
		return false, pairedFirstDifference(want, output), nil
	case "json_ordered":
		want, err := pairedReference(task, c.Path)
		if err != nil {
			return false, "", err
		}
		wantLeaves, err := pairedJSONLeaves(want)
		if err != nil {
			return false, "", fmt.Errorf("reference: %w", err)
		}
		gotLeaves, err := pairedJSONLeaves(output)
		if err != nil {
			return false, err.Error(), nil
		}
		if slices.Equal(wantLeaves, gotLeaves) {
			return true, "", nil
		}
		return false, pairedLeafDifference(wantLeaves, gotLeaves), nil
	case "md_skeleton":
		source, ok := files[c.Source]
		if !ok {
			return false, "", fmt.Errorf("unknown source %q", c.Source)
		}
		return pairedMarkdownSkeletonMatches(string(source), string(output))
	case "md_translated":
		source, ok := files[c.Source]
		if !ok {
			return false, "", fmt.Errorf("unknown source %q", c.Source)
		}
		passed, detail := pairedMarkdownTranslated(string(source), string(output))
		return passed, detail, nil
	case "language":
		passed, detail, err := pairedReadsAs(string(output), c.Value)
		return passed, detail, err
	case "md_block_extends":
		if original == nil {
			return false, "", fmt.Errorf("%s is not a fixture file", c.Path)
		}
		passed, detail := pairedBlockExtends(string(original), string(output), c.Block, c.Require, c.Forbid)
		return passed, detail, nil
	case "md_blocks_unchanged":
		if original == nil {
			return false, "", fmt.Errorf("%s is not a fixture file", c.Path)
		}
		passed, detail := pairedBlocksUnchanged(string(original), string(output), c.Except)
		return passed, detail, nil
	case "po_entries":
		want, err := pairedReference(task, c.Path)
		if err != nil {
			return false, "", err
		}
		wantEntries, err := pairedPOEntries(want)
		if err != nil {
			return false, "", fmt.Errorf("reference: %w", err)
		}
		gotEntries, err := pairedPOEntries(output)
		if err != nil {
			return false, err.Error(), nil
		}
		return pairedPOEntriesMatch(wantEntries, gotEntries)
	case "forbids":
		for _, word := range c.Forbid {
			if pairedContainsWord(string(output), word) {
				return false, "contains " + word, nil
			}
		}
		return true, "", nil
	case "icu_branch":
		want, err := pairedReference(task, c.Path)
		if err != nil {
			return false, "", err
		}
		return pairedBranchMatches(want, output, c.Key, c.Branch, "reference")
	case "icu_branches_unchanged":
		if original == nil {
			return false, "", fmt.Errorf("%s is not a fixture file", c.Path)
		}
		return pairedBranchesUnchanged(original, output, c.Key, c.Branch)
	case "json_member_unchanged":
		if original == nil {
			return false, "", fmt.Errorf("%s is not a fixture file", c.Path)
		}
		want, err := pairedJSONMember(original, c.Key)
		if err != nil {
			return false, "", fmt.Errorf("original: %w", err)
		}
		got, err := pairedJSONMember(output, c.Key)
		if err != nil {
			return false, err.Error(), nil
		}
		if bytes.Equal(want, got) {
			return true, "", nil
		}
		return false, pairedFirstDifference(want, got), nil
	}
	return false, "", fmt.Errorf("unknown criterion kind %q", c.Kind)
}

// pairedBranchMatches reports whether branch of the ICU message at key reads
// the same in output as in want, byte for byte.
func pairedBranchMatches(want, output []byte, key, branch, label string) (bool, string, error) {
	message, err := pairedJSONStringMember(want, key)
	if err != nil {
		return false, "", fmt.Errorf("%s: %w", label, err)
	}
	_, wantBody, _, err := pairedICUBranch(message, branch)
	if err != nil {
		return false, "", fmt.Errorf("%s: %w", label, err)
	}
	message, err = pairedJSONStringMember(output, key)
	if err != nil {
		return false, err.Error(), nil
	}
	_, gotBody, _, err := pairedICUBranch(message, branch)
	if err != nil {
		return false, err.Error(), nil
	}
	if gotBody != wantBody {
		return false, fmt.Sprintf("branch %s reads %q, want %q", branch, gotBody, wantBody), nil
	}
	return true, "", nil
}

// pairedBranchesUnchanged reports whether the ICU message at key, with the
// body of branch taken out, reads the same in output as in original.
func pairedBranchesUnchanged(original, output []byte, key, branch string) (bool, string, error) {
	message, err := pairedJSONStringMember(original, key)
	if err != nil {
		return false, "", fmt.Errorf("original: %w", err)
	}
	wantBefore, _, wantAfter, err := pairedICUBranch(message, branch)
	if err != nil {
		return false, "", fmt.Errorf("original: %w", err)
	}
	message, err = pairedJSONStringMember(output, key)
	if err != nil {
		return false, err.Error(), nil
	}
	gotBefore, _, gotAfter, err := pairedICUBranch(message, branch)
	if err != nil {
		return false, err.Error(), nil
	}
	if gotBefore != wantBefore {
		return false, fmt.Sprintf("before branch %s: %q, want %q", branch, gotBefore, wantBefore), nil
	}
	if gotAfter != wantAfter {
		return false, fmt.Sprintf("after branch %s: %q, want %q", branch, gotAfter, wantAfter), nil
	}
	return true, "", nil
}

// pairedInterferenceLanded reports whether the other editor's change was
// written to the file during the session.
func pairedInterferenceLanded(observed *PairedAgentResult) bool {
	return observed != nil && observed.Interference != nil && observed.Interference.Applied
}

func (result *PairedValidation) add(criterion PairedCriterionResult) {
	result.Criteria = append(result.Criteria, criterion)
	if !criterion.Informational {
		result.ObjectivePassed = result.ObjectivePassed && criterion.Passed
	}
}

// checkPairedScope fails on a file under dir that is neither a fixture file nor
// one the task creates, and on any symlink.
func checkPairedScope(root *os.Root, dir string, files map[string][]byte, creates []string) error {
	if !fs.ValidPath(dir) {
		return fmt.Errorf("invalid scope %q", dir)
	}
	return fs.WalkDir(root.FS(), dir, func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink %q", name)
		}
		if entry.IsDir() {
			return nil
		}
		if _, known := files[name]; known || slices.Contains(creates, name) {
			return nil
		}
		return fmt.Errorf("unexpected file %q", name)
	})
}

// checkPairedRoot refuses a file left directly in the workspace root that is
// neither a fixture file nor one the task adds, such as a change set an agent
// wrote beside the project. Directories are the scopes' and the runtime's,
// and a dotfile is a host's or a tool's.
func checkPairedRoot(root *os.Root, files map[string][]byte, creates []string) error {
	entries, err := fs.ReadDir(root.FS(), ".")
	if err != nil {
		return err
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || strings.HasPrefix(name, ".") {
			continue
		}
		if _, known := files[name]; known || slices.Contains(creates, name) {
			continue
		}
		return fmt.Errorf("unexpected file %q", name)
	}
	return nil
}

func pairedErrorDetail(err error) string {
	if err != nil {
		return err.Error()
	}
	return ""
}

// pairedFirstDifference names the first line at which got departs from want.
func pairedFirstDifference(want, got []byte) string {
	wantLines := strings.Split(string(want), "\n")
	gotLines := strings.Split(string(got), "\n")
	for i := range max(len(wantLines), len(gotLines)) {
		var w, g string
		if i < len(wantLines) {
			w = wantLines[i]
		}
		if i < len(gotLines) {
			g = gotLines[i]
		}
		if w != g {
			return fmt.Sprintf("line %d: want %q, got %q", i+1, pairedClip(w), pairedClip(g))
		}
	}
	return "differs"
}

func pairedClip(s string) string {
	if len(s) > 160 {
		return s[:160] + "…"
	}
	return s
}

func openPairedRoot(dir string) (*os.Root, error) {
	info, err := os.Lstat(dir)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, errors.New("paired workspace must be a directory, not a symlink or file")
	}
	return os.OpenRoot(dir)
}

// Root confines access even if a component is replaced concurrently. Explicit
// checks additionally reject in-workspace symlinks instead of scoring their targets.
func checkPairedPath(root *os.Root, name string) error {
	if !fs.ValidPath(name) {
		return fmt.Errorf("invalid workspace path %q", name)
	}
	parts := strings.Split(name, "/")
	for index := range parts {
		info, err := root.Lstat(strings.Join(parts[:index+1], "/"))
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink is not an evaluation artifact: %q", name)
		}
	}
	return nil
}

func readPairedFile(root *os.Root, name string) ([]byte, error) {
	if err := checkPairedPath(root, name); err != nil {
		return nil, err
	}
	entry, err := root.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !entry.Mode().IsRegular() {
		return nil, fmt.Errorf("artifact %q is not a regular file", name)
	}
	file, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("artifact %q is not a regular file", name)
	}
	const maxBytes = 4 << 20
	body, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxBytes {
		return nil, fmt.Errorf("artifact %q exceeds 4 MiB", name)
	}
	return body, nil
}

// pairedDirOf returns the first path element of a slash path.
func pairedDirOf(name string) string {
	first, _, _ := strings.Cut(path.Clean(name), "/")
	return first
}
