package host

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/tool"
)

// planReuse puts the drafting step's reuse question to the plan (#2369).
//
// A pass serves a block from the project block store instead of a provider
// when the store holds a translation of that exact source, made under the
// producer's current configuration fingerprint and the governing context in
// force (blockstore.TargetOverlay.ReusableFor). The configuration fingerprint
// is assembled inside the producer from everything it was built with, so the
// plan does not reconstruct it: it builds the flow's producers the way a pass
// builds them, for the locale and governance point each unit sits at, and asks
// each producer directly (tool.StoredTargetReuser). One function answers for
// the run and for the plan, so the two cannot drift.
//
// The chain is built once per (locale, point) and reads nothing but the store
// and the content memory. Building it calls no provider: a producer resolves
// its configuration at construction and calls out only when handed a block.
type planReuse struct {
	app        *App
	cmd        Command
	pctx       *project.ProjectContext
	projectDir string
	fl         upPlanFlow
	store      blockstore.Store
	bindings   *localeBindings
	// producers caches each (locale, point)'s reusers. A nil entry is a chain
	// that could not be built, whose units stay priced as provider work.
	producers map[string][]tool.StoredTargetReuser
	cleanups  []func()
}

// newPlanReuse prepares the reuse question for a plan, or returns nil when
// there is nothing to ask: no store on disk (a dry run creates none, and an
// absent store holds no drafts), or a flow with no drafting step.
func (a *App) newPlanReuse(ctx context.Context, basis upPlanBasis, proj *project.KapiProject, fl upPlanFlow) *planReuse {
	if basis.store == nil || !fl.Drafts || fl.Spec == nil || basis.projectPath == "" {
		return nil
	}
	cmd, err := a.unitCommand(ctx, basis.projectPath)
	if err != nil {
		return nil
	}
	abs, err := filepath.Abs(basis.projectPath)
	if err != nil {
		abs = basis.projectPath
	}
	return &planReuse{
		app:        a,
		cmd:        cmd,
		pctx:       project.NewProjectContext(proj, basis.projectPath),
		projectDir: filepath.Dir(abs),
		fl:         fl,
		store:      basis.store,
		bindings:   a.newLocaleBindings(cmd, proj, basis.projectPath),
		producers:  map[string][]tool.StoredTargetReuser{},
	}
}

// reuses reports whether a drafting producer of the flow would serve the stored
// target for b rather than call a provider. An absent overlay, one no producer
// stands behind, or a chain that could not be built all read as no; a store
// that cannot be read is an error, as it is for every other reader.
func (p *planReuse) reuses(ctx context.Context, u VerifyUnit, b *model.Block) (bool, error) {
	if p == nil {
		return false, nil
	}
	stored, ok, err := p.storedTarget(ctx, u, b)
	if err != nil || !ok {
		return false, err
	}
	for _, r := range p.producersFor(u) {
		if r.ReusesStoredTarget(ctx, b, stored) {
			return true, nil
		}
	}
	return false, nil
}

// storedTarget reads the overlay the store holds for one block of a unit, as
// the producer will read it. A payload the producer cannot decode is one it
// re-drafts over, so it reads as no stored target here too.
func (p *planReuse) storedTarget(ctx context.Context, u VerifyUnit, b *model.Block) (blockstore.TargetOverlay, bool, error) {
	var none blockstore.TargetOverlay
	kind, key, ok := storedTargetKey(u, b)
	if !ok {
		return none, false, nil
	}
	sess, err := p.store.Begin(ctx)
	if err != nil {
		return none, false, fmt.Errorf("read the stored targets for %s: %w", u.Locale, err)
	}
	defer sess.Close()
	o, err := sess.GetOverlay(kind, key)
	if err != nil {
		if errors.Is(err, blockstore.ErrNotFound) {
			return none, false, nil
		}
		return none, false, fmt.Errorf("read %s overlay for block %s of %s: %w", kind, blockKey(b), u.DisplayPath, err)
	}
	if len(o.Payload) == 0 {
		return none, false, nil
	}
	var stored blockstore.TargetOverlay
	if err := json.Unmarshal(o.Payload, &stored); err != nil {
		return none, false, nil
	}
	return stored, true, nil
}

// producersFor returns the drafting producers built for the unit's locale at
// its governance point, building them on first ask.
func (p *planReuse) producersFor(u VerifyUnit) []tool.StoredTargetReuser {
	point := p.app.unitGovernancePoint(p.projectDir, u)
	key := strings.Join([]string{u.Locale, point.Profile, point.Collection, point.Path}, "\x00")
	if r, ok := p.producers[key]; ok {
		return r
	}
	r := p.build(u.Locale, point)
	p.producers[key] = r
	return r
}

// build assembles the flow's tool chain for one locale at one governance point
// exactly as a pass does: a worker for the locale, the bindings resolved at the
// point for that locale, and buildProjectFlowTools over the same spec. It keeps
// the producers that answer the reuse question.
//
// A chain that cannot be built answers nothing, and the units it would have
// served stay priced as provider work. The run could not start there either,
// and the ceiling is the honest figure until it can.
func (p *planReuse) build(locale string, point project.GovernancePoint) []tool.StoredTargetReuser {
	b, err := p.bindings.at(point, locale)
	if err != nil {
		return nil
	}
	worker := p.app.convergeWorker(locale, nil)
	worker.ProjectContext = p.pctx
	worker.ProjectBindings = b
	rCtx := flow.ResourceContext{ProjectDir: p.projectDir, SourceLocale: worker.SourceLang, TargetLocale: locale}
	tools, cleanup, err := worker.buildProjectFlowTools(p.cmd, p.fl.Name, p.fl.Spec, &rCtx, nil)
	if cleanup != nil {
		p.cleanups = append(p.cleanups, cleanup)
	}
	if err != nil {
		return nil
	}
	var out []tool.StoredTargetReuser
	for _, t := range tools {
		if r, ok := t.(tool.StoredTargetReuser); ok {
			out = append(out, r)
		}
	}
	return out
}

// close releases what building the chains opened. Safe on a nil receiver, so a
// plan with nothing to ask defers it all the same.
func (p *planReuse) close() {
	if p == nil {
		return
	}
	for _, fn := range p.cleanups {
		fn()
	}
	p.cleanups = nil
}
