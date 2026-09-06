package jobs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/neokapi/neokapi/bowrain/observe"
	"github.com/neokapi/neokapi/bowrain/service"
)

// billingOpExtraction is the credit-ledger operation an extraction job's AI
// settles under. Ledger operation values are persisted and read back by the
// billing view, so this one names the work rather than reusing the translation
// label a job that extracted entities would make a lie of. It sits beside
// ai_translation and ai_flow_run.
const billingOpExtraction = "ai_extraction"

// usageOpEntityExtract is the ai_usage operation an extraction's model calls
// are recorded under: the entity-extract tool id with its hyphen as an
// underscore, which is the shape every other operation in that table takes.
const usageOpEntityExtract = "entity_extract"

// errExtractionQuotaExceeded ends a job whose workspace has passed the monthly
// token ceiling. That ceiling is the internal abuse cap (QuotaStore), separate
// from the credit balance a customer sees and spends.
var errExtractionQuotaExceeded = errors.New("workspace monthly AI quota exceeded")

// extractionAccountant meters what one extraction job spends on the platform's
// AI key, through the two ledgers every other AI path writes to: the ai_usage
// abuse cap, which must see every call, and the credit ledger, which is what a
// customer spends.
//
// It is bound to the job it accounts for, because the abuse cap is keyed on the
// workspace slug the job carries while the credit ledger is keyed on the
// workspace id.
type extractionAccountant struct {
	deps *ExtractionWorkerDeps
	job  *ExtractionJob
	// source decides credit deduction: only the platform-held key is metered
	// in credits, while the abuse cap records both (Epic 004).
	source ProviderSource
}

// accountantFor returns the accounting seam for one extraction job, or nil on a
// deployment that meters nothing.
func accountantFor(deps *ExtractionWorkerDeps, job *ExtractionJob, source ProviderSource) service.AIAccountant {
	if deps.QuotaStore == nil && deps.BillingHooks == nil {
		return nil
	}
	return extractionAccountant{deps: deps, job: job, source: source}
}

// Admit refuses a job whose workspace has nothing left to spend, before the
// first model call.
//
// Deduction is post-hoc, so without this a zero-credit workspace could extract
// over a whole item and drive the ledger deeply negative. It degrades to
// allowing wherever the answer is unknown: a self-hosted instance with no
// billing store, a workspace with no allocation yet, an unreadable balance.
func (a extractionAccountant) Admit(ctx context.Context, workspaceID string) error {
	if a.source == ProviderSourcePlatform && a.deps.BillingHooks != nil &&
		a.deps.BillingHooks.Store != nil && workspaceID != "" {
		remaining, err := a.deps.BillingHooks.Store.CheckCredits(ctx, workspaceID)
		if err == nil && remaining <= 0 {
			return service.ErrOutOfCredits
		}
	}
	if a.deps.QuotaStore == nil || a.job.WorkspaceSlug == "" {
		return nil
	}
	remaining, err := a.deps.QuotaStore.CheckQuota(ctx, a.job.WorkspaceSlug)
	if err != nil {
		slog.WarnContext(ctx, "extraction: quota check failed",
			"workspace", a.job.WorkspaceSlug, "error", err)
		return nil
	}
	if remaining <= 0 {
		return fmt.Errorf("workspace %s: %w", a.job.WorkspaceSlug, errExtractionQuotaExceeded)
	}
	return nil
}

// Record writes one usage row per operation and model, then deducts the job's
// total once.
//
// Recording is fail-open by policy, the way the translation worker's is: the
// tokens are already spent by the time this runs, so a meter that is down must
// not also cost the customer the extraction. The discarded record is still
// logged and counted, because it is the only trace that spend leaves.
func (a extractionAccountant) Record(ctx context.Context, spend service.AISpend) {
	if spend.Total.TotalTokens() <= 0 {
		return
	}
	if a.deps.QuotaStore != nil {
		for _, op := range spend.ByOperation {
			if err := a.deps.QuotaStore.RecordUsage(ctx, AIUsageRecord{
				WorkspaceSlug: a.job.WorkspaceSlug,
				WorkspaceID:   spend.WorkspaceID,
				ProjectID:     spend.ProjectID,
				JobID:         a.job.ID,
				Model:         op.Model,
				Operation:     op.Operation,
				PromptTokens:  op.Usage.InputTokens,
				OutputTokens:  op.Usage.OutputTokens,
				TotalTokens:   op.Usage.TotalTokens(),
			}); err != nil {
				observe.MeteringDiscarded(ctx, observe.MeterAITokens, float64(op.Usage.TotalTokens()), err,
					"operation", op.Operation, "job_id", a.job.ID,
					"workspace", a.job.WorkspaceSlug, "model", op.Model)
			}
		}
	}
	// A workspace bring-your-own key records usage above and burns no credits
	// (Epic 004), the same gate the translation worker's deduction sits behind.
	if spend.WorkspaceID != "" && a.source == ProviderSourcePlatform {
		a.deps.BillingHooks.DeductTokens(ctx, spend.WorkspaceID, spend.Total.TotalTokens(),
			billingOpExtraction, spend.ReferenceID)
	}
}
