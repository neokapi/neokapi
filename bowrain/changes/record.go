package changes

import (
	"context"
	"slices"
	"strconv"
	"strings"
	"time"

	platev "github.com/neokapi/neokapi/bowrain/core/event"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/id"
)

// WithChange returns ctx carrying the attribution of one change set: a
// correlation id, which every block_history and change_log row the change set
// writes carries and the record is named by, and reason, the edit reason those
// rows give. An id ctx already carries is kept, so a change set inside a
// larger operation (a push, a revert) is filed under it.
func WithChange(ctx context.Context, reason string) (context.Context, string) {
	cc := bstore.ChangeContextFromContext(ctx)
	if cc.CorrelationID == "" {
		cc.CorrelationID = id.New()
	}
	if reason != "" {
		cc.Reason = reason
	}
	return bstore.WithChangeContext(ctx, cc), cc.CorrelationID
}

// Recorder records an applied change set by announcing it on the event bus.
// Its rows are written already: the stream home writes block_history and
// change_log on the transaction that stores the blocks. The record is named
// by the correlation id those rows carry (WithChange).
type Recorder struct {
	// Publish delivers the event. Nil announces nothing, and the record is
	// still named.
	Publish func(ctx context.Context, ev platev.Event)
	// WorkspaceID, ProjectID and Stream name where the change landed.
	WorkspaceID string
	ProjectID   string
	Stream      string
}

var _ change.Recorder = Recorder{}

// Record announces rec as a content.changed event and returns the correlation
// id of its rows.
func (r Recorder) Record(ctx context.Context, rec change.Record) (string, error) {
	cc := bstore.ChangeContextFromContext(ctx)
	recordID := cc.CorrelationID
	if recordID == "" {
		recordID = id.New()
	}
	if r.Publish == nil {
		return recordID, nil
	}
	var docs []string
	for _, d := range rec.Docs {
		if d.Written && !slices.Contains(docs, d.Doc) {
			docs = append(docs, d.Doc)
		}
	}
	data := map[string]string{
		"origin":         rec.Origin,
		"actor_kind":     string(rec.Actor.Kind),
		"actor_name":     rec.Actor.Name,
		"stream":         r.Stream,
		"items":          strings.Join(docs, ","),
		"transitions":    strconv.Itoa(len(rec.Transitions)),
		"overridden":     strconv.Itoa(len(rec.Overridden)),
		"fingerprint":    rec.Fingerprint,
		"correlation_id": recordID,
	}
	if rec.Actor.Session != "" {
		data["session"] = rec.Actor.Session
	}
	if rec.Set != nil && rec.Set.Note != "" {
		data["note"] = rec.Set.Note
	}
	actor := cc.Actor
	if actor == "" && rec.Actor.Kind == change.ActorPerson {
		actor = rec.Actor.Name
	}
	r.Publish(ctx, platev.Event{
		ID:           id.New(),
		Type:         platev.EventContentChanged,
		Source:       "server",
		WorkspaceID:  r.WorkspaceID,
		ProjectID:    r.ProjectID,
		Actor:        actor,
		Data:         data,
		CausationID:  recordID,
		ResourceType: "stream",
		ResourceID:   r.Stream,
		Timestamp:    time.Now().UTC(),
	})
	return recordID, nil
}
