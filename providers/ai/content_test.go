package aiprovider

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTextMessageAndText(t *testing.T) {
	m := TextMessage("user", "hello")
	require.Len(t, m.Parts, 1)
	assert.Equal(t, ContentText, m.Parts[0].Kind)
	assert.Equal(t, "hello", m.Text())

	// Media parts contribute nothing to Text().
	m2 := Message{Role: "user", Parts: []ContentPart{
		TextPart("describe: "),
		MediaPart(ContentImage, &model.Media{Data: []byte{1}, MimeType: "image/png"}),
		TextPart("(end)"),
	}}
	assert.Equal(t, "describe: (end)", m2.Text())
}

func TestContentKindModality(t *testing.T) {
	for _, tc := range []struct {
		kind ContentKind
		mod  Modality
		ok   bool
	}{
		{ContentText, "", false},
		{ContentImage, ModalityImage, true},
		{ContentAudio, ModalityAudio, true},
		{ContentVideo, ModalityVideo, true},
	} {
		mod, ok := tc.kind.Modality()
		assert.Equal(t, tc.ok, ok, tc.kind)
		assert.Equal(t, tc.mod, mod, tc.kind)
	}
}

func TestModalities(t *testing.T) {
	parts := []ContentPart{
		TextPart("a"),
		MediaPart(ContentImage, &model.Media{}),
		MediaPart(ContentImage, &model.Media{}), // dedup
		MediaPart(ContentAudio, &model.Media{}),
	}
	assert.Equal(t, []Modality{ModalityImage, ModalityAudio}, Modalities(parts))
	assert.Nil(t, Modalities([]ContentPart{TextPart("x")}))
}

func TestResolveMediaBytes(t *testing.T) {
	// inline Data
	b, mime, err := resolveMediaBytes(&model.Media{Data: []byte("PNGDATA"), MimeType: "image/png"})
	require.NoError(t, err)
	assert.Equal(t, []byte("PNGDATA"), b)
	assert.Equal(t, "image/png", mime)

	// local file URI
	dir := t.TempDir()
	p := filepath.Join(dir, "x.png")
	require.NoError(t, os.WriteFile(p, []byte("FILEBYTES"), 0o600))
	b, _, err = resolveMediaBytes(&model.Media{URI: p, MimeType: "image/png"})
	require.NoError(t, err)
	assert.Equal(t, []byte("FILEBYTES"), b)

	// blob-store-backed → caller must materialize
	_, _, err = resolveMediaBytes(&model.Media{BlobKey: "sha256:abc", MimeType: "image/png"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "materialize")

	// nothing to send
	_, _, err = resolveMediaBytes(&model.Media{MimeType: "image/png"})
	require.Error(t, err)

	// nil
	_, _, err = resolveMediaBytes(nil)
	require.Error(t, err)
}

func TestResolveMediaDataURLAndBase64(t *testing.T) {
	m := &model.Media{Data: []byte("abc"), MimeType: "image/png"}
	want := base64.StdEncoding.EncodeToString([]byte("abc"))

	durl, err := resolveMediaDataURL(m)
	require.NoError(t, err)
	assert.Equal(t, "data:image/png;base64,"+want, durl)

	b64, mime, err := resolveMediaBase64(m)
	require.NoError(t, err)
	assert.Equal(t, want, b64)
	assert.Equal(t, "image/png", mime)

	// empty mime falls back on data URL
	durl, err = resolveMediaDataURL(&model.Media{Data: []byte("abc")})
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(durl, "data:application/octet-stream;base64,"))
}

func imageMessage() []Message {
	return []Message{{Role: "user", Parts: []ContentPart{
		TextPart("what is this?"),
		MediaPart(ContentImage, &model.Media{Data: []byte("IMG"), MimeType: "image/png"}),
	}}}
}

func TestToAnthropicMessages_Image(t *testing.T) {
	msgs, err := toAnthropicMessages(imageMessage())
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	require.Len(t, msgs[0].Content, 2)
	assert.Equal(t, "text", msgs[0].Content[0].Type)
	assert.Equal(t, "image", msgs[0].Content[1].Type)
	require.NotNil(t, msgs[0].Content[1].Source)
	assert.Equal(t, "base64", msgs[0].Content[1].Source.Type)
	assert.Equal(t, "image/png", msgs[0].Content[1].Source.MediaType)
	assert.Equal(t, base64.StdEncoding.EncodeToString([]byte("IMG")), msgs[0].Content[1].Source.Data)
}

func TestToOpenAIMessages_TextAndImage(t *testing.T) {
	// text-only → string content
	textOnly, err := toOpenAIMessages([]Message{TextMessage("user", "hi")})
	require.NoError(t, err)
	assert.Equal(t, "hi", textOnly[0].Content)

	// image → array of parts
	msgs, err := toOpenAIMessages(imageMessage())
	require.NoError(t, err)
	parts, ok := msgs[0].Content.([]openaiContentPart)
	require.True(t, ok)
	require.Len(t, parts, 2)
	assert.Equal(t, "text", parts[0].Type)
	assert.Equal(t, "image_url", parts[1].Type)
	require.NotNil(t, parts[1].ImageURL)
	assert.True(t, strings.HasPrefix(parts[1].ImageURL.URL, "data:image/png;base64,"))
}

func TestToOllamaMessages_Image(t *testing.T) {
	msgs, err := toOllamaMessages(imageMessage())
	require.NoError(t, err)
	assert.Equal(t, "what is this?", msgs[0].Content)
	require.Len(t, msgs[0].Images, 1)
	assert.Equal(t, base64.StdEncoding.EncodeToString([]byte("IMG")), msgs[0].Images[0])
}

func TestMessagesToGeminiContents_Image(t *testing.T) {
	contents, err := messagesToGeminiContents(imageMessage())
	require.NoError(t, err)
	require.Len(t, contents, 1)
	require.Len(t, contents[0].Parts, 2)
	assert.Equal(t, "what is this?", contents[0].Parts[0].Text)
	require.NotNil(t, contents[0].Parts[1].InlineData)
	assert.Equal(t, "image/png", contents[0].Parts[1].InlineData.MimeType)
	assert.Equal(t, base64.StdEncoding.EncodeToString([]byte("IMG")), contents[0].Parts[1].InlineData.Data)
}

func TestProviderInputModalities(t *testing.T) {
	assert.Equal(t, []Modality{ModalityImage}, NewAnthropicProvider(Config{Model: "m"}).InputModalities())
	assert.Equal(t, []Modality{ModalityImage}, NewOpenAIProvider(Config{Model: "m"}).InputModalities())
	assert.Equal(t, []Modality{ModalityImage}, NewOllamaProvider(Config{Model: "m"}).InputModalities())
	assert.Contains(t, NewGeminiProvider(Config{Model: "m"}).InputModalities(), ModalityVideo)
	assert.Nil(t, NewDemoProvider(Config{}).InputModalities())
}

// A provider given a modality it cannot encode returns a clear error rather than
// silently dropping the media.
func TestUnsupportedModalityError(t *testing.T) {
	audio := []Message{{Role: "user", Parts: []ContentPart{
		MediaPart(ContentAudio, &model.Media{Data: []byte("WAV"), MimeType: "audio/wav"}),
	}}}
	_, err := toAnthropicMessages(audio)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported content kind")
}
