package backend

import (
	"encoding/base64"
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"strings"
)

// mediaMimeByExt covers the media extensions Go's built-in mime table can miss
// (audio/video especially), so the data: URL carries a type the browser plays.
var mediaMimeByExt = map[string]string{
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".gif":  "image/gif",
	".webp": "image/webp",
	".bmp":  "image/bmp",
	".tif":  "image/tiff",
	".tiff": "image/tiff",
	".svg":  "image/svg+xml",
	".mp3":  "audio/mpeg",
	".wav":  "audio/wav",
	".m4a":  "audio/mp4",
	".aac":  "audio/aac",
	".flac": "audio/flac",
	".ogg":  "audio/ogg",
	".opus": "audio/opus",
	".mp4":  "video/mp4",
	".mov":  "video/quicktime",
	".m4v":  "video/mp4",
	".mkv":  "video/x-matroska",
	".webm": "video/webm",
}

// mediaMimeType resolves a media MIME type from a file extension, preferring the
// explicit media table and falling back to Go's mime registry.
func mediaMimeType(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	if mt, ok := mediaMimeByExt[ext]; ok {
		return mt
	}
	if mt := mime.TypeByExtension(ext); mt != "" {
		return mt
	}
	return "application/octet-stream"
}

// MediaDataURL reads a media file (image / audio / video) referenced by a content
// tree's media node and returns it as a base64 `data:` URL the frontend can put
// straight into an <img>/<audio>/<video> element. The desktop is a local working
// copy, so media is read directly from disk; `path` is the media node's URI as
// emitted by the image/audio/video readers (MediaView.uri in the ContentTree).
func (a *App) MediaDataURL(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("media: empty path")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("media: read %s: %w", filepath.Base(path), err)
	}
	return "data:" + mediaMimeType(path) + ";base64," + base64.StdEncoding.EncodeToString(data), nil
}
