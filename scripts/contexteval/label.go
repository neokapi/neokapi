package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"sort"
	"strings"
)

// Labelling: turning saved outputs into the human verdicts the judge is
// measured against.
//
// The plumbing to USE labels has been here since the judge was written
// (-judge-validate, per-criterion kappa, a bar the dashboard enforces). What
// was missing is the part a person can actually do: nobody hand-writes 150
// items of JSON, so the labels file stayed empty and both judged evals stayed
// unvalidated, publishing scores nobody had checked.
//
// Four things decide whether the resulting kappa means anything, and all four
// are properties of this loop rather than of the arithmetic afterwards:
//
//   - BLIND. The judge's verdict is never shown. A labeller who sees it agrees
//     with it, and the measurement becomes a measurement of suggestibility.
//   - BLIND TO CONDITION. The variant (bare or steered) is hidden too, or the
//     labeller scores the condition rather than the text.
//   - SHUFFLED, deterministically. Fatigue is real and rises through a
//     session; if the order correlates with model or variant, so does the
//     tiredness. Seeded from the corpus so a re-run presents the same order.
//   - SKIPPABLE. "I am not sure" is an answer. A forced guess is noise that
//     kappa cannot tell from disagreement, and it drags the estimate toward
//     chance in a way that looks like a bad judge.
//
// Resumable after every single answer, because the realistic session is twenty
// minutes on a train, not two hours at a desk.

// labelSession is the on-disk state of a labelling run: the same shape the
// validator reads, plus enough to resume.
type labelSession struct {
	// Note is for whoever opens the file expecting to hand-edit it.
	Note string `json:"_note"`
	// Rubric is the digest of the criteria these labels were made against.
	// Rewording a criterion changes the question, which makes older labels
	// answers to a different one.
	Rubric string `json:"rubric"`
	// Order is the shuffled item order, so a resumed session continues where
	// it stopped rather than reshuffling.
	Order []string `json:"order"`
	// Items are the labelled ones, keyed by id. Unsure items are recorded with
	// no criteria, so the loop does not offer them again.
	Items []labelItem `json:"items"`
	// Unsure lists ids the labeller passed on, kept so the count is visible
	// rather than looking like items nobody reached.
	Unsure []string `json:"unsure,omitempty"`
}

const labelSessionNote = "Human verdicts for judge validation. Written by `make judge-label`; " +
	"read by contexteval -judge-validate. Hand-editing is allowed but the loop is easier."

// candidate is one item to label, with the fields the labeller must not see
// kept separate from the ones they must.
type candidate struct {
	id          string
	target      string
	source      string
	translation string
	// hidden, and named so that a future edit which prints them is obviously
	// wrong rather than merely unfortunate.
	hiddenModel   string
	hiddenVariant string
}

// loadCandidates reads a -save-outputs file into labelling candidates.
//
// An item's id is a digest of what the labeller will actually see. That makes
// a label survive a re-run of the sweep that produced it, and it makes two
// identical translations from different models the same item, which is
// correct: the labeller is scoring text, not provenance.
func loadCandidates(path string) ([]candidate, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var log outputLog
	if err := json.Unmarshal(raw, &log); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}

	seen := map[string]bool{}
	var out []candidate
	for _, it := range log.Items {
		if strings.TrimSpace(it.Translation) == "" {
			continue
		}
		if !judgeableTarget(it.Target) {
			// The sweeps do not judge same-language items, so agreement
			// measured on them would validate the judge on a distribution it
			// never scores.
			continue
		}
		id := itemID(it.Target, it.Source, it.Translation)
		if seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, candidate{
			id: id, target: it.Target, source: it.Source, translation: it.Translation,
			hiddenModel: it.Model, hiddenVariant: it.Variant,
		})
	}
	return out, nil
}

func itemID(target, source, translation string) string {
	sum := sha256.Sum256([]byte(target + "\x00" + source + "\x00" + translation))
	return fmt.Sprintf("%x", sum[:6])
}

// shuffleDeterministic orders candidates by a hash of their id, so the sequence
// is stable across resumes and across machines while bearing no relation to
// model, variant or fixture.
func shuffleDeterministic(cands []candidate, seed string) {
	key := func(c candidate) uint64 {
		sum := sha256.Sum256([]byte(seed + "\x00" + c.id))
		return binary.BigEndian.Uint64(sum[:8])
	}
	sort.SliceStable(cands, func(i, j int) bool { return key(cands[i]) < key(cands[j]) })
}

// runLabelling drives the interactive loop.
func runLabelling(candidatesPath, sessionPath string, want int) error {
	cands, err := loadCandidates(candidatesPath)
	if err != nil {
		return err
	}
	if len(cands) == 0 {
		return errors.New("no candidates: run contexteval with -save-outputs first")
	}

	sess, err := loadSession(sessionPath)
	if err != nil {
		return err
	}
	digest := rubricDigest()
	if sess.Rubric != "" && sess.Rubric != digest {
		return fmt.Errorf(
			"this session was labelled against rubric %s and the rubric is now %s.\n"+
				"Rewording a criterion changes the question, so the old answers are answers to a different one.\n"+
				"Start a new session file, or revert the rubric",
			sess.Rubric, digest)
	}
	sess.Rubric = digest
	sess.Note = labelSessionNote

	shuffleDeterministic(cands, digest)

	done := map[string]bool{}
	for _, it := range sess.Items {
		done[it.ID] = true
	}
	for _, id := range sess.Unsure {
		done[id] = true
	}

	todo := make([]candidate, 0, len(cands))
	for _, c := range cands {
		if !done[c.id] {
			todo = append(todo, c)
		}
	}

	remaining := want - len(sess.Items)
	if remaining <= 0 {
		fmt.Printf("Already have %d labelled items, which meets the target of %d.\n", len(sess.Items), want)
		fmt.Printf("Validate with:\n  make judge-validate\n")
		return nil
	}

	printPreamble(len(sess.Items), want, len(todo))

	in := bufio.NewScanner(os.Stdin)
	criteria := rubric()
	labelled := 0

	for _, c := range todo {
		if labelled >= remaining {
			break
		}
		verdicts, action := askOne(in, c, criteria, len(sess.Items)+1, want)
		switch action {
		case actionQuit:
			return finish(sess, sessionPath, want)
		case actionUnsure:
			sess.Unsure = append(sess.Unsure, c.id)
		case actionLabelled:
			sess.Items = append(sess.Items, labelItem{
				ID: c.id, Target: c.target, Source: c.source,
				Translation: c.translation, Criteria: verdicts,
			})
			labelled++
		}
		// Saved after every answer: the realistic session is twenty minutes on
		// a train, not two hours at a desk.
		if err := saveSession(sess, sessionPath); err != nil {
			return err
		}
	}
	return finish(sess, sessionPath, want)
}

type labelAction int

const (
	actionLabelled labelAction = iota
	actionUnsure
	actionQuit
)

func printPreamble(have, want, available int) {
	fmt.Printf(`
Judge validation labelling
==========================

You will see a source sentence and a translation, and answer one yes/no
question per rubric criterion. You will NOT see the judge's verdict, the model,
or whether the translation came from the steered or the bare pass — seeing any
of those would make this a measurement of agreement with a hint rather than
with the text.

  y / n    answer
  s        skip this item (you are not sure; better than a guess)
  q        stop and save

Target: %d labelled, %d so far, %d candidates available.

`, want, have, available)
}

func askOne(in *bufio.Scanner, c candidate, criteria []Criterion, n, want int) (map[string]bool, labelAction) {
	// Labelled-so-far rather than seen-so-far: a skip is not progress toward
	// the target, and showing it as one would make the session look further
	// along than it is.
	fmt.Printf("─── labelled %d of %d ─── (%s)\n", n-1, want, c.target)
	fmt.Printf("  source:      %s\n", c.source)
	fmt.Printf("  translation: %s\n\n", c.translation)

	verdicts := map[string]bool{}
	for _, crit := range criteria {
		for {
			fmt.Printf("  %s\n  [y/n/s/q] > ", crit.Text)
			if !in.Scan() {
				return nil, actionQuit
			}
			switch strings.ToLower(strings.TrimSpace(in.Text())) {
			case "y", "yes":
				verdicts[crit.ID] = true
			case "n", "no":
				verdicts[crit.ID] = false
			case "s", "skip":
				fmt.Print("  skipped\n\n")
				return nil, actionUnsure
			case "q", "quit":
				return nil, actionQuit
			default:
				fmt.Println("  answer y, n, s to skip, or q to stop.")
				continue
			}
			break
		}
	}
	fmt.Println()
	return verdicts, actionLabelled
}

func finish(sess *labelSession, path string, want int) error {
	if err := saveSession(sess, path); err != nil {
		return err
	}
	fmt.Printf("\nSaved %d labelled items", len(sess.Items))
	if len(sess.Unsure) > 0 {
		fmt.Printf(" (%d skipped as unsure)", len(sess.Unsure))
	}
	fmt.Printf(" → %s\n", path)

	switch {
	case len(sess.Items) >= want:
		fmt.Printf("That meets the target. Measure agreement with:\n  make judge-validate\n")
	case len(sess.Items) >= MinJudgeItems:
		fmt.Printf("Above the %d-item floor, below the %d target. `make judge-label` again to continue.\n",
			MinJudgeItems, want)
	default:
		fmt.Printf("Below the %d-item floor, so kappa here would be noise. `make judge-label` again to continue.\n",
			MinJudgeItems)
	}
	return nil
}

func loadSession(path string) (*labelSession, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return &labelSession{Note: labelSessionNote}, nil
	}
	if err != nil {
		return nil, err
	}
	var s labelSession
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &s, nil
}

func saveSession(s *labelSession, path string) error {
	// Sorted so a resumed session produces a stable diff rather than a
	// reshuffled file every time.
	slices.SortStableFunc(s.Items, func(a, b labelItem) int { return strings.Compare(a.ID, b.ID) })
	slices.Sort(s.Unsure)

	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}
