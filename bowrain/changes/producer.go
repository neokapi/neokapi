package changes

import (
	"context"

	platev "github.com/neokapi/neokapi/bowrain/core/event"
	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/venue"
)

// Producer commits what a server job's tool produced from the blocks of one
// stream it read: the job records the state of each block before the tool
// changes it in place (Read), and commits the blocks the tool hands back
// (Commit) through the stream's change service, as the tool. Nothing holds
// the job to a request's permissions; the request that started it was
// authorized.
type Producer struct {
	home     *Home
	registry *registry.FormatRegistry
	rec      Recorder
	states   map[string]BlockState
	items    map[string]string
}

// NewProducer returns a producer for one stream of proj. publish announces each
// change set that lands; nil announces nothing.
func NewProducer(cs store.ContentStore, proj *store.Project, stream string, reg *registry.FormatRegistry, publish func(context.Context, platev.Event)) *Producer {
	if stream == "" {
		stream = "main"
	}
	return &Producer{
		home: &Home{Store: cs, ProjectID: proj.ID, Stream: stream, SourceLocale: proj.DefaultSourceLanguage,
			Locales: proj.TargetLanguages, Registry: reg},
		registry: reg,
		rec:      Recorder{Publish: publish, WorkspaceID: proj.WorkspaceID, ProjectID: proj.ID, Stream: stream},
		states:   map[string]BlockState{},
		items:    map[string]string{},
	}
}

// Read records the state of each block the job read, before a tool changes
// it in place.
func (p *Producer) Read(blocks []*venue.StoredBlock) {
	for _, sb := range blocks {
		if sb == nil || sb.Block == nil {
			continue
		}
		p.states[sb.Block.ID] = Snapshot(sb.Block, p.home.SourceLocale)
		p.items[sb.Block.ID] = sb.ItemName
	}
}

// Track records the state blocks the producer read now hold, once a commit
// landed them, so the next pass of a job over the same blocks sends only what
// that pass changes.
func (p *Producer) Track(blocks []*model.Block) {
	for _, b := range blocks {
		if b == nil {
			continue
		}
		if _, ok := p.states[b.ID]; ok {
			p.states[b.ID] = Snapshot(b, p.home.SourceLocale)
		}
	}
}

// Commit writes what the tool made of the blocks it ran over and returns the
// blocks that landed. A block the job did not read is left out; one a person
// changed since the read, or whose source moved, keeps what it holds
// (CommitDrafts).
func (p *Producer) Commit(ctx context.Context, tool string, blocks []*model.Block) ([]*model.Block, error) {
	drafts := make([]Draft, 0, len(blocks))
	byID := make(map[string]*model.Block, len(blocks))
	for _, b := range blocks {
		if b == nil {
			continue
		}
		state, ok := p.states[b.ID]
		if !ok {
			continue
		}
		drafts = append(drafts, Draft{Doc: p.items[b.ID], Before: state, After: b})
		byID[b.ID] = b
	}
	if len(drafts) == 0 {
		return nil, nil
	}
	svc := NewService(p.home, p.registry, change.WithRecorder(p.rec))
	ctx, _ = WithChange(ctx, "")
	_, _, landed, err := CommitDrafts(ctx, svc, tool, drafts)
	if err != nil {
		return nil, err
	}
	out := make([]*model.Block, 0, len(landed))
	for _, id := range landed {
		if b := byID[id]; b != nil {
			out = append(out, b)
		}
	}
	return out, nil
}
