package eml_test

import (
	"bytes"
	"mime"
	"net/mail"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/format/spec"
	"github.com/neokapi/neokapi/core/formats/eml"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/require"
)

func TestEditedMessage(t *testing.T) {
	raw := fixture(t, "multipart")
	out, err := roundTrip(t, raw, func(b *model.Block) {
		if b.Name == "subject" {
			b.SetTargetText("fr", "Décisions")
		} else {
			b.SetTargetText("fr", "Café et thé")
		}
	}, "fr")
	require.NoError(t, err)
	expected := bytes.Replace(raw, []byte("caf=E9 and=\r\n tea"), []byte("Caf=E9 et th=E9"), 1)
	before := bytes.Index(expected, []byte("=?iso-8859-1?"))
	after := bytes.Index(expected[before:], []byte("MIME-Version:")) + before
	require.Equal(t, expected[:before], out[:before])
	require.True(t, bytes.HasSuffix(out, expected[after:]), "an attachment or an unrelated header changed")
	msg, err := mail.ReadMessage(bytes.NewReader(out))
	require.NoError(t, err)
	subject, err := new(mime.WordDecoder).DecodeHeader(msg.Header.Get("Subject"))
	require.NoError(t, err)
	require.Equal(t, "Décisions", subject)
}

func TestLegacyBodyUnicodeEdit(t *testing.T) {
	raw := fixture(t, "plain")
	want := "Oversettelse til norsk, 日本語.\nNeste linje."
	out, err := roundTrip(t, raw, func(b *model.Block) {
		if b.Name == "body" {
			b.SetSourceText(want)
		}
	}, "")
	require.NoError(t, err)
	msg, err := mail.ReadMessage(bytes.NewReader(out))
	require.NoError(t, err)
	require.Equal(t, "text/plain; charset=utf-8", msg.Header.Get("Content-Type"))
	require.Equal(t, "base64", msg.Header.Get("Content-Transfer-Encoding"))
	require.Equal(t, "author@example.org", msg.Header.Get("From"))
	parts, err := spec.ReadParts(eml.NewReader(), out)
	require.NoError(t, err)
	require.Equal(t, want, parts[2].Resource.(*model.Block).SourceText())
}

func TestMIMEBoundaryInAnEdit(t *testing.T) {
	raw := fixture(t, "multipart")
	want := "First\n--outer\n--inner--\nLast"
	out, err := roundTrip(t, raw, func(b *model.Block) {
		if b.Name != "subject" {
			b.SetSourceText(want)
		}
	}, "")
	require.NoError(t, err)
	require.Contains(t, string(out), "Content-Transfer-Encoding: base64")
	require.Equal(t, bytes.Count(raw, []byte("--outer")), bytes.Count(out, []byte("--outer")))
	parts, err := spec.ReadParts(eml.NewReader(), out)
	require.NoError(t, err)
	require.Equal(t, want, parts[2].Resource.(*model.Block).SourceText())
}

func TestUnsafeSubjectFailsBeforeWriting(t *testing.T) {
	for _, bad := range []string{"Hello\r\nBcc: private@example.org", "zero\x00byte"} {
		var out bytes.Buffer
		w := eml.NewWriter()
		w.SetOriginalContent(fixture(t, "plain"))
		require.NoError(t, w.SetOutputWriter(&out))
		b := model.NewBlock("subject", bad)
		ch := make(chan *model.Part, 1)
		ch <- &model.Part{Type: model.PartBlock, Resource: b}
		close(ch)
		require.ErrorContains(t, w.Write(t.Context(), ch), "control character")
		require.Empty(t, out.Bytes())
	}
}

func TestEncodedWordAndLongSubjectRemainLiteral(t *testing.T) {
	for _, value := range []string{"=?UTF-8?B?SGVsbG8=?=", strings.Repeat("訂正", 130), strings.Repeat("a", 1400)} {
		out, err := roundTrip(t, fixture(t, "plain"), func(b *model.Block) {
			if b.Name == "subject" {
				b.SetSourceText(value)
			}
		}, "")
		require.NoError(t, err)
		msg, err := mail.ReadMessage(bytes.NewReader(out))
		require.NoError(t, err)
		subject, err := new(mime.WordDecoder).DecodeHeader(msg.Header.Get("Subject"))
		require.NoError(t, err)
		require.Equal(t, value, subject)
		for _, line := range bytes.Split(out, []byte("\r\n")) {
			require.LessOrEqual(t, len(line), 998)
		}
	}
}

func TestHeaderOnlyMessage(t *testing.T) {
	raw := []byte("From: author@example.org\r\nSubject: Meeting\r\n")
	out, err := roundTrip(t, raw, func(b *model.Block) { b.SetSourceText("Agenda") }, "")
	require.NoError(t, err)
	require.Equal(t, bytes.Replace(raw, []byte("Meeting"), []byte("Agenda"), 1), out)
}
