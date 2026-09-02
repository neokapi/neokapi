package server

import (
	"context"
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

// QAIssueResponse is a single QA finding returned by the API.
//
// {type, severity ("error"|"warning"), message} is the shape the editor's
// Problems panel has always read, and it is unchanged. The QA tools emit
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
type QAIssueResponse struct {
	Type     string `json:"type"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
	// Position anchors the finding to the checked runs; absent when the checker
	// judged the whole block.
	Position     *model.Anchor `json:"position,omitempty"`
	Suggestion   string        `json:"suggestion,omitempty"`
	OriginalText string        `json:"original_text,omitempty"`
}

// FileQAResultResponse holds QA results for a single block.
type FileQAResultResponse struct {
	BlockID string            `json:"blockId"`
	Issues  []QAIssueResponse `json:"issues"`
}

// QACheckRequest asks for the checks on one block or one item. The editor
// sends it as the request body; the fields also read from the query string, so
// a hand-driven call keeps working.
type QACheckRequest struct {
	BlockID string `json:"block_id,omitempty"`
	Item    string `json:"item,omitempty"`
	Locale  string `json:"locale"`
}

// bindQACheckRequest reads the request body, falling back to the query string
// for each field it leaves empty.
func bindQACheckRequest(c echo.Context) QACheckRequest {
	var req QACheckRequest
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

// HandleQACheckBlock runs the checks in force on a single block.
// POST /:ws/:id/actions/:ref/qa-check-block  {block_id, locale}
func (s *Server) HandleQACheckBlock(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermViewContent); err != nil {
		return err
	}

	if s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}

	ctx := c.Request().Context()
	pid := projectParam(c)
	stream := streamParam(c)
	req := bindQACheckRequest(c)
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
	checks := s.checksAtPoint(ctx, pid, stream, sb.ItemName, wsID, c.Param("ws"), model.LocaleID(req.Locale))
	return c.JSON(http.StatusOK, runChecksOnBlock(ctx, sb.Block, checks))
}

// HandleQACheckFile runs the checks in force on every block in an item.
// POST /:ws/:id/actions/:ref/qa-check  {item, locale}
func (s *Server) HandleQACheckFile(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermViewContent); err != nil {
		return err
	}

	if s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "editor not configured"})
	}

	ctx := c.Request().Context()
	pid := projectParam(c)
	stream := streamParam(c)
	req := bindQACheckRequest(c)
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
	checks := s.checksAtPoint(ctx, pid, stream, req.Item, wsID, c.Param("ws"), model.LocaleID(req.Locale))

	results := make([]FileQAResultResponse, 0, len(storedBlocks))
	for _, sb := range storedBlocks {
		results = append(results, FileQAResultResponse{
			BlockID: sb.Block.ID,
			Issues:  runChecksOnBlock(ctx, sb.Block, checks),
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
// Every resolution is best-effort. A store that cannot answer narrows the set
// rather than failing the request: a check that could not run is reported as
// silence, which is the same shape as a clean block, so nothing here is allowed
// to be noisier than the content it judges.
func (s *Server) checksAtPoint(ctx context.Context, projectID, stream, itemName, workspaceID, workspaceSlug string, locale model.LocaleID) pointChecks {
	checks := pointChecks{TargetLocale: locale}
	if s.ContentStore == nil {
		return checks
	}
	proj, err := s.ContentStore.GetProject(ctx, projectID)
	if err != nil || proj == nil {
		return checks
	}
	checks.SourceLocale = proj.DefaultSourceLanguage
	checks.DNT = jobs.ProjectDNTTerms(proj)

	voiceCtx := s.editorVoiceContext()
	checks.Terms = editorTerms(ctx, voiceCtx, workspaceSlug)

	// The same binding the translation assembles from, so a check and the
	// translation it judges resolve one voice at one point.
	b := jobs.TranslateBinding{
		Store:            s.ContentStore,
		Voice:            voiceCtx.Voice,
		WorkspaceDefault: voiceCtx.WorkspaceDefault,
		Project:          proj,
		WorkspaceID:      workspaceID,
		ProjectID:        projectID,
		Stream:           stream,
		ItemName:         itemName,
		TargetLocale:     locale,
	}
	checks.Voice = b.VoiceProfile(ctx, b.Collection(ctx))
	return checks
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
func runChecksOnBlock(ctx context.Context, block *model.Block, checks pointChecks) []QAIssueResponse {
	scratch := *block
	scratch.Annotations = nil // fresh findings surface; SetAnno re-creates lazily
	part := &model.Part{
		Type:     model.PartBlock,
		Resource: &scratch,
	}

	// The standard set: what every target in this locale is judged by.
	// (Errors are ignored: these tools are deterministic and report by
	// annotating.)
	_, _ = tools.NewQACheckTool(tools.NewQACheckConfig(checks.TargetLocale)).ApplyContext(ctx, part)

	// Protected terms, when the project declares any. A term that must survive
	// verbatim is checked here in the same pass the editor already asks for,
	// rather than only after a batch job has run.
	if len(checks.DNT) > 0 {
		dntCfg := tools.NewDNTCheckConfig(checks.TargetLocale)
		dntCfg.Terms = checks.DNT
		_, _ = tools.NewDNTCheckTool(dntCfg).ApplyContext(ctx, part)
	}

	issues := qaIssuesFromFindings(check.Findings(tool.NewBlockViewWithContext(ctx, &scratch)))

	// The vocabulary governing this point: the profile's own rules and the
	// workspace's retired, forbidden and competitor terms, located in the
	// source. It reports on its own annotation rather than the unified surface,
	// so it is read separately.
	if checks.Voice != nil {
		vocab := tools.NewVoiceVocabCheckTool(checks.Voice, checks.Terms).InSourceLocale(checks.SourceLocale)
		_, _ = vocab.ApplyContext(ctx, part)
		if ann, ok := model.AnnoAs[*coreprofile.VoiceAnnotation](&scratch, "voice"); ok {
			issues = append(issues, qaIssuesFromFindings(ann.Findings)...)
		}
	}

	return issues
}

// qaIssuesFromFindings maps core/check.Finding onto the QA wire shape.
//
// Everything the finding locates or suggests rides along; only the severity is
// narrowed, to the two values the Problems panel has always styled. The result
// is an empty slice, never nil, so a clean block encodes as [].
func qaIssuesFromFindings(findings []check.Finding) []QAIssueResponse {
	result := make([]QAIssueResponse, 0, len(findings))
	for _, f := range findings {
		issue := QAIssueResponse{
			Type:         f.Category,
			Severity:     qaWireSeverity(f.Severity),
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

// qaWireSeverity maps a core/check.Severity onto the two-valued severity the QA
// API has always returned: critical/major are "error", minor/neutral "warning".
func qaWireSeverity(s check.Severity) string {
	switch s {
	case check.SeverityCritical, check.SeverityMajor:
		return "error"
	default:
		return "warning"
	}
}
