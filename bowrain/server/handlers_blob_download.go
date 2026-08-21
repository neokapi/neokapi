package server

import (
	"io"
	"net/http"

	"github.com/labstack/echo/v4"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
)

// A blob store that cannot presign still has to deliver its bytes.
//
// `generateDownloadURL` returns "" when the store cannot mint a pre-signed GET,
// and every caller treated that empty string as "no media here". The local
// filesystem backend cannot presign — and it is the DEFAULT backend
// (`if backend == "" { backend = "local" }`), so on every deployment that had
// not configured S3, every asset variant resolved to no URL, `pull` skipped it,
// and the translated file was written without its media and reported success.
//
// The two transports a chunk upload already distinguishes apply here in
// reverse: a store that can presign hands out a URL to object storage, and one
// that cannot serves the bytes through this route. What a caller gets is a URL
// either way, which is the only thing it was ever entitled to assume.

// HandleBlobDownload streams a blob by its storage key.
//
// GET /sync/:ref/blobs/:key
//
// It is project-scoped and permission-checked like every other content route:
// the key names content inside a project the caller can already read, so this
// grants nothing a pull did not. A key that is not a content hash is refused
// for the same reason the upload grant refuses one — the key IS the scope.
func (s *Server) HandleBlobDownload(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermViewContent); err != nil {
		return err
	}
	if s.BlobStore == nil {
		return apiErr(c, http.StatusServiceUnavailable, "blob store not configured")
	}

	key := c.Param("key")
	if !isContentHash(key) {
		return apiErr(c, http.StatusBadRequest, "key is not a sha256 digest")
	}

	rc, err := s.BlobStore.Download(c.Request().Context(), key)
	if err != nil {
		return apiErr(c, http.StatusNotFound, "blob not found")
	}
	defer rc.Close()

	c.Response().Header().Set("Content-Type", "application/octet-stream")
	c.Response().WriteHeader(http.StatusOK)
	_, copyErr := io.Copy(c.Response(), rc)
	return copyErr
}
