package jobs

import (
	"context"
	"log/slog"

	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
)

// persistDraftVoiceScores runs the profile's deterministic gate
// (coreprofile.Findings — the same zero-AI vocabulary and pattern matching behind
// every HTTP scoring surface) over freshly persisted draft targets and stores one brand voice
// score per block. This is what makes the dashboard's compliance rate
// voice-informed without any extra AI spend: every server-side draft the
// convergence loop produces (content memory-recycled or AI-translated) leaves a measured
// score behind it, and later passes overwrite staleness by appending a newer
// measurement (readers keep the latest per block+locale).
//
// Strictly best-effort: scoring must never fail (or meaningfully slow) a
// translation job. A store error is logged and abandons the remainder of the
// batch — the dashboard then falls back to checks-only for the unscored
// blocks.
func persistDraftVoiceScores(ctx context.Context, deps *WorkerDeps, job *TranslationJob, profile *coreprofile.VoiceProfile, blocks []*model.Block, locale model.LocaleID) {
	if deps == nil || deps.VoiceStore == nil || profile == nil || job == nil {
		return
	}
	stored := 0
	for _, b := range blocks {
		if b == nil || !b.Translatable {
			continue
		}
		text := b.TargetText(locale)
		if text == "" {
			continue
		}
		// Anchor findings to a single synthetic text run over the checked text,
		// exactly like the HTTP check surfaces (HandleCheckBrandVoice).
		runs := []model.Run{{Text: &model.TextRun{Text: text}}}
		findings := coreprofile.Findings(profile, text, runs)
		score := coreprofile.CalculateScore(findings)
		err := deps.VoiceStore.StoreScore(ctx, &coreprofile.StoredScore{
			ProjectID:      job.ProjectID,
			Stream:         "main",
			BlockID:        b.ID,
			ProfileID:      profile.ID,
			ProfileVersion: profile.Version,
			Locale:         locale,
			Score:          score.Overall,
			Dimensions:     score.Dimensions,
			Findings:       findings,
		})
		if err != nil {
			slog.WarnContext(ctx, "voice score persist failed; compliance rate falls back to checks for the unscored drafts",
				"job_id", job.ID, "block_id", b.ID, "error", err)
			return
		}
		stored++
	}
	if stored > 0 {
		slog.DebugContext(ctx, "persisted draft voice scores",
			"job_id", job.ID, "locale", string(locale), "blocks", stored)
	}
}
