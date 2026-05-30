package addin

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/neokapi/neokapi/bowrain/connector"
	platconn "github.com/neokapi/neokapi/bowrain/core/connector"
	aitools "github.com/neokapi/neokapi/core/ai/tools"
	"github.com/neokapi/neokapi/core/brand"
	"github.com/neokapi/neokapi/core/model"
)

// GoogleEvent is the subset of the Google Workspace add-on HTTP event payload
// the handlers read. Google POSTs this JSON to each trigger endpoint.
//
// https://developers.google.com/workspace/add-ons/guides/alternate-runtimes
type GoogleEvent struct {
	CommonEventObject struct {
		HostApp    string                     `json:"hostApp"` // DOCS|SHEETS|SLIDES
		Platform   string                     `json:"platform"`
		FormInputs map[string]googleFormInput `json:"formInputs"`
		Parameters map[string]string          `json:"parameters"`
	} `json:"commonEventObject"`
	AuthorizationEventObject struct {
		UserOAuthToken string `json:"userOAuthToken"`
	} `json:"authorizationEventObject"`
	Docs   *googleEditorContext `json:"docs"`
	Sheets *googleEditorContext `json:"sheets"`
	Slides *googleEditorContext `json:"slides"`
}

type googleFormInput struct {
	StringInputs struct {
		Value []string `json:"value"`
	} `json:"stringInputs"`
}

type googleEditorContext struct {
	ID                          string `json:"id"`
	Title                       string `json:"title"`
	AddonHasFileScopePermission bool   `json:"addonHasFileScopePermission"`
}

// activeFile returns the editor context for whichever host opened the add-on.
func (e *GoogleEvent) activeFile() *googleEditorContext {
	switch {
	case e.Docs != nil:
		return e.Docs
	case e.Sheets != nil:
		return e.Sheets
	case e.Slides != nil:
		return e.Slides
	default:
		return nil
	}
}

// formValue returns the first value submitted for a form field.
func (e *GoogleEvent) formValue(name string) string {
	if fi, ok := e.CommonEventObject.FormInputs[name]; ok && len(fi.StringInputs.Value) > 0 {
		return fi.StringInputs.Value[0]
	}
	return ""
}

// RegisterGoogleRoutes mounts the Google Workspace add-on Card-JSON endpoints.
// These are the HTTPS URLs referenced by the add-on deployment manifest's
// homepageTrigger / onFileScopeGrantedTrigger / button actions. They must be
// reachable by Google and should verify the inbound system ID token in
// production (see VerifyGoogleRequest hook below); they are NOT behind the
// Bowrain user-auth middleware because Google — not the user's browser — calls
// them.
//
//	POST /google/homepage   homepage + onFileScopeGranted trigger
//	POST /google/authorize  returns the drive.file scope-request action
//	POST /google/scan       fetch the active doc, return a findings card
//	POST /google/translate  fetch, translate on-brand, write back, notify
func (s *Service) RegisterGoogleRoutes(g *echo.Group) {
	g.POST("/google/homepage", s.handleGoogleHomepage)
	g.POST("/google/authorize", s.handleGoogleAuthorize)
	g.POST("/google/scan", s.handleGoogleScan)
	g.POST("/google/translate", s.handleGoogleTranslate)
}

func (s *Service) handleGoogleHomepage(c echo.Context) error {
	var ev GoogleEvent
	if err := c.Bind(&ev); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Error: "invalid event: " + err.Error()})
	}
	file := ev.activeFile()
	if file == nil || !file.AddonHasFileScopePermission {
		// Not yet authorized for this document — show the grant-access card.
		return c.JSON(http.StatusOK, pushHomepage(grantAccessCard(s.PublicURL)))
	}
	title := file.Title
	if title == "" {
		title = "Untitled"
	}
	return c.JSON(http.StatusOK, pushHomepage(homeCard(s.PublicURL, title, nil, nil, defaultTargetItems(""))))
}

func (s *Service) handleGoogleAuthorize(c echo.Context) error {
	return c.JSON(http.StatusOK, requestFileScope())
}

func (s *Service) handleGoogleScan(c echo.Context) error {
	var ev GoogleEvent
	if err := c.Bind(&ev); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Error: "invalid event: " + err.Error()})
	}
	file := ev.activeFile()
	if file == nil || file.ID == "" {
		return c.JSON(http.StatusOK, updateCard(grantAccessCard(s.PublicURL), "Grant access to scan."))
	}

	items, err := s.fetchActiveFile(c.Request().Context(), ev, file.ID)
	if err != nil {
		slog.Warn("google addon: scan failed", "error", err, "file", file.ID)
		return c.JSON(http.StatusOK, updateCard(homeCard(s.PublicURL, file.Title, nil, nil, defaultTargetItems("")), "Couldn't read the document."))
	}
	text := joinBlocks(items)

	checkRes, _ := s.Check(c.Request().Context(), CheckRequest{Text: text})
	termsRes, _ := s.Terms(c.Request().Context(), TermsRequest{Text: text})

	target := ev.formValue("targetLang")
	card := homeCard(s.PublicURL, file.Title, checkRes, termsRes, defaultTargetItems(target))
	return c.JSON(http.StatusOK, updateCard(card, "Scan complete."))
}

func (s *Service) handleGoogleTranslate(c echo.Context) error {
	var ev GoogleEvent
	if err := c.Bind(&ev); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Error: "invalid event: " + err.Error()})
	}
	file := ev.activeFile()
	if file == nil || file.ID == "" {
		return c.JSON(http.StatusOK, updateCard(grantAccessCard(s.PublicURL), "Grant access to translate."))
	}
	target := ev.formValue("targetLang")
	if target == "" {
		target = "fr"
	}

	// fail logs the real error server-side and returns a generic toast so we
	// never leak upstream (Google / connector) error detail to the client.
	fail := func(stage string, err error) error {
		slog.Warn("google addon: translate failed", "stage", stage, "error", err, "file", file.ID)
		return c.JSON(http.StatusOK, updateCard(
			homeCard(s.PublicURL, file.Title, nil, nil, defaultTargetItems(target)),
			"Translation failed. Please try again."))
	}

	ctx := c.Request().Context()
	conn, err := s.googleConnector(ev, file.ID)
	if err != nil {
		return fail("connect", err)
	}
	defer conn.Close()

	items, err := conn.Fetch(ctx, platconn.FetchOptions{Paths: []string{file.ID}})
	if err != nil {
		return fail("fetch", err)
	}
	if err := s.translateItems(ctx, items, model.LocaleID(target)); err != nil {
		return fail("translate", err)
	}
	if err := conn.Publish(ctx, items, platconn.PublishOptions{Locales: []model.LocaleID{model.LocaleID(target)}}); err != nil {
		return fail("write-back", err)
	}

	card := homeCard(s.PublicURL, file.Title, nil, nil, defaultTargetItems(target))
	return c.JSON(http.StatusOK, updateCard(card, "Translated this document to "+target+"."))
}

// ---------------------------------------------------------------------------
// connector glue
// ---------------------------------------------------------------------------

// googleConnector builds a Google Workspace connector scoped to one file,
// authenticated with the user's per-file OAuth token from the event.
func (s *Service) googleConnector(ev GoogleEvent, fileID string) (*connector.GoogleWorkspaceConnector, error) {
	cfg := map[string]string{
		"oauth_access_token": ev.AuthorizationEventObject.UserOAuthToken,
		"file_ids":           fileID,
	}
	if s.GoogleBaseURL != "" {
		cfg["base_url"] = s.GoogleBaseURL
	}
	return connector.NewGoogleWorkspaceConnector(cfg)
}

func (s *Service) fetchActiveFile(ctx context.Context, ev GoogleEvent, fileID string) ([]*platconn.ContentItem, error) {
	conn, err := s.googleConnector(ev, fileID)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	return conn.Fetch(ctx, platconn.FetchOptions{Paths: []string{fileID}})
}

// translateItems translates every block in the items on-brand into the target
// locale, reusing one provider + tool for the whole document.
func (s *Service) translateItems(ctx context.Context, items []*platconn.ContentItem, target model.LocaleID) error {
	provider, err := s.newProvider(ctx)
	if err != nil {
		return err
	}
	defer provider.Close()

	var profile *brand.VoiceProfile
	if p, perr := s.resolveProfile("", string(target)); perr == nil {
		profile = p
	}

	t := aitools.NewAITranslateTool(provider, aitools.AITranslateConfig{
		SourceLocale: model.LocaleEnglish,
		TargetLocale: target,
		Provider:     string(provider.Name()),
		Profile:      profile,
	})

	for _, item := range items {
		for _, b := range item.Blocks {
			if err := processBlock(ctx, t, b); err != nil {
				return err
			}
		}
	}
	return nil
}

// joinBlocks concatenates the source text of every block across items.
func joinBlocks(items []*platconn.ContentItem) string {
	var sb strings.Builder
	for _, item := range items {
		for _, b := range item.Blocks {
			if t := b.SourceText(); t != "" {
				sb.WriteString(t)
				sb.WriteByte('\n')
			}
		}
	}
	return sb.String()
}
