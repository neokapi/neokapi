package server

import (
	"context"
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"

	corestorage "github.com/neokapi/neokapi/core/storage"
	"github.com/neokapi/neokapi/bowrain/core/store"
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

func assetToResponse(a *store.Asset) AssetResponse {
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

func variantToResponse(v *store.AssetVariant) AssetVariantResponse {
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

// generateDownloadURL attempts to generate a SAS/pre-signed download URL.
// Returns empty string if BlobStore is nil or doesn't support pre-signed URLs.
func (s *Server) generateDownloadURL(blobKey string) string {
	if s.BlobStore == nil {
		return ""
	}
	url, err := s.BlobStore.GenerateDownloadURL(context.Background(), blobKey, corestorage.SignOptions{})
	if err != nil {
		return ""
	}
	return url
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
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
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
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
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

	asset := &store.Asset{
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
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	resp := assetToResponse(asset)
	resp.DownloadURL = s.generateDownloadURL(asset.BlobKey)
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
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	result := make([]AssetResponse, len(assets))
	for i, a := range assets {
		result[i] = assetToResponse(a)
		result[i].DownloadURL = s.generateDownloadURL(a.BlobKey)
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
	resp.DownloadURL = s.generateDownloadURL(asset.BlobKey)
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
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
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

	variant := &store.AssetVariant{
		AssetID:    aid,
		Locale:     req.Locale,
		BlobKey:    req.BlobKey,
		Status:     req.Status,
		MimeType:   req.MimeType,
		SizeBytes:  req.SizeBytes,
		Properties: req.Properties,
	}

	if err := s.ContentStore.StoreAssetVariant(c.Request().Context(), pid, variant); err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	resp := variantToResponse(variant)
	resp.DownloadURL = s.generateDownloadURL(variant.BlobKey)
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
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	result := make([]AssetVariantResponse, len(variants))
	for i, v := range variants {
		result[i] = variantToResponse(v)
		result[i].DownloadURL = s.generateDownloadURL(v.BlobKey)
	}

	return c.JSON(http.StatusOK, map[string]any{"variants": result})
}
