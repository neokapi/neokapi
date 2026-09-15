package server

import (
	"context"
	"log/slog"
	"strconv"
	"time"

	platev "github.com/neokapi/neokapi/bowrain/core/event"
	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/id"
)

// shipGatesOnPushGroup is the durable consumer group that announces a project's
// quality gates once a push has landed.
const shipGatesOnPushGroup = "ship-gates-on-push"

// shipInputsChanged is called by every write that changes what a project's ship
// states are derived from: block content and targets, review decisions, and a
// landed push. It drops the project's cached dashboard and announces the gates
// whose result the write changed. Reads derive ship states and never announce,
// so a gate event follows the change that moved the gate, whoever reads next.
func (s *Server) shipInputsChanged(ctx context.Context, workspaceID, projectID, stream string) {
	s.invalidateDashboardCache(workspaceID, projectID)
	s.announceShipGatesForProject(context.WithoutCancel(ctx), projectID, stream)
}

// subscribeShipGatesOnPush announces the gates a landed push changed. The worker
// applies a push and publishes EventPushCompleted once its content is stored, so
// the server derives the pushed stream's ship states from that event. The
// handler always acknowledges: an announcement that fails is logged, and the
// next write to the project announces again.
func (s *Server) subscribeShipGatesOnPush() {
	if s.EventBus == nil {
		return
	}
	s.EventBus.SubscribeGroup(shipGatesOnPushGroup, func(ev platev.Event) error {
		if ev.Type != platev.EventPushCompleted || ev.ProjectID == "" || s.ContentStore == nil {
			return nil
		}
		// Event-bus callback: the announcement outlives the request that
		// published the event, so no request context applies.
		ctx := context.Background()
		proj, err := s.ContentStore.GetProject(ctx, ev.ProjectID)
		if err != nil || proj == nil {
			slog.WarnContext(ctx, "ship gate: could not load the pushed project to announce its gates", "project", ev.ProjectID, "error", err)
			return nil
		}
		s.shipInputsChanged(ctx, proj.WorkspaceID, proj.ID, pushedStream(ev.Data["stream"], proj))
		return nil
	})
}

// announceShipGates publishes the quality gate events for a project's derived
// ship states. For each target language it reads the gates the derivation
// evaluated (LocaleTranslationStats.ShipGates, set by store.EvaluateShipState)
// and asks the content store's failure record whether each result is news. An
// unmet gate publishes quality.gate.fail when it opens a failure or changes an
// open one's not-checked flag. A met gate publishes quality.gate.pass when it
// clears an announced failure. The record makes a repeated derivation announce
// nothing, so this runs after every write that changes a ship state's inputs
// (shipInputsChanged) and when a convergence run ends. No read calls it.
//
// Only project-wide language scopes are announced. A store that keeps no failure
// record cannot tell a change from a repeat, and announces nothing. A result that
// cannot be recorded is logged and its event is skipped, so a write never fails
// over an announcement.
func (s *Server) announceShipGates(ctx context.Context, proj *store.Project, stream string, stats *store.TranslationDashboardStats) {
	if s.EventBus == nil || proj == nil || stats == nil {
		return
	}
	failures, ok := store.ShipGateFailures(s.ContentStore)
	if !ok {
		return
	}
	stream = runStream(stream)
	slug := s.workspaceSlug(ctx, "", proj.WorkspaceID)

	for _, ls := range stats.LocaleStats {
		for _, g := range ls.ShipGates {
			var news bool
			var err error
			if g.Met {
				news, err = failures.CloseShipGateFailure(ctx, proj.ID, stream, ls.Locale, g.Gate)
			} else {
				news, err = failures.OpenShipGateFailure(ctx, store.ShipGateFailure{
					ProjectID: proj.ID, Stream: stream, Locale: ls.Locale, Gate: g.Gate,
					NotChecked: g.NotChecked, Actual: g.Actual, Required: g.Required,
				})
			}
			if err != nil {
				slog.WarnContext(ctx, "ship gate: could not record a gate result, so its event is not published",
					"project", proj.ID, "stream", stream, "locale", ls.Locale, "gate", g.Gate, "error", err)
				continue
			}
			if !news {
				continue
			}
			ev := platev.Event{
				ID:        id.New(),
				Type:      platev.EventQualityGateFail,
				Source:    "ship_gate",
				ProjectID: proj.ID,
				Timestamp: time.Now().UTC(),
				Data: map[string]string{
					"workspace_id":   proj.WorkspaceID,
					"workspace_slug": slug,
					"stream":         stream,
					"locale":         ls.Locale,
					"gate_name":      g.Gate,
					"actual":         strconv.Itoa(g.Actual),
					"required":       strconv.Itoa(g.Required),
					"not_checked":    strconv.FormatBool(g.NotChecked),
					"ship_state":     string(ls.ShipState),
				},
			}
			if g.Met {
				ev.Type = platev.EventQualityGatePass
			}
			s.EventBus.Publish(ev)
		}
	}
}

// announceShipGatesForProject derives a project's ship states on a stream and
// announces what changed. It serves a caller that holds no dashboard stats: a
// write that changed the project, or a convergence run that has just finished.
func (s *Server) announceShipGatesForProject(ctx context.Context, projectID, stream string) {
	if s.EventBus == nil || s.ContentStore == nil {
		return
	}
	if _, ok := store.ShipGateFailures(s.ContentStore); !ok {
		return
	}
	proj, err := s.ContentStore.GetProject(ctx, projectID)
	if err != nil || proj == nil {
		slog.WarnContext(ctx, "ship gate: could not load the project to announce its gates", "project", projectID, "error", err)
		return
	}
	stream = runStream(stream)
	stats, err := editorGetDashboardStats(ctx, s.ContentStore, proj, stream)
	if err != nil {
		slog.WarnContext(ctx, "ship gate: could not derive the project's coverage", "project", projectID, "error", err)
		return
	}
	gate := s.resolveTermGate(ctx, proj, stream, proj.WorkspaceID)
	if err := applyShipStates(ctx, s.ContentStore, s.VoiceStore, proj.ID, stream, gate, stats); err != nil {
		slog.WarnContext(ctx, "ship gate: could not derive the project's ship states", "project", projectID, "error", err)
		return
	}
	s.announceShipGates(ctx, proj, stream, stats)
}
