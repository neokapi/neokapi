package server

import (
	"context"
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"

	corestorage "github.com/neokapi/neokapi/core/storage"
	"github.com/neokapi/neokapi/core/venue"
)

// ---------------------------------------------------------------------------
// Request / Response types
// ---------------------------------------------------------------------------

type AssetResponse struct {
	ID               string            `json:"id"`
	ProjectID        string            `json:"project_id"`
	ItemName         string            `json:"item_name"`
	SourceID         string            `json:"source_id"`
	BlobKey          string            `json:"blob_key"`
	MimeType         string            `json:"mime_type"`
	Filename         string            `json:"filename"`
	SizeBytes        int64             `json:"size_bytes"`
	AltText          string            `json:"alt_text"`
	Properties       map[string]string `json:"properties,omitempty"`
	ProcessingStatus string            `json:"processing_status"`
	ProcessingHint   string            `json:"processing_hint,omitempty"`
	DownloadURL      string            `json:"download_url,omitempty"`
	CreatedAt        string            `json:"created_at"`
	UpdatedAt        string            `json:"updated_at"`
}

type CreateAssetRequest struct {
	BlobKey        string            `json:"blob_key"`
	ItemName       string            `json:"item_name"`
	SourceID       string            `json:"source_id"`
	MimeType       string            `json:"mime_type"`
	Filename       string            `json:"filename"`
	SizeBytes      int64             `json:"size_bytes"`
	AltText        string            `json:"alt_text"`
	Properties     map[string]string `json:"properties"`
	ProcessingHint string            `json:"processing_hint"`
}

type UploadURLRequest struct {
	BlobKey     string `json:"blob_key"`
	ContentType string `json:"content_type"`
	Size        int64  `json:"size"`
}

type UploadURLResponse struct {
	UploadURL string `json:"upload_url,omitempty"`
	Exists    bool   `json:"exists"`
}

type AssetVariantResponse struct {
	AssetID     string            `json:"asset_id"`
	Locale      string            `json:"locale"`
	BlobKey     string            `json:"blob_key"`
	Status      string            `json:"status"`
	MimeType    string            `json:"mime_type"`
	SizeBytes   int64             `json:"size_bytes"`
	Properties  map[string]string `json:"properties,omitempty"`
	DownloadURL string            `json:"download_url,omitempty"`
	CreatedAt   string            `json:"created_at"`
	UpdatedAt   string            `json:"updated_at"`
}

type CreateVariantRequest struct {
	Locale     string            `json:"locale"`
	BlobKey    string            `json:"blob_key"`
	Status     string            `json:"status"`
	MimeType   string            `json:"mime_type"`
	SizeBytes  int64             `json:"size_bytes"`
	Properties map[string]string `json:"properties"`
}

type VariantUploadURLRequest struct {
	Locale      string `json:"locale"`
	BlobKey     string `json:"blob_key"`
	ContentType string `json:"content_type"`
}

// ---------------------------------------------------------------------------
// Conversion helpers
// ---------------------------------------------------------------------------

func assetToResponse(a *venue.Asset) AssetResponse {
	return AssetResponse{
		ID:               a.ID,
		ProjectID:        a.ProjectID,
		ItemName:         a.ItemName,
		SourceID:         a.SourceID,
		BlobKey:          a.BlobKey,
		MimeType:         a.MimeType,
		Filename:         a.Filename,
		SizeBytes:        a.SizeBytes,
		AltText:          a.AltText,
		Properties:       a.Properties,
		ProcessingStatus: a.ProcessingStatus,
		ProcessingHint:   a.ProcessingHint,
		CreatedAt:        a.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:        a.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}
}

func variantToResponse(v *venue.AssetVariant) AssetVariantResponse {
	return AssetVariantResponse{
		AssetID:    v.AssetID,
		Locale:     v.Locale,
		BlobKey:    v.BlobKey,
		Status:     v.Status,
		MimeType:   v.MimeType,
		SizeBytes:  v.SizeBytes,
		Properties: v.Properties,
		CreatedAt:  v.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:  v.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}
}

// generateDownloadURL attempts to generate a pre-signed download URL. Returns
// empty string if BlobStore is nil or doesn't support pre-signed URLs.
//
// The store call is bound to the caller's context, so a client that disconnects
// mid-request aborts it rather than leaving it blocked on a detached one.
func (s *Server) generateDownloadURL(ctx context.Context, blobKey string) string {
	if s.BlobStore == nil {
		return ""
	}
	url, err := s.BlobStore.GenerateDownloadURL(ctx, blobKey, corestorage.SignOptions{})
	if err != nil {
		return ""
	}
	return url
}

// downloadURLFor is where a caller should fetch this blob from — and unlike
// generateDownloadURL it always answers.
//
// A store that can pre-sign answers with a URL straight to object storage. One
// that cannot — the local filesystem backend, which is the DEFAULT backend —
// answers with this server's own streaming route.
//
// The distinction matters because the empty string used to be the answer for
// the second case, and every caller read it as "no media here". So on any
// deployment that had not configured S3, every asset variant resolved to
// nothing, the pull skipped it, and the translated file was written without its
// media and reported as a success.
func (s *Server) downloadURLFor(c echo.Context, blobKey string) string {
	if s.BlobStore == nil || blobKey == "" {
		return ""
	}
	if url := s.generateDownloadURL(c.Request().Context(), blobKey); url != "" {
		return url
	}
	return blobDownloadPath(c, blobKey)
}

// blobDownloadPath is this server's streaming route for one blob, on the same
// project and stream the request arrived on — so the route the caller follows
// is authorized exactly as the one that handed it over.
func blobDownloadPath(c echo.Context, blobKey string) string {
	projectID := c.Param("id")
	if projectID == "" || blobKey == "" {
		return ""
	}
	ws := c.Param("ws")
	if ws == "" {
		return "/api/v1/projects/" + projectID + "/sync/" + refParam(c) + "/blobs/" + blobKey
	}
	return "/api/v1/" + ws + "/" + projectID + "/sync/" + refParam(c) + "/blobs/" + blobKey
}

// ---------------------------------------------------------------------------
// Asset CRUD handlers
// ---------------------------------------------------------------------------

// HandleAssetUploadURL returns a pre-signed upload URL for direct client upload.
func (s *Server) HandleAssetUploadURL(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageAssets); err != nil {
		return err
	}

	if s.ContentStore == nil || s.BlobStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "blob storage not configured"})
	}

	var req UploadURLRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}
	if req.BlobKey == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "blob_key is required"})
	}

	// Check if blob already exists (dedup).
	exists, err := s.BlobStore.Exists(c.Request().Context(), req.BlobKey)
	if err != nil {
		return serverErr(c, err)
	}
	if exists {
		return c.JSON(http.StatusOK, UploadURLResponse{Exists: true})
	}

	// Generate upload URL.
	url, err := s.BlobStore.GenerateUploadURL(c.Request().Context(), req.BlobKey, corestorage.SignOptions{})
	if err != nil {
		if errors.Is(err, corestorage.ErrNotSupported) {
			// Local backend: client should use direct upload through server proxy.
			return c.JSON(http.StatusOK, UploadURLResponse{Exists: false})
		}
		return serverErr(c, err)
	}

	return c.JSON(http.StatusOK, UploadURLResponse{UploadURL: url, Exists: false})
}

// HandleCreateAsset registers asset metadata after the blob has been uploaded.
func (s *Server) HandleCreateAsset(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageAssets); err != nil {
		return err
	}

	if s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "store not configured"})
	}

	pid := c.Param("id")
	stream := streamParam(c)
	var req CreateAssetRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}
	if req.BlobKey == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "blob_key is required"})
	}
	if req.MimeType == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "mime_type is required"})
	}

	asset := &venue.Asset{
		ItemName:       req.ItemName,
		SourceID:       req.SourceID,
		BlobKey:        req.BlobKey,
		MimeType:       req.MimeType,
		Filename:       req.Filename,
		SizeBytes:      req.SizeBytes,
		AltText:        req.AltText,
		Properties:     req.Properties,
		ProcessingHint: req.ProcessingHint,
	}

	if err := s.ContentStore.StoreAsset(c.Request().Context(), pid, stream, asset); err != nil {
		return serverErr(c, err)
	}

	resp := assetToResponse(asset)
	resp.DownloadURL = s.downloadURLFor(c, asset.BlobKey)
	return c.JSON(http.StatusCreated, resp)
}

// HandleListAssets lists assets for a project, optionally filtered by item_name.
func (s *Server) HandleListAssets(c echo.Context) error {
	if s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "store not configured"})
	}

	pid := c.Param("id")
	stream := streamParam(c)
	itemName := c.QueryParam("item_name")

	assets, err := s.ContentStore.ListAssets(c.Request().Context(), pid, stream, itemName)
	if err != nil {
		return serverErr(c, err)
	}

	result := make([]AssetResponse, len(assets))
	for i, a := range assets {
		result[i] = assetToResponse(a)
		result[i].DownloadURL = s.downloadURLFor(c, a.BlobKey)
	}

	return c.JSON(http.StatusOK, map[string]any{"assets": result})
}

// HandleGetAsset returns a single asset with a download URL.
func (s *Server) HandleGetAsset(c echo.Context) error {
	if s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "store not configured"})
	}

	pid := c.Param("id")
	aid := c.Param("aid")
	stream := streamParam(c)

	asset, err := s.ContentStore.GetAsset(c.Request().Context(), pid, stream, aid)
	if err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}

	resp := assetToResponse(asset)
	resp.DownloadURL = s.downloadURLFor(c, asset.BlobKey)
	return c.JSON(http.StatusOK, resp)
}

// HandleDeleteAsset deletes an asset and its variants.
func (s *Server) HandleDeleteAsset(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageAssets); err != nil {
		return err
	}

	if s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "store not configured"})
	}

	pid := c.Param("id")
	aid := c.Param("aid")
	stream := streamParam(c)

	if err := s.ContentStore.DeleteAsset(c.Request().Context(), pid, stream, aid); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}

	return c.NoContent(http.StatusNoContent)
}

// ---------------------------------------------------------------------------
// Locale variant handlers
// ---------------------------------------------------------------------------

// HandleVariantUploadURL returns a pre-signed upload URL for a locale variant.
func (s *Server) HandleVariantUploadURL(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageAssets); err != nil {
		return err
	}

	if s.BlobStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "blob storage not configured"})
	}

	var req VariantUploadURLRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}
	if req.BlobKey == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "blob_key is required"})
	}

	url, err := s.BlobStore.GenerateUploadURL(c.Request().Context(), req.BlobKey, corestorage.SignOptions{})
	if err != nil {
		if errors.Is(err, corestorage.ErrNotSupported) {
			return c.JSON(http.StatusOK, UploadURLResponse{Exists: false})
		}
		return serverErr(c, err)
	}

	return c.JSON(http.StatusOK, UploadURLResponse{UploadURL: url})
}

// HandleCreateVariant registers a locale variant for an asset.
func (s *Server) HandleCreateVariant(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageAssets); err != nil {
		return err
	}

	if s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "store not configured"})
	}

	pid := c.Param("id")
	aid := c.Param("aid")
	var req CreateVariantRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}
	if req.Locale == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "locale is required"})
	}
	if req.BlobKey == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "blob_key is required"})
	}

	variant := &venue.AssetVariant{
		AssetID:    aid,
		Locale:     req.Locale,
		BlobKey:    req.BlobKey,
		Status:     req.Status,
		MimeType:   req.MimeType,
		SizeBytes:  req.SizeBytes,
		Properties: req.Properties,
	}

	if err := s.ContentStore.StoreAssetVariant(c.Request().Context(), pid, variant); err != nil {
		return serverErr(c, err)
	}

	resp := variantToResponse(variant)
	resp.DownloadURL = s.downloadURLFor(c, variant.BlobKey)
	return c.JSON(http.StatusCreated, resp)
}

// HandleListVariants lists all locale variants for an asset.
func (s *Server) HandleListVariants(c echo.Context) error {
	if s.ContentStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "store not configured"})
	}

	pid := c.Param("id")
	aid := c.Param("aid")

	variants, err := s.ContentStore.ListAssetVariants(c.Request().Context(), pid, aid)
	if err != nil {
		return serverErr(c, err)
	}

	result := make([]AssetVariantResponse, len(variants))
	for i, v := range variants {
		result[i] = variantToResponse(v)
		result[i].DownloadURL = s.downloadURLFor(c, v.BlobKey)
	}

	return c.JSON(http.StatusOK, map[string]any{"variants": result})
}
