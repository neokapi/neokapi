package host

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/tool"
	coretools "github.com/neokapi/neokapi/core/tools"
)

// CommitCheck returns the check the change service runs on the editions a
// change set changes, before anything is written, for the project cmd
// resolves. It holds each edition to what `kapi check` holds it to, resolved
// the same way: the voice and the terms at the point its document sits at,
// and, for a translation, the term rules gateTermRules gives for its language.
// Only the deterministic analyzers run; a model-backed analyzer belongs to
// `kapi check` and to the hooks.
//
// Outside a project nothing governs the content, and the check holds each
// edition to the hygiene analyzer alone.
func (a *App) CommitCheck(cmd Command) change.CommitCheck {
	return &commitCheck{app: a, cmd: cmd}
}

// commitCheck is the host's change.CommitCheck. Each Check call resolves the
// governance once and caches it per point, so a change set over many blocks of
// one document resolves its voice and its terms once.
type commitCheck struct {
	app *App
	cmd Command
	// outside says the check governs documents outside any project, whatever
	// project cmd would resolve: hygiene alone, in the language cmd names.
	outside bool
}

var _ change.CommitCheck = (*commitCheck)(nil)

// Check returns the findings on each changed edition before and after the
// change, and the governance fingerprint the editions were checked under.
//
// Each edition is checked on its own. The authoritative edition, and any other
// edition in its language, meets the source analyzers: hygiene, the terms, and
// the voice's pattern rules and constraints. A translation meets the
// placeholder check against the block's authoritative edition and the term
// rules for its language. An edition the change set created has no findings
// before, and one it removed has none after.
//
// A call resolves the project, its source language and its governance for
// itself and writes none of them to the App, so one App checks change sets
// for several projects, two at a time.
func (c *commitCheck) Check(ctx context.Context, changes []change.EditionChange) ([]change.CheckOutcome, string, error) {
	a := c.app
	a.InitRegistries()
	in, err := c.project()
	if err != nil {
		return nil, "", err
	}
	res, err := a.newCommitResolution(ctx, c.cmd, in)
	if err != nil {
		return nil, "", err
	}
	defer res.close()

	passes, err := c.plan(in, changes)
	if err != nil {
		return nil, "", err
	}
	sources := sourceChanges(changes)
	outcomes := make([]change.CheckOutcome, len(changes))
	for _, p := range passes {
		// Both sides of a translation are held to the term rules in force
		// now, which the pass resolves once.
		var rules []profile.TermRule
		if p.target {
			if rules, err = res.vocab.rulesFor(p.file, string(p.locale)); err != nil {
				return nil, "", err
			}
		}
		for _, before := range []bool{true, false} {
			blocks, owners := p.blocks(changes, sources, before)
			if len(blocks) == 0 {
				continue
			}
			diags, err := c.run(ctx, res, p, rules, blocks)
			if err != nil {
				return nil, "", err
			}
			for _, d := range diags {
				for _, i := range owners.of(d.Location.Block) {
					f := commitFinding(d, changes[i].Ref)
					if before {
						outcomes[i].Before = append(outcomes[i].Before, f)
					} else {
						outcomes[i].After = append(outcomes[i].After, f)
					}
				}
			}
		}
	}

	fingerprint, err := c.fingerprint(in, res, passes)
	if err != nil {
		return nil, "", err
	}
	return outcomes, fingerprint, nil
}

// commitProject is the project one commit check resolves from its command:
// the recipe, the directory it sits in, the recipe loaded, and the language
// the project's content is written in. The recipe is empty outside a project.
type commitProject struct {
	recipe string
	root   string
	proj   *project.KapiProject
	source string
}

// project is the project the check governs: none for a check built outside
// one, else the project its command names.
func (c *commitCheck) project() (commitProject, error) {
	if c.outside {
		return commitProject{source: c.app.commitSourceLocale(c.cmd, nil)}, nil
	}
	return c.app.resolveCommitProject(c.cmd)
}

// resolveCommitProject resolves the project cmd names, loading its recipe
// once for everything the check resolves from it.
func (a *App) resolveCommitProject(cmd Command) (commitProject, error) {
	recipe, err := ResolveProjectPath(cmd)
	if err != nil {
		return commitProject{}, err
	}
	in := commitProject{recipe: recipe}
	if recipe != "" {
		in.root = filepath.Dir(recipe)
		in.proj, err = project.LoadWithOptions(recipe, project.LoadOptions{SkipRequiresCheck: true})
		if err != nil {
			return commitProject{}, fmt.Errorf("load project %s: %w", DisplayName(recipe), err)
		}
	}
	in.source = a.commitSourceLocale(cmd, in.proj)
	return in, nil
}

// commitSourceLocale is the language a commit check reads the project's
// content in, in ResolveSourceLocale's order: a --source-lang cmd carries, or
// the one named when the MCP server started; then the recipe's
// defaults.source_language; then DefaultSourceLang.
//
// The App's SourceLang is neither read nor written. Kapi Desktop and the MCP
// server keep one App for several projects and run calls at once, and the
// field holds the language of whichever project resolved one last
// (mcpCallSourceLocale reads an MCP call's language the same way). A term
// lookup matches the locale exactly, so content read in another project's
// language would be held to none of its terms.
func (a *App) commitSourceLocale(cmd Command, proj *project.KapiProject) string {
	named := a.mcpNamedSourceLang
	if cmd != nil {
		if f := cmd.Flags().Lookup(sourceLangFlag); f != nil && f.Changed {
			named = f.Value.String()
		}
	}
	var recipe model.LocaleID
	if proj != nil {
		recipe = proj.Defaults.SourceLanguage
	}
	return ResolveSourceLocale(named, recipe)
}

// newCommitResolution resolves what the commit check holds editions to: the
// voice and the terms the project binds at each point, whatever flags cmd
// carries, read in the project's source language. It keeps no execution
// record, so the analyzers run without their canaries.
func (a *App) newCommitResolution(ctx context.Context, cmd Command, in commitProject) (*checkResolution, error) {
	voice, err := a.checkVoiceAt(cmd, nil, in.recipe, in.proj, func(root string) (profile.Store, func(), error) {
		return a.ProjectVoiceStore(ctx, root)
	})
	if err != nil {
		return nil, err
	}
	vocab := a.checkTermsAt(cmd, in.recipe, in.proj)
	vocab.sourceLocale = in.source
	opts := checkRunOptions{sourceLocale: in.source, editionsOnly: true}
	return &checkResolution{voice: voice, vocab: vocab, opts: opts}, nil
}

// commitPass is the changed editions of one document that meet one set of
// analyzers: the source analyzers, or the translation analyzers for one
// language.
type commitPass struct {
	// file is where the document sits, which picks its governance point.
	file string
	// locale is the language of the editions: the source language for the
	// source analyzers, a target language for the translation analyzers.
	locale model.LocaleID
	// target says the editions are translations.
	target bool
	// changes are the indexes of the changes the pass checks.
	changes []int
}

// plan groups the changes into passes, in the order their documents first
// appear.
func (c *commitCheck) plan(in commitProject, changes []change.EditionChange) ([]*commitPass, error) {
	var passes []*commitPass
	byKey := map[string]*commitPass{}
	for i, ch := range changes {
		file, err := commitDocFile(in.root, ch.Ref.Doc)
		if err != nil {
			return nil, err
		}
		target, locale := editionLanguage(ch, in.source)
		key := file + "\x00" + string(locale)
		if !target {
			key = file + "\x00"
		}
		p := byKey[key]
		if p == nil {
			p = &commitPass{file: file, locale: locale, target: target}
			byKey[key] = p
			passes = append(passes, p)
		}
		p.changes = append(p.changes, i)
	}
	return passes, nil
}

// commitDocFile resolves a document reference to the file whose point governs
// it. A member of an archive (container!entry) sits where its container sits.
func commitDocFile(root, doc string) (string, error) {
	if doc == "" {
		return "", errors.New("a changed edition names no document")
	}
	path, _, _ := strings.Cut(doc, "!")
	switch {
	case filepath.IsAbs(path):
		return path, nil
	case root != "":
		return filepath.Join(root, filepath.FromSlash(path)), nil
	}
	return filepath.Abs(filepath.FromSlash(path))
}

// editionLanguage says whether a changed edition is a translation, and the
// language it is in. The authoritative edition, and an edition in the source
// language such as a channel edition, is source-language prose.
func editionLanguage(ch change.EditionChange, sourceLang string) (target bool, locale model.LocaleID) {
	src := model.NormalizeLocale(model.LocaleID(sourceLang))
	if ch.Block != nil && ch.Block.SourceLocale != "" {
		src = model.NormalizeLocale(ch.Block.SourceLocale)
	}
	if authoritative(ch) {
		return false, src
	}
	loc := model.NormalizeLocale(ch.Ref.Edition.Locale)
	if loc == src {
		return false, src
	}
	return true, loc
}

// authoritative reports whether a changed edition is the block's
// authoritative edition.
func authoritative(ch change.EditionChange) bool {
	switch {
	case ch.Role == change.RoleAuthoritative, ch.Ref.Edition.IsZero():
		return true
	case ch.Role == change.RoleDerived:
		return false
	}
	return ch.Block != nil && ch.Block.IsSourceEdition(ch.Ref.Edition)
}

// removed reports whether the change set removed the edition, which leaves
// nothing after the change to check: remove_edition, or delete_block taking
// the edition with its block. The service says so with an AfterRev of
// model.AbsentRevision. A change that carries no revisions says it with no
// runs after the change, on a derived edition or in a block that is gone. The
// authoritative edition of a block that still exists cannot be removed, and
// one with no runs was emptied, which the hygiene analyzer holds it to.
func removed(ch change.EditionChange) bool {
	switch {
	case ch.AfterRev == model.AbsentRevision:
		return true
	case len(ch.After) > 0:
		return false
	}
	return ch.Block == nil || !authoritative(ch)
}

// blockRef names one block of one document.
type blockRef struct{ doc, block string }

// sourceChanges maps each block whose authoritative edition the change set
// changed to that change.
func sourceChanges(changes []change.EditionChange) map[blockRef]int {
	out := map[blockRef]int{}
	for i, ch := range changes {
		if authoritative(ch) {
			out[blockRef{ch.Ref.Doc, ch.Ref.Block}] = i
		}
	}
	return out
}

// commitOwners maps the key of each analysis block to the change it stands
// for.
type commitOwners struct {
	byKey map[string]int
	all   []int
}

// of returns the changes a diagnostic located at block belongs to: the one
// whose analysis block it names, or every change of the pass for a diagnostic
// that names no block.
func (o commitOwners) of(block string) []int {
	if i, ok := o.byKey[block]; ok {
		return []int{i}
	}
	return o.all
}

// blocks builds one analysis block per change of the pass that holds the
// edition on that side of the change. Each is keyed by its change's index, so
// a diagnostic names the change it was found on.
//
// A translation is checked against the authoritative edition beside it on the
// same side: after the change, the block's; before it, the authoritative
// edition as it stood, which sources gives when the change set changed that
// too.
func (p *commitPass) blocks(changes []change.EditionChange, sources map[blockRef]int, before bool) ([]*model.Block, commitOwners) {
	owners := commitOwners{byKey: map[string]int{}}
	var blocks []*model.Block
	for _, i := range p.changes {
		ch := changes[i]
		runs := ch.After
		if before {
			runs = ch.Before
			if runs == nil {
				// The change set created the edition.
				continue
			}
		} else if removed(ch) {
			continue
		}
		key := fmt.Sprintf("edition-%d", i)
		b := analysisBlock(ch.Block, key)
		if p.target {
			if j, ok := sources[blockRef{ch.Ref.Doc, ch.Ref.Block}]; ok && before {
				b.Source = changes[j].Before
			}
			b.SetTargetRuns(p.locale, runs)
		} else {
			b.Source = runs
		}
		blocks = append(blocks, b)
		owners.byKey[key] = i
		owners.all = append(owners.all, i)
	}
	return blocks, owners
}

// analysisBlock is a block the analyzers read in place of from: its kind, its
// language and its properties, with from's authoritative edition as its
// source and no check results. Nothing the analyzers write reaches from.
func analysisBlock(from *model.Block, key string) *model.Block {
	b := &model.Block{ID: key, Translatable: true}
	if from == nil {
		return b
	}
	b.Type, b.MimeType = from.Type, from.MimeType
	b.Translatable = from.Translatable
	b.SourceLocale = from.SourceLocale
	b.PreserveWhitespace = from.PreserveWhitespace
	b.Source = from.Source
	if len(from.Properties) > 0 {
		b.Properties = make(map[string]string, len(from.Properties))
		for k, v := range from.Properties {
			switch k {
			case coretools.PropTermCheckErrors, coretools.PropTermCheckWarnings, voiceVocabFindingsProp:
				continue
			}
			b.Properties[k] = v
		}
	}
	return b
}

// voiceVocabFindingsProp is the property a vocabulary check leaves its
// findings on when it cannot annotate (runVoiceVocabOnBlock reads it).
const voiceVocabFindingsProp = "voice-vocab-findings"

// run runs a pass's analyzers over its analysis blocks. rules are the term
// rules a translation pass holds its editions to.
func (c *commitCheck) run(ctx context.Context, res *checkResolution, p *commitPass, rules []profile.TermRule, blocks []*model.Block) ([]check.Diagnostic, error) {
	a := c.app
	if !p.target {
		return a.checkBlocks(ctx, res, p.file, blocks)
	}
	return a.collectBilingualDiagnostics(ctx, blocks, p.file, p.locale, nil, rules, res.opts)
}

// commitFinding is a check diagnostic as the change contract reports it, at
// the edition it was found on.
func commitFinding(d check.Diagnostic, at change.Ref) change.Finding {
	ref := at
	return change.Finding{Rule: d.Rule, Message: d.Message, Fails: d.Fails, Suggested: d.Suggested, At: &ref}
}

// fingerprint is the governance fingerprint of the editions the passes
// checked. Each pass sits at one point, its document's, in one language, and
// the governance there has the fingerprint the staleness gate recomputes for
// that document and language (contextFingerprints): the voice guide and the
// term rules in force. The voice is read from the store the editions were
// checked under.
//
// A change set whose passes share one fingerprint has it. One whose passes
// sit under several, such as a source edit beside its translation or edits to
// documents at two points, has tool.OverlayConfigFingerprint over the distinct
// fingerprints in sorted order, which a reader of the record recomputes from
// the documents and languages of its transitions. Ungoverned editions add
// nothing, and a change set none of whose editions are governed has none.
func (c *commitCheck) fingerprint(in commitProject, res *checkResolution, passes []*commitPass) (string, error) {
	if in.proj == nil {
		return "", nil
	}
	current := &contextFingerprints{
		app: c.app, cmd: c.cmd, proj: in.proj, root: in.root, store: res.voice.store,
		cache: map[string]governingContext{}, source: in.source,
	}
	// The voice and the terms at a point follow from the profile and the
	// channel governing it, as the check's own resolvers assume, so documents
	// at one governance resolve it once.
	type governed struct{ profile, channel, locale string }
	resolved := map[governed]string{}
	var fps []string
	for _, p := range passes {
		point := c.app.governancePointForFile(in.root, p.file)
		rc, err := c.app.ResolveGovernanceAtPoint(c.cmd, in.proj, point)
		if err != nil {
			return "", err
		}
		key := governed{rc.Profile, rc.Channel, string(p.locale)}
		fp, ok := resolved[key]
		if !ok {
			g, err := current.at(point, string(p.locale))
			if err != nil {
				return "", err
			}
			fp = g.fingerprint
			resolved[key] = fp
		}
		if fp != "" && !slices.Contains(fps, fp) {
			fps = append(fps, fp)
		}
	}
	switch len(fps) {
	case 0:
		return "", nil
	case 1:
		return fps[0], nil
	}
	slices.Sort(fps)
	return tool.OverlayConfigFingerprint(fps...), nil
}
