package connector

import (
	"bytes"
	"io"
	"mime"
	"net/mail"
	"os"
	"path/filepath"
	"testing"

	platform "github.com/neokapi/neokapi/bowrain/core/connector"
	"github.com/stretchr/testify/require"
)

func TestEmailDelivery(t *testing.T) {
	dir := t.TempDir()
	const body = "Content from the message.\r\n"
	const original = "From: editor@example.org\r\nSubject: Hello\r\nX-Reference: 123\r\n\r\n" + body
	require.NoError(t, os.WriteFile(filepath.Join(dir, "source.eml"), []byte(original), 0o600))
	c := setupFileConnector(t, dir)
	items, err := c.Fetch(t.Context(), platform.FetchOptions{})
	require.NoError(t, err)
	require.Len(t, items, 1)
	item := items[0]
	require.Equal(t, "eml", item.Format)
	require.Len(t, item.Blocks, 2)
	for i, b := range item.Blocks {
		if b.SourceText() == "Hello" {
			item.Blocks[i] = deliveryCopy(b, "Bonjour")
		}
	}
	item.Metadata = map[string]string{"source_path": item.Path}
	item.Path = "target.eml"
	item.Locale = "fr"
	require.NoError(t, c.Publish(t.Context(), items, platform.PublishOptions{}))

	data, err := os.ReadFile(filepath.Join(dir, item.Path))
	require.NoError(t, err)
	msg, err := mail.ReadMessage(bytes.NewReader(data))
	require.NoError(t, err)
	subject, err := new(mime.WordDecoder).DecodeHeader(msg.Header.Get("Subject"))
	require.NoError(t, err)
	require.Equal(t, "Bonjour", subject)
	require.Equal(t, "123", msg.Header.Get("X-Reference"))
	require.Equal(t, "editor@example.org", msg.Header.Get("From"))
	got, err := io.ReadAll(msg.Body)
	require.NoError(t, err)
	require.Equal(t, body, string(got))
}
