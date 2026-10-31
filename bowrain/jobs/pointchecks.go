package jobs

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/core/voicescope"
	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/neokapi/neokapi/core/tools"
	"github.com/neokapi/neokapi/terms"
)

// PointChecks is the set of checks in force at one point: the standard
// per-locale checks every block gets, plus the ones the governance there adds.
//
// The framework selects the same way: a check run resolves the voice profile
// and the vocabulary at each file's own point, so a project that governs two
// products by two profiles judges each by the one that governs it (host
// check.go). The platform's point is the item's collection, and the ladder is
// voicescope's.
type PointChecks struct {
	// TargetLocale is the locale being checked; SourceLocale is what the
	// vocabulary is looked up in. A check at the source locale checks the
	// source alone.
	TargetLocale model.LocaleID
	SourceLocale model.LocaleID
	// Voice governs the wording here. Nil leaves the vocabulary check out of
	// the set, which is what "nothing is bound at this point" means.
	Voice *coreprofile.VoiceProfile
	// Terms is the vocabulary the voice check reads alongside the profile's
	// own rules. Nil reports what the profile forbids and stays silent about
	// every term the workspace itself retired.
	Terms terms.Terminology
	// DNT are the terms that must survive verbatim. Empty leaves that check
	// out of the set.
	DNT []string
}

// OfSource reports a set of checks at the source locale: there is no
// translation to compare, so only the source-side checks run.
func (pc PointChecks) OfSource() bool {
	return pc.SourceLocale != "" && pc.TargetLocale == pc.SourceLocale
}

// PointCheckBinding is where the checks at a point are read from.
type PointCheckBinding struct {
	// Store reads the project, the stream, the item and its collection.
	Store store.ContentStore
	// Voice resolves a bound voice profile id. Nil checks without voice.
	Voice coreprofile.Store
	// WorkspaceDefault is the base rung of the voice ladder. Nil skips it.
	WorkspaceDefault voicescope.WorkspaceDefault
	// Terms reads the workspace terms. Nil checks without them.
	Terms func(ctx context.Context) (terms.Terminology, error)
}

// ResolvePointChecks resolves the checks in force where an item sits: the
// standard per-locale set, plus the voice profile bound at the item's own
// point (its collection, then the stream, the project, and the workspace
// default), the workspace vocabulary, and the project's protected terms.
//
// Resolution failures are operational errors. They cannot silently narrow
// the configured checks and leave a surface displaying a clean assessment.
func ResolvePointChecks(ctx context.Context, b PointCheckBinding, projectID, stream, itemName, workspaceID string, locale model.LocaleID) (PointChecks, error) {
	checks := PointChecks{TargetLocale: locale}
	if err := ctx.Err(); err != nil {
		return checks, err
	}
	if b.Store == nil {
		return checks, errors.New("check context: content store unavailable")
	}
	proj, err := b.Store.GetProject(ctx, projectID)
	if err != nil {
		return checks, fmt.Errorf("check project: %w", err)
	}
	if proj == nil {
		return checks, errors.New("check project is unavailable")
	}
	checks.SourceLocale = proj.DefaultSourceLanguage
	checks.DNT = ProjectDNTTerms(proj)
	if b.Terms != nil {
		if checks.Terms, err = b.Terms(ctx); err != nil {
			return checks, fmt.Errorf("check terms: %w", err)
		}
	}
	rc := coreprofile.ResolveContext{Locale: locale, ProjectProperties: proj.Properties}
	if stream != "" {
		// A project's main stream need not have a row of its own; a stream
		// with no row carries no properties.
		st, err := b.Store.GetStream(ctx, projectID, stream)
		switch {
		case errors.Is(err, sql.ErrNoRows) && stream == "main":
		case err != nil:
			return checks, fmt.Errorf("check stream: %w", err)
		case st == nil:
			return checks, errors.New("check stream is unavailable")
		default:
			rc.StreamProperties = st.Properties
		}
	}
	if itemName != "" {
		item, err := b.Store.GetItem(ctx, projectID, stream, itemName)
		if err != nil {
			return checks, fmt.Errorf("check item: %w", err)
		}
		if item == nil {
			return checks, errors.New("check item is unavailable")
		}
		if item.CollectionID != "" {
			col, err := b.Store.GetCollection(ctx, projectID, item.CollectionID)
			if err != nil {
				return checks, fmt.Errorf("check collection: %w", err)
			}
			if col == nil {
				return checks, errors.New("check collection is unavailable")
			}
			rc.CollectionConfig = col.ConnectorConfig
		}
	}
	if workspaceID == "" {
		workspaceID = proj.WorkspaceID
	}
	if b.WorkspaceDefault != nil && workspaceID != "" {
		rc.RootProfileID, err = b.WorkspaceDefault.WorkspaceVoiceProfileID(ctx, workspaceID)
		if err != nil {
			return checks, fmt.Errorf("check workspace voice: %w", err)
		}
	}
	checks.Voice, err = coreprofile.ResolveProfileFromContext(ctx, rc, b.Voice)
	if err != nil {
		return checks, fmt.Errorf("check voice: %w", err)
	}
	return checks, nil
}

// StandardFindings runs the standard per-locale checks on a scratch copy of
// block: the rule check with placeholder integrity, and the protected terms
// when the project declares any. They judge the target for checks'
// TargetLocale against the block's source. At the source locale there is no
// target to judge, and they find nothing.
func StandardFindings(ctx context.Context, block *model.Block, checks PointChecks) ([]check.Finding, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if block == nil {
		return nil, errors.New("check block is unavailable")
	}
	if checks.OfSource() {
		return nil, nil
	}
	scratch := *block
	scratch.Annotations = nil // fresh findings surface; SetAnno re-creates lazily
	part := &model.Part{Type: model.PartBlock, Resource: &scratch}

	// Placeholder integrity is asked for explicitly, because the config
	// leaves it off. A catalogue leaf carries {name} in its text rather than
	// as an inline run, so the run-based checks find nothing to compare. The
	// CLI gate asks the same question of the same content (host/verify.go,
	// host/loopchecks.go), and a target converged on this venue is judged
	// the same way.
	ruleCfg := tools.NewRuleCheckConfig(checks.TargetLocale)
	ruleCfg.CheckPlaceholders = true
	if _, err := tools.NewRuleCheckTool(ruleCfg).ApplyContext(ctx, part); err != nil {
		return nil, fmt.Errorf("rule check: %w", err)
	}

	// Protected terms, when the project declares any.
	if len(checks.DNT) > 0 {
		dntCfg := tools.NewDNTCheckConfig(checks.TargetLocale)
		dntCfg.Terms = checks.DNT
		if _, err := tools.NewDNTCheckTool(dntCfg).ApplyContext(ctx, part); err != nil {
			return nil, fmt.Errorf("protected terms check: %w", err)
		}
	}

	return check.Findings(tool.NewBlockViewWithContext(ctx, &scratch)), nil
}

// VoiceFindings runs the vocabulary governing the point on a scratch copy of
// block: the profile's own rules and the workspace's retired, forbidden and
// competitor terms, located in the source. It reports on its own annotation
// rather than the unified surface, so it is read separately. With no voice
// bound at the point it finds nothing.
func VoiceFindings(ctx context.Context, block *model.Block, checks PointChecks) ([]check.Finding, error) {
	if checks.Voice == nil {
		return nil, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if block == nil {
		return nil, errors.New("check block is unavailable")
	}
	scratch := *block
	scratch.Annotations = nil
	part := &model.Part{Type: model.PartBlock, Resource: &scratch}
	vocab := tools.NewVoiceVocabCheckTool(checks.Voice, checks.Terms).InSourceLocale(checks.SourceLocale)
	if _, err := vocab.ApplyContext(ctx, part); err != nil {
		return nil, fmt.Errorf("voice rules check: %w", err)
	}
	if ann, ok := model.AnnoAs[*coreprofile.VoiceAnnotation](&scratch, "voice"); ok {
		return ann.Findings, nil
	}
	return nil, nil
}

// VoiceFamily names the family of a voice finding as `kapi check` reports
// it: a word rule's finding is a terms finding, and every other the voice's.
func VoiceFamily(f check.Finding) string {
	if f.Metadata != nil && f.Metadata["term"] != "" {
		return "terms"
	}
	return "voice"
}
