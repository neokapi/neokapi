package host

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"regexp"
	"slices"
	"sort"
	"strings"
	"unicode/utf8"

	"golang.org/x/sync/errgroup"

	aitools "github.com/neokapi/neokapi/core/ai/tools"
	"github.com/neokapi/neokapi/core/contextop"
	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/host/credentials"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
)

// The deeper look (`kapi context suggest --ai`, and "Deeper look with AI" on
// Kapi Desktop's Map) reads the source blocks the suggestions pass reads and
// asks the connected model about them, through the tools `kapi exec` runs:
// term-extract names the terms the files use, and voice-infer reads the voice
// they are written in.
//
// What the model proposes is checked against the files and filed the way the
// suggestions pass files what it counts: as suggestions by kapi, named with
// the model, that fail no check until a person keeps them. A term the files
// never write, or write in more than one way, is left out. Nothing is written
// to the recipe or to a rules file.
//
// A later run matches each proposal to its record by what it is about, the
// way the pass does, and also to what the pass filed: a proposal the pass, a
// person or another model already recorded is known, and a dropped one stays
// dropped.

// The kinds of finding the deeper look reports.
const (
	// SuggestKindTerm is a term the files use, named by the model.
	SuggestKindTerm = "term"
	// SuggestKindVoice is one voice measure, read by the model.
	SuggestKindVoice = "voice"
)

// ErrNoAIProvider is the deeper look's refusal when nothing names a model to
// ask: no provider in the recipe or the app config, no saved credential and no
// API key in the environment.
var ErrNoAIProvider = errors.New("no AI provider configured")

// deeperLookNotePrefix opens the note of every record the deeper look files.
// The rest names what the record is about and what it says, so a later run
// finds it: "voice:sentence_length = short".
const deeperLookNotePrefix = "kapi context suggest --ai: "

// deeperLookKeyPOV is the key of the note about whom the files address.
const deeperLookKeyPOV = "voice:person_pov"

// How much of a project the model reads. The blocks go to the model in chunks
// of about deeperLookChunkRunes characters, at most deeperLookMaxChunks of
// them, deeperLookParallel at a time. The bound keeps the cost of a run over a
// large project to a fixed number of calls; the counts in the result say how
// many blocks the model read.
const (
	deeperLookChunkRunes = 2000
	deeperLookMaxChunks  = 12
	deeperLookParallel   = 4
)

// A term the model names is filed when the files write it, as the model wrote
// it, in at least minDeeperTermBlocks blocks; the maxDeeperTerms used in the
// most blocks are filed.
const (
	minDeeperTermBlocks = 2
	maxDeeperTerms      = 10
)

// DeeperLookRequest asks for the deeper look over one project.
type DeeperLookRequest struct {
	// Project is the recipe path, or the workspace key of a project with a
	// checkout on this machine.
	Project string
	// DryRun reports what the deeper look would file without recording
	// anything. The model is still asked.
	DryRun bool
}

// DeeperLookResult is what the model proposed and what became of each
// proposal.
type DeeperLookResult struct {
	Project string `json:"project"`
	DryRun  bool   `json:"dry_run,omitempty"`
	// Provider and Model name the model that was asked. The suggestions it
	// filed carry the model as their actor's name.
	Provider string `json:"provider"`
	Model    string `json:"model"`
	// Files and Blocks count the project's source content; Asked counts the
	// blocks the model read.
	Files  int `json:"files"`
	Blocks int `json:"blocks"`
	Asked  int `json:"asked"`
	// Usage is the token count the provider reported, when it reported one.
	Usage *aiprovider.TokenUsage `json:"usage,omitempty"`
	// Suggestions are the proposals, terms first.
	Suggestions []SuggestedFinding `json:"suggestions"`
}

// FiledCount counts the suggestions the run recorded new.
func (r DeeperLookResult) FiledCount() int {
	n := 0
	for _, s := range r.Suggestions {
		if s.Outcome == SuggestFiled {
			n++
		}
	}
	return n
}

// FormatText prints one line per proposal, in the words the review queue
// uses, then what the model read, then where to decide.
func (r DeeperLookResult) FormatText(w io.Writer) error {
	var b strings.Builder
	waiting := writeSuggestLines(&b, r.Suggestions, r.DryRun)
	b.WriteString(r.modelLine() + "\n")
	if waiting && !r.DryRun {
		b.WriteString("\nDecide with kapi context review.\n")
	}
	_, err := io.WriteString(w, b.String())
	return err
}

// modelLine says what the model read: "demo-stub read 4 blocks in 1 file (312
// tokens)."
func (r DeeperLookResult) modelLine() string {
	read := countWord(r.Asked, "block", "blocks")
	if r.Asked < r.Blocks {
		read = fmt.Sprintf("%d of %s", r.Asked, countWord(r.Blocks, "block", "blocks"))
	}
	line := r.Model + " read " + read + " in " + countWord(r.Files, "file", "files")
	if r.Usage != nil && r.Usage.TotalTokens() > 0 {
		line += " (" + countWord(r.Usage.TotalTokens(), "token", "tokens") + ")"
	}
	if len(r.Suggestions) == 0 {
		return line + ": nothing more to suggest."
	}
	return line + "."
}

// DeeperLookActor is who files what the model proposed: kapi, named with the
// model.
func DeeperLookActor(model string) contextop.Actor {
	return contextop.Actor{Kind: contextop.ActorTool, Name: model}
}

// deeperLookRecord reports a record the deeper look filed, whichever model it
// asked.
func deeperLookRecord(r contextop.Record) bool {
	return r.Actor.Kind == contextop.ActorTool && strings.HasPrefix(r.Note, deeperLookNotePrefix)
}

// DeeperLook asks the connected model about a project's source blocks and
// files what it proposes as suggestions by kapi, unless the request is a dry
// run.
func (a *App) DeeperLook(ctx context.Context, req DeeperLookRequest) (DeeperLookResult, error) {
	s, err := a.contextOps(ctx, req.Project)
	if err != nil {
		return DeeperLookResult{}, err
	}
	if !s.checkedOut() {
		return DeeperLookResult{}, fmt.Errorf("project %s has no checkout on this machine, so there are no files to read", s.key)
	}
	provider, modelName, ok := a.deeperLookModel(s.proj)
	if !ok {
		return DeeperLookResult{}, fmt.Errorf("%w: connect one with kapi models setup, then run this again", ErrNoAIProvider)
	}
	out := DeeperLookResult{
		Project:     string(s.key),
		DryRun:      req.DryRun,
		Provider:    provider,
		Model:       modelName,
		Suggestions: []SuggestedFinding{},
	}

	corpus, err := s.suggestCorpus(ctx)
	if err != nil {
		return out, err
	}
	out.Files, out.Blocks = len(corpus.files), len(corpus.blocks)
	chunks, asked := deeperLookChunks(corpus)
	out.Asked = asked
	if len(chunks) == 0 {
		return out, nil
	}
	if err := s.redactForModel(ctx, chunks, corpus.language); err != nil {
		return out, err
	}

	var usage aiprovider.TokenUsage
	terms, used, err := a.deeperLookTerms(ctx, s.proj, corpus.language, chunks)
	usage = usage.Add(used)
	if err != nil {
		return out, err
	}
	var draft *coreprofile.VoiceProfile
	if !s.bindsVoice(corpus) {
		// A project that binds a voice states its measures already.
		draft, used, err = a.deeperLookVoice(ctx, s.proj, corpus.language, chunks)
		usage = usage.Add(used)
		if err != nil {
			return out, err
		}
	}
	if usage.TotalTokens() > 0 {
		out.Usage = &usage
	}

	records, err := s.ledger.Records(ctx, contextop.Filter{Project: s.key, Subjects: true})
	if err != nil {
		return out, err
	}
	f := &suggestFiler{s: s, actor: DeeperLookActor(modelName), ai: true, records: records, dry: req.DryRun, scopes: map[string]suggestPoint{}}
	findings := append(deeperTermFindings(corpus, terms), deeperVoiceFindings(corpus, draft)...)
	for _, fd := range findings {
		sug, err := f.file(ctx, fd)
		if err != nil {
			return out, err
		}
		out.Suggestions = append(out.Suggestions, sug)
	}
	return out, nil
}

// deeperLookModel is the provider and model the deeper look asks, by the
// resolution `kapi exec term-extract` makes: the recipe's preset for the tool,
// then the app config. ok is false when nothing names a model to ask: no
// provider there, no saved credential and no API key in the environment.
func (a *App) deeperLookModel(proj *project.KapiProject) (provider, modelName string, ok bool) {
	res := EffectiveAIModel(a.Config, proj, "term-extract", "")
	ok = res.Configured || len(credentials.ProvidersWithEnvKey()) > 0 ||
		(a.Credentials != nil && len(a.Credentials.List()) > 0)
	provider, modelName = res.Provider, res.Model
	if modelName == "" {
		if info, found := aiprovider.ProviderInfoFor(aiprovider.ProviderID(provider)); found {
			modelName = info.DefaultModel
		}
	}
	if modelName == "" && provider == string(aiprovider.Demo) {
		modelName = aiprovider.DemoModelName
	}
	if modelName == "" {
		modelName = provider
	}
	return provider, modelName, ok
}

// deeperLookTool builds one of the deeper look's tools the way `kapi exec`
// builds it: the recipe's preset for the tool, then the default provider and
// model the app config or the environment names, then the registry's
// credential resolution. The provider and model come from EffectiveAIModel,
// the resolution deeperLookModel reports, so the model asked is the model
// named.
func (a *App) deeperLookTool(proj *project.KapiProject, toolName, language string) (any, error) {
	a.InitRegistries()
	cfg := map[string]any{}
	if proj != nil {
		maps.Copy(cfg, proj.Defaults.Tools[toolName])
	}
	if _, set := cfg["locale"]; !set && toolName == "term-extract" {
		cfg["locale"] = language
	}
	if res := EffectiveAIModel(a.Config, proj, toolName, ""); res.Configured {
		if _, set := cfg["provider"]; !set && res.ProviderOrigin != AIModelFromBuiltIn {
			cfg["provider"] = res.Provider
		}
		if _, set := cfg["model"]; !set && res.Model != "" {
			cfg["model"] = res.Model
		}
	}
	return a.ToolReg.NewToolWithConfig(registry.ToolID(toolName), cfg, "")
}

// deeperLookChunks gathers the project's distinct block texts into chunks for
// the model, in the order the pass read them, up to the bound. asked counts
// the blocks the chunks cover, each copy of a repeated text included.
func deeperLookChunks(c suggestCorpus) (chunks []*model.Block, asked int) {
	seen := map[string]bool{}
	var cur strings.Builder
	runes := 0
	flush := func() {
		if cur.Len() == 0 {
			return
		}
		chunks = append(chunks, model.NewBlock(fmt.Sprintf("deeper-look-%d", len(chunks)+1), cur.String()))
		cur.Reset()
		runes = 0
	}
	for _, b := range c.blocks {
		text := strings.Join(strings.Fields(b.plain), " ")
		if text == "" {
			continue
		}
		if seen[text] {
			asked++
			continue
		}
		n := utf8.RuneCountInString(text)
		if runes > 0 && runes+n > deeperLookChunkRunes {
			flush()
		}
		if len(chunks) == deeperLookMaxChunks {
			break
		}
		seen[text] = true
		if cur.Len() > 0 {
			cur.WriteString("\n\n")
		}
		cur.WriteString(text)
		runes += n
		asked++
	}
	if len(chunks) < deeperLookMaxChunks {
		flush()
	}
	return chunks, asked
}

// redactForModel applies the project's declared redaction policy to the
// chunks before any of them reaches a model, keeping the originals in the
// project's vault, as every route that sends source out of kapi does.
func (s *contextOpsSession) redactForModel(ctx context.Context, chunks []*model.Block, language string) error {
	spec := ProjectRedaction(s.proj)
	if spec == nil {
		return nil
	}
	layout := project.LayoutAt(s.root)
	if err := layout.PrepareVault(); err != nil {
		return err
	}
	return RedactAtIngest(ctx, chunks, spec, layout.Root, layout.RedactionVaultPath(), model.LocaleID(language))
}

// deeperLookTerms runs term-extract over each chunk and returns the terms it
// named, in the order the chunks name them, with the tokens it used.
func (a *App) deeperLookTerms(ctx context.Context, proj *project.KapiProject, language string, chunks []*model.Block) ([]aitools.TermEntry, aiprovider.TokenUsage, error) {
	built, err := a.deeperLookTool(proj, "term-extract", language)
	if err != nil {
		return nil, aiprovider.TokenUsage{}, err
	}
	extract, ok := built.(*aitools.AITerminologyTool)
	if !ok {
		return nil, aiprovider.TokenUsage{}, errors.New("term-extract: the registry returned a different tool")
	}
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(deeperLookParallel)
	for _, c := range chunks {
		g.Go(func() error {
			_, err := extract.ApplyContext(gctx, &model.Part{Type: model.PartBlock, Resource: c})
			return err
		})
	}
	err = g.Wait()
	usage := extract.TotalUsage()
	if err != nil {
		return nil, usage, err
	}
	var terms []aitools.TermEntry
	for _, c := range chunks {
		raw := c.Properties["terminology"]
		if raw == "" {
			continue
		}
		var named []aitools.TermEntry
		if json.Unmarshal([]byte(raw), &named) == nil {
			terms = append(terms, named...)
		}
	}
	return terms, usage, nil
}

// deeperLookVoice runs voice-infer over the chunks as one corpus and returns
// the draft it read, nil when the chunks hold no text.
func (a *App) deeperLookVoice(ctx context.Context, proj *project.KapiProject, language string, chunks []*model.Block) (*coreprofile.VoiceProfile, aiprovider.TokenUsage, error) {
	built, err := a.deeperLookTool(proj, "voice-infer", language)
	if err != nil {
		return nil, aiprovider.TokenUsage{}, err
	}
	infer, ok := built.(*aitools.VoiceInferTool)
	if !ok {
		return nil, aiprovider.TokenUsage{}, errors.New("voice-infer: the registry returned a different tool")
	}
	texts := make([]string, 0, len(chunks))
	for _, c := range chunks {
		texts = append(texts, c.SourceText())
	}
	draft, _, err := infer.InferFrom(ctx, strings.Join(texts, "\n\n"))
	usage := infer.TotalUsage()
	if errors.Is(err, aitools.ErrEmptyCorpus) {
		return nil, usage, nil
	}
	return draft, usage, err
}

// --- terms ---------------------------------------------------------------

// deeperTermFindings checks each term the model named against the files and
// states the ones they write as findings. A term is kept when the files write
// it as the model wrote it in at least minDeeperTermBlocks blocks and never in
// a form a rule about it would avoid; the maxDeeperTerms used in the most
// blocks are kept.
//
// A term with forms to avoid (the spaced, hyphenated and closed spellings of a
// compound) is a term rule, as `kapi context note --term` records one. A word
// with none is a note naming the term, since a rule holding the word alone
// would read as a word to avoid.
func deeperTermFindings(c suggestCorpus, named []aitools.TermEntry) []suggestFinding {
	type candidate struct {
		term, definition string
	}
	var candidates []candidate
	index := map[string]int{}
	for _, e := range named {
		term := strings.TrimSpace(e.Term)
		if letterCount(term) < 2 || utf8.RuneCountInString(term) > 60 {
			continue
		}
		key := spellingKey(term)
		if i, ok := index[key]; ok {
			if candidates[i].definition == "" {
				candidates[i].definition = strings.TrimSpace(e.Definition)
			}
			continue
		}
		index[key] = len(candidates)
		candidates = append(candidates, candidate{term: term, definition: strings.TrimSpace(e.Definition)})
	}

	var out []suggestFinding
	for _, cand := range candidates {
		fd, ok := deeperTermFinding(c, cand.term, cand.definition)
		if ok {
			out = append(out, fd)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].blocks != out[j].blocks {
			return out[i].blocks > out[j].blocks
		}
		return out[i].forms[0].Form < out[j].forms[0].Form
	})
	if len(out) > maxDeeperTerms {
		out = out[:maxDeeperTerms]
	}
	return out
}

// deeperTermFinding states one term as a finding, when the files write it as
// the term rule about it would have them write it.
func deeperTermFinding(c suggestCorpus, term, definition string) (suggestFinding, bool) {
	uses := formMatcher([]string{term}, true)
	form := SuggestedForm{Form: term}
	files := map[string]bool{}
	sample := -1
	for i, b := range c.blocks {
		n := countUses(b.plain, uses)
		if n == 0 {
			continue
		}
		form.Count += n
		form.Blocks++
		files[b.file] = true
		if sample < 0 {
			sample = i
		}
	}
	if form.Blocks < minDeeperTermBlocks {
		return suggestFinding{}, false
	}
	form.Files = slices.Sorted(maps.Keys(files))

	rule := contextop.ObservedRule(term, nil)
	letters := spellingKey(term)
	fd := suggestFinding{
		kind:    SuggestKindTerm,
		letters: letters,
		forms:   []SuggestedForm{form},
		blocks:  form.Blocks,
		files:   form.Files,
	}
	fd.evidence = fileEvidence(fd.files, c.blocks[sample], term)
	if rule.Replacement == "" {
		fd.key = "term:" + letters
		fd.signature = term
		fd.note = deeperLookNotePrefix + fd.key + " = " + term
		text := fmt.Sprintf("The files use the term %q", term)
		if definition != "" {
			text += ": " + strings.TrimRight(definition, ". ")
		}
		fd.subject = contextop.Subject{Kind: contextop.SubjectNote, Text: text + "."}
		return fd, true
	}

	// A file that writes a form the rule avoids writes the term more than one
	// way, which is the pass over the files' finding to make.
	avoided := formMatcher(append([]string{rule.Term}, rule.Forms...), rule.MatchesCase())
	for _, b := range c.blocks {
		if countUses(b.plain, avoided) > 0 {
			return suggestFinding{}, false
		}
	}
	rule.Note = definition
	fd.key = "spelling:" + letters
	fd.signature = ruleSignature(rule)
	fd.note = deeperLookNotePrefix + "term"
	fd.subject = contextop.Subject{Kind: contextop.SubjectTerm, Term: &rule, Text: definition}
	fd.usage = &contextop.Signal{Source: contextop.SignalUsage, Preferred: form.Count}
	return fd, true
}

// --- voice ---------------------------------------------------------------

// deeperContractionRe matches an English contraction, with a straight or a
// curly apostrophe.
var deeperContractionRe = regexp.MustCompile(`(?i)\b\p{L}+['’](?:t|s|re|ve|ll|d|m)\b`)

// deeperVoiceFindings states the measures the model read as notes, one per
// measure, each naming the measure and its value as the pass over the files
// names the reader addressed as you: whom the files address, contractions,
// sentence length, the active voice and formality. A measure the model left
// empty or gave a value outside the voice profile's own is passed over, as is
// the passive voice, which no rule asks for.
func deeperVoiceFindings(c suggestCorpus, draft *coreprofile.VoiceProfile) []suggestFinding {
	if draft == nil {
		return nil
	}
	type measure struct {
		name, value, sentence string
		// sample finds a block that shows the measure, nil for any block.
		sample func(suggestBlock) bool
	}
	var measures []measure
	addressed := func(words []string) func(suggestBlock) bool {
		return func(b suggestBlock) bool {
			for _, w := range pronounRe.FindAllString(b.text, -1) {
				if slices.Contains(words, strings.ToLower(w)) {
					return true
				}
			}
			return false
		}
	}
	switch draft.Style.PersonPOV {
	case "second":
		measures = append(measures, measure{"person_pov", "second", "The files address the reader as you", addressed(pronounsYou)})
	case "first_plural":
		measures = append(measures, measure{"person_pov", "first_plural", "The files speak as we", addressed(pronounsWe)})
	case "third":
		measures = append(measures, measure{"person_pov", "third", "The files write in the third person", nil})
	}
	contracted := func(b suggestBlock) bool { return deeperContractionRe.MatchString(b.plain) }
	switch draft.Style.Contractions {
	case "always":
		measures = append(measures, measure{"contractions", "always", "The files use contractions", contracted})
	case "sometimes":
		measures = append(measures, measure{"contractions", "sometimes", "The files use contractions in places", contracted})
	case "never":
		measures = append(measures, measure{"contractions", "never", "The files write without contractions", nil})
	}
	switch draft.Style.SentenceLength {
	case "short":
		measures = append(measures, measure{"sentence_length", "short", "The files keep sentences short", nil})
	case "medium":
		measures = append(measures, measure{"sentence_length", "medium", "The files write sentences of medium length", nil})
	case "varied":
		measures = append(measures, measure{"sentence_length", "varied", "The files vary the length of their sentences", nil})
	}
	if draft.Style.ActiveVoice {
		measures = append(measures, measure{"active_voice", "true", "The files write in the active voice", nil})
	}
	switch draft.Tone.Formality {
	case "casual", "neutral", "formal", "technical":
		measures = append(measures, measure{"formality", draft.Tone.Formality, "The files read as " + draft.Tone.Formality, nil})
	}

	var out []suggestFinding
	for _, m := range measures {
		sample, blocks := 0, len(c.blocks)
		files := c.files
		if m.sample != nil {
			sample, blocks = -1, 0
			seen := map[string]bool{}
			for i, b := range c.blocks {
				if !m.sample(b) {
					continue
				}
				if sample < 0 {
					sample = i
				}
				blocks++
				seen[b.file] = true
			}
			if sample < 0 {
				// The model read a habit no block shows.
				continue
			}
			files = slices.Sorted(maps.Keys(seen))
		}
		key := "voice:" + m.name
		fd := suggestFinding{
			kind:      SuggestKindVoice,
			key:       key,
			signature: m.value,
			note:      deeperLookNotePrefix + key + " = " + m.value,
			measure:   m.name,
			value:     m.value,
			blocks:    blocks,
			files:     files,
			subject: contextop.Subject{Kind: contextop.SubjectNote,
				Text: fmt.Sprintf("%s (%s: %s).", m.sentence, m.name, m.value)},
			evidence: fileEvidence(files, c.blocks[sample], ""),
		}
		if key == deeperLookKeyPOV && m.value == "second" {
			fd.aliases = []string{"address:second"}
		}
		out = append(out, fd)
	}
	return out
}
