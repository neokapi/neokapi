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
	"path"
	"strings"
	"time"

	platconn "github.com/neokapi/neokapi/bowrain/core/connector"
	coreformat "github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
)

// graphScopeDefault is the app-only scope that requests every
// admin-consented application permission for the registered app.
const graphScopeDefault = "https://graph.microsoft.com/.default"

// originalContentSetter is implemented by faithful writers (e.g. openxml) that
// reconstruct by splicing translations into the original document bytes rather
// than regenerating from scratch.
type originalContentSetter interface {
	SetOriginalContent(content []byte)
}

// officeExtensions are the OOXML file extensions the connector translates.
var officeExtensions = map[string]bool{
	".docx": true, ".docm": true,
	".xlsx": true, ".xlsm": true,
	".pptx": true, ".pptm": true,
}

// Microsoft365Connector reads and writes Office documents (.docx/.xlsx/.pptx)
// stored in OneDrive or SharePoint via Microsoft Graph. Graph returns opaque
// file bytes — it does not parse OOXML — so the connector streams the bytes
// through the framework's native `openxml` DataFormat reader/writer, exactly as
// the file connector does for local files. No temporary files are used.
type Microsoft365Connector struct {
	id        string
	connName  string
	client    *http.Client
	graphBase string // e.g. https://graph.microsoft.com/v1.0
	formats   *registry.FormatRegistry

	// drive selection (one of):
	driveID string // explicit /drives/{id}
	site    string // SharePoint site path "host:/sites/team"
	library string // document-library (drive) name within the site

	resolvedDriveBase string // cached "{graphBase}/drives/{id}" or "{graphBase}/me/drive"
	config            map[string]string
}

// NewMicrosoft365Connector creates a Microsoft 365 connector.
//
// Config keys: oauth_* ([oauthConfigFromMap]); `tenant_id`, `client_id`,
// `client_secret` (app-only client-credentials convenience aliases — when set,
// the Entra token endpoint and the Graph .default scope are filled in
// automatically); `drive_id` (explicit OneDrive/SharePoint drive), or `site`
// ("host:/sites/team") + optional `library` (document-library name) to resolve a
// SharePoint drive; `base_url` (Graph host override for tests); `id`/`name`.
func NewMicrosoft365Connector(config map[string]string) (*Microsoft365Connector, error) {
	cfg := withMicrosoftAuthAliases(config)
	oc := oauthConfigFromMap(cfg, "", []string{graphScopeDefault})
	if !oc.hasCredentials() {
		return nil, errors.New("microsoft365 connector requires credentials (oauth_access_token, or tenant_id + client_id + client_secret)")
	}
	client, err := oc.httpClient(context.Background())
	if err != nil {
		return nil, fmt.Errorf("microsoft365 connector: %w", err)
	}

	id := config["id"]
	if id == "" {
		id = "microsoft365"
	}
	graphBase := strings.TrimRight(config["base_url"], "/")
	if graphBase == "" {
		graphBase = "https://graph.microsoft.com/v1.0"
	}

	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)

	return &Microsoft365Connector{
		id:        id,
		connName:  config["name"],
		client:    client,
		graphBase: graphBase,
		formats:   reg,
		driveID:   config["drive_id"],
		site:      config["site"],
		library:   config["library"],
		config:    config,
	}, nil
}

// withMicrosoftAuthAliases maps the friendlier tenant_id/client_id/client_secret
// keys onto the standard oauth_* keys and derives the Entra token endpoint.
func withMicrosoftAuthAliases(config map[string]string) map[string]string {
	out := make(map[string]string, len(config))
	for k, v := range config {
		out[k] = v
	}
	if out["oauth_client_id"] == "" && config["client_id"] != "" {
		out["oauth_client_id"] = config["client_id"]
	}
	if out["oauth_client_secret"] == "" && config["client_secret"] != "" {
		out["oauth_client_secret"] = config["client_secret"]
	}
	if out["oauth_token_url"] == "" {
		if tenant := config["tenant_id"]; tenant != "" {
			out["oauth_token_url"] = fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/v2.0/token", url.PathEscape(tenant))
		}
	}
	return out
}

func (c *Microsoft365Connector) ID() string                  { return c.id }
func (c *Microsoft365Connector) Name() string                { return c.connName }
func (c *Microsoft365Connector) Category() platconn.Category { return platconn.CategoryProductivity }

func (c *Microsoft365Connector) Configure(config map[string]string) error {
	for k, v := range config {
		c.config[k] = v
	}
	return nil
}

func (c *Microsoft365Connector) Close() error { return nil }

// driveBase resolves (and caches) the Graph drive root for this connector:
// an explicit drive, a SharePoint site's library, or the token's OneDrive.
func (c *Microsoft365Connector) driveBase(ctx context.Context) (string, error) {
	if c.resolvedDriveBase != "" {
		return c.resolvedDriveBase, nil
	}
	switch {
	case c.driveID != "":
		c.resolvedDriveBase = fmt.Sprintf("%s/drives/%s", c.graphBase, url.PathEscape(c.driveID))
	case c.site != "":
		base, err := c.resolveSiteDrive(ctx)
		if err != nil {
			return "", err
		}
		c.resolvedDriveBase = base
	default:
		c.resolvedDriveBase = c.graphBase + "/me/drive"
	}
	return c.resolvedDriveBase, nil
}

type graphSite struct {
	ID string `json:"id"`
}

type graphDriveList struct {
	Value []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"value"`
}

func (c *Microsoft365Connector) resolveSiteDrive(ctx context.Context) (string, error) {
	var site graphSite
	if err := c.getJSON(ctx, fmt.Sprintf("%s/sites/%s", c.graphBase, c.site), &site); err != nil {
		return "", fmt.Errorf("resolve site %q: %w", c.site, err)
	}
	if c.library == "" {
		return fmt.Sprintf("%s/sites/%s/drive", c.graphBase, url.PathEscape(site.ID)), nil
	}
	var drives graphDriveList
	if err := c.getJSON(ctx, fmt.Sprintf("%s/sites/%s/drives", c.graphBase, url.PathEscape(site.ID)), &drives); err != nil {
		return "", fmt.Errorf("list site drives: %w", err)
	}
	for _, d := range drives.Value {
		if strings.EqualFold(d.Name, c.library) {
			return fmt.Sprintf("%s/drives/%s", c.graphBase, url.PathEscape(d.ID)), nil
		}
	}
	return "", fmt.Errorf("document library %q not found in site %q", c.library, c.site)
}

// graphDriveItem is the subset of a driveItem the connector reads.
type graphDriveItem struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	ETag     string `json:"eTag"`
	Modified string `json:"lastModifiedDateTime"`
	File     *struct {
		MimeType string `json:"mimeType"`
	} `json:"file"`
}

type graphChildren struct {
	Value    []graphDriveItem `json:"value"`
	NextLink string           `json:"@odata.nextLink"`
}

// listItems enumerates Office files in the drive root, following pagination.
func (c *Microsoft365Connector) listItems(ctx context.Context) ([]graphDriveItem, string, error) {
	driveBase, err := c.driveBase(ctx)
	if err != nil {
		return nil, "", err
	}
	next := fmt.Sprintf("%s/root/children?$select=%s", driveBase,
		url.QueryEscape("id,name,file,eTag,lastModifiedDateTime"))
	var items []graphDriveItem
	for next != "" {
		var page graphChildren
		if err := c.getJSON(ctx, next, &page); err != nil {
			return nil, "", fmt.Errorf("list drive children: %w", err)
		}
		for _, it := range page.Value {
			if it.File != nil && officeExtensions[strings.ToLower(path.Ext(it.Name))] {
				items = append(items, it)
			}
		}
		next = page.NextLink
	}
	return items, driveBase, nil
}

// List returns the Office files in the drive without downloading content.
func (c *Microsoft365Connector) List(ctx context.Context) ([]*platconn.ContentItem, error) {
	items, _, err := c.listItems(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*platconn.ContentItem, 0, len(items))
	for _, it := range items {
		modified, _ := time.Parse(time.RFC3339, it.Modified)
		out = append(out, &platconn.ContentItem{
			ID:          it.ID,
			Name:        it.Name,
			Path:        it.Name,
			Format:      c.formatFor(it.Name),
			LastChanged: modified,
			Metadata:    map[string]string{"ms_item_id": it.ID, "ms_etag": it.ETag},
		})
	}
	return out, nil
}

// Fetch downloads each Office file and extracts translatable blocks via the
// native openxml reader.
func (c *Microsoft365Connector) Fetch(ctx context.Context, opts platconn.FetchOptions) ([]*platconn.ContentItem, error) {
	items, driveBase, err := c.listItems(ctx)
	if err != nil {
		return nil, err
	}

	want := map[string]bool{}
	for _, p := range opts.Paths {
		want[p] = true
	}

	var result []*platconn.ContentItem
	for _, it := range items {
		if len(want) > 0 && !want[it.ID] && !want[it.Name] {
			continue
		}
		blocks, format, err := c.fetchItem(ctx, driveBase, it)
		if err != nil {
			return nil, fmt.Errorf("fetch %s: %w", it.Name, err)
		}
		modified, _ := time.Parse(time.RFC3339, it.Modified)
		result = append(result, &platconn.ContentItem{
			ID:          it.ID,
			Name:        it.Name,
			Path:        it.Name,
			Format:      format,
			Blocks:      blocks,
			LastChanged: modified,
			Metadata:    map[string]string{"ms_item_id": it.ID, "ms_etag": it.ETag},
		})
	}
	return result, nil
}

func (c *Microsoft365Connector) fetchItem(ctx context.Context, driveBase string, it graphDriveItem) ([]*model.Block, string, error) {
	format := c.formatFor(it.Name)
	if format == "" {
		return nil, "", fmt.Errorf("no registered format for %s", it.Name)
	}

	reader, err := c.formats.NewReader(registry.FormatID(format))
	if err != nil {
		return nil, "", fmt.Errorf("create reader %s: %w", format, err)
	}
	defer reader.Close()

	body, err := c.download(ctx, driveBase, it.ID)
	if err != nil {
		return nil, "", err
	}

	doc := &model.RawDocument{URI: it.Name, FormatID: format, Reader: io.NopCloser(bytes.NewReader(body))}
	if err := reader.Open(ctx, doc); err != nil {
		return nil, "", fmt.Errorf("open %s: %w", it.Name, err)
	}

	var blocks []*model.Block
	for pr := range reader.Read(ctx) {
		if pr.Error != nil {
			return nil, "", fmt.Errorf("read %s: %w", it.Name, pr.Error)
		}
		if pr.Part != nil && pr.Part.Type == model.PartBlock {
			if b, ok := pr.Part.Resource.(*model.Block); ok {
				blocks = append(blocks, b)
			}
		}
	}
	return blocks, format, nil
}

// Publish writes translated blocks back into each Office file via the openxml
// writer, uploading the regenerated bytes to Graph.
func (c *Microsoft365Connector) Publish(ctx context.Context, items []*platconn.ContentItem, opts platconn.PublishOptions) error {
	driveBase, err := c.driveBase(ctx)
	if err != nil {
		return err
	}
	for _, item := range items {
		itemID := item.Metadata["ms_item_id"]
		if itemID == "" {
			itemID = item.ID
		}
		locale := publishLocale(opts, item.Blocks)

		// Re-download the current bytes: OOXML reconstruction splices the
		// translated runs into the live document by block ID rather than
		// regenerating the file from scratch, so the writer needs the
		// original archive as its base. This also picks up any structural
		// edits made since the fetch.
		original, err := c.download(ctx, driveBase, itemID)
		if err != nil {
			return fmt.Errorf("re-download %s: %w", item.Name, err)
		}
		out, err := c.renderItem(ctx, item, locale, original)
		if err != nil {
			return fmt.Errorf("render %s: %w", item.Name, err)
		}
		if err := c.upload(ctx, driveBase, itemID, item.Name, out); err != nil {
			return fmt.Errorf("upload %s: %w", item.Name, err)
		}
	}
	return nil
}

// renderItem regenerates the Office file bytes from the item's translated
// blocks, splicing the given target locale into the original archive.
//
// Faithful formats (openxml) reconstruct from a skeleton stream plus the
// original bytes rather than re-serializing a parse tree. The skeleton is a
// deterministic function of the original document, so the connector rebuilds it
// here by re-reading the original — keeping the connector stateless across the
// fetch→store→publish boundary while still producing a byte-faithful document.
func (c *Microsoft365Connector) renderItem(ctx context.Context, item *platconn.ContentItem, locale model.LocaleID, original []byte) ([]byte, error) {
	formatID := item.Format
	if formatID == "" {
		formatID = c.formatFor(item.Name)
	}
	writer, err := c.formats.NewWriter(registry.FormatID(formatID))
	if err != nil {
		return nil, fmt.Errorf("create writer %s: %w", formatID, err)
	}
	defer writer.Close()

	if locale != "" {
		writer.SetLocale(locale)
	}
	if oc, ok := writer.(originalContentSetter); ok {
		oc.SetOriginalContent(original)
	}
	if sc, ok := writer.(coreformat.SkeletonStoreConsumer); ok {
		store, serr := coreformat.NewSkeletonStore()
		if serr != nil {
			return nil, fmt.Errorf("skeleton store: %w", serr)
		}
		defer store.Close()
		if err := c.populateSkeleton(ctx, formatID, original, store); err != nil {
			return nil, fmt.Errorf("rebuild skeleton: %w", err)
		}
		sc.SetSkeletonStore(store)
	}

	var buf bytes.Buffer
	if err := writer.SetOutputWriter(&buf); err != nil {
		return nil, fmt.Errorf("set output writer: %w", err)
	}

	ch := make(chan *model.Part, len(item.Blocks))
	for _, b := range item.Blocks {
		ch <- &model.Part{Type: model.PartBlock, Resource: b}
	}
	close(ch)
	if err := writer.Write(ctx, ch); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// populateSkeleton re-reads the original document with a skeleton store wired,
// reproducing the byte-exact reconstruction stream the faithful writer needs.
// Block IDs are deterministic across reads, so the translated blocks supplied to
// the writer line up with the skeleton's block references.
func (c *Microsoft365Connector) populateSkeleton(ctx context.Context, formatID string, original []byte, store *coreformat.SkeletonStore) error {
	reader, err := c.formats.NewReader(registry.FormatID(formatID))
	if err != nil {
		return err
	}
	defer reader.Close()

	em, ok := reader.(coreformat.SkeletonStoreEmitter)
	if !ok {
		return nil // format does not use a skeleton stream
	}
	em.SetSkeletonStore(store)

	doc := &model.RawDocument{URI: "original", FormatID: formatID, Reader: io.NopCloser(bytes.NewReader(original))}
	if err := reader.Open(ctx, doc); err != nil {
		return err
	}
	for pr := range reader.Read(ctx) {
		if pr.Error != nil {
			return pr.Error
		}
	}
	return nil
}

func (c *Microsoft365Connector) Status(ctx context.Context) (*platconn.SyncStatus, error) {
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
// Graph content transfer
// ---------------------------------------------------------------------------

func (c *Microsoft365Connector) download(ctx context.Context, driveBase, itemID string) ([]byte, error) {
	u := fmt.Sprintf("%s/items/%s/content", driveBase, url.PathEscape(itemID))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("graph download: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	return io.ReadAll(resp.Body)
}

func (c *Microsoft365Connector) upload(ctx context.Context, driveBase, itemID, name string, data []byte) error {
	u := fmt.Sprintf("%s/items/%s/content", driveBase, url.PathEscape(itemID))
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, u, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("graph upload: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	return nil
}

func (c *Microsoft365Connector) getJSON(ctx context.Context, u string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("graph API: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// formatFor returns the registered format name for a filename, or "" if none.
func (c *Microsoft365Connector) formatFor(name string) string {
	fid, err := c.formats.Detector().DetectByExtension(path.Ext(name))
	if err != nil {
		return ""
	}
	return fid
}
