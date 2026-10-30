package host

import (
	"context"
	"fmt"
	"io"
	"maps"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/contextop"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/profile/packs"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/workspace"
)

// The suggestions pass (`kapi context suggest`) reads the source blocks of
// the files the recipe declares, with no model and no network. It counts
// three things: one word written several ways, words a starter voice pack
// avoids, and the reader addressed as you.
//
// Each finding is an observation by kapi, recorded the way `kapi context
// note` records one. It fails no check until a person keeps it.
//
// A later run matches each finding to its record by what it is about.
// Unchanged files record nothing, a dropped finding stays dropped, and a
// changed one is withdrawn and filed again.

// SuggestActor is who files what the suggestions pass sees: kapi itself. An
// agent's notes carry the agent; these carry kapi.
var SuggestActor = contextop.Actor{Kind: contextop.ActorTool, Name: "kapi"}

// The kinds of finding the pass reports.
const (
	// SuggestKindSpelling is one word written in more than one way.
	SuggestKindSpelling = "spelling"
	// SuggestKindPackWords is words a starter voice pack avoids.
	SuggestKindPackWords = "pack-words"
	// SuggestKindAddress is the files addressing the reader as you.
	SuggestKindAddress = "address"
)

// What became of each finding.
const (
	// SuggestFiled is a finding recorded as a new suggestion.
	SuggestFiled = "filed"
	// SuggestUpdated is a finding the pass filed before, withdrawn and filed
	// again because the files changed what it says.
	SuggestUpdated = "updated"
	// SuggestUnchanged is a finding the log already holds as it stands.
	SuggestUnchanged = "unchanged"
	// SuggestDropped is a finding a person dropped, which the pass leaves
	// dropped.
	SuggestDropped = "dropped"
	// SuggestKnown is a finding about a word the project already has a rule
	// or a suggestion for from someone else.
	SuggestKnown = "known"
)

// ContextSuggestRequest asks for the suggestions pass over one project.
type ContextSuggestRequest struct {
	// Project is the recipe path, or the workspace key of a project with a
	// checkout on this machine.
	Project string
	// DryRun reports what the pass would file without recording anything.
	DryRun bool
}

// ContextSuggestResult is what the pass saw and what it did about it.
type ContextSuggestResult struct {
	// Project is the project's key in the workspace.
	Project string `json:"project"`
	// DryRun reports that nothing was recorded.
	DryRun bool `json:"dry_run,omitempty"`
	// Files and Blocks count what the pass read.
	Files  int `json:"files"`
	Blocks int `json:"blocks"`
	// Suggestions are the findings, in the order the pass made them.
	Suggestions []SuggestedFinding `json:"suggestions"`
}

// SuggestedFinding is one finding.
type SuggestedFinding struct {
	// Kind is what sort of finding it is (SuggestKindSpelling and its
	// neighbours).
	Kind string `json:"kind"`
	// Outcome is what became of it (SuggestFiled and its neighbours).
	Outcome string `json:"outcome"`
	// ID names the suggestion in the log, and Short is the form a person
	// types. Empty on a dry run for a finding not yet recorded.
	ID    string `json:"id,omitempty"`
	Short string `json:"short,omitempty"`
	// Sentence is the suggestion in the words the review queue shows.
	Sentence string `json:"sentence"`
	// Subject is what is recorded.
	Subject contextop.Subject `json:"subject"`
	// Forms are the spellings of a word, or the pack words found, each with
	// its counts. The first spelling of a word is the one proposed.
	Forms []SuggestedForm `json:"forms,omitempty"`
	// Pack is the starter pack a pack-words finding is about.
	Pack string `json:"pack,omitempty"`
	// Address is the count behind an address finding.
	Address *SuggestedAddress `json:"address,omitempty"`
	// Blocks and Files say where the finding was seen.
	Blocks int      `json:"blocks"`
	Files  []string `json:"files"`
	// Quote is one place it was seen.
	Quote *contextop.Evidence `json:"quote,omitempty"`
}

// SuggestedForm is one spelling of a word, or one pack word, with where the
// files use it.
type SuggestedForm struct {
	Form   string   `json:"form"`
	Count  int      `json:"count"`
	Blocks int      `json:"blocks"`
	Files  []string `json:"files"`
}

// SuggestedAddress is the count behind an address finding: of the blocks
// that use a personal pronoun, how many address the reader as you and how
// many say we.
type SuggestedAddress struct {
	You       int    `json:"you"`
	We        int    `json:"we"`
	Pronouns  int    `json:"pronouns"`
	PersonPOV string `json:"person_pov"`
}

// FormatText prints one line per finding, in the words the review queue
// uses, then where to decide.
func (r ContextSuggestResult) FormatText(w io.Writer) error {
	var b strings.Builder
	if len(r.Suggestions) == 0 {
		fmt.Fprintf(&b, "Nothing to suggest in %s across %s.\n",
			countWord(r.Blocks, "block", "blocks"), countWord(r.Files, "file", "files"))
		_, err := io.WriteString(w, b.String())
		return err
	}
	waiting := false
	for _, s := range r.Suggestions {
		label := suggestOutcomeLabel(s.Outcome, r.DryRun)
		line := fmt.Sprintf("%-14s", label)
		if s.Short != "" {
			line += s.Short + "  "
		}
		line += s.Sentence
		if detail := s.detail(); detail != "" {
			line += " (" + detail + ")"
		}
		b.WriteString(strings.TrimRight(line, " ") + "\n")
		if s.Outcome == SuggestFiled || s.Outcome == SuggestUpdated || s.Outcome == SuggestUnchanged {
			waiting = true
		}
	}
	if waiting && !r.DryRun {
		b.WriteString("\nDecide with kapi context review.\n")
	}
	_, err := io.WriteString(w, b.String())
	return err
}

// suggestOutcomeLabel is the word a line opens with.
func suggestOutcomeLabel(outcome string, dry bool) string {
	switch outcome {
	case SuggestFiled:
		if dry {
			return "Would file"
		}
		return "Filed"
	case SuggestUpdated:
		if dry {
			return "Would update"
		}
		return "Updated"
	case SuggestUnchanged:
		return "Unchanged"
	case SuggestDropped:
		return "Dropped"
	case SuggestKnown:
		return "Known"
	}
	return outcome
}

// detail is the count a spelling finding's line carries. The other kinds
// state their counts in their own sentence.
func (s SuggestedFinding) detail() string {
	if s.Kind != SuggestKindSpelling {
		return ""
	}
	parts := make([]string, 0, len(s.Forms))
	for _, f := range s.Forms {
		parts = append(parts, f.Form+" "+strconv.Itoa(f.Count))
	}
	return strings.Join(parts, ", ") + "; " + countWord(s.Blocks, "block", "blocks") + " in " + countWord(len(s.Files), "file", "files")
}

// countWord renders a count with its noun: "1 block", "3 blocks".
func countWord(n int, one, many string) string {
	return strconv.Itoa(n) + " " + pluralWord(n, one, many)
}

// SuggestFromFiles runs the suggestions pass over a project and files what it
// finds as suggestions by kapi, unless the request is a dry run.
func (a *App) SuggestFromFiles(ctx context.Context, req ContextSuggestRequest) (ContextSuggestResult, error) {
	s, err := a.contextOps(ctx, req.Project)
	if err != nil {
		return ContextSuggestResult{}, err
	}
	return s.suggestFromFiles(ctx, req.DryRun)
}

// suggestFromFiles runs the suggestions pass over the session's checkout. A
// run that is not a dry run stamps the checkout (noteSuggestPass), whatever it
// found.
func (s *contextOpsSession) suggestFromFiles(ctx context.Context, dry bool) (ContextSuggestResult, error) {
	if !s.checkedOut() {
		return ContextSuggestResult{}, fmt.Errorf("project %s has no checkout on this machine, so there are no files to read", s.key)
	}
	out := ContextSuggestResult{Project: string(s.key), DryRun: dry, Suggestions: []SuggestedFinding{}}

	corpus, err := s.suggestCorpus(ctx)
	if err != nil {
		return out, err
	}
	out.Files, out.Blocks = len(corpus.files), len(corpus.blocks)

	records, err := s.ledger.Records(ctx, contextop.Filter{Project: s.key, Subjects: true})
	if err != nil {
		return out, err
	}
	f := &suggestFiler{s: s, records: records, dry: dry, scopes: map[string]suggestPoint{}}

	findings := spellingFindings(corpus)
	if english(corpus.language) {
		if !s.bindsVoice(corpus) {
			found, err := packFindings(corpus)
			if err != nil {
				return out, err
			}
			findings = append(findings, found...)
		}
		if found, ok := addressFinding(corpus); ok {
			findings = append(findings, found)
		}
	}
	for _, fd := range findings {
		sug, err := f.file(ctx, fd)
		if err != nil {
			return out, err
		}
		out.Suggestions = append(out.Suggestions, sug)
	}
	if !dry {
		if err := s.noteSuggestPass(ctx, corpus); err != nil {
			return out, err
		}
	}
	return out, nil
}

// suggestPassStampPath is the path of the stamp that says the suggestions
// pass is settled for a checkout, in the table where `kapi store import`
// stamps each context file it read (workspace.ContextImportStamp). It begins
// with a slash, so no project-relative file path is spelled the same way. The
// pass writes it after each run, since a run that files nothing leaves
// nothing in the context log. The first look at a project (lookFirstPass)
// reads it, and writes it for a project whose log already holds rules.
const suggestPassStampPath = "/kapi context suggest"

// noteSuggestPass stamps the checkout as one the suggestions pass is settled
// for. A pass that read no blocks has seen nothing, and leaves the checkout
// unstamped.
func (s *contextOpsSession) noteSuggestPass(ctx context.Context, c suggestCorpus) error {
	if len(c.blocks) == 0 {
		return nil
	}
	return stampSuggestPass(ctx, s.ws, s.key, s.root)
}

// stampSuggestPass writes the stamp for the checkout at root. A workspace
// opened for reading records nothing.
func stampSuggestPass(ctx context.Context, ws *workspace.Workspace, key workspace.ProjectKey, root string) error {
	if ws.Describe().ReadOnly {
		return nil
	}
	return ws.NoteContextImports(ctx, workspace.ContextImportStamp{
		Project:  key,
		Checkout: normalizedCheckout(root),
		Path:     suggestPassStampPath,
	})
}

// suggestPassSettled reports whether the checkout at root carries the stamp.
func suggestPassSettled(ctx context.Context, ws *workspace.Workspace, key workspace.ProjectKey, root string) (bool, error) {
	stamps, err := ws.ContextImports(ctx, key, normalizedCheckout(root))
	if err != nil {
		return false, err
	}
	return slices.ContainsFunc(stamps, func(st workspace.ContextImportStamp) bool {
		return st.Path == suggestPassStampPath
	}), nil
}

// english reports a source language the English word lists apply to.
func english(tag string) bool {
	primary, _, _ := strings.Cut(strings.ToLower(tag), "-")
	primary, _, _ = strings.Cut(primary, "_")
	return primary == "en"
}

// suggestCorpus is the project's source content as the pass reads it.
type suggestCorpus struct {
	language string
	// files are the project-relative paths read, sorted.
	files  []string
	blocks []suggestBlock
}

// suggestBlock is one source block: the file it sits in, its text with each
// inline code standing as one ObjectReplacement rune, and its plain text for
// a quote.
type suggestBlock struct {
	file  string
	text  string
	plain string
}

// suggestCorpus reads the source blocks of every file the recipe declares as
// content, through the formats and reader settings the recipe binds, and
// from the project's block cache where a file is unchanged. A file no reader
// opens is passed over.
func (s *contextOpsSession) suggestCorpus(ctx context.Context) (suggestCorpus, error) {
	a := s.app
	a.InitRegistries()
	c := suggestCorpus{language: string(s.proj.Defaults.SourceLanguage)}
	if c.language == "" {
		c.language = a.SourceLocale()
	}
	pctx := project.NewProjectContext(s.proj, filepath.Join(s.root, "x.kapi"))
	resolved, err := pctx.ResolveContent(a.FormatReg)
	if err != nil {
		return c, fmt.Errorf("resolve the project's content: %w", err)
	}
	closeCache := a.openParseCacheDefer(s.root)
	defer closeCache()
	seen := map[string]bool{}
	for _, rf := range resolved {
		if seen[rf.Path] || rf.CommentsOnly() {
			continue
		}
		seen[rf.Path] = true
		if err := ctx.Err(); err != nil {
			return c, err
		}
		blocks, err := a.readBlocksAs(ctx, rf.Path, rf.Format, mergedFormatConfig(s.proj, rf.Format, rf.Item), c.language)
		if err != nil {
			continue
		}
		rel := filepath.ToSlash(rf.Relative)
		if rel == "" {
			rel = relSlash(s.root, rf.Path)
		}
		c.files = append(c.files, rel)
		for _, b := range blocks {
			text := model.RunsHygieneText(b.SourceRuns())
			if strings.TrimSpace(text) == "" {
				continue
			}
			c.blocks = append(c.blocks, suggestBlock{file: rel, text: text, plain: b.SourceText()})
		}
	}
	sort.Strings(c.files)
	return c, nil
}

// bindsVoice reports whether the recipe binds a voice at any file the pass
// read: on the project's defaults or on the profile a collection resolves to.
func (s *contextOpsSession) bindsVoice(c suggestCorpus) bool {
	if s.proj.Defaults.Voice != nil {
		return true
	}
	for _, file := range c.files {
		rc, err := s.proj.ResolveGovernanceFor(project.GovernancePoint{Path: file})
		if err == nil && rc != nil && rc.Voice != nil {
			return true
		}
	}
	return false
}

// suggestFinding is one thing the pass found, before it is filed.
type suggestFinding struct {
	kind string
	// key names what the finding is about, so a later run finds the
	// suggestion it filed: the letters of a word, a pack, a habit.
	key string
	// signature is what the finding says, so a later run can tell whether
	// the files changed it.
	signature string
	subject   contextop.Subject
	// note is what the record carries in its note.
	note     string
	forms    []SuggestedForm
	pack     string
	address  *SuggestedAddress
	blocks   int
	files    []string
	evidence []contextop.Evidence
	// usage is the count a spelling finding's usage signal records.
	usage *contextop.Signal
}

// The notes the pass's records carry. A record's note is how a later run
// tells a pack or an address finding it filed from any other note.
const (
	suggestNoteSpelling = "kapi context suggest: one word written in more than one way"
	suggestNoteAddress  = "kapi context suggest: the files address the reader as you"
)

// suggestNotePack is the note a pack-words finding about one pack carries,
// before the words it lists.
func suggestNotePack(pack string) string {
	return "kapi context suggest: words the " + pack + " starter pack avoids: "
}

// maxSuggestEvidence bounds the files a suggestion names as evidence. The
// counts cover every file.
const maxSuggestEvidence = 8

// --- one word written several ways ---------------------------------------

// suggestToken is one word of a block, as the spelling measure reads it.
type suggestToken struct {
	word string
	// pairs reports that the next token follows with only white space
	// between, so the two can be read as one two-word phrase.
	pairs bool
}

// suggestTokens splits a block's text into the words the spelling measure
// counts. A word is letters, with single hyphens inside it. Anything else
// (a number, a URL, a path, an address, a contraction, a code span) is not a
// word, and no phrase reaches across it or across punctuation.
func suggestTokens(text string) []suggestToken {
	var out []suggestToken
	breakPair := func() {
		if len(out) > 0 {
			out[len(out)-1].pairs = false
		}
	}
	for _, chunk := range strings.FieldsFunc(text, func(r rune) bool {
		return unicode.IsSpace(r) || r == model.ObjectReplacement
	}) {
		lower := strings.ToLower(chunk)
		if strings.Contains(lower, "://") || strings.HasPrefix(lower, "www.") {
			breakPair()
			continue
		}
		core := strings.TrimLeftFunc(chunk, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
		if len(core) != len(chunk) {
			breakPair()
		}
		trimmed := strings.TrimRightFunc(core, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
		if !isSuggestWord(trimmed) {
			breakPair()
			continue
		}
		out = append(out, suggestToken{word: trimmed, pairs: len(trimmed) == len(core)})
	}
	if len(out) > 0 {
		out[len(out)-1].pairs = false
	}
	return out
}

// isSuggestWord reports letters with single hyphens between them.
func isSuggestWord(w string) bool {
	if w == "" {
		return false
	}
	prevHyphen := true
	for _, r := range w {
		switch {
		case r == '-':
			if prevHyphen {
				return false
			}
			prevHyphen = true
		case unicode.IsLetter(r):
			prevHyphen = false
		default:
			return false
		}
	}
	return !prevHyphen
}

// letterCount counts the letters of a form.
func letterCount(form string) int {
	n := 0
	for _, r := range form {
		if unicode.IsLetter(r) {
			n++
		}
	}
	return n
}

// spellingForm is the form a written spelling counts as. A word written
// capitalised or all in capitals is the same form as the word in lower case,
// which is how a sentence, a heading or a label writes it; a capital inside a
// word (GitHub, KapiMart) is part of its spelling. Spaces and hyphens stay.
func spellingForm(written string) string {
	var b strings.Builder
	word := strings.Builder{}
	flush := func() {
		w := word.String()
		lower := strings.ToLower(w)
		if w == strings.ToUpper(w) || w == upperInitial(lower) {
			w = lower
		}
		b.WriteString(w)
		word.Reset()
	}
	for _, r := range written {
		if r == ' ' || r == '-' {
			flush()
			b.WriteRune(r)
			continue
		}
		word.WriteRune(r)
	}
	flush()
	return b.String()
}

// upperInitial capitalises the first letter of a lower-case word.
func upperInitial(word string) string {
	r, size := utf8.DecodeRuneInString(word)
	if size == 0 {
		return word
	}
	return string(unicode.ToUpper(r)) + word[size:]
}

// variantCount is one form's uses.
type variantCount struct {
	count     int
	files     map[string]int
	spellings map[string]int
	// blocks are the blocks that use the form, each once, in order.
	blocks []int
	// sample is the first block that uses the form, for a quote.
	sample int
}

// spellingFindings groups the words and two-word phrases of the corpus by
// their letters, and reports each group the files write in two or more
// forms.
//
// A candidate has at least four letters. A two-word phrase joins two words
// of two letters or more, one of them three or more, with nothing but white
// space between them: "sign in" and "log in" are candidates, "in to" is not.
func spellingFindings(c suggestCorpus) []suggestFinding {
	groups := map[string]map[string]*variantCount{}
	add := func(written string, block int, file string) {
		key := spellingKey(written)
		if letterCount(key) < 4 {
			return
		}
		form := spellingForm(written)
		forms := groups[key]
		if forms == nil {
			forms = map[string]*variantCount{}
			groups[key] = forms
		}
		v := forms[form]
		if v == nil {
			v = &variantCount{files: map[string]int{}, spellings: map[string]int{}, sample: block}
			forms[form] = v
		}
		v.count++
		v.spellings[written]++
		v.files[file]++
		if n := len(v.blocks); n == 0 || v.blocks[n-1] != block {
			v.blocks = append(v.blocks, block)
		}
	}
	for i, b := range c.blocks {
		tokens := suggestTokens(b.text)
		for j, t := range tokens {
			add(t.word, i, b.file)
			if !t.pairs || j+1 >= len(tokens) {
				continue
			}
			next := tokens[j+1].word
			if strings.Contains(t.word, "-") || strings.Contains(next, "-") {
				continue
			}
			l1, l2 := letterCount(t.word), letterCount(next)
			if l1 < 2 || l2 < 2 || max(l1, l2) < 3 {
				continue
			}
			add(t.word+" "+next, i, b.file)
		}
	}

	var out []suggestFinding
	for _, key := range slices.Sorted(maps.Keys(groups)) {
		forms := groups[key]
		if english(c.language) {
			forms = englishVariants(forms)
		}
		if allPhrases(forms) {
			// "the website" and "the WebSite" differ in a word the group of
			// that word reports already.
			continue
		}
		if len(forms) < 2 {
			continue
		}
		out = append(out, spellingFinding(c, key, forms))
	}
	return out
}

// allPhrases reports a group whose every form is two words: a phrase written
// two ways that is never written as one word.
func allPhrases(forms map[string]*variantCount) bool {
	for form := range forms {
		if !strings.Contains(form, " ") {
			return false
		}
	}
	return true
}

// particles are the English adverbs that make a phrasal verb with the word
// before them.
var particles = []string{"up", "out", "in", "on", "off", "down", "over", "back", "away", "through"}

// englishVariants leaves out of a group the spellings English writes
// differently in different places. A phrasal verb is open and its noun is
// closed or hyphenated (set up, the setup). A compound before a noun is
// hyphenated and open elsewhere (real-time events, in real time).
func englishVariants(forms map[string]*variantCount) map[string]*variantCount {
	out := map[string]*variantCount{}
	closed := false
	for form, v := range forms {
		first, second, spaced := strings.Cut(form, " ")
		if spaced && slices.Contains(particles, strings.ToLower(second)) && first != "" {
			continue
		}
		if !strings.ContainsAny(form, " -") {
			closed = true
		}
		out[form] = v
	}
	if !closed {
		hyphen, space := 0, 0
		for form := range out {
			switch {
			case strings.Contains(form, " "):
				space++
			case strings.Contains(form, "-"):
				hyphen++
			}
		}
		if hyphen > 0 && space > 0 && hyphen+space == len(out) {
			return nil
		}
	}
	return out
}

// spellingFinding states one variant group as a term rule: write the form
// the files use most, not the others.
func spellingFinding(c suggestCorpus, key string, forms map[string]*variantCount) suggestFinding {
	type ranked struct {
		form    string
		written string
		v       *variantCount
	}
	list := make([]ranked, 0, len(forms))
	for form, v := range forms {
		list = append(list, ranked{form: form, written: form, v: v})
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].v.count != list[j].v.count {
			return list[i].v.count > list[j].v.count
		}
		return list[i].form < list[j].form
	})
	// The form to write is shown as the files write it. A form to avoid is
	// shown as the form, which a rule matching either case finds however it
	// is capitalised; a rule that tells two spellings apart by case alone
	// matches as written, so it names each capitalisation the files use.
	list[0].written = writtenAs(list[0].form, list[0].v.spellings)

	fd := suggestFinding{kind: SuggestKindSpelling, key: "spelling:" + key, note: suggestNoteSpelling}
	files := map[string]bool{}
	blocks := map[int]bool{}
	others := make([]string, 0, len(list)-1)
	for i, r := range list {
		fd.forms = append(fd.forms, SuggestedForm{Form: r.written, Count: r.v.count, Blocks: len(r.v.blocks), Files: slices.Sorted(maps.Keys(r.v.files))})
		for _, b := range r.v.blocks {
			blocks[b] = true
		}
		for f := range r.v.files {
			files[f] = true
		}
		if i == 0 {
			continue
		}
		others = append(others, r.written)
		if strings.EqualFold(r.form, list[0].written) {
			for _, spelling := range slices.Sorted(maps.Keys(r.v.spellings)) {
				if spelling != r.written {
					others = append(others, spelling)
				}
			}
		}
	}
	fd.files = slices.Sorted(maps.Keys(files))
	fd.blocks = len(blocks)

	rule := contextop.ObservedRule(list[0].written, others)
	fd.subject = contextop.Subject{Kind: contextop.SubjectTerm, Term: &rule, Text: spellingCounts(fd.forms)}
	fd.signature = ruleSignature(rule)

	// The block quoted shows an avoided form, so its file comes first; the
	// other files that write an avoided form follow, since they are where the
	// rule would report, and then the files that write the form to use.
	sample := c.blocks[list[1].v.sample]
	evidence := []contextop.Evidence{{Path: sample.file, Quote: quoteAround(sample.plain, list[1].written)}}
	named := map[string]bool{sample.file: true}
	order := append(slices.Clone(list[1:]), list[0])
	for _, r := range order {
		for _, f := range slices.Sorted(maps.Keys(r.v.files)) {
			if named[f] || len(evidence) == maxSuggestEvidence {
				continue
			}
			named[f] = true
			evidence = append(evidence, contextop.Evidence{Path: f})
		}
	}
	fd.evidence = evidence

	// The usage count a check makes of the rule, so the queue shows how the
	// files write it from the moment it is filed.
	preferred := formMatcher([]string{rule.Replacement}, rule.MatchesCase())
	rejected := formMatcher(append([]string{rule.Term}, rule.Forms...), rule.MatchesCase())
	sig := &contextop.Signal{Source: contextop.SignalUsage}
	for _, b := range c.blocks {
		sig.Preferred += countUses(b.plain, preferred)
		sig.Rejected += countUses(b.plain, rejected)
	}
	if sig.Preferred+sig.Rejected > 0 {
		fd.usage = sig
	}
	return fd
}

// writtenAs is how a form is shown: as the form itself where the files write
// it so, which is how a word reads inside a sentence, and otherwise as the
// spelling written most often, the first in order on a tie. "Sign in" at the
// head of a page and "sign in" in its prose are shown as "sign in".
func writtenAs(form string, spellings map[string]int) string {
	if spellings[form] > 0 {
		return form
	}
	best, n := "", -1
	for _, w := range slices.Sorted(maps.Keys(spellings)) {
		if spellings[w] > n {
			best, n = w, spellings[w]
		}
	}
	return best
}

// spellingCounts says how often the files write each form: `"email" 5
// times, "e-mail" twice`.
func spellingCounts(forms []SuggestedForm) string {
	parts := make([]string, len(forms))
	for i, f := range forms {
		parts[i] = strconv.Quote(f.Form) + " " + times(f.Count)
	}
	return strings.Join(parts, ", ")
}

// ruleSignature is what a term rule says, for telling whether a later run
// says the same: the form to use, then the forms to avoid, sorted.
func ruleSignature(rule profile.TermRule) string {
	avoid := append([]string{rule.Term}, rule.Forms...)
	slices.Sort(avoid)
	return rule.Replacement + "\x00" + strings.Join(avoid, "\x00")
}

// quoteAround is the stretch of a block's text around the first use of a
// form, for the evidence a person reads.
func quoteAround(text, form string) string {
	text = strings.Join(strings.Fields(text), " ")
	const width = 120
	if utf8.RuneCountInString(text) <= width {
		return text
	}
	at := strings.Index(strings.ToLower(text), strings.ToLower(form))
	runes := []rune(text)
	start := 0
	if at > 0 {
		start = max(0, utf8.RuneCountInString(text[:at])-width/3)
	}
	end := min(len(runes), start+width)
	quote := string(runes[start:end])
	if start > 0 {
		if i := strings.IndexByte(quote, ' '); i >= 0 {
			quote = quote[i+1:]
		}
	}
	if end < len(runes) {
		if i := strings.LastIndexByte(quote, ' '); i > 0 {
			quote = quote[:i]
		}
	}
	return quote
}

// --- words a starter pack avoids -----------------------------------------

// minPackWords is how many distinct words a starter pack avoids that the files
// must use before the pass names the pack.
const minPackWords = 3

// packFindings reports, for each starter voice pack kapi ships, the words its
// word rules avoid that the files use, when they use at least minPackWords
// of them.
func packFindings(c suggestCorpus) ([]suggestFinding, error) {
	names, err := packs.List()
	if err != nil {
		return nil, err
	}
	var out []suggestFinding
	for _, name := range names {
		p, err := packs.Load(name)
		if err != nil {
			return nil, err
		}
		var found []SuggestedForm
		blocks := map[int]bool{}
		files := map[string]bool{}
		var sample int
		for _, rule := range p.CarriedTerms().Rules {
			if strings.TrimSpace(rule.Term) == "" {
				continue
			}
			forms := append([]string{rule.Term}, rule.Forms...)
			word := SuggestedForm{Form: rule.Term}
			wordFiles := map[string]bool{}
			for i, b := range c.blocks {
				hits := len(check.FindTermForms(b.text, forms))
				if hits == 0 {
					continue
				}
				if word.Count == 0 && len(found) == 0 {
					sample = i
				}
				word.Count += hits
				word.Blocks++
				wordFiles[b.file] = true
				blocks[i] = true
				files[b.file] = true
			}
			if word.Count > 0 {
				word.Files = slices.Sorted(maps.Keys(wordFiles))
				found = append(found, word)
			}
		}
		if len(found) < minPackWords {
			continue
		}
		fd := suggestFinding{
			kind:   SuggestKindPackWords,
			key:    "pack:" + name,
			pack:   name,
			forms:  found,
			blocks: len(blocks),
			files:  slices.Sorted(maps.Keys(files)),
		}
		words := make([]string, len(found))
		counted := make([]string, len(found))
		for i, f := range found {
			words[i] = f.Form
			counted[i] = f.Form + " " + times(f.Count)
		}
		fd.signature = strings.Join(slices.Sorted(slices.Values(words)), ", ")
		fd.note = suggestNotePack(name) + fd.signature
		fd.subject = contextop.Subject{Kind: contextop.SubjectNote, Text: fmt.Sprintf(
			"The files use %d words the %s starter pack avoids: %s, in %s across %s.",
			len(found), name, andList(counted), countWord(fd.blocks, "block", "blocks"), countWord(len(fd.files), "file", "files"))}
		fd.evidence = fileEvidence(fd.files, c.blocks[sample], found[0].Form)
		out = append(out, fd)
	}
	return out, nil
}

// fileEvidence names the files a finding was seen in, the sample block's file
// first and quoted.
func fileEvidence(files []string, sample suggestBlock, form string) []contextop.Evidence {
	out := []contextop.Evidence{{Path: sample.file, Quote: quoteAround(sample.plain, form)}}
	for _, f := range files {
		if f == sample.file || len(out) == maxSuggestEvidence {
			continue
		}
		out = append(out, contextop.Evidence{Path: f})
	}
	return out
}

// --- the reader addressed as you -----------------------------------------

// The personal pronouns the address measure reads, by person.
var (
	pronounsYou = []string{"you", "your", "yours", "yourself", "yourselves"}
	pronounsWe  = []string{"we", "us", "our", "ours", "ourselves"}
	// pronounsOther are the first person singular and the third person.
	pronounsOther = []string{"i", "me", "my", "mine", "myself", "he", "him", "his", "himself",
		"she", "her", "hers", "herself", "they", "them", "their", "theirs", "themselves"}
)

// addressShare is the share of blocks with a pronoun that must address the
// reader as you, as a percentage, and minAddressBlocks the fewest such blocks
// the measure reports on.
const (
	addressShare     = 70
	minAddressBlocks = 3
)

// pronounRe matches a run of letters, so a contraction reads as its pronoun
// ("you're" is "you").
var pronounRe = regexp.MustCompile(`\p{L}+`)

// addressFinding reports a project whose files address the reader as you: at
// least addressShare percent of the blocks that use a personal pronoun say
// you, none says we, and at least minAddressBlocks say you.
func addressFinding(c suggestCorpus) (suggestFinding, bool) {
	var you, we, any int
	files := map[string]bool{}
	sample := -1
	for i, b := range c.blocks {
		var hasYou, hasWe, hasOther bool
		for _, word := range pronounRe.FindAllString(b.text, -1) {
			if word == "US" {
				// The country, not the pronoun.
				continue
			}
			lower := strings.ToLower(word)
			switch {
			case slices.Contains(pronounsYou, lower):
				hasYou = true
			case slices.Contains(pronounsWe, lower):
				hasWe = true
			case slices.Contains(pronounsOther, lower) && (lower != "i" || word == "I"):
				hasOther = true
			}
		}
		if hasYou || hasWe || hasOther {
			any++
		}
		if hasWe {
			we++
		}
		if hasYou {
			you++
			files[b.file] = true
			if sample < 0 {
				sample = i
			}
		}
	}
	if we > 0 || you < minAddressBlocks || you*100 < addressShare*any {
		return suggestFinding{}, false
	}
	fd := suggestFinding{
		kind:      SuggestKindAddress,
		key:       "address:second",
		signature: "second",
		note:      suggestNoteAddress,
		address:   &SuggestedAddress{You: you, We: we, Pronouns: any, PersonPOV: "second"},
		blocks:    you,
		files:     slices.Sorted(maps.Keys(files)),
	}
	fd.subject = contextop.Subject{Kind: contextop.SubjectNote, Text: fmt.Sprintf(
		"The files address the reader as you (person_pov: second): %d of %d blocks with a personal pronoun say you, and none says we.",
		you, any)}
	fd.evidence = fileEvidence(fd.files, c.blocks[sample], "you")
	return fd, true
}

// --- filing ----------------------------------------------------------------

// suggestFiler files findings into the log, against what the log already
// holds.
type suggestFiler struct {
	s       *contextOpsSession
	records []contextop.Record
	dry     bool
	scopes  map[string]suggestPoint
}

// suggestPoint is the governance and the point one file resolves to.
type suggestPoint struct {
	basis contextop.Basis
	scope contextop.Scope
}

// file records one finding, or reports what the log already holds for it.
func (f *suggestFiler) file(ctx context.Context, fd suggestFinding) (SuggestedFinding, error) {
	out := SuggestedFinding{
		Kind:     fd.kind,
		Subject:  fd.subject,
		Sentence: ruleSentence(fd.subject),
		Forms:    fd.forms,
		Pack:     fd.pack,
		Address:  fd.address,
		Blocks:   fd.blocks,
		Files:    fd.files,
	}
	if len(fd.evidence) > 0 {
		q := fd.evidence[0]
		out.Quote = &q
	}
	prior, dropped := f.prior(fd)
	switch {
	case dropped != nil:
		out.Outcome, out.ID = SuggestDropped, dropped.ID
		out.Short = contextop.ShortID(dropped.ID)
		return out, nil
	case prior == nil && fd.kind == SuggestKindSpelling && f.known(fd):
		out.Outcome = SuggestKnown
		return out, nil
	case prior != nil && (f.signature(*prior) == fd.signature || prior.Established || prior.Status != contextop.StatusSuggested):
		// Unchanged, or no longer the pass's to change: a person kept it, or
		// it waits on a choice between two sides.
		out.Outcome, out.ID = SuggestUnchanged, prior.ID
		out.Short = contextop.ShortID(prior.ID)
		if !f.dry {
			if err := f.recordUsage(ctx, *prior, fd); err != nil {
				return out, err
			}
		}
		return out, nil
	}
	out.Outcome = SuggestFiled
	if prior != nil {
		out.Outcome = SuggestUpdated
	}
	if f.dry {
		if prior != nil {
			out.ID, out.Short = prior.ID, contextop.ShortID(prior.ID)
		}
		return out, nil
	}
	if prior != nil {
		if _, err := f.s.setAside(ctx, contextop.KindWithdraw, SuggestActor, *prior,
			"the files changed what it says: filed again as it now stands"); err != nil {
			return out, err
		}
	}
	basis, scope := f.pointOf(fd.files)
	op, err := f.s.appendObservation(ctx, contextop.Record{
		Actor:    SuggestActor,
		Kind:     contextop.KindObserve,
		Project:  f.s.key,
		Subject:  fd.subject,
		Evidence: fd.evidence,
		Basis:    basis,
		Scope:    scope,
		Note:     fd.note,
	})
	if err != nil {
		return out, err
	}
	out.ID, out.Short = op.ID, contextop.ShortID(op.ID)
	if err := f.recordUsage(ctx, op.Record, fd); err != nil {
		return out, err
	}
	return out, nil
}

// prior finds what the pass filed before about the same thing: the newest
// record of it that still answers, or failing that the one a person dropped.
// A withdrawn or reset record says nothing about it either way. For a
// spelling, a suggestion about the same letters that a person dropped counts
// whoever filed it.
func (f *suggestFiler) prior(fd suggestFinding) (answering, dropped *contextop.Record) {
	for i := range f.records {
		r := &f.records[i]
		if r.Kind != contextop.KindObserve && r.Kind != contextop.KindCorrect {
			continue
		}
		mine := r.Actor == SuggestActor
		if !mine && !(fd.kind == SuggestKindSpelling && r.Status == contextop.StatusDropped) {
			continue
		}
		if f.keyOf(*r) != fd.key {
			continue
		}
		switch {
		case r.Status.Answers() && mine:
			if answering == nil {
				answering = r
			}
		case r.Status == contextop.StatusDropped:
			if dropped == nil && answering == nil {
				dropped = r
			}
		}
	}
	return answering, dropped
}

// keyOf names what a record the pass could have filed is about, in the terms
// suggestFinding.key uses; empty for anything else.
func (f *suggestFiler) keyOf(r contextop.Record) string {
	switch r.Subject.Kind {
	case contextop.SubjectTerm:
		rule, ok := r.Rule()
		if !ok || rule.Replacement == "" {
			return ""
		}
		return "spelling:" + spellingKey(rule.Replacement)
	case contextop.SubjectNote:
		switch {
		case r.Note == suggestNoteAddress:
			return "address:second"
		case strings.HasPrefix(r.Note, "kapi context suggest: words the "):
			rest := strings.TrimPrefix(r.Note, "kapi context suggest: words the ")
			if name, _, ok := strings.Cut(rest, " starter pack avoids: "); ok {
				return "pack:" + name
			}
		}
	}
	return ""
}

// signature is what a record the pass filed says, in the terms
// suggestFinding.signature uses.
func (f *suggestFiler) signature(r contextop.Record) string {
	switch r.Subject.Kind {
	case contextop.SubjectTerm:
		rule, _ := r.Rule()
		return ruleSignature(rule)
	case contextop.SubjectNote:
		if r.Note == suggestNoteAddress {
			return "second"
		}
		if _, words, ok := strings.Cut(r.Note, " starter pack avoids: "); ok {
			return words
		}
	}
	return ""
}

// known reports a spelling the project already has a rule or a suggestion
// about from someone else: one that still answers and whose form to use, or
// a form it avoids, has the same letters.
func (f *suggestFiler) known(fd suggestFinding) bool {
	letters := strings.TrimPrefix(fd.key, "spelling:")
	for _, r := range f.records {
		if r.Actor == SuggestActor || !r.Status.Answers() {
			continue
		}
		rule, ok := r.Rule()
		if !ok {
			continue
		}
		for _, form := range append([]string{rule.Replacement, rule.Term}, rule.Forms...) {
			if form != "" && spellingKey(form) == letters {
				return true
			}
		}
	}
	return false
}

// recordUsage records a spelling's usage count against its suggestion when it
// differs from the latest count recorded for it, the way a check records
// one, so the queue shows how the files write each form.
func (f *suggestFiler) recordUsage(ctx context.Context, target contextop.Record, fd suggestFinding) error {
	if fd.usage == nil {
		return nil
	}
	signals, err := f.s.ledger.Records(ctx, contextop.Filter{Project: f.s.key, Kinds: []contextop.Kind{contextop.KindSignal}})
	if err != nil {
		return err
	}
	for _, r := range signals {
		if r.Target != target.ID || r.Signal == nil || r.Signal.Source != contextop.SignalUsage || !r.Status.Answers() {
			continue
		}
		if r.Signal.Within == "" && r.Signal.Preferred == fd.usage.Preferred && r.Signal.Rejected == fd.usage.Rejected {
			return nil
		}
		break
	}
	sig := *fd.usage
	_, err = f.s.ledger.Append(ctx, contextop.Record{
		Actor:   SuggestActor,
		Kind:    contextop.KindSignal,
		Target:  target.ID,
		Project: target.Project,
		Signal:  &sig,
	})
	return err
}

// pointOf is the governance and the point a finding seen across several
// files holds at: the profile and channel every file shares, and the
// coordinates on which every file agrees. A finding seen at two points holds
// across both.
func (f *suggestFiler) pointOf(files []string) (contextop.Basis, contextop.Scope) {
	var basis contextop.Basis
	var scope contextop.Scope
	for i, file := range files {
		p, ok := f.scopes[file]
		if !ok {
			p.basis, p.scope = f.s.basisAt([]contextop.Evidence{{Path: file}})
			f.scopes[file] = p
		}
		if i == 0 {
			basis = p.basis
			scope = contextop.Scope{Level: p.scope.Level}
			if len(p.scope.Coordinates) > 0 {
				scope.Coordinates = maps.Clone(p.scope.Coordinates)
			}
			continue
		}
		if basis.Profile != p.basis.Profile {
			basis.Profile = ""
		}
		if basis.Channel != p.basis.Channel {
			basis.Channel = ""
		}
		for axisName, value := range scope.Coordinates {
			if p.scope.Coordinates[axisName] != value {
				delete(scope.Coordinates, axisName)
			}
		}
	}
	if len(scope.Coordinates) == 0 {
		scope.Coordinates = nil
	}
	if scope.Level == "" {
		scope.Level = contextop.LevelProject
	}
	return basis, scope
}

// andList joins items the way a sentence lists them together: "a", "a and
// b", "a, b and c".
func andList(items []string) string {
	switch len(items) {
	case 0:
		return ""
	case 1:
		return items[0]
	}
	return strings.Join(items[:len(items)-1], ", ") + " and " + items[len(items)-1]
}
