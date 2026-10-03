package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPairedExecutedWords(t *testing.T) {
	for _, tc := range []struct {
		command string
		want    []string
	}{
		{"ls .agents/skills/kapi", []string{"ls"}},
		{"kapi inspect a.md --jsonl", []string{"kapi"}},
		{"cat a | kapi apply - && echo done; kgrep x *.md", []string{"cat", "kapi", "echo", "kgrep"}},
		{"KAPI_ACTOR=agent kapi apply x.json", []string{"kapi"}},
		{"env -u FOO KAPI_ACTOR=agent /opt/homebrew/bin/kapi apply x", []string{"env", "/opt/homebrew/bin/kapi"}},
		{"timeout 30 kapi check", []string{"timeout", "kapi"}},
		{"xargs -n 1 kapi-files inspect < list", []string{"xargs", "kapi-files"}},
		{"command -v kapi", []string{"command"}},
		{"sudo -u root kapi version", []string{"sudo", "kapi"}},
		{`/bin/zsh -c "sed -n '1,240p' .agents/skills/kapi/SKILL.md && rg -n -F 'Back up' docs"`, []string{"/bin/zsh", "sed", "rg"}},
		{"bash -lc 'kapi inspect a.md | head'", []string{"bash", "kapi", "head"}},
		{`eval "kapi apply x.json"`, []string{"eval", "kapi"}},
		{"echo $(kapi version) `kcat a.md`", []string{"kapi", "kcat", "echo"}},
		{`echo "rev: $(kapi inspect a.md | jq -r .rev)"`, []string{"kapi", "jq", "echo"}},
		{"diff <(kcat a.md) <(kcat b.md)", []string{"kcat", "kcat", "diff"}},
		{"find docs -name '*.md' -exec kapi-files inspect {} \\;", []string{"find", "kapi-files"}},
		{"(cd docs && kapi inspect a.md)", []string{"cd", "kapi"}},
		{"if kapi check; then echo ok; fi", []string{"kapi", "echo"}},
		{"for f in kapi kcat; do echo $f; done", []string{"echo"}},
		{"kapi apply x.json 2>&1 > out.txt", []string{"kapi"}},
		{"kapi apply x.json >/tmp/kapi", []string{"kapi"}},
		{"cat <<'EOF' > edits.json\n{\"note\": \"kapi apply\"}\n/opt/homebrew/bin/kapi\nEOF\nkapi apply edits.json", []string{"cat", "kapi"}},
		{"cat <<-EOF\n\tkapi\n\tEOF\nwc -l", []string{"cat", "wc"}},
		{"grep -c kapi <<< 'kapi kapi'", []string{"grep"}},
		{"grep -rn 'kapi' docs # kapi apply", []string{"grep"}},
		{"echo $((1 + 2)); kapi", []string{"echo", "kapi"}},
		{"printf 'unterminated", []string{"printf"}},
	} {
		assert.Equal(t, tc.want, pairedExecutedWords(tc.command), tc.command)
	}
}
