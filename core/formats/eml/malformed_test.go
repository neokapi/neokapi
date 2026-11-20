package eml_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/format/spec"
	"github.com/neokapi/neokapi/core/formats/eml"
	"github.com/stretchr/testify/require"
)

func TestMalformed(t *testing.T) {
	tooDeep := "Content-Type: text/plain\n\nText\n"
	for i := range 35 {
		b := fmt.Sprintf("part-%d", i)
		tooDeep = "Content-Type: multipart/mixed; boundary=" + b + "\n\n--" + b + "\n" + tooDeep + "--" + b + "--\n"
	}
	for name, input := range map[string]string{
		"truncated headers": "Subject: Report\r",
		"broken header":     "missing colon\n\ntext",
		"multiple subjects": "Subject: first\nSubject: second\n\ntext",
		"invalid encoding":  "Content-Transfer-Encoding: base64\n\n!!%%%!",
		"missing delimiter": "Content-Type: multipart/mixed; boundary=x\n\n--x\n\ntext",
		"depth":             tooDeep,
		"header size":       "Subject: " + strings.Repeat("x", 256<<10) + "\n\n",
		"part count":        "Content-Type: multipart/mixed; boundary=x\n\n" + strings.Repeat("--x\n\ntext\n", 1000) + "--x--\n",
	} {
		t.Run(name, func(t *testing.T) {
			parts, err := spec.ReadParts(eml.NewReader(), []byte(input))
			require.Error(t, err)
			require.NotContains(t, err.Error(), "reader panicked")
			require.Empty(t, parts)
		})
	}
}
