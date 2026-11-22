package eml_test

import (
	"bytes"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/format/spec"
	"github.com/neokapi/neokapi/core/formats/eml"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/require"
)

func TestOpaquePartsStayOutOfTheModel(t *testing.T) {
	for _, header := range []string{
		"Content-Type: text/html", "Content-Type: text/plain; format=flowed",
		"Content-Type: text/plain; charset=unknown", "Content-Type: message/rfc822",
		"Content-Type: text/plain; name=notes.txt", "Content-Disposition: attachment",
		"Content-Transfer-Encoding: x-opaque", "Content-Type: multipart/encrypted; boundary=enc",
	} {
		in := []byte("Subject: Subject\n" + header + "\n\nPrivate content\n")
		var names []string
		out, err := roundTrip(t, in, func(b *model.Block) { names = append(names, b.Name); b.SetSourceText("New subject") }, "")
		require.NoError(t, err)
		require.Equal(t, []string{"subject"}, names)
		require.Equal(t, bytes.Replace(in, []byte("Subject: Subject"), []byte("Subject: New subject"), 1), out)
	}
}

func TestSkeletonOnlyMerge(t *testing.T) {
	r, w := eml.NewReader(), eml.NewWriter()
	// Only the skeleton and a translated block reach a source-free merge.
	store := format.NewMemorySkeletonStore()
	r.SetSkeletonStore(store)
	_, err := spec.ReadParts(r, fixture(t, "plain"))
	require.NoError(t, err)
	w.SetSkeletonStore(store)
	t.Cleanup(func() { require.NoError(t, store.Close()) })
	b := model.NewBlock("body", "")
	b.SetTargetText("fr", "Texte.")
	w.SetLocale("fr")
	out, err := spec.WriteParts(w, []*model.Part{{Type: model.PartBlock, Resource: b}}, nil)
	require.NoError(t, err)
	require.Equal(t, bytes.Replace(fixture(t, "plain"), []byte("The draft is ready."), []byte("Texte."), 1), out)
}

func TestMIMEPartIdentitiesSelectOneBody(t *testing.T) {
	in := []byte("Subject: Two notices\nContent-Type: multipart/mixed; boundary=b\n\n" +
		"--b\nContent-Type: text/plain\n\nPlease review.\n" +
		"--b\nContent-Type: text/plain\n\nPlease review.\n--b--\n")
	var names []string
	out, err := roundTrip(t, in, func(b *model.Block) {
		names = append(names, b.Name)
		if b.Name == "body.2" {
			b.SetSourceText("Approved.")
		}
	}, "")
	require.NoError(t, err)
	require.Equal(t, []string{"subject", "body.1", "body.2"}, names)
	require.Equal(t, bytes.Replace(in, []byte("Please review.\n--b--"), []byte("Approved.\n--b--"), 1), out)
}
