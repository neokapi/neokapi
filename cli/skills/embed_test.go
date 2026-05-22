package skills

import (
	"os"
	"path/filepath"
	"testing"
)

func TestListEmbedsAllSkills(t *testing.T) {
	all, err := List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) < 4 {
		t.Fatalf("expected at least 4 skills, got %d", len(all))
	}
	var kapi, bowrain int
	for _, s := range all {
		if s.Name == "" || s.Description == "" {
			t.Errorf("skill %q missing name or description", s.Name)
		}
		switch s.Family {
		case "kapi":
			kapi++
		case "bowrain":
			bowrain++
		default:
			t.Errorf("skill %q has unexpected family %q", s.Name, s.Family)
		}
	}
	if kapi == 0 || bowrain == 0 {
		t.Errorf("expected both kapi and bowrain skills, got kapi=%d bowrain=%d", kapi, bowrain)
	}
}

func TestGetKnownSkill(t *testing.T) {
	s, err := Get("kapi-brand")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if s.Name != "kapi-brand" {
		t.Errorf("name = %q", s.Name)
	}
	if s.Family != "kapi" {
		t.Errorf("family = %q", s.Family)
	}
}

func TestBowrainSkillFamily(t *testing.T) {
	s, err := Get("bowrain")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if s.Family != "bowrain" {
		t.Errorf("bowrain skill family = %q, want bowrain", s.Family)
	}
}

func TestInstallToWritesByteIdentical(t *testing.T) {
	dir := t.TempDir()
	written, err := InstallTo(dir, nil)
	if err != nil {
		t.Fatalf("InstallTo: %v", err)
	}
	if len(written) < 4 {
		t.Fatalf("expected >=4 files written, got %d", len(written))
	}
	// Each installed file must equal the embedded content byte-for-byte.
	s, _ := Get("kapi-brand")
	got, err := os.ReadFile(filepath.Join(dir, "kapi-brand", "SKILL.md"))
	if err != nil {
		t.Fatalf("read installed: %v", err)
	}
	if string(got) != s.Content {
		t.Error("installed SKILL.md is not byte-identical to embedded source")
	}
}

func TestInstallSubset(t *testing.T) {
	dir := t.TempDir()
	written, err := InstallTo(dir, []string{"kapi-brand"})
	if err != nil {
		t.Fatalf("InstallTo subset: %v", err)
	}
	if len(written) != 1 {
		t.Fatalf("expected 1 file, got %d", len(written))
	}
}
