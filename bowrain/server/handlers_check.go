package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/labstack/echo/v4"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/jobs"
	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/neokapi/neokapi/core/tools"
	"github.com/neokapi/neokapi/terms"
)

// CheckIssueResponse is a single finding returned by the API.
//
// {type, severity ("error"|"warning"), message} is the shape the editor's
// Problems panel has always read, and it is unchanged. The check tools emit
// core/check.Finding, which also locates the finding; dropping that at the
// boundary left the caller with an issue it could name but not point at, so a
// run-native consumer could only record it as a block-level annotation
// (preview/toContentTree). Position, suggestion and the offending snippet now
// ride along.
//
// Position is a pointer with omitempty rather than a value, because
// model.Anchor's zero value is a legitimate reading — "the checker located
// nothing" — and a zero range serialized as {0,0,0,0} is indistinguishable
// from a real span at the start of the first run.
type CheckIssueResponse struct {
	Type     string `json:"type"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
	// Position anchors the finding to the checked runs; absent when the checker
	// judged the whole block.
	Position     *model.Anchor `json:"position,omitempty"`
	Suggestion   string        `json:"suggestion,omitempty"`
	OriginalText string        `json:"original_text,omitempty"`
}

// FileCheckResultResponse holds the findings for a single block.
type FileCheckResultResponse struct {
	BlockID string               `json:"blockId"`
	Issues  []CheckIssueResponse `json:"issues"`
}

// CheckRequest asks for the checks on one block or one item. The editor
// sends it as the request body; the fields also read from the query string, so
// a hand-driven call keeps working.
type CheckRequest struct {
	BlockID string `json:"block_id,omitempty"`
	Item    string `json:"item,omitempty"`
	Locale  string `json:"locale"`
}

// bindCheckRequest reads the request body, falling back to the query string
// for each field it leaves empty.
func bindCheckRequest(c echo.Context) CheckRequest {
	var req CheckRequest
	_ = c.Bind(&req)
	if req.BlockID == "" {
		req.BlockID = firstNonEmpty(c.QueryParam("block_id"), c.Param("bid"))
	}
	if req.Item == "" {
		req.Item = fileParam(c)
	}
	if req.Locale == "" {
		req.Locale = c.QueryParam("locale")
	}
	return req
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// HandleCheckBlock runs the checks in force on a single block.
// POST /:ws/:id/actions/:ref/qa-check-block  {block_id, locale}
func (s *Server) HandleCheckBlock(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermViewContent); err != nil {
		return err
	}

	if s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}

	ctx := c.Request().Context()
	pid := projectParam(c)
	stream := streamParam(c)
	req := bindCheckRequest(c)
	if req.Locale == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "locale is required"})
	}
	if req.BlockID == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "block_id is required"})
	}

	sb, err := s.ContentStore.GetBlock(ctx, pid, stream, req.BlockID)
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}

	wsID, _ := c.Get("workspace_id").(string)
	checks, err := s.checksAtPoint(ctx, pid, stream, sb.ItemName, wsID, c.Param("ws"), model.LocaleID(req.Locale))
	if err != nil {
		return serverErrStatus(c, http.StatusServiceUnavailable, fmt.Errorf("resolve block checks: %w", err))
	}
	issues, err := runChecksOnBlock(ctx, sb.Block, checks)
	if err != nil {
		return serverErrStatus(c, http.StatusServiceUnavailable, fmt.Errorf("run block checks: %w", err))
	}
	return c.JSON(http.StatusOK, issues)
}

// HandleCheckFile runs the checks in force on every block in an item.
// POST /:ws/:id/actions/:ref/qa-check  {item, locale}
func (s *Server) HandleCheckFile(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermViewContent); err != nil {
		return err
	}

	if s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}

	ctx := c.Request().Context()
	pid := projectParam(c)
	stream := streamParam(c)
	req := bindCheckRequest(c)
	if req.Locale == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "locale is required"})
	}

	storedBlocks, err := s.ContentStore.GetBlocks(ctx, store.BlockQuery{
		ProjectID: pid,
		Stream:    stream,
		ItemName:  req.Item,
	})
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}

	// Resolved once for the item: every block in it sits at the same point, and
	// the resolution reads the collection, the voice profile and the terms.
	wsID, _ := c.Get("workspace_id").(string)
	checks, err := s.checksAtPoint(ctx, pid, stream, req.Item, wsID, c.Param("ws"), model.LocaleID(req.Locale))
	if err != nil {
		return serverErrStatus(c, http.StatusServiceUnavailable, fmt.Errorf("resolve item checks: %w", err))
	}

	results := make([]FileCheckResultResponse, 0, len(storedBlocks))
	for _, sb := range storedBlocks {
		issues, err := runChecksOnBlock(ctx, sb.Block, checks)
		if err != nil {
			return serverErrStatus(c, http.StatusServiceUnavailable, fmt.Errorf("run item block checks: %w", err))
		}
		results = append(results, FileCheckResultResponse{
			BlockID: sb.Block.ID,
			Issues:  issues,
		})
	}

	return c.JSON(http.StatusOK, results)
}

// pointChecks is the set of checks in force at one point: the standard
// per-locale checks every block gets, plus the ones the governance there adds.
//
// The framework selects the same way — a check run resolves the voice profile
// and the vocabulary at each file's own point, so a project that governs two
// products by two profiles judges each by the one that governs it rather than
// by whichever was resolved first (host/check.go). The platform's point is the
// item's collection, and the ladder is voicescope's.
type pointChecks struct {
	// TargetLocale is the locale being checked; SourceLocale is what the
	// vocabulary is looked up in.
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

// checksAtPoint resolves the checks in force where an item sits: the standard
// per-locale set, plus the voice profile bound at the item's own point (its
// collection, then the stream, the project, and the workspace default), the
// workspace vocabulary, and the project's protected terms.
//
// Resolution failures are operational errors. They cannot silently narrow the
// configured checks and leave the editor displaying a clean assessment.
func (s *Server) checksAtPoint(ctx context.Context, projectID, stream, itemName, workspaceID, workspaceSlug string, locale model.LocaleID) (pointChecks, error) {
	checks := pointChecks{TargetLocale: locale}
	if err := ctx.Err(); err != nil {
		return checks, err
	}
	if s.ContentStore == nil {
		return checks, errors.New("check context: content store unavailable")
	}
	proj, err := s.ContentStore.GetProject(ctx, projectID)
	if err != nil {
		return checks, fmt.Errorf("check project: %w", err)
	}
	if proj == nil {
		return checks, errors.New("check project is unavailable")
	}
	checks.SourceLocale = proj.DefaultSourceLanguage
	checks.DNT = jobs.ProjectDNTTerms(proj)
	voiceCtx := s.editorVoiceContext()
	if voiceCtx.Stores != nil && workspaceSlug != "" {
		checks.Terms, err = voiceCtx.Stores.getTerms(workspaceSlug)
		if err != nil {
			return checks, fmt.Errorf("check terms: %w", err)
		}
	}
	rc := coreprofile.ResolveContext{Locale: locale, ProjectProperties: proj.Properties}
	if stream != "" {
		st, err := s.ContentStore.GetStream(ctx, projectID, stream)
		if err != nil {
			return checks, fmt.Errorf("check stream: %w", err)
		}
		if st == nil {
			return checks, errors.New("check stream is unavailable")
		}
		rc.StreamProperties = st.Properties
	}
	if itemName != "" {
		item, err := s.ContentStore.GetItem(ctx, projectID, stream, itemName)
		if err != nil {
			return checks, fmt.Errorf("check item: %w", err)
		}
		if item == nil {
			return checks, errors.New("check item is unavailable")
		}
		if item.CollectionID != "" {
			col, err := s.ContentStore.GetCollection(ctx, projectID, item.CollectionID)
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
	if voiceCtx.WorkspaceDefault != nil && workspaceID != "" {
		rc.RootProfileID, err = voiceCtx.WorkspaceDefault.WorkspaceVoiceProfileID(ctx, workspaceID)
		if err != nil {
			return checks, fmt.Errorf("check workspace voice: %w", err)
		}
	}
	checks.Voice, err = coreprofile.ResolveProfileFromContext(ctx, rc, voiceCtx.Voice)
	if err != nil {
		return checks, fmt.Errorf("check voice: %w", err)
	}
	return checks, nil
}

// runChecksOnBlock runs the checks in force on a single block and returns the
// findings for exactly this block+locale.
//
// The tools run against a scratch copy with a private annotation surface:
// findings ACCUMULATE on the block's unified FindingsAnnotation by design
// (check.Annotate), so running the checks for several locales over the same
// in-memory block — the dashboard ship-state/compliant pass and convergence's
// countFailingBlocks both do — would otherwise leak one locale's findings into
// every later locale's read (findings carry no locale), and mutate a shared
// block as a side effect of a read.
//
// A zero pointChecks (locale only) is the standard set, which is what the
// dashboard passes: they judge a whole project a block at a time and resolve no
// point of their own.
func runChecksOnBlock(ctx context.Context, block *model.Block, checks pointChecks) ([]CheckIssueResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if block == nil {
		return nil, errors.New("check block is unavailable")
	}
	scratch := *block
	scratch.Annotations = nil // fresh findings surface; SetAnno re-creates lazily
	part := &model.Part{
		Type:     model.PartBlock,
		Resource: &scratch,
	}

	// The standard set: what every target in this locale is judged by.
	if _, err := tools.NewRuleCheckTool(tools.NewRuleCheckConfig(checks.TargetLocale)).ApplyContext(ctx, part); err != nil {
		return nil, fmt.Errorf("rule check: %w", err)
	}

	// Protected terms, when the project declares any. A term that must survive
	// verbatim is checked in the same pass the editor asks for, so a person
	// sees it on the block they are editing.
	if len(checks.DNT) > 0 {
		dntCfg := tools.NewDNTCheckConfig(checks.TargetLocale)
		dntCfg.Terms = checks.DNT
		if _, err := tools.NewDNTCheckTool(dntCfg).ApplyContext(ctx, part); err != nil {
			return nil, fmt.Errorf("protected terms check: %w", err)
		}
	}

	issues := checkIssuesFromFindings(check.Findings(tool.NewBlockViewWithContext(ctx, &scratch)))

	// The vocabulary governing this point: the profile's own rules and the
	// workspace's retired, forbidden and competitor terms, located in the
	// source. It reports on its own annotation rather than the unified surface,
	// so it is read separately.
	if checks.Voice != nil {
		vocab := tools.NewVoiceVocabCheckTool(checks.Voice, checks.Terms).InSourceLocale(checks.SourceLocale)
		if _, err := vocab.ApplyContext(ctx, part); err != nil {
			return nil, fmt.Errorf("voice rules check: %w", err)
		}
		if ann, ok := model.AnnoAs[*coreprofile.VoiceAnnotation](&scratch, "voice"); ok {
			issues = append(issues, checkIssuesFromFindings(ann.Findings)...)
		}
	}

	return issues, nil
}

// checkIssuesFromFindings maps core/check.Finding onto the CheckIssueResponse wire shape.
//
// Everything the finding locates or suggests rides along; only the severity is
// narrowed, to the two values the Problems panel has always styled. The result
// is an empty slice, never nil, so a clean block encodes as [].
func checkIssuesFromFindings(findings []check.Finding) []CheckIssueResponse {
	result := make([]CheckIssueResponse, 0, len(findings))
	for _, f := range findings {
		issue := CheckIssueResponse{
			Type:         f.Category,
			Severity:     checkWireSeverity(f.Severity),
			Message:      f.Message,
			Suggestion:   f.Suggestion,
			OriginalText: f.OriginalText,
		}
		if !f.Position.IsZero() {
			pos := f.Position
			issue.Position = &pos
		}
		result = append(result, issue)
	}
	return result
}

// checkWireSeverity maps a core/check.Severity onto the two-valued severity the
// endpoint has always returned: critical/major are "error", minor/neutral
// "warning".
func checkWireSeverity(s check.Severity) string {
	switch s {
	case check.SeverityCritical, check.SeverityMajor:
		return "error"
	default:
		return "warning"
	}
}
