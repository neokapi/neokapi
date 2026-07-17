package server

import (
	"context"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/neokapi/neokapi/bowrain/billing"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/jobs"
	"github.com/neokapi/neokapi/sievepen"
)

// This file is the server-computed pre-flight ESTIMATE (roadmap epic 019, theme
// B): the source-readiness-first, provider-free preview the Run-now dialog and
// `kapi up` show before starting a run. It answers "what would a run do and
// cost" without starting one — source readiness (settle first), then the
// per-locale TM/AI split and credit cost for the ready source, then the
// workspace balance and how much of the AI work it covers.

// convergenceEstimateView is the REST shape of a convergence estimate. It wraps
// the provider-free jobs.ConvergenceEstimate with the workspace balance so the
// consent surface can sequence source-readiness → credits in one round trip.
type convergenceEstimateView struct {
	// Source is the source-first readiness split (ready vs. held on the gate) —
	// shown FIRST: "M of K blocks ready; N held — settle first".
	Source jobs.SourceReadiness `json:"source"`
	// Locales is the per-locale translation work over the READY source (pending,
	// TM-recyclable, paid-AI, ~tokens).
	Locales []jobs.EstimateLocaleWork `json:"locales"`
	// Totals rolls the per-locale work up across locales.
	Totals jobs.EstimateTotals `json:"totals"`
	// Credits is the credit/$ estimate for the AI remainder plus the workspace
	// balance and coverage. Nil when billing is not configured (self-hosted).
	Credits *convergenceCreditsView `json:"credits,omitempty"`
	// Note documents the estimation basis for agents reading the JSON.
	Note string `json:"note"`
}

// convergenceCreditsView is the credit/$ side of the estimate: what the AI
// remainder costs (1 token ≈ 1 credit), the workspace's spendable balance, and
// whether that balance covers the whole AI remainder (and if not, how much).
type convergenceCreditsView struct {
	// EstimatedCredits is the credits the AI remainder would spend (≈ tokens).
	EstimatedCredits int64 `json:"estimated_credits"`
	// EstimatedUSD is the dollar cost at the pack rate ($5 = 200K credits).
	EstimatedUSD float64 `json:"estimated_usd"`
	// Balance is the workspace's spendable credit balance (plan + purchased).
	Balance int64 `json:"balance"`
	// CoversAllAI is true when the balance covers the entire AI remainder.
	CoversAllAI bool `json:"covers_all_ai"`
	// CoversAIUnits is roughly how many of the AI units the balance covers when
	// it does NOT cover them all (0 when it covers all, or when the estimate is
	// zero-cost). It scales AI units by the balance/estimate ratio.
	CoversAIUnits int `json:"covers_ai_units"`
}

// dollarsPerCreditPack is the pack price ($5) and its credit grant (200K) — the
// $/credit rate the estimate prices the AI remainder at.
const (
	creditPackUSD     = 5.0
	creditPackCredits = float64(billing.CreditPackCredits) // 200_000
)

// buildConvergenceEstimate assembles the provider-free estimate for a project:
// source readiness + per-locale work (via jobs.EstimateConvergence, reusing the
// run's recycle split) + the workspace credit balance. It is read-only and
// starts no run.
func (o *convergenceOrchestrator) buildConvergenceEstimate(ctx context.Context, proj *platstore.Project) (convergenceEstimateView, error) {
	s := o.server
	var tm sievepen.TMStore
	if s.wsStores != nil {
		if resolved, err := s.wsStores.getTM(o.workspaceSlug(ctx, proj)); err == nil {
			tm = resolved
		}
	}

	est, err := jobs.EstimateConvergence(ctx, s.ContentStore, tm, proj)
	if err != nil {
		return convergenceEstimateView{}, err
	}

	view := convergenceEstimateView{
		Source:  est.Source,
		Locales: est.Locales,
		Totals:  est.Totals,
		Note:    est.Note,
	}
	view.Credits = o.creditsEstimate(ctx, proj, est)
	return view, nil
}

// creditsEstimate prices the AI remainder and reads the workspace balance. It
// returns nil when billing is not configured (self-hosted deployments), so the
// consent surface simply omits the credit line rather than showing a $0 cost.
func (o *convergenceOrchestrator) creditsEstimate(ctx context.Context, proj *platstore.Project, est jobs.ConvergenceEstimate) *convergenceCreditsView {
	s := o.server
	if s.BillingStore == nil || proj.WorkspaceID == "" {
		return nil
	}
	credits := billing.TokensToCredits(est.Totals.TokenEstimate)
	view := &convergenceCreditsView{
		EstimatedCredits: credits,
		EstimatedUSD:     float64(credits) / creditPackCredits * creditPackUSD,
	}
	balance, err := s.BillingStore.CheckCredits(ctx, proj.WorkspaceID)
	if err != nil {
		// No allocation signal (never provisioned): treat as zero spendable so the
		// consent surface shows "0 covered" rather than hiding the credit line.
		balance = 0
	}
	view.Balance = balance
	switch {
	case credits == 0:
		view.CoversAllAI = true // nothing to pay for
	case balance >= credits:
		view.CoversAllAI = true
	default:
		// Scale AI units by the fraction of the estimate the balance covers, so
		// "your balance covers ~N of the AI units" is legible even when it can't
		// cover them all.
		if credits > 0 && est.Totals.ViaAI > 0 {
			view.CoversAIUnits = int(float64(est.Totals.ViaAI) * float64(balance) / float64(credits))
		}
	}
	return view
}

// HandleConvergenceEstimate returns the provider-free pre-flight estimate for a
// project's convergence run: source readiness first, then the per-locale TM/AI
// split and credit cost for the ready source, then the workspace balance. It
// starts no run and calls no AI provider.
// GET /…/projects/:id/convergence/estimate
func (s *Server) HandleConvergenceEstimate(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermViewContent); err != nil {
		return err
	}
	if s.convergence == nil || s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "convergence not configured"})
	}
	proj, err := s.ContentStore.GetProject(c.Request().Context(), c.Param("id"))
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: "project not found"})
	}
	view, err := s.convergence.buildConvergenceEstimate(c.Request().Context(), proj)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}
	return c.JSON(http.StatusOK, view)
}
