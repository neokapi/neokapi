package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/neokapi/neokapi/bowrain/jobs"
	"github.com/neokapi/neokapi/bowrain/observe"
	"github.com/neokapi/neokapi/bowrain/service"
)

// billingOpFlowRun is the credit-ledger operation a flow run's AI settles
// under. The ledger's operation values are persisted and read back by the
// billing view, so this one names the work rather than reusing the
// translation label a run whose steps reviewed and checked would make a lie
// of.
const billingOpFlowRun = "ai_flow_run"

// errAIQuotaExceeded ends a run whose workspace has passed the monthly token
// ceiling. That ceiling is the internal abuse cap (jobs.QuotaStore), separate
// from the credit balance a customer sees and spends.
var errAIQuotaExceeded = errors.New("workspace monthly AI quota exceeded")

// flowAIAccountant meters what a platform run spends on the hosted AI key,
// through the two ledgers every other AI path on the server writes to: the
// ai_usage abuse cap, which must see every call, and the credit ledger, which
// is what a customer spends.
//
// A run's steps are granted the platform provider unless a step names a
// provider or a key of its own. Both kinds are recorded against the cap; only
// the platform-keyed part deducts credits.
type flowAIAccountant struct{ srv *Server }

// Admit refuses a run whose workspace has nothing left to spend, before the
// first model-backed step is built.
//
// Deduction is post-hoc, so without this a zero-credit workspace could start a
// run over a whole project and drive the ledger deeply negative. It degrades
// to allowing wherever the answer is unknown: a self-hosted instance with no
// billing store, a workspace with no allocation yet, an unreadable balance.
//
// A step on the workspace's own key burns no credits, so only the abuse cap
// answers for it. The cap applies to both, which is the whole point of a
// ceiling that bounds runaway usage.
func (a flowAIAccountant) Admit(ctx context.Context, workspaceID string, source service.SpendSource) error {
	if source == service.SpendPlatformKey && a.srv.insufficientPlatformCredits(ctx, workspaceID, "platform") {
		return service.ErrOutOfCredits
	}
	if a.srv.QuotaStore == nil {
		return nil
	}
	slug := a.srv.workspaceSlug(ctx, "", workspaceID)
	if slug == "" {
		return nil
	}
	remaining, err := a.srv.QuotaStore.CheckQuota(ctx, slug)
	if err != nil {
		slog.WarnContext(ctx, "flow run: quota check failed", "workspace", slug, "error", err)
		return nil
	}
	if remaining <= 0 {
		return fmt.Errorf("workspace %s: %w", slug, errAIQuotaExceeded)
	}
	return nil
}

// Record writes one usage row per operation and model, then deducts the
// platform-keyed part of the run once.
//
// Recording is fail-open by policy, the way the worker's is: the tokens are
// already spent by the time this runs, so a meter that is down must not also
// cost the customer the run. The discarded record is still logged and counted,
// because it is the only trace that spend leaves.
func (a flowAIAccountant) Record(ctx context.Context, spend service.AISpend) {
	if spend.WorkspaceID == "" || spend.Total.TotalTokens() <= 0 {
		return
	}
	slug := a.srv.workspaceSlug(ctx, "", spend.WorkspaceID)
	if a.srv.QuotaStore != nil {
		for _, op := range spend.ByOperation {
			if err := a.srv.QuotaStore.RecordUsage(ctx, jobs.AIUsageRecord{
				WorkspaceSlug: slug,
				WorkspaceID:   spend.WorkspaceID,
				ProjectID:     spend.ProjectID,
				JobID:         spend.RunID,
				Model:         op.Model,
				Operation:     op.Operation,
				PromptTokens:  op.Usage.InputTokens,
				OutputTokens:  op.Usage.OutputTokens,
				TotalTokens:   op.Usage.TotalTokens(),
			}); err != nil {
				observe.MeteringDiscarded(ctx, observe.MeterAITokens, float64(op.Usage.TotalTokens()), err,
					"operation", op.Operation, "run_id", spend.RunID,
					"workspace", slug, "model", op.Model)
			}
		}
	}
	// A step on the workspace's own key recorded its usage above and burns no
	// credits (Epic 004), so only the platform-keyed part is deducted.
	if spend.Billable.TotalTokens() > 0 {
		a.srv.BillingHooks.DeductTokens(ctx, spend.WorkspaceID, spend.Billable.TotalTokens(),
			billingOpFlowRun, spend.ReferenceID)
	}
}
