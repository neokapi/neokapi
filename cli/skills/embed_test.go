package skills

import (
	"os"
	"path/filepath"
	"testing"
)

func TestListEmbedsKapiSkill(t *testing.T) {
	all, err := List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) == 0 {
		t.Fatal("expected at least one embedded skill")
	}
	var found bool
	for _, s := range all {
		if s.Name == "" || s.Description == "" {
			t.Errorf("skill %q missing name or description", s.Name)
		}
		if s.Name == "kapi" {
			found = true
		}
	}
	if !found {
		t.Error("expected a skill named kapi")
	}
}

func TestGetKapiSkill(t *testing.T) {
	s, err := Get("kapi")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if s.Name != "kapi" {
		t.Errorf("name = %q", s.Name)
	}
	if s.Description == "" {
		t.Error("description is empty")
	}
}

// InstallTo must copy the whole skill tree — the SKILL.md router and the
// progressive-disclosure reference files — byte-identically.
func TestInstallToCopiesTreeByteIdentical(t *testing.T) {
	dir := t.TempDir()
	written, err := InstallTo(dir, nil)
	if err != nil {
		t.Fatalf("InstallTo: %v", err)
	}
	if len(written) < 2 {
		t.Fatalf("expected the router + reference files, got %d", len(written))
	}

	// SKILL.md is byte-identical to the embedded source.
	s, _ := Get("kapi")
	got, err := os.ReadFile(filepath.Join(dir, "kapi", "SKILL.md"))
	if err != nil {
		t.Fatalf("read installed SKILL.md: %v", err)
	}
	if string(got) != s.Content {
		t.Error("installed SKILL.md is not byte-identical to embedded source")
	}

	// A reference file shipped too.
	for _, ref := range []string{"brand", "localize", "i18n"} {
		p := filepath.Join(dir, "kapi", "references", ref+".md")
		if _, err := os.Stat(p); err != nil {
			t.Errorf("expected reference file %s: %v", p, err)
		}
	}
}

func TestInstallSubset(t *testing.T) {
	dir := t.TempDir()
	if _, err := InstallTo(dir, []string{"kapi"}); err != nil {
		t.Fatalf("InstallTo subset: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "kapi", "SKILL.md")); err != nil {
		t.Fatalf("kapi/SKILL.md not installed: %v", err)
	}
	if _, err := InstallTo(dir, []string{"does-not-exist"}); err == nil {
		t.Error("expected error for unknown skill name")
	}
}
