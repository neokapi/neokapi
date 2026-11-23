package mo

import "testing"

// TestMsgidWithContextIsGettexts pins the msgid an MO file stores for an
// entry with a context: the context, EOT and the msgid, as gettext writes it.
func TestMsgidWithContextIsGettexts(t *testing.T) {
	if got := msgidWithContext("menu", "Open"); got != "menu\x04Open" {
		t.Fatalf("msgidWithContext = %q", got)
	}
	if got := msgidWithContext("", "Open"); got != "Open" {
		t.Fatalf("msgidWithContext without a context = %q", got)
	}
}
