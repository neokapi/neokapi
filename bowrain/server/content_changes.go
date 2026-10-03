package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/neokapi/neokapi/bowrain/changes"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platev "github.com/neokapi/neokapi/bowrain/core/event"
	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/model"
)

// maxChangeSetBytes bounds the body of a change set sent to a stream.
const maxChangeSetBytes = 16 << 20

// noteAnnotation is the annotation type a person's note on a block is: a
// stand-off span on the block's own edition, written with annotate and
// removed with unannotate.
const noteAnnotation = "note"

// The span properties the server stamps on a note when it lands: who wrote it
// and when. A change set cannot set them: a note that carries an author keeps
// it, and the server stamps only a note it is the first to store.
const (
	notePropAuthorID = "author_id"
	notePropAuthor   = "author"
	notePropCreated  = "created_at"
)

// streamChange is what the change service needs for one stream on one
// request: the home over its rows, the sender's policy, the commit check, the
// decisions and the recorder.
type streamChange struct {
	s      *Server
	c      echo.Context
	proj   *store.Project
	stream string
	wsID   string
	wsSlug string
	sender changeSender

	home    *changes.Home
	policy  *streamPolicy
	check   *streamCommitCheck
	decide  *streamDecisions
	mu      sync.Mutex
	written map[string][]string
	// completed says an approval the change set landed emptied the project's
	// review queue and handed off to a completing run.
	completed bool
	// holdLoop leaves the review loop to the caller: a pass of many change
	// sets continues it once, after the last.
	holdLoop bool
}

// newStreamChange builds the change for one stream. c is the request a person
// sends the change set on, nil for an agent's or a job's.
func (s *Server) newStreamChange(ctx context.Context, c echo.Context, proj *store.Project, stream, wsID, wsSlug string, sender changeSender) *streamChange {
	if wsID == "" {
		wsID = proj.WorkspaceID
	}
	sc := &streamChange{s: s, c: c, proj: proj, stream: stream, wsID: wsID, wsSlug: wsSlug, sender: sender, written: map[string][]string{}}
	rows := newRowLookup(s, proj.ID, stream)
	sc.home = &changes.Home{
		Store:        s.ContentStore,
		ProjectID:    proj.ID,
		Stream:       stream,
		SourceLocale: proj.DefaultSourceLanguage,
		Locales:      proj.TargetLanguages,
		Registry:     s.FormatRegistry,
		Stamp:        stampNotes(sender, time.Now),
		Committed: func(doc string, ids []string) {
			sc.mu.Lock()
			defer sc.mu.Unlock()
			sc.written[doc] = append(sc.written[doc], ids...)
		},
	}
	sc.policy = &streamPolicy{ctx: ctx, s: s, proj: proj, stream: stream, wsID: wsID, sender: sender, rows: rows}
	sc.check = &streamCommitCheck{s: s, proj: proj, stream: stream, wsID: wsID, wsSlug: wsSlug}
	sc.decide = &streamDecisions{s: s, c: c, proj: proj, stream: stream, rows: rows, sender: sender}
	return sc
}

// service is the change service over the stream.
func (sc *streamChange) service() *change.Service {
	return changes.NewService(sc.home, sc.s.FormatRegistry,
		change.WithPolicy(sc.policy),
		change.WithCommitCheck(sc.check),
		change.WithAssets(sc.decide),
		change.WithRecorder(sc.recorder()),
	)
}

// recorder announces each change set that lands on the event bus, with the
// request it came on when there is one.
func (sc *streamChange) recorder() changes.Recorder {
	return changes.Recorder{WorkspaceID: sc.wsID, ProjectID: sc.proj.ID, Stream: sc.stream,
		Publish: func(_ context.Context, ev platev.Event) {
			if sc.s.EventBus == nil {
				return
			}
			if sc.c != nil {
				meta := requestMeta(sc.c)
				ev.RequestID, ev.IP, ev.UserAgent = meta.RequestID, meta.IP, meta.UserAgent
			}
			sc.s.EventBus.Publish(ev)
		}}
}

// apply applies set as actor and then does what a landed change asks of the
// rest of the server: the ship inputs move, watchers refresh the blocks it
// wrote, and an approval continues the review loop.
func (sc *streamChange) apply(ctx context.Context, set change.Set, actor change.Actor) (*change.Result, error) {
	ctx, _ = changes.WithChange(ctx, set.Note)
	res, err := sc.service().Apply(ctx, set, actor)
	if err != nil {
		return nil, err
	}
	sc.completed = sc.landed(ctx, res)
	return res, nil
}

// applyEach applies set as actor, leaving out each block a refused
// operation names and applying the rest (changes.ApplyEach), and then does
// what the change set that landed asks of the rest of the server. It returns
// the result, the operations it holds the outcomes of, and the blocks left
// out.
func (sc *streamChange) applyEach(ctx context.Context, set change.Set, actor change.Actor) (*change.Result, []change.Op, []changes.Refusal, error) {
	ctx, _ = changes.WithChange(ctx, set.Note)
	res, ops, refused, err := changes.ApplyEach(ctx, sc.service(), set, actor)
	if err != nil {
		return nil, nil, refused, err
	}
	sc.completed = sc.landed(ctx, res)
	return res, ops, refused, nil
}

// landed does what a change set that landed asks of the rest of the server.
// It reports whether an approval emptied the project's review queue and handed
// off to a completing run.
func (sc *streamChange) landed(ctx context.Context, res *change.Result) bool {
	if res == nil || (res.Status != change.SetApplied && res.Status != change.SetPartial) {
		return false
	}
	sc.mu.Lock()
	written := maps.Clone(sc.written)
	sc.mu.Unlock()
	decided := len(sc.decide.approved) > 0 || slices.ContainsFunc(res.Ops, func(r change.OpResult) bool {
		return r.Op == change.KindDecide && r.Status == change.OpApplied
	})
	if len(written) == 0 && !decided {
		return false
	}
	sc.s.shipInputsChanged(ctx, sc.wsID, sc.proj.ID, sc.stream)
	if sc.c != nil {
		for doc, ids := range written {
			for _, id := range ids {
				sc.s.emitEditorBlockChange(sc.c, sc.proj.ID, id, doc, sc.stream, "updated")
			}
		}
	}
	if len(sc.decide.approved) == 0 || sc.holdLoop {
		return false
	}
	locales := make([]model.LocaleID, 0, len(sc.decide.approved))
	for l := range sc.decide.approved {
		locales = append(locales, l)
	}
	slices.Sort(locales)
	return sc.s.advanceReviewLoop(ctx, sc.proj, sc.stream, locales, sc.sender.userID)
}

// stampNotes stamps each note a change set adds with its author and the time
// it landed, and keeps its text as the note payload the model registers
// (model.Notes), which a row stores and reads back whole.
func stampNotes(sender changeSender, now func() time.Time) func(*model.Block) {
	return func(b *model.Block) {
		for i := range b.Overlays {
			o := &b.Overlays[i]
			if o.Type != model.OverlayType(noteAnnotation) {
				continue
			}
			for j := range o.Spans {
				sp := &o.Spans[j]
				if sp.Props[notePropAuthorID] != "" || sp.Props[notePropAuthor] != "" {
					continue
				}
				props := maps.Clone(sp.Props)
				if props == nil {
					props = map[string]string{}
				}
				props[notePropAuthorID] = sender.userID
				props[notePropAuthor] = sender.name
				props[notePropCreated] = now().UTC().Format(time.RFC3339)
				sp.Props = props
				if _, ok := sp.Value.(*model.Notes); !ok {
					sp.Value = &model.Notes{Items: []*model.NoteAnnotation{{Text: noteText(sp.Value), From: sender.name}}}
				}
			}
		}
	}
}

// requestSender is the person a request is sent by, with the permissions the
// project access middleware resolved for them.
func requestSender(c echo.Context) changeSender {
	userID, _ := c.Get("user_id").(string)
	name := extractAuthor(c)
	if name == "" {
		name, _ = c.Get("name").(string)
	}
	return changeSender{userID: userID, name: name, allows: func(perm platauth.Permission, locale string) bool {
		if locale == "" || !perm.LanguageScoped() {
			return hasPermission(c, perm)
		}
		return allowsLanguage(c, perm, locale)
	}}
}

// HandleApplyChanges applies a kapi.change/v1 change set to one stream: how a
// person changes content on the server, decides on a translation, and
// annotates a block. Each operation addresses a block of an item by
// {doc: the item's path, block, edition} and names the revision it read; the
// result is kapi.change-result/v1.
//
// A refused change set writes nothing and is answered with the status its
// first refusal's code maps to: 400 invalid, 404 not_found, 409 stale,
// ambiguous or doc_changed, 422 guard, gate_failed or unsupported, 403
// not_permitted, 413 budget_exceeded, 503 unreachable. A change set that
// applied, previewed or partly landed is answered 200 with its status.
//
// POST /:ws/projects/:id/streams/:stream/changes
func (s *Server) HandleApplyChanges(c echo.Context) error {
	if s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, change.ErrorResult(&change.Error{Code: change.CodeUnreachable, Message: "the content store is not configured"}))
	}
	ctx := c.Request().Context()
	proj, err := s.ContentStore.GetProject(ctx, projectParam(c))
	if err != nil || proj == nil {
		return c.JSON(http.StatusNotFound, change.ErrorResult(&change.Error{Code: change.CodeNotFound, Message: "project not found"}))
	}
	// A stream that holds no item named by an operation answers it not_found.
	stream := unescapeParam(c.Param("stream"))
	set, err := change.Decode(http.MaxBytesReader(c.Response(), c.Request().Body, maxChangeSetBytes))
	if err != nil {
		ce, ok := errors.AsType[*change.Error](err)
		if _, tooLarge := errors.AsType[*http.MaxBytesError](err); tooLarge {
			ce, ok = &change.Error{Code: change.CodeBudgetExceeded, Message: fmt.Sprintf("a change set is at most %d bytes; split it", maxChangeSetBytes)}, true
		}
		if !ok {
			ce = &change.Error{Code: change.CodeInvalid, Message: err.Error()}
		}
		return c.JSON(ce.Code.HTTPStatus(), change.ErrorResult(ce))
	}
	wsID, _ := c.Get("workspace_id").(string)
	sender := requestSender(c)
	sc := s.newStreamChange(ctx, c, proj, stream, wsID, c.Param("ws"), sender)
	res, err := sc.apply(ctx, set, change.Actor{Kind: change.ActorPerson, Name: sender.userID})
	if err != nil {
		return serverErr(c, err)
	}
	if res.Status == change.SetApplied || res.Status == change.SetPartial {
		s.trackContentChange(sender.userID, proj.ID, set, res)
		s.notifyNoteMentions(c, proj.ID, set, res)
	}
	return c.JSON(resultStatus(res), res)
}

// resultStatus is the HTTP status a change set's result is answered with.
func resultStatus(res *change.Result) int {
	if res.Status != change.SetRefused {
		return http.StatusOK
	}
	if res.Error != nil {
		return res.Error.Code.HTTPStatus()
	}
	for _, op := range res.Ops {
		if op.Status == change.OpRefused && op.Error != nil {
			return op.Error.Code.HTTPStatus()
		}
	}
	return http.StatusConflict
}

// trackContentChange counts each translation a change set saved, as the
// product analytics count a save.
func (s *Server) trackContentChange(userID, projectID string, set change.Set, res *change.Result) {
	for i, op := range res.Ops {
		if op.Status != change.OpApplied || op.At == nil || op.At.Edition.IsZero() {
			continue
		}
		switch set.Ops[i].Kind {
		case change.KindSetContent, change.KindReplaceText:
			s.trackEvent(userID, "translation_saved", map[string]any{
				"project_id": projectID,
				"block_id":   op.At.Block,
				"locale":     string(op.At.Edition.Locale),
			})
		}
	}
}

// notifyNoteMentions tells each person a note the change set added mentions
// (@name) that they were mentioned.
func (s *Server) notifyNoteMentions(c echo.Context, projectID string, set change.Set, res *change.Result) {
	if s.NotificationDispatcher == nil || s.AuthStore == nil {
		return
	}
	ctx := c.Request().Context()
	actorID, _ := c.Get("user_id").(string)
	actorName, _ := c.Get("name").(string)
	for i, op := range res.Ops {
		body, ok := set.Ops[i].Body.(*change.Annotate)
		if !ok || body.Type != noteAnnotation || op.Status != change.OpApplied {
			continue
		}
		var note struct {
			Text string `json:"text"`
		}
		if json.Unmarshal(body.Value, &note) != nil || note.Text == "" {
			continue
		}
		for _, username := range parseMentions(note.Text) {
			if user, err := s.AuthStore.GetUserByEmail(ctx, username); err == nil && user != nil {
				s.NotificationDispatcher.DispatchMention(ctx, user.ID, actorID, actorName, note.Text, projectID, "")
			}
		}
	}
}
