package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"

	"github.com/labstack/echo/v4"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	corestorage "github.com/neokapi/neokapi/core/storage"
)

// Content goes to object storage, not through the API.
//
// Every byte a push sent used to transit this server: the client PUT each
// chunk to a request handler, the handler wrote it to the blob store, and the
// worker downloaded it again by hash. Object storage was already the
// destination — the API was a hop in the middle of it, and it cost a 2 MiB
// request cap that shaped the chunking, an interactive server's throughput and
// memory spent on bulk transfer, and uploads that could not run in parallel
// because they contended with the API's own concurrency.
//
// Presigned PUTs need the key decided before the upload. Content addressing
// already gives that: a chunk is named by the SHA-256 of the exact bytes being
// sent, so the grant is a write to one key whose name IS the content. A client
// that PUTs something else has not corrupted anything — it has written an
// object whose name does not match its content, and the commit, which verifies
// every chunk by hash, refuses it.

// HandleSyncPushUploads grants presigned PUTs for a set of content hashes, and
// says which of them the venue already holds.
//
// POST /sync/:ref/push/uploads  {"hashes": ["<sha256>", ...]}
//
//	→ {"urls": {"<sha256>": "<url>"}, "have": ["<sha256>"], "transport": "direct"}
//
// A hash already in storage gets no URL: the object is content-addressed, so
// holding it is holding exactly those bytes, and re-uploading them would move
// the same content twice. A push that re-sends an unchanged chunk after a
// failed commit pays nothing for it.
func (s *Server) HandleSyncPushUploads(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageFiles); err != nil {
		return err
	}
	if s.BlobStore == nil {
		return apiErr(c, http.StatusServiceUnavailable, "blob store not configured")
	}

	var req struct {
		Hashes []string `json:"hashes"`
	}
	if err := c.Bind(&req); err != nil {
		return apiErr(c, http.StatusBadRequest, err.Error())
	}
	if len(req.Hashes) > maxUploadURLsPerRequest {
		return apiErr(c, http.StatusBadRequest, "too many hashes in one request")
	}

	ctx := c.Request().Context()
	urls := map[string]string{}
	have := []string{}
	for _, hash := range req.Hashes {
		if !isContentHash(hash) {
			return apiErr(c, http.StatusBadRequest, "hash is not a sha256 digest: "+hash)
		}
		if exists, err := s.BlobStore.Exists(ctx, hash); err == nil && exists {
			have = append(have, hash)
			continue
		}
		url, err := s.BlobStore.GenerateUploadURL(ctx, hash, corestorage.SignOptions{})
		if err != nil {
			// A store with no presigning — the local filesystem backend of a
			// self-hosted deployment — is not an error. The client is told the
			// transport it actually has and proxies, which is what Transport
			// exists to distinguish.
			return c.JSON(http.StatusOK, map[string]any{
				"transport": transportProxy,
				"urls":      map[string]string{},
				"have":      have,
			})
		}
		urls[hash] = url
	}

	return c.JSON(http.StatusOK, map[string]any{
		"transport": transportDirect,
		"urls":      urls,
		"have":      have,
	})
}

// maxUploadURLsPerRequest bounds one grant request. The client asks in windows
// so its own memory stays bounded; this keeps the server's answer bounded for
// the same reason, and keeps a presigning burst from one caller off the path of
// everyone else's interactive traffic.
const maxUploadURLsPerRequest = 256

// isContentHash reports whether a string is a lowercase hex SHA-256 digest.
//
// It is a guard on what a presigned URL can be minted FOR. The grant is scoped
// to one key, so the shape of the key is the scope: a caller that could name an
// arbitrary key would be handed a write to it.
func isContentHash(s string) bool {
	if len(s) != 64 {
		return false
	}
	for i := range len(s) {
		c := s[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// chunkTransport is how this deployment takes a push's chunks.
//
// It is answered by asking the blob store for a grant it will throw away
// rather than by type-switching on the backend. A store that can presign is one
// that answers; a directory cannot, and says so. The alternative — asserting
// for an interface — was how the previous generation of this decided, and it
// was wrong in exactly the case that mattered: s3blob deliberately does NOT
// implement ChunkedBlobStore, so the only path that could have gone direct was
// gated on a capability the production store does not claim.
func (s *Server) chunkTransport(ctx context.Context) string {
	if s.BlobStore == nil {
		return transportProxy
	}
	// A digest of nothing: a well-formed key that names no object, so a store
	// that presigns it grants a write nobody will ever perform.
	probe := sha256.Sum256(nil)
	if _, err := s.BlobStore.GenerateUploadURL(ctx, hex.EncodeToString(probe[:]), corestorage.SignOptions{}); err != nil {
		return transportProxy
	}
	return transportDirect
}

const (
	transportDirect = "direct"
	transportProxy  = "proxy"
)
