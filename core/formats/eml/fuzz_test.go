package eml_test

import (
	"github.com/neokapi/neokapi/core/format/spec"
	"github.com/neokapi/neokapi/core/formats/eml"
	"github.com/stretchr/testify/require"
	"testing"
)

func FuzzMessage(f *testing.F) {
	for _, seed := range []string{"From: writer@example.org\n\nBody", "Subject: hi\r\n\r\n", "Content-Type: multipart/mixed; boundary=x\n\n--x\n\nhello\n--x--\n"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, wire string) {
		if len(wire) > 64<<10 {
			t.Skip()
		}
		src := []byte(wire)
		_, err := spec.ReadParts(eml.NewReader(), src)
		if err != nil {
			require.NotContains(t, err.Error(), "reader panicked")
			return
		}
		got, err := roundTrip(t, src, nil, "")
		require.NoError(t, err)
		require.Equal(t, src, got)
	})
}
