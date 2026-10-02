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
func (c *commitCheck) Check(ctx context.Context, changes []change.EditionChange) ([]change.CheckOutcome, string, error) {
	a := c.app
	a.InitRegistries()
	a.applyProjectSourceLang(c.cmd)
	recipe, err := ResolveProjectPath(c.cmd)
	if err != nil {
		return nil, "", err
	}
	root := ""
	if recipe != "" {
		root = filepath.Dir(recipe)
	}

	res, err := a.newCommitResolution(ctx, c.cmd)
	if err != nil {
		return nil, "", err
	}
	defer res.close()

	outcomes := make([]change.CheckOutcome, len(changes))
	passes, err := c.plan(root, res.opts.source(a), changes)
	if err != nil {
		return nil, "", err
	}
	for _, p := range passes {
		for _, before := range []bool{true, false} {
			blocks, owners := p.blocks(changes, before)
			if len(blocks) == 0 {
				continue
			}
			diags, err := c.run(ctx, res, p, blocks)
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

	fingerprint, err := c.fingerprint(recipe, root, passes)
	if err != nil {
		return nil, "", err
	}
	return outcomes, fingerprint, nil
}

// newCommitResolution resolves what the commit check holds editions to: the
// voice and the terms the project cmd resolves binds at each point, whatever
// flags cmd carries, read in the project's source language. It keeps no
// execution record, so the analyzers run without their canaries.
func (a *App) newCommitResolution(ctx context.Context, cmd Command) (*checkResolution, error) {
	voice, err := a.newProjectCheckVoice(ctx, cmd, nil)
	if err != nil {
		return nil, err
	}
	vocab, err := a.newCheckTerms(cmd)
	if err != nil {
		voice.close()
		return nil, err
	}
	opts := checkRunOptions{sourceLocale: a.SourceLocale(), editionsOnly: true}
	return &checkResolution{voice: voice, vocab: vocab, opts: opts}, nil
}

// commitPass is the changed editions of one document that meet one set of
// analyzers: the source analyzers, or the translation analyzers for one
// language.
type commitPass struct {
	// file is where the document sits, which picks its governance point.
	file string
	// rel is file relative to the project root, empty outside a project.
	rel string
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
func (c *commitCheck) plan(root, sourceLang string, changes []change.EditionChange) ([]*commitPass, error) {
	var passes []*commitPass
	byKey := map[string]*commitPass{}
	for i, ch := range changes {
		file, rel, err := commitDocFile(root, ch.Ref.Doc)
		if err != nil {
			return nil, err
		}
		target, locale := editionLanguage(ch, sourceLang)
		key := file + "\x00" + string(locale)
		if !target {
			key = file + "\x00"
		}
		p := byKey[key]
		if p == nil {
			p = &commitPass{file: file, rel: rel, locale: locale, target: target}
			byKey[key] = p
			passes = append(passes, p)
		}
		p.changes = append(p.changes, i)
	}
	return passes, nil
}

// commitDocFile resolves a document reference to the file whose point governs
// it. A member of an archive (container!entry) sits where its container sits.
func commitDocFile(root, doc string) (file, rel string, err error) {
	if doc == "" {
		return "", "", errors.New("a changed edition names no document")
	}
	path, _, _ := strings.Cut(doc, "!")
	if filepath.IsAbs(path) {
		file = path
	} else if root != "" {
		file = filepath.Join(root, filepath.FromSlash(path))
	} else if file, err = filepath.Abs(filepath.FromSlash(path)); err != nil {
		return "", "", err
	}
	if root != "" {
		rel, _ = projectRelPath(root, file)
	}
	return file, rel, nil
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
func (p *commitPass) blocks(changes []change.EditionChange, before bool) ([]*model.Block, commitOwners) {
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
		} else if len(runs) == 0 && !authoritative(ch) {
			// The change set removed a derived edition. The authoritative
			// edition is never removed, so an empty one was emptied, which
			// the hygiene analyzer holds it to.
			continue
		}
		key := fmt.Sprintf("edition-%d", i)
		b := analysisBlock(ch.Block, key)
		if p.target {
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

// run runs a pass's analyzers over its analysis blocks.
func (c *commitCheck) run(ctx context.Context, res *checkResolution, p *commitPass, blocks []*model.Block) ([]check.Diagnostic, error) {
	a := c.app
	if !p.target {
		return a.checkBlocks(ctx, res, p.file, blocks)
	}
	rules, err := res.vocab.rulesFor(p.file, string(p.locale))
	if err != nil {
		return nil, err
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
// checked: the one the staleness gate recomputes for a document and a
// language (contextFingerprints), the voice guide and the term rules in force
// there. A change set whose editions sit under one governance has that
// fingerprint; one spanning several has a fingerprint over theirs, in sorted
// order. Ungoverned editions add nothing, and a change set none of whose
// editions are governed has none.
func (c *commitCheck) fingerprint(recipe, root string, passes []*commitPass) (string, error) {
	if recipe == "" {
		return "", nil
	}
	proj, err := project.LoadWithOptions(recipe, project.LoadOptions{SkipRequiresCheck: true})
	if err != nil {
		return "", fmt.Errorf("load project for the governance fingerprint: %w", err)
	}
	current, err := newContextFingerprints(c.app, c.cmd, proj, root)
	if err != nil {
		return "", err
	}
	defer current.close()
	// The voice and the terms at a point follow from the profile and the
	// channel governing it, as the check's own resolvers assume, so documents
	// at one governance resolve it once.
	type governed struct{ profile, channel, locale string }
	resolved := map[governed]string{}
	var fps []string
	for _, p := range passes {
		point := c.app.GovernancePointFor("", p.rel)
		rc, err := c.app.ResolveGovernanceAtPoint(c.cmd, proj, point)
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
