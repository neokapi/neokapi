package host

import (
	"context"
	"fmt"
	"io"

	"github.com/neokapi/neokapi/core/change/filehome"
	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/model"
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

// openToolOutput digests the destination of a tool run over one file and
// opens its follower. It is called before the run reads the file.
func (a *App) openToolOutput(ctx context.Context, d flow.Document) (*toolOutput, error) {
	if err := flow.CheckOutputPath(d.OutputPath); err != nil {
		return nil, err
	}
	before, err := filehome.Digest(d.OutputPath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", d.OutputPath, err)
	}
	home, docs := a.flowDocuments(ctx, nil, a.projectRoot())
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
	defer p.Release()
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
