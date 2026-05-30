package connector

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	platconn "github.com/neokapi/neokapi/bowrain/core/connector"
	"github.com/neokapi/neokapi/core/model"
)

// Google Workspace MIME types for native editor files.
const (
	gwsMimeDoc   = "application/vnd.google-apps.document"
	gwsMimeSheet = "application/vnd.google-apps.spreadsheet"
	gwsMimeSlide = "application/vnd.google-apps.presentation"
)

// gwsKind classifies the editor type of a Drive file; stored in
// ContentItem.Metadata["gws_kind"] so Publish can route write-back correctly.
const (
	gwsKindDoc   = "doc"
	gwsKindSheet = "sheet"
	gwsKindSlide = "slide"
)

// Default Google OAuth token endpoint and the least-privilege editor scopes.
const googleTokenURL = "https://oauth2.googleapis.com/token"

var googleDefaultScopes = []string{
	"https://www.googleapis.com/auth/drive.file",
	"https://www.googleapis.com/auth/documents",
	"https://www.googleapis.com/auth/spreadsheets",
	"https://www.googleapis.com/auth/presentations",
}

// googleEndpoints holds the base URLs for the four Workspace REST APIs. In
// production each lives on its own host; tests point all four at one httptest
// server via the connector's `base_url` config.
type googleEndpoints struct {
	drive  string // .../drive/v3
	docs   string // .../v1  (documents/{id})
	sheets string // .../v4  (spreadsheets/{id})
	slides string // .../v1  (presentations/{id})
}

func googleEndpointsFor(base string) googleEndpoints {
	if base == "" {
		return googleEndpoints{
			drive:  "https://www.googleapis.com/drive/v3",
			docs:   "https://docs.googleapis.com/v1",
			sheets: "https://sheets.googleapis.com/v4",
			slides: "https://slides.googleapis.com/v1",
		}
	}
	base = strings.TrimRight(base, "/")
	return googleEndpoints{
		drive:  base + "/drive/v3",
		docs:   base + "/v1",
		sheets: base + "/v4",
		slides: base + "/v1",
	}
}

// GoogleWorkspaceConnector reads and writes translatable text in Google Docs,
// Sheets, and Slides via the Workspace REST APIs. It authenticates with an
// OAuth2 token source built from the connector config (see [oauthConfig]) and
// handles its own content (de)serialization — there is no DataFormat round-trip
// because the structured Docs/Sheets/Slides APIs are the format.
type GoogleWorkspaceConnector struct {
	id        string
	connName  string
	client    *http.Client
	endpoints googleEndpoints
	fileIDs   []string // optional explicit file scope (else discover via Drive)
	config    map[string]string
}

// NewGoogleWorkspaceConnector creates a Google Workspace connector.
//
// Config keys: the standard oauth_* keys ([oauthConfigFromMap]); `file_ids`
// (optional comma-separated Drive file IDs to scope the connector to); `base_url`
// (optional API host override, used by tests); `id`/`name` (identity).
func NewGoogleWorkspaceConnector(config map[string]string) (*GoogleWorkspaceConnector, error) {
	oc := oauthConfigFromMap(config, googleTokenURL, googleDefaultScopes)
	if !oc.hasCredentials() {
		return nil, errors.New("google connector requires OAuth credentials (oauth_access_token, or a refresh token + client)")
	}
	client, err := oc.httpClient(context.Background())
	if err != nil {
		return nil, fmt.Errorf("google connector: %w", err)
	}

	id := config["id"]
	if id == "" {
		id = "google-workspace"
	}

	return &GoogleWorkspaceConnector{
		id:        id,
		connName:  config["name"],
		client:    client,
		endpoints: googleEndpointsFor(config["base_url"]),
		fileIDs:   splitList(config["file_ids"]),
		config:    config,
	}, nil
}

func (c *GoogleWorkspaceConnector) ID() string                  { return c.id }
func (c *GoogleWorkspaceConnector) Name() string                { return c.connName }
func (c *GoogleWorkspaceConnector) Category() platconn.Category { return platconn.CategoryProductivity }

func (c *GoogleWorkspaceConnector) Configure(config map[string]string) error {
	for k, v := range config {
		c.config[k] = v
	}
	return nil
}

func (c *GoogleWorkspaceConnector) Close() error { return nil }

// driveFile is the subset of a Drive v3 file resource the connector reads.
type driveFile struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	MimeType string `json:"mimeType"`
	Modified string `json:"modifiedTime"`
}

type driveFileList struct {
	Files         []driveFile `json:"files"`
	NextPageToken string      `json:"nextPageToken"`
}

// listFiles enumerates the editor files in scope: either the explicitly
// configured file_ids (metadata fetched per id) or a Drive query for all
// Docs/Sheets/Slides the token can see.
func (c *GoogleWorkspaceConnector) listFiles(ctx context.Context) ([]driveFile, error) {
	if len(c.fileIDs) > 0 {
		var files []driveFile
		for _, id := range c.fileIDs {
			u := fmt.Sprintf("%s/files/%s?fields=%s&supportsAllDrives=true",
				c.endpoints.drive, url.PathEscape(id), url.QueryEscape("id,name,mimeType,modifiedTime"))
			var f driveFile
			if err := c.getJSON(ctx, u, &f); err != nil {
				return nil, fmt.Errorf("get file %s: %w", id, err)
			}
			files = append(files, f)
		}
		return files, nil
	}

	q := fmt.Sprintf("(mimeType='%s' or mimeType='%s' or mimeType='%s') and trashed=false",
		gwsMimeDoc, gwsMimeSheet, gwsMimeSlide)
	var files []driveFile
	pageToken := ""
	for {
		u := fmt.Sprintf("%s/files?q=%s&fields=%s&pageSize=100&supportsAllDrives=true&includeItemsFromAllDrives=true",
			c.endpoints.drive, url.QueryEscape(q), url.QueryEscape("nextPageToken,files(id,name,mimeType,modifiedTime)"))
		if pageToken != "" {
			u += "&pageToken=" + url.QueryEscape(pageToken)
		}
		var list driveFileList
		if err := c.getJSON(ctx, u, &list); err != nil {
			return nil, fmt.Errorf("list drive files: %w", err)
		}
		files = append(files, list.Files...)
		if list.NextPageToken == "" {
			break
		}
		pageToken = list.NextPageToken
	}
	return files, nil
}

// List returns the editor files in scope without extracting their content.
func (c *GoogleWorkspaceConnector) List(ctx context.Context) ([]*platconn.ContentItem, error) {
	files, err := c.listFiles(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]*platconn.ContentItem, 0, len(files))
	for _, f := range files {
		kind := kindForMime(f.MimeType)
		if kind == "" {
			continue
		}
		modified, _ := time.Parse(time.RFC3339, f.Modified)
		items = append(items, &platconn.ContentItem{
			ID:          f.ID,
			Name:        f.Name,
			Path:        kind + "/" + f.ID,
			Format:      "gws-" + kind,
			LastChanged: modified,
			Metadata:    map[string]string{"gws_kind": kind, "gws_file_id": f.ID},
		})
	}
	return items, nil
}

// Fetch extracts translatable blocks from each in-scope file.
func (c *GoogleWorkspaceConnector) Fetch(ctx context.Context, opts platconn.FetchOptions) ([]*platconn.ContentItem, error) {
	files, err := c.listFiles(ctx)
	if err != nil {
		return nil, err
	}

	want := map[string]bool{}
	for _, p := range opts.Paths {
		want[p] = true
	}

	var items []*platconn.ContentItem
	for _, f := range files {
		kind := kindForMime(f.MimeType)
		if kind == "" {
			continue
		}
		if len(want) > 0 && !want[f.ID] && !want[kind+"/"+f.ID] {
			continue
		}

		var blocks []*model.Block
		var ferr error
		switch kind {
		case gwsKindDoc:
			blocks, ferr = c.fetchDoc(ctx, f.ID)
		case gwsKindSheet:
			blocks, ferr = c.fetchSheet(ctx, f.ID)
		case gwsKindSlide:
			blocks, ferr = c.fetchSlides(ctx, f.ID)
		}
		if ferr != nil {
			return nil, fmt.Errorf("fetch %s %s: %w", kind, f.ID, ferr)
		}

		modified, _ := time.Parse(time.RFC3339, f.Modified)
		items = append(items, &platconn.ContentItem{
			ID:          f.ID,
			Name:        f.Name,
			Path:        kind + "/" + f.ID,
			Format:      "gws-" + kind,
			Blocks:      blocks,
			LastChanged: modified,
			Metadata:    map[string]string{"gws_kind": kind, "gws_file_id": f.ID},
		})
	}
	return items, nil
}

// Publish writes translated targets back into each file. The target locale is
// taken from opts.Locales[0]; if empty, the first target locale present on the
// blocks is used.
func (c *GoogleWorkspaceConnector) Publish(ctx context.Context, items []*platconn.ContentItem, opts platconn.PublishOptions) error {
	for _, item := range items {
		kind := item.Metadata["gws_kind"]
		fileID := item.Metadata["gws_file_id"]
		if fileID == "" {
			fileID = item.ID
		}
		locale := publishLocale(opts, item.Blocks)
		if locale == "" {
			continue
		}

		var err error
		switch kind {
		case gwsKindDoc:
			err = c.publishDoc(ctx, fileID, item.Blocks, locale)
		case gwsKindSheet:
			err = c.publishSheet(ctx, fileID, item.Blocks, locale)
		case gwsKindSlide:
			err = c.publishSlides(ctx, fileID, item.Blocks, locale)
		default:
			err = fmt.Errorf("unknown gws_kind %q for item %s", kind, item.ID)
		}
		if err != nil {
			return fmt.Errorf("publish %s %s: %w", kind, fileID, err)
		}
	}
	return nil
}

func (c *GoogleWorkspaceConnector) Status(ctx context.Context) (*platconn.SyncStatus, error) {
	items, err := c.List(ctx)
	if err != nil {
		return nil, err
	}
	return &platconn.SyncStatus{
		ConnectorID: c.id,
		LastSync:    time.Now(),
		ItemCount:   len(items),
	}, nil
}

// ---------------------------------------------------------------------------
// Docs
// ---------------------------------------------------------------------------

type docsDocument struct {
	Title string   `json:"title"`
	Body  docsBody `json:"body"`
	// RevisionId enables optimistic concurrency on batchUpdate.
	RevisionID string `json:"revisionId"`
}

type docsBody struct {
	Content []docsStructuralElement `json:"content"`
}

type docsStructuralElement struct {
	Paragraph *docsParagraph `json:"paragraph"`
	Table     *docsTable     `json:"table"`
}

type docsTable struct {
	TableRows []struct {
		TableCells []struct {
			Content []docsStructuralElement `json:"content"`
		} `json:"tableCells"`
	} `json:"tableRows"`
}

type docsParagraph struct {
	Elements []docsParagraphElement `json:"elements"`
}

type docsParagraphElement struct {
	TextRun *docsTextRun `json:"textRun"`
}

type docsTextRun struct {
	Content string `json:"content"`
}

func (c *GoogleWorkspaceConnector) fetchDoc(ctx context.Context, fileID string) ([]*model.Block, error) {
	u := fmt.Sprintf("%s/documents/%s", c.endpoints.docs, url.PathEscape(fileID))
	var doc docsDocument
	if err := c.getJSON(ctx, u, &doc); err != nil {
		return nil, err
	}
	var blocks []*model.Block
	idx := 0
	var walk func(content []docsStructuralElement)
	walk = func(content []docsStructuralElement) {
		for _, se := range content {
			switch {
			case se.Paragraph != nil:
				var sb strings.Builder
				for _, el := range se.Paragraph.Elements {
					if el.TextRun != nil {
						sb.WriteString(el.TextRun.Content)
					}
				}
				if b := makeTextBlock(fmt.Sprintf("%s:doc:%d", fileID, idx), sb.String()); b != nil {
					blocks = append(blocks, b)
					idx++
				}
			case se.Table != nil:
				for _, row := range se.Table.TableRows {
					for _, cell := range row.TableCells {
						walk(cell.Content)
					}
				}
			}
		}
	}
	walk(doc.Body.Content)
	return blocks, nil
}

// docsRequest models the subset of documents.batchUpdate requests used here.
type docsRequest struct {
	ReplaceAllText *docsReplaceAllText `json:"replaceAllText,omitempty"`
}

type docsReplaceAllText struct {
	ContainsText docsSubstringMatch `json:"containsText"`
	ReplaceText  string             `json:"replaceText"`
}

type docsSubstringMatch struct {
	Text      string `json:"text"`
	MatchCase bool   `json:"matchCase"`
}

func (c *GoogleWorkspaceConnector) publishDoc(ctx context.Context, fileID string, blocks []*model.Block, locale model.LocaleID) error {
	var reqs []docsRequest
	for _, b := range blocks {
		src, tgt := strings.TrimSpace(b.SourceText()), b.TargetText(locale)
		if src == "" || tgt == "" || src == tgt {
			continue
		}
		reqs = append(reqs, docsRequest{ReplaceAllText: &docsReplaceAllText{
			ContainsText: docsSubstringMatch{Text: src, MatchCase: true},
			ReplaceText:  tgt,
		}})
	}
	if len(reqs) == 0 {
		return nil
	}
	u := fmt.Sprintf("%s/documents/%s:batchUpdate", c.endpoints.docs, url.PathEscape(fileID))
	return c.postJSON(ctx, u, map[string]any{"requests": reqs}, nil)
}

// ---------------------------------------------------------------------------
// Sheets
// ---------------------------------------------------------------------------

type sheetsSpreadsheet struct {
	Sheets []struct {
		Properties struct {
			Title string `json:"title"`
		} `json:"properties"`
	} `json:"sheets"`
}

type sheetsValueRange struct {
	Range  string  `json:"range"`
	Values [][]any `json:"values"`
}

func (c *GoogleWorkspaceConnector) fetchSheet(ctx context.Context, fileID string) ([]*model.Block, error) {
	metaURL := fmt.Sprintf("%s/spreadsheets/%s?fields=%s",
		c.endpoints.sheets, url.PathEscape(fileID), url.QueryEscape("sheets.properties.title"))
	var meta sheetsSpreadsheet
	if err := c.getJSON(ctx, metaURL, &meta); err != nil {
		return nil, err
	}

	var blocks []*model.Block
	for _, sh := range meta.Sheets {
		title := sh.Properties.Title
		if title == "" {
			continue
		}
		valURL := fmt.Sprintf("%s/spreadsheets/%s/values/%s?valueRenderOption=UNFORMATTED_VALUE",
			c.endpoints.sheets, url.PathEscape(fileID), url.PathEscape(title))
		var vr sheetsValueRange
		if err := c.getJSON(ctx, valURL, &vr); err != nil {
			return nil, err
		}
		for r, row := range vr.Values {
			for col, cell := range row {
				s, ok := cell.(string)
				if !ok {
					continue
				}
				if strings.TrimSpace(s) == "" || strings.HasPrefix(s, "=") {
					continue
				}
				a1 := fmt.Sprintf("%s!%s%d", title, columnLetter(col), r+1)
				b := makeTextBlock(fmt.Sprintf("%s:cell:%s", fileID, a1), s)
				if b == nil {
					continue
				}
				if b.Properties == nil {
					b.Properties = map[string]string{}
				}
				b.Properties["gws_cell"] = a1
				blocks = append(blocks, b)
			}
		}
	}
	return blocks, nil
}

func (c *GoogleWorkspaceConnector) publishSheet(ctx context.Context, fileID string, blocks []*model.Block, locale model.LocaleID) error {
	var data []sheetsValueRange
	for _, b := range blocks {
		a1 := b.Properties["gws_cell"]
		tgt := b.TargetText(locale)
		if a1 == "" || tgt == "" {
			continue
		}
		data = append(data, sheetsValueRange{Range: a1, Values: [][]any{{tgt}}})
	}
	if len(data) == 0 {
		return nil
	}
	u := fmt.Sprintf("%s/spreadsheets/%s/values:batchUpdate", c.endpoints.sheets, url.PathEscape(fileID))
	return c.postJSON(ctx, u, map[string]any{
		"valueInputOption": "RAW",
		"data":             data,
	}, nil)
}

// ---------------------------------------------------------------------------
// Slides
// ---------------------------------------------------------------------------

type slidesPresentation struct {
	Slides []struct {
		PageElements []slidesPageElement `json:"pageElements"`
	} `json:"slides"`
}

type slidesPageElement struct {
	ObjectID string `json:"objectId"`
	Shape    *struct {
		Text *slidesTextContent `json:"text"`
	} `json:"shape"`
	Table *struct {
		TableRows []struct {
			TableCells []struct {
				Text *slidesTextContent `json:"text"`
			} `json:"tableCells"`
		} `json:"tableRows"`
	} `json:"table"`
}

type slidesTextContent struct {
	TextElements []struct {
		TextRun *struct {
			Content string `json:"content"`
		} `json:"textRun"`
	} `json:"textElements"`
}

func (c *GoogleWorkspaceConnector) fetchSlides(ctx context.Context, fileID string) ([]*model.Block, error) {
	u := fmt.Sprintf("%s/presentations/%s", c.endpoints.slides, url.PathEscape(fileID))
	var pres slidesPresentation
	if err := c.getJSON(ctx, u, &pres); err != nil {
		return nil, err
	}
	var blocks []*model.Block
	idx := 0
	collect := func(tc *slidesTextContent) {
		if tc == nil {
			return
		}
		var sb strings.Builder
		for _, te := range tc.TextElements {
			if te.TextRun != nil {
				sb.WriteString(te.TextRun.Content)
			}
		}
		if b := makeTextBlock(fmt.Sprintf("%s:slide:%d", fileID, idx), sb.String()); b != nil {
			blocks = append(blocks, b)
			idx++
		}
	}
	for _, slide := range pres.Slides {
		for _, pe := range slide.PageElements {
			if pe.Shape != nil {
				collect(pe.Shape.Text)
			}
			if pe.Table != nil {
				for _, row := range pe.Table.TableRows {
					for _, cell := range row.TableCells {
						collect(cell.Text)
					}
				}
			}
		}
	}
	return blocks, nil
}

type slidesRequest struct {
	ReplaceAllText *slidesReplaceAllText `json:"replaceAllText,omitempty"`
}

type slidesReplaceAllText struct {
	ContainsText docsSubstringMatch `json:"containsText"`
	ReplaceText  string             `json:"replaceText"`
}

func (c *GoogleWorkspaceConnector) publishSlides(ctx context.Context, fileID string, blocks []*model.Block, locale model.LocaleID) error {
	var reqs []slidesRequest
	for _, b := range blocks {
		src, tgt := strings.TrimSpace(b.SourceText()), b.TargetText(locale)
		if src == "" || tgt == "" || src == tgt {
			continue
		}
		reqs = append(reqs, slidesRequest{ReplaceAllText: &slidesReplaceAllText{
			ContainsText: docsSubstringMatch{Text: src, MatchCase: true},
			ReplaceText:  tgt,
		}})
	}
	if len(reqs) == 0 {
		return nil
	}
	u := fmt.Sprintf("%s/presentations/%s:batchUpdate", c.endpoints.slides, url.PathEscape(fileID))
	return c.postJSON(ctx, u, map[string]any{"requests": reqs}, nil)
}

// ---------------------------------------------------------------------------
// HTTP helpers
// ---------------------------------------------------------------------------

func (c *GoogleWorkspaceConnector) getJSON(ctx context.Context, u string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	return c.do(req, out)
}

func (c *GoogleWorkspaceConnector) postJSON(ctx context.Context, u string, body, out any) error {
	buf, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(buf))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	return c.do(req, out)
}

func (c *GoogleWorkspaceConnector) do(req *http.Request, out any) error {
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("google API: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	if out == nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// ---------------------------------------------------------------------------
// shared helpers
// ---------------------------------------------------------------------------

func kindForMime(mime string) string {
	switch mime {
	case gwsMimeDoc:
		return gwsKindDoc
	case gwsMimeSheet:
		return gwsKindSheet
	case gwsMimeSlide:
		return gwsKindSlide
	default:
		return ""
	}
}

// makeTextBlock builds a translatable block from raw text, returning nil for
// content that is empty or whitespace-only. The trailing newline that Docs/
// Slides text runs carry is trimmed so replaceAllText matches the visible text.
func makeTextBlock(id, text string) *model.Block {
	trimmed := strings.TrimRight(text, "\n")
	if strings.TrimSpace(trimmed) == "" {
		return nil
	}
	b := model.NewBlock(id, trimmed)
	b.SourceLocale = model.LocaleEnglish
	return b
}

// publishLocale resolves the target locale for a publish: the first requested
// locale, else the first target locale present on the blocks.
func publishLocale(opts platconn.PublishOptions, blocks []*model.Block) model.LocaleID {
	if len(opts.Locales) > 0 {
		return opts.Locales[0]
	}
	for _, b := range blocks {
		for vk := range b.Targets {
			return vk.Locale
		}
	}
	return ""
}

func splitList(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// columnLetter converts a zero-based column index to spreadsheet letters
// (0→A, 25→Z, 26→AA).
func columnLetter(col int) string {
	var sb strings.Builder
	col++
	for col > 0 {
		col--
		sb.WriteByte(byte('A' + col%26))
		col /= 26
	}
	// reverse
	b := []byte(sb.String())
	for i, j := 0, len(b)-1; i < j; i, j = i+1, j-1 {
		b[i], b[j] = b[j], b[i]
	}
	return string(b)
}
