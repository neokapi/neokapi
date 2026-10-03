package host

import (
	"context"
	"fmt"
	"io"
	"path/filepath"

	"github.com/neokapi/neokapi/core/change/filehome"
	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
)

// toolOutput is one document `kapi exec` writes, on its way to the file home
// a flow commits through (host/flowchanges.go): the destination's digest when
// the run began, and the follower that records or prints what the run
// changed.
type toolOutput struct {
	home   *filehome.Home
	before string
	run    flow.DocumentRun
	done   bool
}

// projectRoot is the root of the project in scope, "" outside one.
func (a *App) projectRoot() string {
	if a.ProjectContext == nil {
		return ""
	}
	return a.ProjectContext.ProjectDir
}

// toolRunRoot is the root of the project a tool run over one file commits and
// records in: the project the command resolved (ToolRunConfig.Project) when it
// holds both the file the run reads and the file it writes, else none. It is
// the project kapi apply resolves for that file, so the run locks the file's
// lock in the directory kapi apply locks it in, records what it changed in the
// project's history, and names the document as kapi apply does.
func (cfg ToolRunConfig) toolRunRoot(paths ...string) string {
	if cfg.Project == "" {
		return ""
	}
	root, err := filepath.Abs(filepath.Dir(cfg.Project))
	if err != nil {
		return ""
	}
	for _, p := range paths {
		abs, err := filepath.Abs(p)
		if err != nil {
			return ""
		}
		if rel, rerr := filepath.Rel(root, abs); rerr != nil || !filepath.IsLocal(rel) {
			return ""
		}
	}
	return root
}

// toolSourceLocale is the language a tool run's documents are written in, as
// the change service of the project at root reads them: the one the command
// names, else the recipe's, else the default.
func (a *App) toolSourceLocale(root string) model.LocaleID {
	recipe := ""
	if root != "" && a.SourceLang == "" {
		if proj, err := project.LoadWithOptions(project.LayoutAt(root).RecipePath, project.LoadOptions{SkipRequiresCheck: true}); err == nil {
			recipe = string(proj.Defaults.SourceLanguage)
		}
	}
	return model.LocaleID(ResolveSourceLocale(a.SourceLang, model.LocaleID(recipe)))
}

// openToolOutput digests the destination of a tool run over one file and
// opens its follower. It is called before the run reads the file.
func (a *App) openToolOutput(ctx context.Context, cfg ToolRunConfig, d flow.Document) (*toolOutput, error) {
	if err := flow.CheckOutputPath(d.OutputPath); err != nil {
		return nil, err
	}
	before, err := filehome.Digest(d.OutputPath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", d.OutputPath, err)
	}
	root := cfg.toolRunRoot(d.InputPath, d.OutputPath)
	home, docs := a.flowDocumentsIn(ctx, nil, root, a.toolSourceLocale(root))
	out := &toolOutput{home: home, before: before}
	if docs != nil {
		if out.run, err = docs.Open(ctx, d); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// enter shows the follower each block as the reader gave it.
func (t *toolOutput) enter(parts []*model.Part) {
	if t == nil || t.run == nil {
		return
	}
	for _, p := range parts {
		if b, ok := p.Resource.(*model.Block); ok && p.Type == model.PartBlock && b != nil {
			t.run.Enter(b)
		}
	}
}

// leave shows the follower a block as the writer receives it.
func (t *toolOutput) leave(p *model.Part) {
	if t == nil || t.run == nil {
		return
	}
	if b, ok := p.Resource.(*model.Block); ok && p.Type == model.PartBlock && b != nil {
		t.run.Leave(b)
	}
}

// write stages what produce writes as the document at path and commits it.
func (t *toolOutput) write(ctx context.Context, path string, produce func(io.Writer) error) error {
	p, err := t.home.Produce(ctx, path, t.before, produce)
	if err != nil {
		return err
	}
	defer func() { _ = p.Release() }()
	t.done = true
	if t.run != nil {
		return t.run.Commit(ctx, p)
	}
	return p.Commit(ctx)
}

// abort ends a document the run did not write.
func (t *toolOutput) abort() {
	if t == nil || t.done {
		return
	}
	t.done = true
	if t.run != nil {
		t.run.Abort()
	}
}
