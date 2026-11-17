package server

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/venue"
)

// Bounds on the walk that finds a pattern rule's examples. The walk stops at
// whichever comes first, so a rule nothing matches costs a bounded read
// rather than the whole project.
const (
	ruleExampleLimit     = 3
	ruleExampleMaxBlocks = 20000
	ruleExampleBudget    = 4 * time.Second
)

// errExamplesDone ends the block walk once it has what it came for.
var errExamplesDone = errors.New("examples found")

// ruleExample is one place a pattern rule matches the project's source text.
type ruleExample struct {
	coreprofile.PatternExcerpt
	// Item is the file the text sits in.
	Item string `json:"item,omitempty"`
}

// ruleExamplesResponse is where a pattern rule matches the project's source
// text.
type ruleExamplesResponse struct {
	Examples []ruleExample `json:"examples"`
	// Scanned is how many blocks the walk read, and Complete says it read
	// them all. An incomplete walk with no examples has not shown the rule
	// matches nothing.
	Scanned  int  `json:"scanned"`
	Complete bool `json:"complete"`
}

// HandleContextRuleExamples answers GET /:ws/:id/context/deck/examples
// ?profile=&id=: up to three places the voice rule id of the bound profile
// matches the project's source text, so a reviewer sees what the rule
// catches. The walk is bounded in blocks and time.
func (s *Server) HandleContextRuleExamples(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermViewContent); err != nil {
		return err
	}
	if s.ContentStore == nil {
		return serverErr(c, errors.New("content store not configured"))
	}
	ctx := c.Request().Context()
	project, err := s.ContentStore.GetProject(ctx, projectParam(c))
	if err != nil || project == nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: errNoProject.Error()})
	}
	wsID, _ := c.Get("workspace_id").(string)
	src := deckSource{wsID: wsID, wsSlug: c.Param("ws"), project: project}
	rule, ok := s.boundPatternRule(ctx, src, c.QueryParam("profile"), c.QueryParam("id"))
	if !ok {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: "no bound voice profile of this project has that rule"})
	}
	out, err := s.ruleExamples(ctx, project.ID, rule)
	if err != nil {
		return serverErr(c, err)
	}
	return c.JSON(http.StatusOK, out)
}

// boundPatternRule is the pattern rule id of the bound voice profile named
// profileID, as patternIdentity names it.
func (s *Server) boundPatternRule(ctx context.Context, src deckSource, profileID, id string) (coreprofile.Pattern, bool) {
	if profileID == "" || id == "" {
		return coreprofile.Pattern{}, false
	}
	for _, profile := range s.boundVoiceProfiles(ctx, src) {
		if profile.ID != profileID {
			continue
		}
		for list, patterns := range map[string][]coreprofile.Pattern{
			"prohibited": profile.Style.ProhibitedPatterns,
			"required":   profile.Style.RequiredPatterns,
		} {
			for _, p := range patterns {
				if pid, _ := patternIdentity(list, p); pid == id {
					return p, true
				}
			}
		}
	}
	return coreprofile.Pattern{}, false
}

// ruleExamples walks the project's source blocks for places rule matches.
func (s *Server) ruleExamples(ctx context.Context, projectID string, rule coreprofile.Pattern) (ruleExamplesResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, ruleExampleBudget)
	defer cancel()
	out := ruleExamplesResponse{Examples: []ruleExample{}}
	scope := platstore.BlockQuery{ProjectID: projectID, Stream: reviewInboxStream, TextAny: ruleCandidates(rule.Regex)}
	err := platstore.EachBlockBatch(ctx, s.ContentStore, scope, 0, func(blocks []*venue.StoredBlock) error {
		for _, b := range blocks {
			out.Scanned++
			if b == nil || b.Block == nil {
				continue
			}
			for _, ex := range coreprofile.PatternExcerpts(rule, b.SourceRuns(), ruleExampleLimit-len(out.Examples)) {
				out.Examples = append(out.Examples, ruleExample{PatternExcerpt: ex, Item: b.ItemName})
			}
			if len(out.Examples) >= ruleExampleLimit || out.Scanned >= ruleExampleMaxBlocks {
				return errExamplesDone
			}
		}
		return nil
	})
	switch {
	case err == nil:
		out.Complete = true
	case errors.Is(err, errExamplesDone):
	case errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded):
	default:
		return out, err
	}
	return out, nil
}

// ruleCandidates is the text a block must contain for rule to match it, when
// the rule is a list of words: each word, with an open end trimmed ("util…"
// is any block containing "util"). A rule with no word list walks every
// block.
func ruleCandidates(regex string) []string {
	words := coreprofile.PatternWords(regex)
	out := make([]string, 0, len(words))
	for _, w := range words {
		w = strings.TrimSuffix(w, "…")
		if w == "" {
			return nil
		}
		out = append(out, w)
	}
	return out
}
