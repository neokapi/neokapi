package service

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"

	aitools "github.com/neokapi/neokapi/core/ai/tools"
	"github.com/neokapi/neokapi/core/id"
	"github.com/neokapi/neokapi/core/registry"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
)

// ErrOutOfCredits is what a model-backed step fails with when the workspace
// has nothing left to spend. It ends the run with that reason, which the
// step log and the run's event stream carry, so a run that stopped for want
// of credits says so instead of failing somewhere in the provider.
var ErrOutOfCredits = errors.New("workspace is out of AI credits")

// AIAccountant accounts for the AI a platform run spends.
//
// FlowService holds no quota store and no billing hooks, the same way it holds
// no provider: the server passes both in, so a run started by an automation
// rule, by the MCP run_flow tool or by the gRPC flow route is metered by one
// implementation. A service with no accountant meters nothing, which is what a
// self-hosted instance and every unit test get.
type AIAccountant interface {
	// Admit reports whether the workspace may make model calls paid for by
	// source. It is asked once per run and source, before the first tool that
	// spends that way is built, and an error ends the run with that reason.
	Admit(ctx context.Context, workspaceID string, source SpendSource) error

	// Record accounts for what a run spent: a usage row per operation and
	// model, and one credit deduction for the platform-keyed part of it.
	Record(ctx context.Context, spend AISpend)
}

// SpendSource says whose key paid for a model call.
//
// Credits are deducted only for the platform's key; a workspace's own key
// burns none. Both are recorded against the ai_usage abuse cap, whose whole
// contract is that it sees every call.
type SpendSource string

const (
	// SpendPlatformKey is a call on the platform's own credential.
	SpendPlatformKey SpendSource = "platform"
	// SpendOwnKey is a call on a credential the workspace supplied, through a
	// step that names its own provider or carries its own key.
	SpendOwnKey SpendSource = "own"
)

// AISpend is what one run spent on the platform's AI key.
type AISpend struct {
	// WorkspaceID is the workspace the project belongs to, which is what the
	// usage row and the credit deduction are written against.
	WorkspaceID string
	// ProjectID scopes the usage row.
	ProjectID string
	// RunID names the platform run the spend belongs to (an automation run id),
	// empty when the surface that started the run has none of its own.
	RunID string
	// ReferenceID is unique per run, so two runs of the same flow over the same
	// project settle as two meter events rather than collapsing into one.
	ReferenceID string
	// ByOperation splits the spend by the operation that made it, the model
	// that served it and whose key paid, ordered by operation then model.
	ByOperation []OperationSpend
	// Total is the sum over ByOperation, which is what the abuse cap sees.
	Total aiprovider.TokenUsage
	// Billable is the part of Total paid for with the platform's key, which is
	// what credits are deducted for.
	Billable aiprovider.TokenUsage
}

// OperationSpend is one operation's tokens on one model, under one key.
type OperationSpend struct {
	Operation string
	Model     string
	Source    SpendSource
	Usage     aiprovider.TokenUsage
}

// operationModel keys the per-run accumulator.
type operationModel struct {
	operation string
	model     string
	source    SpendSource
}

// AIRun is the metering scope of one run over a project's content: every
// model-backed tool built inside it reports what it spends here, and Settle
// records and deducts it once.
//
// Accumulating over the run and settling at the end keeps a run to one credit
// deduction and one Stripe meter event. Deducting per model call would emit
// one event per call, which is what the meter charges for and what makes a
// run's cost unreadable.
type AIRun struct {
	accountant  AIAccountant
	workspaceID string
	projectID   string
	runID       string
	referenceID string

	mu       sync.Mutex
	admitted map[SpendSource]error
	spend    map[operationModel]aiprovider.TokenUsage
}

// BeginAIRun opens the metering scope of one run over a project's content.
//
// runID is the platform run the flow belongs to (an automation run), empty
// when the surface has none. The result is never nil, so a caller settles
// unconditionally; a service with no accountant yields a scope that admits
// everything and records nothing.
func (s *FlowService) BeginAIRun(ctx context.Context, projectID, runID string) *AIRun {
	return NewAIRun(s.accountant, s.workspaceFor(ctx, projectID), projectID, runID)
}

// NewAIRun opens a metering scope for a caller that holds its own accountant
// and already knows the workspace, which is what a worker driving one job has.
// The result is never nil, and a nil accountant yields a scope that admits
// everything and records nothing.
func NewAIRun(accountant AIAccountant, workspaceID, projectID, runID string) *AIRun {
	return &AIRun{
		accountant:  accountant,
		workspaceID: workspaceID,
		projectID:   projectID,
		runID:       runID,
		referenceID: flowSpendReference(projectID, runID),
	}
}

// workspace names the workspace the run's AI is scoped and billed to.
func (r *AIRun) workspace() string {
	if r == nil {
		return ""
	}
	return r.workspaceID
}

// Admit asks the accountant once per run and source whether the workspace may
// spend that way.
//
// The answer is memoized: a flow with twenty model-backed steps checks the
// balance once, and a workspace that runs dry mid-run finishes the run it
// started rather than failing a step halfway through a pass.
func (r *AIRun) Admit(ctx context.Context, source SpendSource) error {
	if r == nil || r.accountant == nil || r.workspaceID == "" {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if err, asked := r.admitted[source]; asked {
		return err
	}
	err := r.accountant.Admit(ctx, r.workspaceID, source)
	if r.admitted == nil {
		r.admitted = make(map[SpendSource]error, 2)
	}
	r.admitted[source] = err
	return err
}

// Meter wraps a provider so every call it serves lands in this run's
// accumulator under the operation the tool performs and the key that paid.
func (r *AIRun) Meter(inner aiprovider.LLMProvider, operation string, source SpendSource) aiprovider.LLMProvider {
	if r == nil || r.accountant == nil || inner == nil {
		return inner
	}
	return meteredProvider(inner, r, operation, source)
}

// Observer is what a tool that builds its own provider is given, so the call it
// makes on a key the platform never held is still counted. Nil when this run
// meters nothing.
func (r *AIRun) Observer(operation string, source SpendSource) aitools.ProviderObserver {
	if r == nil || r.accountant == nil {
		return nil
	}
	return func(inner aiprovider.LLMProvider) aiprovider.LLMProvider {
		return r.Meter(inner, operation, source)
	}
}

// add folds one call's usage into the run's accumulator.
func (r *AIRun) add(operation, model string, source SpendSource, usage aiprovider.TokenUsage) {
	if r == nil || usage.TotalTokens() <= 0 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.spend == nil {
		r.spend = make(map[operationModel]aiprovider.TokenUsage, 4)
	}
	k := operationModel{operation: operation, model: model, source: source}
	r.spend[k] = r.spend[k].Add(usage)
}

// Settle records what the run spent and deducts it once.
//
// Safe on a run that spent nothing and on a run that failed: the tokens a
// failed run burned before it stopped are spent either way, so a step that
// errored halfway is still accounted for. Pass a context that outlives the
// run's own cancellation.
func (r *AIRun) Settle(ctx context.Context) {
	if r == nil || r.accountant == nil {
		return
	}
	spend, ok := r.settlement()
	if !ok {
		return
	}
	r.accountant.Record(ctx, spend)
}

// settlement folds the accumulator into an AISpend and clears it, so a scope
// settled twice reports its spend once. It reports false when nothing was
// spent.
func (r *AIRun) settlement() (AISpend, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.spend) == 0 {
		return AISpend{}, false
	}
	spend := AISpend{
		WorkspaceID: r.workspaceID,
		ProjectID:   r.projectID,
		RunID:       r.runID,
		ReferenceID: r.referenceID,
		ByOperation: make([]OperationSpend, 0, len(r.spend)),
	}
	for k, usage := range r.spend {
		spend.ByOperation = append(spend.ByOperation, OperationSpend{
			Operation: k.operation,
			Model:     k.model,
			Source:    k.source,
			Usage:     usage,
		})
		spend.Total = spend.Total.Add(usage)
		if k.source == SpendPlatformKey {
			spend.Billable = spend.Billable.Add(usage)
		}
	}
	// A map iterates in random order and these rows are asserted on and read
	// by a human, so they are ordered before they leave.
	slices.SortFunc(spend.ByOperation, func(a, b OperationSpend) int {
		if c := strings.Compare(a.Operation, b.Operation); c != 0 {
			return c
		}
		if c := strings.Compare(a.Model, b.Model); c != 0 {
			return c
		}
		return strings.Compare(string(a.Source), string(b.Source))
	})
	clear(r.spend)
	return spend, true
}

// flowSpendReference is the billing reference a run settles under: unique per
// run so two runs never collapse into one meter event, and carrying the run it
// belongs to so a deduction can be traced back to the work.
func flowSpendReference(projectID, runID string) string {
	parts := make([]string, 0, 3)
	if projectID != "" {
		parts = append(parts, projectID)
	}
	if runID != "" {
		parts = append(parts, runID)
	}
	return strings.Join(append(parts, id.New()), ":")
}

// aiOperation names the usage operation a tool's model calls are recorded
// under.
//
// These values are persisted and read back by the usage dashboard, so they are
// a rename boundary: "translate" and "qa_check" are the names the editor and
// the worker have always written, and every other tool is recorded under its
// tool id with hyphens as underscores.
func aiOperation(name registry.ToolID) string {
	if name == "qa" {
		return "qa_check"
	}
	return strings.ReplaceAll(string(name), "-", "_")
}
