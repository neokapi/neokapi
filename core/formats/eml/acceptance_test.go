//go:build acceptance

package eml_test

import (
	"bytes"
	"os/exec"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/require"
)

func TestAcceptancePythonEmail(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 is unavailable")
	}
	out, err := roundTrip(t, fixture(t, "python-email"), func(b *model.Block) {
		if b.Name == "subject" {
			b.SetTargetText("ja", "お知らせ")
		} else {
			b.SetTargetText("ja", "今月の予定です。")
		}
	}, "ja")
	require.NoError(t, err)
	const consumer = `from email import policy
from email.parser import BytesParser
import sys
m = BytesParser(policy=policy.strict).parse(sys.stdin.buffer)
assert str(m["Subject"]) == "お知らせ"
assert m.get_body(preferencelist=("plain",)).get_content() == "今月の予定です。"
assert m.get_body(preferencelist=("html",)).get_content().strip() == "<p>Ankündigung für März.</p>"
a, = m.iter_attachments()
assert a.get_filename() == "sample.bin"
assert a.get_payload(decode=True) == bytes.fromhex("ff00fe10")
assert all(not p.defects for p in m.walk())
`
	cmd := exec.CommandContext(t.Context(), python, "-I", "-c", consumer)
	cmd.Stdin = bytes.NewReader(out)
	result, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s", result)
}
