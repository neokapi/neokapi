package aiprovider

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/neokapi/neokapi/core/model"
)

// Modality is the set of non-text inputs a provider can accept. Text is always
// accepted, so it is not a Modality value — InputModalities advertises only the
// media a backend understands.
type Modality string

const (
	ModalityImage Modality = "image"
	ModalityAudio Modality = "audio"
	ModalityVideo Modality = "video"
)

// ContentKind discriminates a message content part. It is a closed set, named
// with constants like ProviderID / model.PartType, not a bare string.
type ContentKind string

const (
	ContentText  ContentKind = "text"
	ContentImage ContentKind = "image"
	ContentAudio ContentKind = "audio"
	ContentVideo ContentKind = "video"
)

// Modality returns the non-text Modality a ContentKind maps to; ok is false for
// ContentText, which every provider accepts.
func (k ContentKind) Modality() (Modality, bool) {
	switch k {
	case ContentImage:
		return ModalityImage, true
	case ContentAudio:
		return ModalityAudio, true
	case ContentVideo:
		return ModalityVideo, true
	default:
		return "", false
	}
}

// ContentPart is one part of a multimodal message: either text, or a media slice
// carried by reference. The media payload is a *model.Media (precedence
// BlobKey > URI > Data), never a bare []byte, so a large slice is never forced
// into memory by the framework; resolveMediaBytes materializes it only here, at
// the provider boundary.
type ContentPart struct {
	Kind  ContentKind
	Text  string       // Kind == ContentText
	Media *model.Media // otherwise — image/audio/video slice
}

// TextPart builds a text content part.
func TextPart(text string) ContentPart {
	return ContentPart{Kind: ContentText, Text: text}
}

// MediaPart builds a media content part of the given kind.
func MediaPart(kind ContentKind, m *model.Media) ContentPart {
	return ContentPart{Kind: kind, Media: m}
}

// Modalities returns the distinct non-text modalities a content-part list uses,
// so a caller can check them against a provider's InputModalities before a call.
func Modalities(parts []ContentPart) []Modality {
	seen := map[Modality]bool{}
	var out []Modality
	for _, p := range parts {
		if mod, ok := p.Kind.Modality(); ok && !seen[mod] {
			seen[mod] = true
			out = append(out, mod)
		}
	}
	return out
}

// hasMedia reports whether any part is non-text.
func hasMedia(parts []ContentPart) bool {
	for _, p := range parts {
		if p.Kind != ContentText {
			return true
		}
	}
	return false
}

// resolveMediaBytes materializes a media slice to bytes and a MIME type for the
// provider wire (base64). It reads the inline Data, or a local-file URI; a
// store-backed slice (BlobKey only) or a remote URI must be materialized by the
// caller (the MediaSlicer produces inline-Data slices), so those return an error
// rather than coupling the provider to a blob store. resolveMediaURL covers the
// providers that can fetch a URL instead.
func resolveMediaBytes(m *model.Media) (data []byte, mimeType string, err error) {
	if m == nil {
		return nil, "", errors.New("aiprovider: nil media part")
	}
	mimeType = m.MimeType
	switch {
	case len(m.Data) > 0:
		return m.Data, mimeType, nil
	case isLocalPath(m.URI):
		b, rerr := os.ReadFile(m.URI)
		if rerr != nil {
			return nil, "", fmt.Errorf("aiprovider: read media %q: %w", m.URI, rerr)
		}
		return b, mimeType, nil
	case m.BlobKey != "":
		return nil, "", fmt.Errorf("aiprovider: media is blob-store-backed (%q); materialize it before the provider call", m.BlobKey)
	default:
		return nil, "", errors.New("aiprovider: media has no inline data or local URI to send")
	}
}

// resolveMediaDataURL returns a base64 data: URL for the slice, for providers
// whose wire form is a data URL (OpenAI image_url, Anthropic url source).
func resolveMediaDataURL(m *model.Media) (string, error) {
	data, mime, err := resolveMediaBytes(m)
	if err != nil {
		return "", err
	}
	if mime == "" {
		mime = "application/octet-stream"
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data), nil
}

// resolveMediaBase64 returns the slice as a base64 string plus its MIME type,
// for providers whose wire form is separate base64 + media-type fields
// (Anthropic base64 source, Gemini inlineData, Ollama images).
func resolveMediaBase64(m *model.Media) (b64, mimeType string, err error) {
	data, mime, err := resolveMediaBytes(m)
	if err != nil {
		return "", "", err
	}
	return base64.StdEncoding.EncodeToString(data), mime, nil
}

// isLocalPath reports whether s is a filesystem path (not an http(s)/data URL).
func isLocalPath(s string) bool {
	if s == "" {
		return false
	}
	if strings.HasPrefix(s, "/") || strings.HasPrefix(s, "./") || strings.HasPrefix(s, "../") {
		return true
	}
	u, err := url.Parse(s)
	if err != nil {
		return true // not a URL — treat as a path
	}
	return u.Scheme == "" || u.Scheme == "file"
}
