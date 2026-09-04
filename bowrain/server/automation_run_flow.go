package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	platev "github.com/neokapi/neokapi/bowrain/core/event"
	"github.com/neokapi/neokapi/bowrain/event"
	"github.com/neokapi/neokapi/bowrain/service"
)

// knownAutomationActions is the set of action types doExecuteAction runs. A
// rule naming any other type is rejected when it is saved, and one that
// reaches the executor anyway fails its step instead of doing nothing.
var knownAutomationActions = map[string]bool{
	"auto_translate":            true,
	"auto_extract":              true,
	"auto_translate_new_locale": true,
	"notify":                    true,
	"create_review_tasks":       true,
	"create_source_review":      true,
	"write_overlay":             true,
	"run_flow":                  true,
}

// actionReportsOwnOutcome names the actions whose work outlives
// doExecuteAction and which close their own step and history entry when it
// ends, rather than at dispatch.
func actionReportsOwnOutcome(actionType string) bool {
	return actionType == "run_flow"
}

// flowCatalog is the catalog every surface resolves a flow id through: the
// built-in flows plus the project's stored definitions.
func (s *Server) flowCatalog() *service.FlowCatalog {
	return service.NewFlowCatalog(s.FlowDefStore)
}

// runFlowRequest builds the FlowRun a run_flow action asks for.
//
// The flow is resolved under the event's project, so a rule can only run a
// flow that project can see: a built-in one, or one authored on the project
// itself. The stream and the items come from the action when it names them
// and from the event otherwise, so a rule on a push runs over the pushed
// items. The target locales come from the action's target_locales, falling
// back to the project's target languages.
func (s *Server) runFlowRequest(ctx context.Context, action event.AutomationAction, ev platev.Event) (service.FlowRun, error) {
	flowID := strings.TrimSpace(action.Config["flow"])
	if flowID == "" {
		return service.FlowRun{}, errors.New("run_flow: the action names no flow")
	}
	if ev.ProjectID == "" {
		return service.FlowRun{}, errors.New("run_flow: the event carries no project")
	}
	def, err := s.flowCatalog().Get(ctx, ev.ProjectID, flowID)
	if err != nil {
		return service.FlowRun{}, fmt.Errorf("run_flow: %w", err)
	}

	stream := strings.TrimSpace(action.Config["stream"])
	if stream == "" {
		stream = ev.Data["stream"]
	}
	items := splitConfigList(action.Config["items"])
	if len(items) == 0 {
		items = splitConfigList(ev.Data["items"])
	}
	locales := splitConfigList(action.Config["target_locales"])
	if len(locales) == 0 && s.ContentStore != nil {
		proj, err := s.ContentStore.GetProject(ctx, ev.ProjectID)
		if err != nil {
			return service.FlowRun{}, fmt.Errorf("run_flow: load project: %w", err)
		}
		for _, l := range proj.TargetLanguages {
			locales = append(locales, string(l))
		}
	}

	return service.FlowRun{
		Definition:    def,
		ProjectID:     ev.ProjectID,
		Stream:        stream,
		Items:         items,
		TargetLocales: locales,
		Source:        "automation",
	}, nil
}

// splitConfigList splits a comma-separated config value, dropping blanks.
func splitConfigList(v string) []string {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// executeRunFlow runs the flow a run_flow action names over the event's
// project, then closes the automation step and writes the history entry with
// the outcome.
//
// Every failure is loud. The step is marked failed with the error, the error
// is logged against the step, the history entry records it, and the server
// log carries it, so a rule that names an unknown flow, a flow another
// project authored, or a flow whose tool fails shows up in the run history
// with the reason rather than doing nothing.
func (s *Server) executeRunFlow(ctx context.Context, action event.AutomationAction, ev platev.Event, stepID string) {
	startedAt := time.Now().UTC()
	err := s.runFlowAction(ctx, action, ev, stepID)
	// The closing writes happen after the flow, which may have used up the
	// action's own timeout; a step that ran out of time must still close.
	closing := context.WithoutCancel(ctx)
	if err != nil {
		slog.ErrorContext(closing, "automation: run_flow failed",
			"rule", action.Name, "project", ev.ProjectID, "flow", action.Config["flow"], "error", err)
		s.appendAutomationLog(closing, stepID, "error", err.Error(), map[string]string{
			"flow": action.Config["flow"], "rule": action.Name,
		})
	}
	s.completeAutomationStep(closing, stepID, err)
	s.recordAutomationHistory(closing, ev, startedAt, time.Now().UTC(), err)
}

// runFlowAction resolves and runs the flow. A panic inside the flow is turned
// into an error here so the step still closes with a reason.
func (s *Server) runFlowAction(ctx context.Context, action event.AutomationAction, ev platev.Event, stepID string) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("run_flow: panic: %v", r)
		}
	}()
	if s.Services == nil || s.Services.Flow == nil {
		return errors.New("run_flow: flow service not configured")
	}
	run, err := s.runFlowRequest(ctx, action, ev)
	if err != nil {
		return err
	}
	logData := map[string]string{
		"flow":    run.Definition.ID,
		"rule":    action.Name,
		"stream":  run.Stream,
		"items":   strings.Join(run.Items, ","),
		"locales": strings.Join(run.TargetLocales, ","),
	}
	s.appendAutomationLog(ctx, stepID, "info",
		fmt.Sprintf("run_flow: running %q on %s", run.Definition.ID, describeFlowScope(run)), logData)

	res, err := s.Services.Flow.RunFlow(ctx, run)
	if err != nil {
		return fmt.Errorf("run_flow: flow %q: %w", run.Definition.ID, err)
	}
	s.appendAutomationLog(ctx, stepID, "info",
		fmt.Sprintf("run_flow: %q wrote %d blocks across %d items in %d passes",
			run.Definition.ID, res.Blocks, res.Items, res.Passes), logData)
	return nil
}

// describeFlowScope phrases a run's scope for the step log.
func describeFlowScope(run service.FlowRun) string {
	var b strings.Builder
	switch len(run.Items) {
	case 0:
		b.WriteString("every item")
	case 1:
		b.WriteString("item " + run.Items[0])
	default:
		fmt.Fprintf(&b, "%d items", len(run.Items))
	}
	if run.Stream != "" {
		b.WriteString(" of stream " + run.Stream)
	}
	switch len(run.TargetLocales) {
	case 0:
		b.WriteString(" with no target locale")
	default:
		b.WriteString(" for " + strings.Join(run.TargetLocales, ", "))
	}
	return b.String()
}

// completeAutomationStep closes a step whose action reported its own outcome.
func (s *Server) completeAutomationStep(ctx context.Context, stepID string, err error) {
	if s.runManager == nil || stepID == "" {
		return
	}
	s.runManager.CompleteStep(ctx, stepID, err)
}
