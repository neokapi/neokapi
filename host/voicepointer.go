package host

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/project"
)

// AssistantFileNames lists the files an assistant reads at a project root, in
// the order the voice pointer prefers them. AGENTS.md is the vendor-neutral
// name; a CLAUDE.md that imports it (`@AGENTS.md`) reads it too.
var AssistantFileNames = []string{"AGENTS.md", "CLAUDE.md"}

// AssistantFile returns the assistant file a pointer is written into: the
// first of AssistantFileNames present at root, else AGENTS.md, which does not
// exist yet. exists reports which of the two it was.
func AssistantFile(root string) (path string, exists bool) {
	for _, name := range AssistantFileNames {
		p := filepath.Join(root, name)
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p, true
		}
	}
	return filepath.Join(root, AssistantFileNames[0]), false
}

// VoicePointerAction is what WriteVoicePointer did to the assistant file.
type VoicePointerAction string

const (
	// VoicePointerCreated: the assistant file did not exist and was created
	// holding the section.
	VoicePointerCreated VoicePointerAction = "created"
	// VoicePointerUpdated: the section was appended to, or replaced in, an
	// existing file.
	VoicePointerUpdated VoicePointerAction = "updated"
	// VoicePointerUnchanged: the file already carried this exact section.
	VoicePointerUnchanged VoicePointerAction = "unchanged"
	// VoicePointerRemoved: the project binds no voice, and a section written
	// earlier was taken out so the file stops claiming one.
	VoicePointerRemoved VoicePointerAction = "removed"
	// VoicePointerNone: the project binds no voice and no file holds a
	// section; nothing was written.
	VoicePointerNone VoicePointerAction = "none"
)

// VoicePointerResult reports what WriteVoicePointer did.
type VoicePointerResult struct {
	// File is the assistant file the section lives in, absolute. Empty when
	// nothing was written.
	File string `json:"file,omitempty"`
	// Created is true when File was created by this call.
	Created bool `json:"created,omitempty"`
	// Action is what happened to the section.
	Action VoicePointerAction `json:"action"`
	// Voice is the name the section carries; empty when the binding could
	// not be named.
	Voice string `json:"voice,omitempty"`
	// Warning says why the voice could not be named when a binding exists but
	// did not load. The section is still written, unnamed.
	Warning string `json:"warning,omitempty"`
}

// ProjectVoiceInfo describes the voice a project binds at its default point,
// as far as the pointer needs it.
type ProjectVoiceInfo struct {
	// Name is the profile's name, empty when the binding could not be loaded.
	Name string
	// Source is where the profile was loaded from (a path, `pack:<name>`,
	// `store:<name>`), empty when it was not loaded.
	Source string
	// Field is the recipe key the binding came from, or empty for a
	// convention file.
	Field string
	// PerFile is true when the recipe declares profiles, so the voice in force
	// depends on where a file sits.
	PerFile bool
	// Project is the recipe's name, for the title of a file created from
	// nothing.
	Project string
	// Problem is why a binding that exists did not load. A profile file the
	// recipe points at but nobody has written yet is not a problem: the
	// scaffold binds it before the author fills it in.
	Problem error
}

// DescribeProjectVoice reports whether the project at root binds a voice, and
// what the pointer can say about it. has is false when nothing binds one: no
// defaults.voice, no convention file, and no profile with a voice of its own.
func (a *App) DescribeProjectVoice(ctx context.Context, root string) (info ProjectVoiceInfo, has bool, err error) {
	recipePath := filepath.Join(root, project.RecipeFileName)
	proj, err := project.LoadWithOptions(recipePath, project.LoadOptions{SkipRequiresCheck: true})
	if err != nil {
		return info, false, fmt.Errorf("load project for the voice pointer: %w", err)
	}
	info.Project = proj.Name
	if info.Project == "" {
		info.Project = filepath.Base(root)
	}
	info.PerFile = len(proj.Profiles) > 0

	rc, err := proj.ResolveGovernanceFor(project.GovernancePoint{})
	if err != nil {
		return info, false, err
	}
	if rc.Voice != nil {
		info.Field = rc.VoiceField
		a.nameBoundVoice(ctx, root, rc, &info)
		return info, true, nil
	}

	for _, conv := range VoiceProfileConventions(root) {
		p, lerr := loadProfileFile(conv)
		if lerr != nil {
			info.Problem = lerr
			info.Source = conv
			return info, true, nil
		}
		if p != nil {
			info.Name = p.Name
			info.Source = conv
			return info, true, nil
		}
	}

	// No default voice: the project still has one if any declared profile
	// binds its own, or keeps one at its conventional path.
	for name, pr := range proj.Profiles {
		if pr.Voice != nil {
			info.PerFile = true
			return info, true, nil
		}
		conv := filepath.Join(root, project.RelStatePath(project.ProfilesDirName, name, VoiceConventionalName))
		if fi, serr := os.Stat(conv); serr == nil && !fi.IsDir() {
			info.PerFile = true
			return info, true, nil
		}
	}
	return info, false, nil
}

// nameBoundVoice fills in the name of the voice a governance binds, or the
// reason it has none yet.
func (a *App) nameBoundVoice(ctx context.Context, root string, rc *project.ResolvedGovernance, info *ProjectVoiceInfo) {
	bv := rc.Voice
	if bv.ProfileFile != "" {
		path := bv.ProfileFile
		if !filepath.IsAbs(path) {
			path = filepath.Join(root, path)
		}
		if _, serr := os.Stat(path); errors.Is(serr, os.ErrNotExist) {
			// Bound, not yet written: the ordinary state right after a
			// scaffold that points at a file the author fills in.
			return
		}
	}
	var store coreprofile.Store
	if bv.Profile != "" {
		s, release, serr := a.ProjectVoiceStore(ctx, root)
		if serr != nil {
			info.Problem = serr
			return
		}
		defer release()
		store = s
	}
	p, src, found, lerr := a.loadBoundVoiceProfile(ctx, bv, root, store, rc.VoiceField)
	switch {
	case lerr != nil:
		info.Problem = lerr
	case found:
		info.Name = p.Name
		info.Source = src
	}
}

// WriteVoicePointer writes the section that tells an assistant the project's
// voice is held by kapi into the project's assistant file, and reports what
// it did. The section is delimited by markers, so a second run replaces it in
// place and the hand-written rest of the file is never touched; when the
// project binds no voice, a section written earlier is removed and nothing is
// created.
func (a *App) WriteVoicePointer(ctx context.Context, root string) (*VoicePointerResult, error) {
	info, has, err := a.DescribeProjectVoice(ctx, root)
	if err != nil {
		return nil, err
	}
	path, exists := AssistantFile(root)
	res := &VoicePointerResult{Voice: info.Name}
	if info.Problem != nil {
		res.Warning = info.Problem.Error()
	}

	if !has {
		res.Action = VoicePointerNone
		if !exists {
			return res, nil
		}
		doc, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil, fmt.Errorf("read %s: %w", path, rerr)
		}
		out, removed, perr := coreprofile.RemoveVoicePointer(doc)
		if perr != nil {
			return nil, fmt.Errorf("%s: %w", path, perr)
		}
		if !removed {
			return res, nil
		}
		if werr := os.WriteFile(path, out, 0o644); werr != nil {
			return nil, fmt.Errorf("write %s: %w", path, werr)
		}
		res.File = path
		res.Action = VoicePointerRemoved
		return res, nil
	}

	section := coreprofile.RenderVoicePointer(coreprofile.VoicePointer{Name: info.Name, PerFile: info.PerFile})
	res.File = path
	if !exists {
		out := []byte("# " + info.Project + "\n\n" + section)
		if werr := os.WriteFile(path, out, 0o644); werr != nil {
			return nil, fmt.Errorf("write %s: %w", path, werr)
		}
		res.Created = true
		res.Action = VoicePointerCreated
		return res, nil
	}
	doc, rerr := os.ReadFile(path)
	if rerr != nil {
		return nil, fmt.Errorf("read %s: %w", path, rerr)
	}
	out, perr := coreprofile.UpsertVoicePointer(doc, section)
	if perr != nil {
		return nil, fmt.Errorf("%s: %w", path, perr)
	}
	if bytes.Equal(out, doc) {
		res.Action = VoicePointerUnchanged
		return res, nil
	}
	if werr := os.WriteFile(path, out, 0o644); werr != nil {
		return nil, fmt.Errorf("write %s: %w", path, werr)
	}
	res.Action = VoicePointerUpdated
	return res, nil
}
