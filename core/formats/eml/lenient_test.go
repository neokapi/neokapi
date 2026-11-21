package eml_test

import (
	"bytes"
	"mime"
	"net/mail"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/format/spec"
	"github.com/neokapi/neokapi/core/formats/eml"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/require"
)

func lenientFixture(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "lenient", name+".eml"))
	require.NoError(t, err)
	return raw
}

// blockTexts reads raw and returns each block's name and source text.
func blockTexts(t *testing.T, raw []byte) map[string]string {
	t.Helper()
	parts, err := spec.ReadParts(eml.NewReader(), raw)
	require.NoError(t, err)
	texts := map[string]string{}
	for _, p := range parts {
		if b, ok := p.Resource.(*model.Block); ok {
			texts[b.Name] = b.SourceText()
		}
	}
	return texts
}

// categories reads raw in validation mode and returns the diagnostic categories.
func categories(t *testing.T, raw []byte, mode format.ValidationMode) []string {
	t.Helper()
	r := eml.NewReader()
	r.Cfg.(*eml.Config).SetValidationMode(mode)
	_, err := spec.ReadParts(r, raw)
	require.NoError(t, err)
	var got []string
	for _, d := range r.Diagnostics() {
		require.Positive(t, d.Line)
		got = append(got, d.Category)
	}
	return got
}

func TestSubjectCharsets(t *testing.T) {
	for name, want := range map[string]string{
		"subject-cp1252-q":    "Café €",
		"subject-cp1252-b":    "Café €",
		"subject-iso2022jp-b": "テスト",
		"subject-iso2022jp-q": "テスト",
	} {
		t.Run(name, func(t *testing.T) {
			raw := lenientFixture(t, name)
			require.Equal(t, map[string]string{"subject": want, "body": "Hello"}, blockTexts(t, raw))
			out, err := roundTrip(t, raw, nil, "")
			require.NoError(t, err)
			require.Equal(t, raw, out)
			out, err = roundTrip(t, raw, func(b *model.Block) {
				if b.Name == "subject" {
					b.SetTargetText("fr", "Thé")
				}
			}, "fr")
			require.NoError(t, err)
			msg, err := mail.ReadMessage(bytes.NewReader(out))
			require.NoError(t, err)
			subject, err := new(mime.WordDecoder).DecodeHeader(msg.Header.Get("Subject"))
			require.NoError(t, err)
			require.Equal(t, "Thé", subject)
		})
	}
}

func TestUndecodableSubjectStaysInTheSkeleton(t *testing.T) {
	for name, category := range map[string]string{
		"subject-unknown": "encoding.eml-subject-charset",
		"subject-8bit":    "encoding.eml-subject-8bit",
	} {
		t.Run(name, func(t *testing.T) {
			raw := lenientFixture(t, name)
			require.Equal(t, map[string]string{"body": "Hello"}, blockTexts(t, raw))
			require.Equal(t, []string{category}, categories(t, raw, format.ValidationReport))
			out, err := roundTrip(t, raw, func(b *model.Block) { b.SetTargetText("fr", "Bonjour") }, "fr")
			require.NoError(t, err)
			subject, _, found := bytes.Cut(raw, []byte("Hello"))
			require.True(t, found)
			require.True(t, bytes.HasPrefix(out, subject), "the subject header changed")
			require.Equal(t, map[string]string{"body": "Bonjour"}, blockTexts(t, out))
		})
	}
}

func TestUndeclaredEightBitBodyStaysInTheSkeleton(t *testing.T) {
	raw := lenientFixture(t, "body-8bit")
	require.Equal(t, map[string]string{"subject": "Hi"}, blockTexts(t, raw))
	require.Equal(t, []string{"encoding.eml-body-charset"}, categories(t, raw, format.ValidationReport))
	out, err := roundTrip(t, raw, func(b *model.Block) { b.SetTargetText("fr", "Salut") }, "fr")
	require.NoError(t, err)
	require.Equal(t, bytes.Replace(raw, []byte("Subject: Hi"), []byte("Subject: Salut"), 1), out)
}

func TestEndOfInputClosesAMultipart(t *testing.T) {
	raw := lenientFixture(t, "missing-delimiter")
	require.Equal(t, map[string]string{"subject": "Hi", "body.1": "Café", "body.2": "no end"}, blockTexts(t, raw))
	require.Equal(t, []string{"structure.eml-missing-delimiter"}, categories(t, raw, format.ValidationReport))
	out, err := roundTrip(t, raw, nil, "")
	require.NoError(t, err)
	require.Equal(t, raw, out)
	out, err = roundTrip(t, raw, func(b *model.Block) {
		if b.Name == "body.2" {
			b.SetTargetText("fr", "sans fin")
		}
	}, "fr")
	require.NoError(t, err)
	require.Equal(t, bytes.Replace(raw, []byte("no end"), []byte("sans fin"), 1), out)
}

func TestUTF8CharsetAlias(t *testing.T) {
	raw := lenientFixture(t, "utf8-alias")
	require.Equal(t, map[string]string{"subject": "Hi", "body": "Café"}, blockTexts(t, raw))
	out, err := roundTrip(t, raw, func(b *model.Block) {
		if b.Name == "body" {
			b.SetTargetText("fr", "Thé")
		}
	}, "fr")
	require.NoError(t, err)
	require.Equal(t, bytes.Replace(raw, []byte("Café"), []byte("Thé"), 1), out)
}

func TestEditEndingInANewlineKeepsTheTransferEncoding(t *testing.T) {
	for name, tc := range map[string]struct {
		raw, want string
	}{
		"line ending follows":    {"Subject: Hi\n\nBody line\n", "Subject: Hi\n\nCorps\n\n"},
		"crlf line ending":       {"Subject: Hi\r\n\r\nBody line\r\n", "Subject: Hi\r\n\r\nCorps\r\n\r\n"},
		"quoted-printable":       {"Subject: Hi\nContent-Transfer-Encoding: quoted-printable\n\nBody line\n", "Subject: Hi\nContent-Transfer-Encoding: quoted-printable\n\nCorps\n\n"},
		"no line ending follows": {"Subject: Hi\n\nBody line", ""},
	} {
		t.Run(name, func(t *testing.T) {
			out, err := roundTrip(t, []byte(tc.raw), func(b *model.Block) {
				if b.Name == "body" {
					b.SetTargetText("fr", "Corps\n")
				}
			}, "fr")
			require.NoError(t, err)
			if tc.want != "" {
				require.Equal(t, tc.want, string(out))
			} else {
				require.Contains(t, string(out), "Content-Transfer-Encoding: base64")
			}
			require.Equal(t, "Corps\n", blockTexts(t, out)["body"])
		})
	}
}

func TestBodyLineEndingIsOutsideTheBlock(t *testing.T) {
	for _, transfer := range []string{"7bit", "quoted-printable", "base64"} {
		body := "Caf=C3=A9"
		switch transfer {
		case "7bit":
			body = "Cafe"
		case "base64":
			body = "Q2Fmw6k="
		}
		raw := "Subject: Hi\nContent-Type: text/plain; charset=utf-8\nContent-Transfer-Encoding: " + transfer + "\n\n" + body + "\n"
		got := blockTexts(t, []byte(raw))["body"]
		require.False(t, strings.HasSuffix(got, "\n"), "%s keeps the line ending in the block", transfer)
	}
}

func TestPlaceholderInATargetIsWritten(t *testing.T) {
	raw := []byte("Subject: Hello {name}\n\nHello {name}, welcome\n")
	runs := []model.Run{
		{Text: &model.TextRun{Text: "Bonjour "}},
		{Ph: &model.PlaceholderRun{ID: "1", Data: "{name}"}},
		{Text: &model.TextRun{Text: ", bienvenue"}},
	}
	out, err := roundTrip(t, raw, func(b *model.Block) { b.SetTargetRuns("fr", runs) }, "fr")
	require.NoError(t, err)
	require.Equal(t, "Subject: Bonjour {name}, bienvenue\n\nBonjour {name}, bienvenue\n", string(out))
}

func TestValidationNotes(t *testing.T) {
	require.Equal(t, []string{"structure.eml-html-alternative"}, categories(t, fixture(t, "multipart"), format.ValidationReport))
	require.Equal(t, []string{"structure.eml-dkim-signature"}, categories(t, lenientFixture(t, "dkim"), format.ValidationReport))
	require.Empty(t, categories(t, lenientFixture(t, "dkim"), format.ValidationOff))
	require.Empty(t, categories(t, fixture(t, "plain"), format.ValidationReport))
}
