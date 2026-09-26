package host

import (
	"context"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/contextop"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/workspace"
)

// ContextRulesAt is what a project's context operations add to the vocabulary
// its own stores already carry, at one point.
//
// Two lists, because they answer differently. The binding rules were
// established by a person and widened to the whole workspace, so they hold at
// the severity each one carries, exactly like a rule in the project's terms
// store. The advisory rules are suggestions the project has accumulated and
// not yet established, and rules a disagreement contests, reported at neutral
// severity so they show up in a check and fail nothing.
func (a *App) ContextRulesAt(ctx context.Context, recipePath string, point project.GovernancePoint) (contextop.Resolution, error) {
	cmd := NewEnvCommand(ctx, "context-rules")
	cmd.Flags().String(projectFlagName, recipePath, "")
	resolver, err := a.newContextRules(cmd, recipePath)
	if err != nil || resolver == nil {
		return contextop.Resolution{}, err
	}
	return resolver.at(point)
}

// contextRules answers "which suggestions and which widened rules hold here" for
// one run.
//
// It reads the operation log and the workspace's widened rules once and folds
// them per point, so a check over a thousand files reads the log once. A run
// outside a project, or one whose workspace cannot be opened, answers with
// nothing rather than failing: an advisory finding is not worth failing a check
// to produce.
type contextRules struct {
	proj    *project.KapiProject
	key     workspace.ProjectKey
	records []contextop.Record
	widened []contextop.WidenedRule
	// held are the terms the project's own stores already answer for, so a
	// workspace-wide rule about one of them stays out of the way.
	held []string
	// cache is one resolution per point.
	cache map[string]contextop.Resolution
}

// newContextRules reads a project's context operations for one run. It answers
// nil outside a project and when the run has recorded nothing.
func (a *App) newContextRules(cmd Command, recipePath string) (*contextRules, error) {
	if recipePath == "" {
		return nil, nil
	}
	identity, _ := recipeIdentity(recipePath)
	if identity == "" {
		return nil, nil
	}
	ctx := CmdContext(cmd)
	ws, err := a.Workspace(ctx)
	if err != nil {
		return nil, err
	}
	key := workspace.ProjectKey(identity)
	records, err := contextop.NewLedger(ws, contextop.PersonDecides).Records(ctx, contextop.Filter{})
	if err != nil {
		return nil, err
	}
	widened, err := contextop.WidenedRules(ctx, ws)
	if err != nil {
		return nil, err
	}
	if len(records) == 0 && len(widened) == 0 {
		return nil, nil
	}
	proj, err := project.LoadWithOptions(recipePath, project.LoadOptions{SkipRequiresCheck: true})
	if err != nil {
		return nil, err
	}
	r := &contextRules{
		proj:    proj,
		key:     key,
		records: records,
		widened: widened,
		cache:   map[string]contextop.Resolution{},
	}
	r.held, err = a.projectDeclaredTerms(cmd)
	if err != nil {
		return nil, err
	}
	return r, nil
}

// at resolves the rules that hold at one point.
func (r *contextRules) at(point project.GovernancePoint) (contextop.Resolution, error) {
	if r == nil {
		return contextop.Resolution{}, nil
	}
	rc, err := r.proj.ResolveGovernanceFor(point)
	if err != nil {
		return contextop.Resolution{}, err
	}
	collection := point.Collection
	if point.Path != "" {
		collection = r.proj.CollectionForPath(point.Path)
	}
	coordinates := project.MergeCoordinates(
		r.proj.Defaults.Coordinates, rc.Ref().Coordinates(), collectionCoordinates(r.proj, collection))

	key := rc.Profile + "\x00" + rc.Channel + "\x00" + collection
	if held, ok := r.cache[key]; ok {
		return held, nil
	}
	resolved := contextop.Resolve(r.records, r.widened, contextop.ResolveRequest{
		Project:     r.key,
		Coordinates: coordinates,
		Held:        r.held,
	})
	r.cache[key] = resolved
	return resolved, nil
}

// projectDeclaredTerms is every term the project's own terms store declares. A
// workspace-wide rule about one of them is left out of the resolution: the
// project has said what it thinks about that word, and the most specific answer
// wins.
func (a *App) projectDeclaredTerms(cmd Command) ([]string, error) {
	concepts, err := a.projectConcepts(cmd, project.GovernancePoint{})
	if err != nil {
		return nil, err
	}
	var out []string
	for _, c := range concepts {
		for _, t := range c.Terms {
			out = append(out, t.Text)
		}
	}
	return out, nil
}

// advisoryDiagnostics reports the suggested rules a text matches.
//
// Suggestions run outside the analyzer the vocabulary gate registers, and on
// purpose. An analyzer's findings are its verdict, and a check counts them,
// scores them and holds the analyzer to a canary; a suggestion is a note about
// a rule nobody has established, so it settles none of that. This is the same
// stance core/check takes with a Warning: reported beside the findings, read by
// no gate.
func advisoryDiagnostics(sets []profile.TermRuleSet, text string, runs []model.Run, loc check.Location) []check.Diagnostic {
	hits := profile.MatchTermRules(sets, text)
	if len(hits) == 0 {
		return nil
	}
	findings := profile.HitsToFindings(hits, text, runs)
	out := make([]check.Diagnostic, 0, len(findings))
	for _, f := range findings {
		out = append(out, check.DiagnosticFrom(f, "terms", loc))
	}
	return out
}
