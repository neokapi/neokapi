package host

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"

	"github.com/neokapi/neokapi/core/contextop"
)

// Settling a project's context by evidence (AD C-11).
//
// A suggestion becomes an established rule when a person's signal backs it and
// nothing open contradicts it. core/contextop reads that from the log; this
// file records the one signal the log cannot see by itself, a change reaching
// the default branch, and runs the settling a surface asks for.

// ContextSettleRequest settles a project's context, after recording the
// evidence of a merge when one is named.
type ContextSettleRequest struct {
	// Project is the recipe path.
	Project string
	// Merged is the range of commits that reached the default branch, in any
	// form `git diff` takes: "abc123..def456", or one commit, which stands for
	// the change it made.
	Merged string
	// PR is the pull request that merged the range, where known. Unset, it is
	// read from the last commit's subject: a squash merge's "(#412)" or a merge
	// commit's "Merge pull request #412".
	PR int
	// Merger is who merged it, where known. Unset, it is the last commit's
	// committer, or its author where the forge committed it.
	Merger string
}

// ContextSettleResult is what settling did.
type ContextSettleResult struct {
	// Range and Commit are the merge the evidence came from, when one was
	// named.
	Range  string `json:"range,omitempty"`
	Commit string `json:"commit,omitempty"`
	// Signals are the merge signals recorded, one per suggestion the change
	// wrote the preferred form for or removed a rejected form of.
	Signals []ContextOperation `json:"signals"`
	// Established names the suggestions settling established.
	Established []string `json:"established"`
}

// FormatText renders what settling did.
func (r ContextSettleResult) FormatText(w io.Writer) error {
	if r.Commit != "" {
		fmt.Fprintf(w, "Merge %s: evidence for %d suggestion(s)\n", shortCommit(r.Commit), len(r.Signals))
	}
	for _, s := range r.Signals {
		fmt.Fprintf(w, "  %s\n", s.line())
	}
	if len(r.Established) == 0 {
		_, err := fmt.Fprintln(w, "Nothing newly established.")
		return err
	}
	for _, line := range r.Established {
		if _, err := fmt.Fprintln(w, line); err != nil {
			return err
		}
	}
	return nil
}

// mergeActor is who records a merge signal: a tool acting for the merge.
var mergeActor = contextop.Actor{Kind: contextop.ActorTool, Name: "merge"}

// SettleContext records the evidence a merge carries, when one is named, and
// establishes every suggestion the log now supports. It is idempotent: the
// same range records the same signals, and a signal or an establishment the
// log already holds is not recorded again.
func (a *App) SettleContext(ctx context.Context, req ContextSettleRequest) (ContextSettleResult, error) {
	s, err := a.contextOps(ctx, req.Project)
	if err != nil {
		return ContextSettleResult{}, err
	}
	out := ContextSettleResult{Signals: []ContextOperation{}, Established: []string{}}
	if req.Merged != "" {
		out.Range = req.Merged
		signals, commit, merr := s.recordMerge(ctx, req)
		if merr != nil {
			return ContextSettleResult{}, merr
		}
		out.Commit, out.Signals = commit, signals
	}
	settled, err := s.settle(ctx)
	if err != nil {
		return ContextSettleResult{}, err
	}
	out.Established = append(out.Established, settled...)
	return out, nil
}

// recordMerge reads the change a range made and records a merge signal for
// every suggestion it bears on.
func (s *contextOpsSession) recordMerge(ctx context.Context, req ContextSettleRequest) ([]ContextOperation, string, error) {
	rng := strings.TrimSpace(req.Merged)
	head := rng
	if i := strings.LastIndex(rng, ".."); i >= 0 {
		head = strings.TrimPrefix(rng[i+2:], ".")
	} else {
		rng += "^!"
	}
	if head == "" {
		head = "HEAD"
	}
	sha, err := gitOutput(ctx, s.root, "rev-parse", "--verify", head+"^{commit}")
	if err != nil {
		return nil, "", fmt.Errorf("settle --merged %s: %w", req.Merged, err)
	}
	commit := strings.TrimSpace(string(sha))
	diff, err := gitOutput(ctx, s.root, "diff", "--no-color", "--no-ext-diff", "--relative", "-U0", rng, "--", ".")
	if err != nil {
		return nil, "", fmt.Errorf("settle --merged %s: %w", req.Merged, err)
	}
	pr, merger := req.PR, req.Merger
	if pr == 0 || merger == "" {
		p, m := s.mergeOf(ctx, commit)
		if pr == 0 {
			pr = p
		}
		if merger == "" {
			merger = m
		}
	}

	candidates, err := s.ledger.Records(ctx, contextop.Filter{Project: s.key, Subjects: true})
	if err != nil {
		return nil, "", err
	}
	files := parseDiffLines(diff)
	var out []ContextOperation
	// Oldest first, so the signals read in the order the suggestions were made.
	for i := len(candidates) - 1; i >= 0; i-- {
		r := candidates[i]
		rule, ok := r.Rule()
		if !ok || rule.Replacement == "" || r.Established || !r.Status.Advises() {
			continue
		}
		preferred, rejected := 0, 0
		use, avoid := formMatcher([]string{rule.Replacement}, rule.MatchesCase()), formMatcher(append([]string{rule.Term}, rule.Forms...), rule.MatchesCase())
		for _, f := range files {
			if _, scope := s.basisAt([]contextop.Evidence{{Path: f.path}}); !r.Scope.Covers(scope.Coordinates) {
				continue
			}
			preferred += countLines(f.added, use)
			rejected += countLines(f.removed, avoid)
		}
		if preferred == 0 && rejected == 0 {
			continue
		}
		written, aerr := s.ledger.Append(ctx, contextop.Record{
			Actor:   mergeActor,
			Kind:    contextop.KindSignal,
			Target:  r.ID,
			Project: r.Project,
			Signal: &contextop.Signal{
				Source: contextop.SignalMerge, Commit: commit, PR: pr, Merger: merger,
				Preferred: preferred, Rejected: rejected,
			},
		})
		if aerr != nil {
			return nil, "", aerr
		}
		out = append(out, ContextOperation{Record: written})
	}
	return out, commit, nil
}

var (
	squashPR = regexp.MustCompile(`\(#(\d+)\)\s*$`)
	mergePR  = regexp.MustCompile(`^Merge pull request #(\d+)`)
)

// mergeOf reads the pull request and the merger from the commit that landed.
func (s *contextOpsSession) mergeOf(ctx context.Context, commit string) (int, string) {
	raw, err := gitOutput(ctx, s.root, "log", "-1", "--format=%s%x00%cn%x00%an", commit)
	if err != nil {
		return 0, ""
	}
	fields := strings.SplitN(strings.TrimSpace(string(raw)), "\x00", 3)
	for len(fields) < 3 {
		fields = append(fields, "")
	}
	subject, committer, author := fields[0], fields[1], fields[2]
	pr := 0
	for _, re := range []*regexp.Regexp{squashPR, mergePR} {
		if m := re.FindStringSubmatch(subject); m != nil {
			pr, _ = strconv.Atoi(m[1])
			break
		}
	}
	merger := committer
	if strings.EqualFold(committer, "GitHub") || committer == "" {
		merger = author
	}
	return pr, merger
}

// diffFile is one file's added and removed lines in a diff.
type diffFile struct {
	path           string
	added, removed []string
}

// parseDiffLines reads a unified diff into each file's added and removed
// lines.
func parseDiffLines(diff []byte) []diffFile {
	var out []diffFile
	sc := bufio.NewScanner(bytes.NewReader(diff))
	sc.Buffer(make([]byte, 0, 64<<10), 16<<20)
	header := false
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "diff --git "):
			out = append(out, diffFile{})
			header = true
		case len(out) == 0:
		case strings.HasPrefix(line, "@@"):
			header = false
		case header:
			if path, ok := strings.CutPrefix(line, "+++ b/"); ok {
				out[len(out)-1].path = path
			}
		case strings.HasPrefix(line, "+"):
			out[len(out)-1].added = append(out[len(out)-1].added, line[1:])
		case strings.HasPrefix(line, "-"):
			out[len(out)-1].removed = append(out[len(out)-1].removed, line[1:])
		}
	}
	return out
}

// formMatcher matches any of the forms as a whole word.
func formMatcher(forms []string, caseSensitive bool) *regexp.Regexp {
	var alts []string
	for _, f := range forms {
		if f = strings.TrimSpace(f); f != "" {
			alts = append(alts, regexp.QuoteMeta(f))
		}
	}
	if len(alts) == 0 {
		return nil
	}
	flags := ""
	if !caseSensitive {
		flags = "(?i)"
	}
	return regexp.MustCompile(flags + `(?:^|[^\p{L}\p{N}_])(?:` + strings.Join(alts, "|") + `)(?:$|[^\p{L}\p{N}_])`)
}

// countLines counts the lines a matcher finds a form in.
func countLines(lines []string, re *regexp.Regexp) int {
	if re == nil {
		return 0
	}
	n := 0
	for _, l := range lines {
		if re.MatchString(l) {
			n++
		}
	}
	return n
}
