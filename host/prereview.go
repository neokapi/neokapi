package host

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strconv"

	aitools "github.com/neokapi/neokapi/core/ai/tools"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/schema"
	"github.com/neokapi/neokapi/core/state"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/neokapi/neokapi/host/config"
	"github.com/neokapi/neokapi/host/credentials"
)

// PreReviewOptions narrows an AI pre-review run and lets a caller follow it.
type PreReviewOptions struct {
	// Languages limits the run to these target languages. Empty is every
	// language the queue holds.
	Languages []string
	// Collection limits the run to one content collection. Empty is every
	// collection.
	Collection string
	// Model is the reviewer the annotations name. Empty resolves it from the
	// review tool's assembled config, the app's AI defaults and its credential
	// store (ReviewerModel).
	Model string
	// NewTool builds the review tool from its assembled config. Nil builds it
	// through the app's tool registry, whose config preprocessor applies the
	// AI defaults and resolves credentials.
	NewTool func(name string, cfg map[string]any, targetLang string) (tool.Tool, error)
	// UnitContext, when set, derives the context each block's model call runs
	// under, so a caller can attribute the call to the block.
	UnitContext func(ctx context.Context, it ReviewQueueItem) context.Context
	// Progress, when set, is called before each block is reviewed.
	Progress func(done, total int, it ReviewQueueItem)
}

// PreReviewResult counts what a pre-review run did.
type PreReviewResult struct {
	// Model is the reviewer the annotations name.
	Model string `json:"model"`
	// Reviewed counts the blocks annotated with a score.
	Reviewed int `json:"reviewed"`
	// Skipped counts the blocks left without an annotation: the model gave no
	// usable score, or the block could not be read again.
	Skipped int `json:"skipped,omitempty"`
}

// formatLine writes the one-line summary of a pre-review. A nil result
// writes nothing.
func (r *PreReviewResult) formatLine(w io.Writer) {
	if r == nil {
		return
	}
	line := fmt.Sprintf("AI pre-review: %s scored", pluralUnit(r.Reviewed, "block", "blocks"))
	if r.Model != "" {
		line += " by " + r.Model
	}
	if r.Skipped > 0 {
		line += fmt.Sprintf(", %d without a usable score", r.Skipped)
	}
	fmt.Fprintln(w, line+". Review them with kapi status --review.")
}

// RunAIPreReview runs the AI review tool over the translations awaiting review
// and records each score and its findings as an advisory annotation in the
// project's state store, bound to the translation it judged
// (RecordAIReviews). The judge for each file is built the way a run builds a
// tool for it (ToolConfigForUnit), so it scores against the voice and terms in
// force there, and it runs inside the redaction chain that covers the file.
// An annotation never decides a block: every block stays in the queue for a
// person.
func (a *App) RunAIPreReview(ctx context.Context, projectPath, sourceLang string, opts PreReviewOptions) (*PreReviewResult, error) {
	a.InitRegistries()
	ctx = ctxOrBackground(ctx)
	proj, err := project.LoadWithOptions(projectPath, project.LoadOptions{SkipRequiresCheck: true})
	if err != nil {
		return nil, fmt.Errorf("load project: %w", err)
	}
	root := filepath.Dir(projectPath)
	a.SourceLang = ResolveSourceLocale(sourceLang, proj.Defaults.SourceLanguage)
	source := a.SourceLang

	res := &PreReviewResult{Model: opts.Model}

	all, err := a.UnitsFromProject(proj, root, "")
	if err != nil {
		return nil, fmt.Errorf("resolve content: %w", err)
	}
	filter := ReviewQueueOptions{Languages: opts.Languages}
	var units []VerifyUnit
	for _, u := range all {
		if filter.wants(u.Locale) && (opts.Collection == "" || u.Collection == opts.Collection) {
			units = append(units, u)
		}
	}

	err = a.withParseCache(root, func() error {
		queue, qerr := a.computeReviewQueue(ctx, proj, root, units, a.newUnreadSetFor("pre-reviewed"), nil)
		if qerr != nil {
			return fmt.Errorf("compute review queue: %w", qerr)
		}
		// One group per (file, locale), so each pair is read once and its
		// annotations land in one state write.
		type groupKey struct{ file, locale string }
		groups := map[groupKey][]ReviewQueueItem{}
		var order []groupKey
		for _, it := range queue {
			k := groupKey{file: it.File, locale: it.Locale}
			if _, seen := groups[k]; !seen {
				order = append(order, k)
			}
			groups[k] = append(groups[k], it)
		}
		p := &preReview{app: a, proj: proj, recipe: projectPath, root: root, source: source,
			opts: opts, res: res, total: len(queue)}
		for _, k := range order {
			if err := p.group(ctx, units, k.file, k.locale, groups[k]); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return res, nil
}

// preReview is one pre-review run in progress.
type preReview struct {
	app    *App
	proj   *project.KapiProject
	recipe string
	root   string
	source string
	opts   PreReviewOptions
	res    *PreReviewResult
	done   int
	total  int
}

// group reviews the queued blocks of one (file, locale) pair and records their
// annotations.
func (p *preReview) group(ctx context.Context, units []VerifyUnit, file, locale string, items []ReviewQueueItem) error {
	var unit *VerifyUnit
	for i := range units {
		if units[i].Locale == locale && units[i].DisplayPath == file {
			unit = &units[i]
			break
		}
	}
	var blocks []*model.Block
	if unit != nil {
		read, missing, err := p.app.bilingualBlocks(ctx, *unit)
		if err == nil && !missing {
			blocks = read
		}
	}
	if blocks == nil {
		p.done += len(items)
		p.res.Skipped += len(items)
		return nil
	}
	byKey := make(map[string]*model.Block, len(blocks))
	for _, b := range blocks {
		if b.Translatable {
			byKey[blockKey(b)] = b
		}
	}

	ref := UnitRef{Collection: unit.Collection, Path: relativeToRoot(p.root, unit.SourcePath), TargetLang: locale}
	chain, reviewer, release, err := p.reviewTool(ctx, ref, unit.SourcePath)
	if err != nil {
		return err
	}
	defer release()
	if p.res.Model == "" {
		p.res.Model = reviewer
	}

	annotations := map[string]state.AIReview{}
	for _, it := range items {
		p.done++
		if p.opts.Progress != nil {
			p.opts.Progress(p.done, p.total, it)
		}
		b, found := byKey[it.Key]
		if !found {
			p.res.Skipped++
			continue
		}
		unitCtx := ctx
		if p.opts.UnitContext != nil {
			unitCtx = p.opts.UnitContext(ctx, it)
		}
		if err := RunTools(unitCtx, chain, b); err != nil {
			return fmt.Errorf("ai pre-review %s:%s: %w", it.File, it.Key, err)
		}
		rev, ok := reviewAnnotation(b, reviewer)
		if !ok {
			p.res.Skipped++
			continue
		}
		annotations[it.Key] = rev
		p.res.Reviewed++
	}
	if len(annotations) == 0 {
		return nil
	}
	if _, err := p.app.RecordAIReviews(ctx, p.recipe, p.source, locale, file, annotations); err != nil {
		return fmt.Errorf("record ai reviews: %w", err)
	}
	return nil
}

// reviewTool builds the review tool for one unit as a run builds it, inside
// the redaction chain that covers the unit's file, and names the reviewer its
// annotations record: PreReviewOptions.Model, or the one the assembled config
// resolves to (ReviewerModel). The release function frees what the assembly
// opened and is always safe to call.
func (p *preReview) reviewTool(ctx context.Context, ref UnitRef, file string) ([]tool.Tool, string, func(), error) {
	noop := func() {}
	const name = "review"
	cfg, release, err := p.app.ToolConfigForUnit(ctx, p.proj, p.recipe, name, ref, map[string]any{"sourceLocale": p.source})
	if err != nil {
		return nil, "", noop, err
	}
	if release == nil {
		release = noop
	}
	reviewer := p.opts.Model
	if reviewer == "" {
		reviewer, err = ReviewerModel(p.app.Config, p.app.Credentials, cfg)
		if err == nil && reviewer == "" {
			err = errors.New("no AI provider is configured for the review tool: run `kapi models setup`")
		}
		if err != nil {
			release()
			return nil, "", noop, err
		}
	}
	var tl tool.Tool
	if p.opts.NewTool != nil {
		tl, err = p.opts.NewTool(name, cfg, ref.TargetLang)
	} else {
		tl, err = p.app.ToolReg.NewToolWithConfig(registry.ToolID(name), cfg, ref.TargetLang)
	}
	if err != nil {
		release()
		return nil, "", noop, err
	}
	chain, err := p.app.RedactionChain(p.recipe, p.proj, file, name, cfg, tl)
	if err != nil {
		release()
		return nil, "", noop, err
	}
	return chain, reviewer, release, nil
}

// reviewAnnotation reads the review tool's result off a block: the score and
// the findings. False when the model gave no usable score.
func reviewAnnotation(b *model.Block, modelID string) (state.AIReview, bool) {
	raw, ok := b.Properties["review-score"]
	if !ok {
		return state.AIReview{}, false
	}
	score, err := strconv.Atoi(raw)
	if err != nil {
		return state.AIReview{}, false
	}
	rev := state.AIReview{Score: score, Model: modelID}
	if parsed, perr := aitools.ParseReviewResult(b.Properties["review"]); perr == nil {
		for _, f := range parsed.Findings {
			rev.Findings = append(rev.Findings, state.AIReviewFinding{
				Severity: f.Severity, Message: f.Message, Suggestion: f.Suggestion,
			})
		}
	}
	return rev, true
}

// ReviewerModel is the name a pre-review records its reviewer under: the model
// the review tool's config names, else the shared ai.model default, the model
// the credential names, or with none of these the provider. toolCfg may be
// nil. A nil store skips credential resolution, and with nothing configured
// the name is empty.
func ReviewerModel(cfg *config.AppConfig, store *credentials.Store, toolCfg map[string]any) (string, error) {
	requires := []string{schema.RequiresCredentials}
	c := make(map[string]any, len(toolCfg))
	for k, v := range toolCfg {
		if k == "provider" || k == "model" {
			c[k] = v
		}
	}
	c = ApplyAIDefaults(cfg, "review", requires, c)
	if store != nil {
		resolved, err := credentials.ResolveCredentials(store, "review", requires, c)
		if err != nil {
			return "", err
		}
		c = resolved
	}
	if m, _ := c["model"].(string); m != "" {
		return m, nil
	}
	if p, _ := c["provider"].(string); p != "" {
		return p, nil
	}
	return "", nil
}
