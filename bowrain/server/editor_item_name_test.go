package server

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestSafeItemName pins what may be stored as an item name. An item name is a
// repository-relative path that the delivery side joins onto a checkout root to
// decide which file to write, so it has to stay inside the tree it is relative
// to — while still allowing the nested names that are the ordinary case.
func TestSafeItemName(t *testing.T) {
	tests := []struct {
		name string
		item string
		want bool
	}{
		// The ordinary shapes, which must keep working.
		{"a bare file name", "en.json", true},
		{"a nested path", "locales/en/messages.json", true},
		{"a deeply nested path", "src/main/resources/i18n/messages_en.properties", true},
		{"a dotted directory", ".github/labels.json", true},
		{"a name containing dots", "en.default.json", true},
		{"a name with spaces", "my strings/en.json", true},
		{"a non-ASCII name", "ロケール/ja.json", true},
		{"a hyphenated name", "values-fr/strings.xml", true},

		// Escapes.
		{"parent traversal", "../secrets.json", false},
		{"traversal in the middle", "locales/../../secrets.json", false},
		{"traversal only", "..", false},
		{"absolute path", "/etc/passwd", false},
		{"absolute path with traversal", "/../../etc/passwd", false},

		// Non-canonical forms, which would mean two things once joined.
		{"current directory prefix", "./en.json", false},
		{"doubled separator", "locales//en.json", false},
		{"trailing separator", "locales/en/", false},
		{"single dot", ".", false},

		// Shapes that mean different things on different platforms.
		{"backslash separator", `locales\en.json`, false},
		{"backslash traversal", `..\..\secrets.json`, false},
		{"windows volume", `C:\secrets.json`, false},

		// Malformed.
		{"empty", "", false},
		{"null byte", "en.json\x00", false},
		{"over the length bound", strings.Repeat("a", maxItemNameLen+1), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, safeItemName(tt.item))
		})
	}
}

// TestEditorAddFilesSkipsUnsafeNames pins the ingest behaviour: an unusable
// name is reported per-file like any other unimportable upload, and does not
// fail the batch or land in the store alongside the good files.
func TestEditorAddFilesSkipsUnsafeNames(t *testing.T) {
	srv, token := newTestServer(t)
	pid := createProject(t, srv, token)
	ctx := t.Context()

	files := map[string][]byte{
		"en.json":               []byte(`{"greeting":"Hello"}`),
		"../../escaped.json":    []byte(`{"greeting":"Escaped"}`),
		"/etc/passwd.json":      []byte(`{"greeting":"Absolute"}`),
		`..\..\windows.json`:    []byte(`{"greeting":"Backslash"}`),
		"locales/fr/msgs.json":  []byte(`{"greeting":"Bonjour"}`),
		"./non-canonical.json":  []byte(`{"greeting":"Dotted"}`),
		"locales//doubled.json": []byte(`{"greeting":"Doubled"}`),
	}

	info, err := editorAddFiles(ctx, srv.ContentStore, srv.FormatRegistry, pid, "main", files)
	if err != nil {
		t.Fatalf("upload must not fail the batch: %v", err)
	}

	skipped := map[string]string{}
	for _, s := range info.Skipped {
		skipped[s.Name] = s.Reason
	}
	for _, bad := range []string{
		"../../escaped.json", "/etc/passwd.json", `..\..\windows.json`,
		"./non-canonical.json", "locales//doubled.json",
	} {
		assert.Contains(t, skipped, bad, "unsafe name must be reported as skipped")
	}

	// The well-formed uploads still landed.
	items, err := srv.ContentStore.ListItems(ctx, pid, "main")
	if err != nil {
		t.Fatalf("list items: %v", err)
	}
	names := map[string]bool{}
	for _, it := range items {
		names[it.Name] = true
	}
	assert.True(t, names["en.json"], "a well-formed upload must still land")
	assert.True(t, names["locales/fr/msgs.json"], "a nested upload must still land")
	for name := range names {
		assert.True(t, safeItemName(name), "stored item name %q must be a contained relative path", name)
	}
}
