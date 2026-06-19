// Package main (mkappcast) generates a Sparkle-style AppCast XML feed for the
// neokapi desktop apps, signed for the Wails v3 native updater
// (github.com/wailsapp/wails/v3/pkg/updater + providers/appcast).
//
// Why a custom generator instead of Sparkle's generate_appcast:
//
// The Wails appcast provider maps <enclosure sparkle:edSignature> into an
// ed25519 Verification and the updater verifies it with
//
//	ed25519.Verify(pub, sha256(artifact), signature)
//
// i.e. the signature must be over the artifact's SHA-256 *digest*. Sparkle's
// own sign_update/generate_appcast instead sign the raw file bytes, so their
// signatures do NOT verify under the Wails updater. This tool therefore signs
// the SHA-256 digest itself — producing an edSignature the Wails updater
// accepts — and emits the minimal appcast vocabulary the provider reads
// (shortVersionString, channel, enclosure url/length/os/edSignature).
package main

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Item is one published build in the feed.
type Item struct {
	// ShortVersion is the human/compare version, e.g. "1.2.0" (maps to
	// sparkle:shortVersionString — the value the Wails provider compares).
	ShortVersion string
	// Channel tags the item (e.g. "beta"); empty for the default channel.
	Channel string
	// URL is the public download URL of the artifact (a .zip of the .app).
	URL string
	// Length is the artifact size in bytes.
	Length int64
	// EdSignature is base64(ed25519_sign(priv, sha256(artifact))).
	EdSignature string
	// OS is the sparkle:os value ("macos").
	OS string
	// PubDate is RFC1123Z, as Sparkle expects.
	PubDate string
}

// signDigest signs the SHA-256 digest of artifact with priv and returns the
// base64 signature that goes in sparkle:edSignature — matching exactly what the
// Wails updater verifies (ed25519 over sha256(file)).
func signDigest(priv ed25519.PrivateKey, artifact []byte) string {
	sum := sha256.Sum256(artifact)
	sig := ed25519.Sign(priv, sum[:])
	return base64.StdEncoding.EncodeToString(sig)
}

// newItem builds a signed Item for an artifact on disk.
func newItem(priv ed25519.PrivateKey, shortVersion, channel, downloadURL, artifactPath, pubDate string) (Item, error) {
	data, err := os.ReadFile(artifactPath)
	if err != nil {
		return Item{}, fmt.Errorf("read artifact %s: %w", artifactPath, err)
	}
	return Item{
		ShortVersion: shortVersion,
		Channel:      channel,
		URL:          downloadURL,
		Length:       int64(len(data)),
		EdSignature:  signDigest(priv, data),
		OS:           "macos",
		PubDate:      pubDate,
	}, nil
}

// ---- appcast XML rendering ----
//
// The provider only needs a small, fixed subset of the Sparkle RSS vocabulary,
// so we render it directly rather than via struct marshalling (the sparkle:
// namespaced attributes are fiddly to express with encoding/xml structs).

const sparkleNS = "http://www.andymatuschak.org/xml-namespaces/sparkle"

// renderAppcast produces the appcast XML document for title + items.
func renderAppcast(title string, items []Item) string {
	var b []byte
	b = append(b, []byte(xml.Header)...)
	b = appendf(b, `<rss version="2.0" xmlns:sparkle="%s">`+"\n", sparkleNS)
	b = appendf(b, "  <channel>\n    <title>%s</title>\n", xmlEscape(title))
	for _, it := range items {
		b = append(b, "    <item>\n"...)
		b = appendf(b, "      <title>%s %s</title>\n", xmlEscape(title), xmlEscape(it.ShortVersion))
		if it.PubDate != "" {
			b = appendf(b, "      <pubDate>%s</pubDate>\n", xmlEscape(it.PubDate))
		}
		b = appendf(b, "      <sparkle:shortVersionString>%s</sparkle:shortVersionString>\n", xmlEscape(it.ShortVersion))
		if it.Channel != "" {
			b = appendf(b, "      <sparkle:channel>%s</sparkle:channel>\n", xmlEscape(it.Channel))
		}
		b = appendf(b,
			`      <enclosure url="%s" sparkle:os="%s" sparkle:edSignature="%s" length="%d" type="application/octet-stream"/>`+"\n",
			xmlEscape(it.URL), xmlEscape(it.OS), xmlEscape(it.EdSignature), it.Length)
		b = append(b, "    </item>\n"...)
	}
	b = append(b, "  </channel>\n</rss>\n"...)
	return string(b)
}

func appendf(b []byte, format string, args ...any) []byte {
	return append(b, []byte(fmt.Sprintf(format, args...))...)
}

func xmlEscape(s string) string {
	var b []byte
	buf := &sliceWriter{&b}
	_ = xml.EscapeText(buf, []byte(s))
	return string(b)
}

type sliceWriter struct{ b *[]byte }

func (w *sliceWriter) Write(p []byte) (int, error) { *w.b = append(*w.b, p...); return len(p), nil }

// nowRFC1123Z formats t the way Sparkle's pubDate expects.
func nowRFC1123Z(t time.Time) string { return t.Format(time.RFC1123Z) }

// downloadURL joins a prefix and a filename for the enclosure URL.
func downloadURL(prefix, artifactPath string) string {
	name := filepath.Base(artifactPath)
	if prefix == "" {
		return name
	}
	if prefix[len(prefix)-1] != '/' {
		prefix += "/"
	}
	return prefix + name
}
