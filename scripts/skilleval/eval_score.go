package main

import (
	"path/filepath"
	"slices"
	"strings"
)

// Scoring, separate from running.
//
// Every function here is pure: it takes what a run left behind (the answer
// key, the fixture, a check report, a context log, a transcript) and returns a
// verdict. Nothing calls a model, and nothing judges by reading prose the way a
// person would. Where a judgement is needed, such as whether a record outside
// the key is harmless, the rule is stated here in code and in the runbook, so
// two people scoring the same saved run get the same number.

// Pass bars, as the design sets them.
const (
	// Measure 1: share of applicable rules followed in the first saved version.
	evalApplyFirstBar = 0.9
	// Measure 2: share of the seven names and spellings recorded, on average
	// over a host's runs, and share of records that match the key or are
	// harmless.
	evalGrowRecallBar    = 0.5
	evalGrowPrecisionBar = 0.8
)

// ── Measure 1: agents apply context ─────────────────────────────────────────

// EvalApplyScore is one run's result on Measure 1.
type EvalApplyScore struct {
	// Applicable are the conventions the work gave occasion to follow, in the
	// first saved version and in the final one.
	ApplicableFirst []string `json:"applicable_first"`
	ApplicableFinal []string `json:"applicable_final"`
	// BrokenFirst and BrokenFinal are the conventions with a failing finding.
	BrokenFirst []string `json:"broken_first"`
	BrokenFinal []string `json:"broken_final"`
	// FollowedFirst are the applicable conventions the first version kept.
	FollowedFirst []string `json:"followed_first"`
	// FailingFirst and FailingFinal count every failing finding, whether or not
	// it traces to a convention.
	FailingFirst int `json:"failing_first"`
	FailingFinal int `json:"failing_final"`
	// RanCheck reports any check call; CheckedBeforeDone reports a check after
	// the session's last write.
	RanCheck          bool `json:"ran_check"`
	CheckedBeforeDone bool `json:"checked_before_done"`
	// Wrote reports whether the session changed any file at all.
	Wrote bool `json:"wrote"`
}

// scoreEvalApply scores one run from its two versions' added text and checks.
func scoreEvalApply(key EvalKey, firstAdded, finalAdded string, first, final EvalCheck, transcript EvalTranscript) EvalApplyScore {
	score := EvalApplyScore{
		ApplicableFirst: []string{}, ApplicableFinal: []string{}, BrokenFirst: []string{},
		BrokenFinal: []string{}, FollowedFirst: []string{},
		Wrote: strings.TrimSpace(finalAdded) != "" || strings.TrimSpace(firstAdded) != "",
	}
	score.BrokenFirst = evalBrokenConventions(key, first)
	score.BrokenFinal = evalBrokenConventions(key, final)
	score.ApplicableFirst = evalApplicable(key, firstAdded, score.BrokenFirst)
	score.ApplicableFinal = evalApplicable(key, finalAdded, score.BrokenFinal)
	for _, id := range score.ApplicableFirst {
		if !slices.Contains(score.BrokenFirst, id) {
			score.FollowedFirst = append(score.FollowedFirst, id)
		}
	}
	score.FailingFirst, score.FailingFinal = len(first.failing()), len(final.failing())
	score.RanCheck, score.CheckedBeforeDone = evalCheckedBeforeDone(transcript.Calls)
	return score
}

// evalApplicable lists the planted conventions a piece of added text gave
// occasion to follow: a habit that governs all prose applies to any text; any
// other convention applies when the text uses one of its forms, either the one
// the project writes or the one it avoids. A convention with a failing finding
// applied, whatever else the text says.
func evalApplicable(key EvalKey, added string, broken []string) []string {
	out := []string{}
	for _, c := range key.planted() {
		applies := slices.Contains(broken, c.ID)
		if c.Everywhere && strings.TrimSpace(added) != "" {
			applies = true
		}
		for _, form := range append(c.terms(), c.uses()...) {
			if !applies && evalCount(added, form, true) != 0 {
				applies = true
			}
		}
		if applies {
			out = append(out, c.ID)
		}
	}
	return out
}

// evalBrokenConventions traces each failing vocabulary finding to the
// convention whose rule it names.
func evalBrokenConventions(key EvalKey, check EvalCheck) []string {
	out := []string{}
	for _, finding := range check.failing() {
		if id := evalConventionOfTerm(key, finding.Term); id != "" {
			out = pairedUnique(out, id)
		}
	}
	return out
}

// evalConventionOfTerm names the planted convention one of whose rules has this
// term, matched without case the way the check matches it.
func evalConventionOfTerm(key EvalKey, term string) string {
	if strings.TrimSpace(term) == "" {
		return ""
	}
	for _, c := range key.planted() {
		for _, rule := range c.Rules {
			if strings.EqualFold(rule.Term, term) {
				return c.ID
			}
		}
	}
	return ""
}

// evalCheckedBeforeDone reports whether a session ran any check, and whether
// one came after its last write. A session that wrote nothing and checked
// counts as having checked before it was done.
func evalCheckedBeforeDone(calls []EvalCall) (ran, beforeDone bool) {
	lastWrite, lastCheck := -1, -1
	for index, call := range calls {
		switch call.Kind {
		case evalKindWrite:
			lastWrite = index
		case evalKindCheck:
			lastCheck = index
		}
	}
	return lastCheck >= 0, lastCheck >= 0 && lastCheck > lastWrite
}

// evalAddedText is what a version added to a file: the lines of after that the
// file did not already hold. Counting lines rather than characters keeps an
// edit to a paragraph from reading as the whole paragraph being new, and keeps
// the fixture's own prose, which is already correct, out of the count.
func evalAddedText(before, after string) string {
	held := map[string]int{}
	for line := range strings.SplitSeq(before, "\n") {
		held[strings.TrimSpace(line)]++
	}
	added := []string{}
	for line := range strings.SplitSeq(after, "\n") {
		key := strings.TrimSpace(line)
		if held[key] > 0 {
			held[key]--
			continue
		}
		if key != "" {
			added = append(added, key)
		}
	}
	return strings.Join(added, "\n")
}

// ── Measure 2: agents grow context ──────────────────────────────────────────

// Record verdicts.
const (
	evalVerdictKey      = "key"
	evalVerdictDecoy    = "decoy"
	evalVerdictHarmless = "harmless"
	evalVerdictNoise    = "noise"
)

// EvalRecordScore is one agent record, placed against the key.
type EvalRecordScore struct {
	Op EvalOperation `json:"op"`
	// Conventions are the planted conventions the record matches.
	Conventions []string `json:"conventions"`
	Verdict     string   `json:"verdict"`
	Reason      string   `json:"reason"`
}

// EvalGrowScore is one run's result on Measure 2.
type EvalGrowScore struct {
	Records []EvalRecordScore `json:"records"`
	// Recalled are the names and spellings some record matched.
	Recalled []string `json:"recalled"`
	// RecallOf is the number of names and spellings in the key.
	RecallOf int `json:"recall_of"`
	// Precise counts records that match the key or are harmless.
	Precise int `json:"precise"`
	// DecoyProposed reports any record that would make the decoy a rule.
	DecoyProposed bool `json:"decoy_proposed"`
}

// recall is the share of the names and spellings this run recorded.
func (s EvalGrowScore) recall() float64 {
	if s.RecallOf == 0 {
		return 0
	}
	return float64(len(s.Recalled)) / float64(s.RecallOf)
}

// scoreEvalGrow places every agent record against the key.
func scoreEvalGrow(f EvalFixture, records []EvalOperation) EvalGrowScore {
	score := EvalGrowScore{Records: []EvalRecordScore{}, Recalled: []string{}}
	for _, c := range f.Key.planted() {
		if c.recalled() {
			score.RecallOf++
		}
	}
	for _, op := range records {
		if op.Actor != evalActorAgent || !op.bearing() {
			continue
		}
		scored := scoreEvalRecord(f, op)
		score.Records = append(score.Records, scored)
		switch scored.Verdict {
		case evalVerdictKey, evalVerdictHarmless:
			score.Precise++
		case evalVerdictDecoy:
			score.DecoyProposed = true
		}
		for _, id := range scored.Conventions {
			if c := evalConventionByID(f.Key, id); c.recalled() && scored.Verdict == evalVerdictKey {
				score.Recalled = pairedUnique(score.Recalled, id)
			}
		}
	}
	slices.Sort(score.Recalled)
	return score
}

// scoreEvalRecord places one record. The decoy is checked first: a record that
// would make it a rule is the failure Measure 2 exists to catch, whatever else
// it matches.
func scoreEvalRecord(f EvalFixture, op EvalOperation) EvalRecordScore {
	scored := EvalRecordScore{Op: op, Conventions: []string{}}
	if reason := evalDecoyProposal(f.Key.decoy(), op); reason != "" {
		scored.Verdict, scored.Reason = evalVerdictDecoy, reason
		return scored
	}
	scored.Conventions = evalRecordConventions(f.Key, op)
	if len(scored.Conventions) != 0 {
		scored.Verdict, scored.Reason = evalVerdictKey, "matches "+strings.Join(scored.Conventions, ", ")
		return scored
	}
	if harmless, reason := evalHarmless(f, op); harmless {
		scored.Verdict, scored.Reason = evalVerdictHarmless, reason
		return scored
	}
	scored.Verdict, scored.Reason = evalVerdictNoise, evalNoiseReason(f, op)
	return scored
}

// evalDirectiveWords mark an observation that tells a writer what to do,
// which makes an observation about the decoy a rule in all but name.
var evalDirectiveWords = []string{"use", "always", "never", "prefer", "should", "must", "instead", "not", "avoid", "one word", "two words"}

// evalDecoyProposal reports a record that would make the decoy a rule: a rule
// with either of its forms on either side, or an observation that names a form
// and tells a writer what to do with it. It returns why, or nothing.
func evalDecoyProposal(decoy EvalConvention, op EvalOperation) string {
	forms := []string{}
	for _, group := range decoy.Forms {
		forms = append(forms, group...)
	}
	if op.statesRule() {
		for _, form := range forms {
			if strings.EqualFold(strings.TrimSpace(op.Term), form) || strings.EqualFold(strings.TrimSpace(op.Replacement), form) {
				return "states a rule about " + form + ", which the project uses both ways"
			}
		}
		return ""
	}
	text := op.Text + " " + op.Note
	for _, form := range forms {
		if evalCount(text, form, true) == 0 {
			continue
		}
		for _, word := range evalDirectiveWords {
			if evalCount(text, word, true) != 0 {
				return "tells a writer what to do about " + form + ", which the project uses both ways"
			}
		}
	}
	return ""
}

// evalRecordConventions lists the planted conventions a record states.
//
// A rule matches a convention when its replacement is a form the convention
// writes, and its term is not that same form; when its term is a form the
// convention avoids; or, for British spelling, when term and replacement are
// the American and British spellings of one word. An observation matches when
// it names a form the convention avoids or one of the convention's cues, and,
// for a name, when it names the name and says something about how it is
// written.
func evalRecordConventions(key EvalKey, op EvalOperation) []string {
	out := []string{}
	for _, c := range key.planted() {
		if op.statesRule() {
			if evalRuleMatches(c, op.Term, op.Replacement) {
				out = append(out, c.ID)
			}
			continue
		}
		if evalObservationMatches(c, op.Text+" "+op.Note) {
			out = append(out, c.ID)
		}
	}
	return out
}

func evalRuleMatches(c EvalConvention, term, replacement string) bool {
	term, replacement = strings.TrimSpace(term), strings.TrimSpace(replacement)
	for _, use := range c.uses() {
		if strings.EqualFold(replacement, use) && term != use {
			return true
		}
	}
	for _, avoided := range c.terms() {
		if strings.EqualFold(term, avoided) {
			return true
		}
	}
	return c.ID == "british-spelling" && evalBritishPair(term, replacement)
}

func evalObservationMatches(c EvalConvention, text string) bool {
	lower := strings.ToLower(text)
	for _, cue := range c.Cues {
		if strings.Contains(lower, cue) {
			return true
		}
	}
	for _, avoided := range c.terms() {
		if avoided != "!" && evalCount(text, avoided, true) != 0 {
			return true
		}
	}
	if c.Category != evalCategoryName {
		return false
	}
	for _, name := range c.uses() {
		if evalCount(text, name, true) == 0 {
			continue
		}
		for _, cue := range evalNameCues {
			if strings.Contains(lower, cue) {
				return true
			}
		}
	}
	return false
}

// evalBritishPairs are the endings that differ between an American spelling
// and a British one.
var evalBritishPairs = [][2]string{
	{"ization", "isation"}, {"izing", "ising"}, {"ized", "ised"}, {"izes", "ises"}, {"ize", "ise"},
	{"yzing", "ysing"}, {"yzed", "ysed"}, {"yze", "yse"},
	{"eled", "elled"}, {"eling", "elling"}, {"eler", "eller"},
	{"ors", "ours"}, {"or", "our"}, {"ers", "res"}, {"er", "re"},
	{"og", "ogue"}, {"ense", "ence"},
}

// evalBritishPair reports whether us and uk are one word's American and
// British spellings, read by their ending.
func evalBritishPair(us, uk string) bool {
	us, uk = strings.ToLower(strings.TrimSpace(us)), strings.ToLower(strings.TrimSpace(uk))
	if us == "" || us == uk {
		return false
	}
	for _, pair := range evalBritishPairs {
		if strings.HasSuffix(us, pair[0]) && strings.TrimSuffix(us, pair[0])+pair[1] == uk {
			return true
		}
		if strings.Contains(us, pair[0]) && strings.Replace(us, pair[0], pair[1], 1) == uk {
			return true
		}
	}
	return false
}

// evalHarmless is the rule for a record outside the key. It is stated without a
// model, so it can be applied to a saved run and argued with.
//
// A rule is harmless when the project already obeys it and it points at
// wording the project uses: its term appears nowhere in the fixture, and its
// replacement does. Such a rule changes nothing about the existing pages and
// steers new ones towards the project's own words.
//
// An observation is harmless when it is grounded in the project: it carries
// evidence, every piece of which names a fixture file or quotes the fixture's
// own words.
func evalHarmless(f EvalFixture, op EvalOperation) (bool, string) {
	if op.statesRule() {
		termUses, _ := evalCorpusCount(f, op.Term, true)
		replacementUses, _ := evalCorpusCount(f, op.Replacement, true)
		if strings.TrimSpace(op.Term) != "" && termUses == 0 && replacementUses > 0 {
			return true, "the project never writes the term and does write the replacement"
		}
		return false, ""
	}
	if len(op.Evidence) == 0 {
		return false, ""
	}
	for _, evidence := range op.Evidence {
		if !evalGrounded(f, evidence) {
			return false, ""
		}
	}
	return true, "an observation whose evidence is in the project's own pages"
}

// evalGrounded reports evidence that names a fixture file, or quotes the
// fixture, or both, and names nothing that is not there.
func evalGrounded(f EvalFixture, evidence EvalEvidence) bool {
	if evidence.Path == "" && evidence.Quote == "" {
		return false
	}
	var inFile []byte
	if evidence.Path != "" {
		name := evalFixtureName(f, evidence.Path)
		if name == "" {
			return false
		}
		inFile = f.Files[name]
	}
	if evidence.Quote == "" {
		return true
	}
	quote := strings.Join(strings.Fields(strings.ToLower(evidence.Quote)), " ")
	if inFile != nil {
		return strings.Contains(strings.Join(strings.Fields(strings.ToLower(string(inFile))), " "), quote)
	}
	for _, name := range f.evalFileNames() {
		if strings.Contains(strings.Join(strings.Fields(strings.ToLower(string(f.Files[name]))), " "), quote) {
			return true
		}
	}
	return false
}

// evalFixtureName resolves an evidence path, which an agent may write relative,
// with a leading "./", or absolute inside its cell, to a fixture file name.
func evalFixtureName(f EvalFixture, path string) string {
	slashed := filepath.ToSlash(strings.TrimSpace(path))
	slashed = strings.TrimPrefix(slashed, "./")
	// A line reference, as in "docs/plans.md:12", names the file.
	if index := strings.LastIndex(slashed, ":"); index > 0 && strings.Trim(slashed[index+1:], "0123456789-") == "" {
		slashed = slashed[:index]
	}
	for _, name := range f.evalFileNames() {
		if slashed == name || strings.HasSuffix(slashed, "/"+evalRepoName+"/"+name) {
			return name
		}
	}
	return ""
}

func evalNoiseReason(f EvalFixture, op EvalOperation) string {
	if op.statesRule() {
		if uses, _ := evalCorpusCount(f, op.Term, true); uses != 0 {
			return "the project writes the term this rule forbids"
		}
		return "the project never writes the replacement"
	}
	if len(op.Evidence) == 0 {
		return "an observation with no evidence"
	}
	return "an observation whose evidence is not in the project's pages"
}

func evalConventionByID(key EvalKey, id string) EvalConvention {
	for _, c := range key.Conventions {
		if c.ID == id {
			return c
		}
	}
	return EvalConvention{}
}
