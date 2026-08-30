package backend

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/neokapi/neokapi/core/graph"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/profile/packs"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/yamledit"
	"github.com/neokapi/neokapi/host"
)

// The voice surface: every point in the project's context space that can carry
// a voice, with the profile resolved at each.
//
// A voice binds at a profile, so the points that can differ are the project's
// own default point and each declared profile. Resolution goes through
// host.LoadCollectionVoice — the same ladder `kapi check`, a run and the
// context explorer take — so this page cannot show a voice the loop would not
// apply.
//
// Each point is resolved twice, and both answers are reported: as declared
// (windows carried) says what the recipe states, and at the governance instant
// (windows applied) says what is in force now. A binding the instant excluded
// is named in Fallback rather than quietly replaced, because governance
// changing on a date has to be visible.

// VoiceValidityDTO is a declared validity window and where it stands at the
// resolution instant.
type VoiceValidityDTO struct {
	From string `json:"from,omitempty"`
	To   string `json:"to,omitempty"`
	// State is "active", "upcoming" or "expired" at the resolution instant.
	State string `json:"state"`
}

// VoiceFallbackDTO records a binding the instant excluded, and what governs in
// its place.
type VoiceFallbackDTO struct {
	Profile string `json:"profile"`
	// Expired is true when the window closed; false when it has not opened.
	Expired  bool   `json:"expired"`
	Boundary string `json:"boundary,omitempty"`
	// Governing is the profile that governs instead, empty for the project's
	// default point.
	Governing string `json:"governing,omitempty"`
	Message   string `json:"message"`
}

// VoiceBindingDTO is the recipe binding that selected a profile.
type VoiceBindingDTO struct {
	// Kind is "profile_file", "pack" or "profile" — the three forms a
	// `voice:` binding takes.
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

// VoicePointDTO is one point and the voice in force there.
type VoicePointDTO struct {
	Point ContextPointDTO `json:"point"`
	// Label names the point for a reader: a profile name, or the project's
	// default point.
	Label       string            `json:"label"`
	Coordinates map[string]string `json:"coordinates,omitempty"`
	// Channels the profile declares. The default point declares none.
	Channels []string `json:"channels,omitempty"`
	// Collections resolving to this point at the governance instant.
	Collections []string `json:"collections"`
	// Field is the recipe key the governing binding was declared on.
	Field string `json:"field,omitempty"`
	// Source is where the profile was loaded from: a path, `pack:<name>` or
	// `store:<name>`.
	Source    string           `json:"source,omitempty"`
	Binding   *VoiceBindingDTO `json:"binding,omitempty"`
	TermStore string           `json:"termstore,omitempty"`
	// Profile is the voice as authored, with the overrides unapplied so each
	// locale, channel and persona can be read as itself.
	Profile  *coreprofile.VoiceProfile `json:"profile,omitempty"`
	Guide    string                    `json:"guide,omitempty"`
	Validity *VoiceValidityDTO         `json:"validity,omitempty"`
	Fallback *VoiceFallbackDTO         `json:"fallback,omitempty"`
	Notes    []string                  `json:"notes,omitempty"`
	// Edit says where a save at this point lands, and whether one can.
	Edit VoiceEditTargetDTO `json:"edit"`
}

// VoiceEditTargetDTO is where a save at a point writes, and whether it may.
type VoiceEditTargetDTO struct {
	// Target is the project-relative file a save writes to.
	Target string `json:"target,omitempty"`
	// Writable is false when the binding names something no file edit can
	// reach: a starter pack, or a profile held in the voice store.
	Writable bool `json:"writable"`
	// Exists is false when a save would create the file.
	Exists bool `json:"exists"`
	// Inherited is true when the point has no voice of its own and reads the
	// one bound coarser. Saving here gives the point its own profile rather
	// than editing what it inherits.
	Inherited bool `json:"inherited"`
	// Reason states why a save cannot land, when Writable is false.
	Reason string `json:"reason,omitempty"`
}

// VoiceSaveResult reports a save, or why it did not happen.
type VoiceSaveResult struct {
	// Saved is false when validation refused the profile.
	Saved bool `json:"saved"`
	// Target is the project-relative file written.
	Target string `json:"target,omitempty"`
	// Changed is false when the file on disk already said this.
	Changed  bool                         `json:"changed"`
	Problems []coreprofile.ProfileProblem `json:"problems"`
	// Guide is the profile as a tool would read it, rendered from what was
	// saved.
	Guide string `json:"guide,omitempty"`
}

// ProjectVoiceResult is every point the recipe declares, with its voice.
type ProjectVoiceResult struct {
	// At is the instant governance was resolved at, RFC3339.
	At     string          `json:"at"`
	Points []VoicePointDTO `json:"points"`
	Notes  []string        `json:"notes,omitempty"`
}

// ProjectVoice resolves the voice profile in force at every point the open
// project declares.
//
// The first point is always the project's own default; a profile follows for
// each the recipe declares, in name order. A point whose profile binds no voice
// of its own reports the one it inherits, with the recipe key that bound it, so
// a reader can see both what governs and where the decision was made.
func (a *App) ProjectVoice(tabID string) (*ProjectVoiceResult, error) {
	op := a.getOpenProject(tabID)
	if op == nil {
		return nil, fmt.Errorf("project tab %q not found", tabID)
	}
	if op.Project == nil || op.Path == "" {
		return nil, errors.New("the tab has no project recipe on disk")
	}
	ctx, cancel := context.WithTimeout(context.Background(), contextExplorerTimeout)
	defer cancel()

	proj := op.Project
	root := filepath.Dir(op.Path)
	engine := a.hostEngine()
	at := engine.GovernanceInstant()

	out := &ProjectVoiceResult{At: at.Format(time.RFC3339), Points: []VoicePointDTO{}}

	// A recipe may bind a profile by name, which only the project's voice store
	// can resolve. Without it such a binding reports as not found, which reads
	// as "nothing governs here" when something does.
	store, release, serr := engine.ProjectVoiceStore(ctx, root)
	if serr != nil {
		out.Notes = append(out.Notes, fmt.Sprintf("voice store: %v", serr))
	}
	defer release()

	collections := collectionsByProfile(proj, at)

	points := []project.GovernancePoint{{At: at}}
	for _, name := range sortedKeys(proj.Profiles) {
		points = append(points, project.GovernancePoint{Profile: name, At: at})
	}

	for _, pt := range points {
		row, err := a.voicePoint(ctx, proj, root, store, pt, collections)
		if err != nil {
			out.Notes = append(out.Notes, fmt.Sprintf("%s: %v", pointLabel(pt.Profile), err))
			continue
		}
		out.Points = append(out.Points, row)
	}
	return out, nil
}

// voicePoint resolves one point: what the recipe declares there, what is in
// force at the instant, and the profile itself.
func (a *App) voicePoint(
	ctx context.Context,
	proj *project.KapiProject,
	root string,
	store coreprofile.Store,
	pt project.GovernancePoint,
	collections map[string][]string,
) (VoicePointDTO, error) {
	// The as-declared resolution carries the window unapplied, so the point
	// reports its own binding even when the instant excluded it.
	declared, err := proj.ResolveGovernanceFor(project.GovernancePoint{Profile: pt.Profile})
	if err != nil {
		return VoicePointDTO{}, err
	}

	profile, rc, source, found, verr := a.hostEngine().LoadCollectionVoice(
		ctx, proj, root, host.VoiceResolveOptions{Store: store, Point: pt},
	)
	if verr != nil {
		return VoicePointDTO{}, verr
	}

	ref := project.ChannelRef{Profile: pt.Profile}
	row := VoicePointDTO{
		Point: ContextPointDTO{
			Profile: pt.Profile,
			Ref:     declared.VoiceField,
			Default: pt.Profile == "",
		},
		Label:       pointLabel(pt.Profile),
		Coordinates: project.MergeCoordinates(proj.Defaults.Coordinates, ref.Coordinates(), nil),
		Channels:    profileChannels(proj, pt.Profile),
		Collections: collections[pt.Profile],
		Field:       declared.VoiceField,
		TermStore:   declared.TermStore,
		Binding:     voiceBinding(declared.Voice),
		Validity:    validityDTO(declared.Validity, pt.At),
	}
	if row.Collections == nil {
		row.Collections = []string{}
	}
	if rc != nil {
		row.Fallback = fallbackDTO(rc.Fallback)
		// The window excluded this point's own binding, so what loaded is the
		// profile that governs in its place, on that profile's recipe key.
		if rc.Fallback != nil {
			row.Field = rc.VoiceField
		}
	}
	if found && profile != nil {
		row.Profile = profile
		row.Source = source
		row.Guide = coreprofile.RenderVoiceGuide(profile)
	} else {
		row.Notes = append(row.Notes, "no voice profile binds at this point")
	}
	row.Edit = voiceEditTarget(declared, root, pt.Profile, source, found)
	return row, nil
}

// voiceEditTarget resolves the file a save at a point writes to.
//
// It inverts the load ladder: the point's own `voice: profile_file` if it has
// one, otherwise the conventional location for that point, which is where the
// loader would look next. A binding naming a starter pack or a stored profile
// has no file to write, and says so rather than inventing one.
func voiceEditTarget(
	declared *project.ResolvedGovernance,
	root, profileName, loadedFrom string,
	found bool,
) VoiceEditTargetDTO {
	own := declared != nil && declared.Voice != nil &&
		(profileName == "" || declared.VoiceField != project.DefaultVoiceField)

	if own {
		switch {
		case declared.Voice.Pack != "":
			return VoiceEditTargetDTO{
				Reason: fmt.Sprintf(
					"%s binds the %q starter pack. Bind a profile file to edit the voice here.",
					declared.VoiceField, declared.Voice.Pack),
			}
		case declared.Voice.Profile != "":
			return VoiceEditTargetDTO{
				Reason: fmt.Sprintf(
					"%s binds %q from the voice store, which no file edit reaches.",
					declared.VoiceField, declared.Voice.Profile),
			}
		case declared.Voice.ProfileFile != "":
			rel := filepath.ToSlash(declared.Voice.ProfileFile)
			return VoiceEditTargetDTO{
				Target:   rel,
				Writable: true,
				Exists:   fileExists(filepath.Join(root, filepath.FromSlash(rel))),
			}
		}
	}

	// Nothing bound at this point: the conventional location for it is where
	// the loader looks, so it is where a save belongs.
	var rel string
	if profileName == "" {
		rel = project.RelStatePath(host.VoiceConventionalName)
		for _, conv := range host.VoiceProfileConventions(root) {
			if fileExists(conv) {
				if r, err := filepath.Rel(root, conv); err == nil {
					rel = r
				}
				break
			}
		}
	} else {
		rel = project.RelStatePath(project.ProfilesDirName, profileName, host.VoiceConventionalName)
	}
	abs := filepath.Join(root, filepath.FromSlash(rel))
	exists := fileExists(abs)
	return VoiceEditTargetDTO{
		Target:   filepath.ToSlash(rel),
		Writable: true,
		Exists:   exists,
		// A point reading a profile loaded from somewhere else has no voice of
		// its own; saving here creates one that shadows what it inherits.
		Inherited: found && !exists && !sameFile(loadedFrom, abs),
	}
}

// fileExists reports whether a regular file sits at path.
func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// sameFile compares two paths after cleaning, so a source and a target that
// name one file are not read as two.
func sameFile(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	return filepath.Clean(a) == filepath.Clean(b)
}

// ValidateVoiceProfile reports what `kapi voice validate` would report for this
// profile, without writing anything.
//
// The profile is marshalled and run back through the same three stages the
// command runs — the lenient load, the strict decode, then the semantic checks
// — so the editor and the CLI cannot disagree about whether a profile is sound.
func (a *App) ValidateVoiceProfile(profile coreprofile.VoiceProfile) ([]coreprofile.ProfileProblem, error) {
	probs, err := validateVoiceProfile(&profile)
	if err != nil {
		return nil, err
	}
	return probs, nil
}

// VoiceFieldValues returns the values each constrained profile field accepts,
// so the editor offers exactly what validation applies.
func (a *App) VoiceFieldValues() map[string]coreprofile.FieldValueSet {
	return coreprofile.FieldValues()
}

// VoiceStarterPacks lists the starter profiles a new voice can begin from.
func (a *App) VoiceStarterPacks() ([]string, error) {
	return packs.List()
}

// VoiceStarterPack returns one starter profile, for seeding a new voice.
func (a *App) VoiceStarterPack(name string) (*coreprofile.VoiceProfile, error) {
	return packs.Load(name)
}

// SaveVoiceProfile writes a voice profile to the file the point resolves to.
//
// Validation runs first and a blocking problem refuses the write, so the file a
// run reads is never one the loader would reject. Warnings do not refuse: a
// tone the usual list does not name is kept and rendered as written.
//
// The write goes through the comment-preserving writer, so an author's
// reasoning and key order survive an edit made here.
func (a *App) SaveVoiceProfile(tabID, profileName string, profile coreprofile.VoiceProfile) (*VoiceSaveResult, error) {
	op := a.getOpenProject(tabID)
	if op == nil {
		return nil, fmt.Errorf("project tab %q not found", tabID)
	}
	if op.Project == nil || op.Path == "" {
		return nil, errors.New("the tab has no project recipe on disk")
	}
	root := filepath.Dir(op.Path)

	declared, err := op.Project.ResolveGovernanceFor(project.GovernancePoint{Profile: profileName})
	if err != nil {
		return nil, err
	}
	target := voiceEditTarget(declared, root, profileName, "", false)
	if !target.Writable {
		return nil, errors.New(target.Reason)
	}

	probs, err := validateVoiceProfile(&profile)
	if err != nil {
		return nil, err
	}
	out := &VoiceSaveResult{Target: target.Target, Problems: probs}
	if len(coreprofile.Blocking(probs)) > 0 {
		return out, nil
	}

	path := filepath.Join(root, filepath.FromSlash(target.Target))
	changed, werr := yamledit.WriteFile(path, &profile, 0o644)
	if werr != nil {
		return nil, werr
	}
	out.Saved = true
	out.Changed = changed
	out.Guide = coreprofile.RenderVoiceGuide(&profile)
	return out, nil
}

// validateVoiceProfile runs the three stages `kapi voice validate` runs.
func validateVoiceProfile(profile *coreprofile.VoiceProfile) ([]coreprofile.ProfileProblem, error) {
	body, err := yaml.Marshal(profile)
	if err != nil {
		return nil, fmt.Errorf("render profile: %w", err)
	}
	probs := []coreprofile.ProfileProblem{}
	if _, lerr := coreprofile.LoadProfileYAML(bytes.NewReader(body)); lerr != nil {
		probs = append(probs, coreprofile.ProfileProblem{Message: lerr.Error()})
	}
	decoded, serr := coreprofile.DecodeProfileStrict(bytes.NewReader(body))
	probs = append(probs, host.StrictDecodeProblems(serr)...)
	probs = append(probs, coreprofile.ValidateProfile(decoded)...)
	return probs, nil
}

// RecipeAxisDTO is one axis a recipe can put a coordinate on.
type RecipeAxisDTO struct {
	Axis string `json:"axis"`
	// Declarable is false for the structural axes, which a collection's
	// channel derives rather than a recipe declaring them.
	Declarable bool `json:"declarable"`
	// Refusal is what a recipe is told when it declares this axis anyway.
	Refusal string `json:"refusal,omitempty"`
	// Values are the ones this axis is known to take, where it has a set.
	Values []string `json:"values,omitempty"`
	// Used is the value the open project declares on this axis.
	Used string `json:"used,omitempty"`
}

// RecipeGovernanceDTO is what the recipe can say about where content sits and
// what governs it there.
type RecipeGovernanceDTO struct {
	// Axes covers the structural axes, the axes the framework names, and every
	// axis this project already declares.
	Axes []RecipeAxisDTO `json:"axes"`
	// Channels are the `profile/channel` references a collection may name.
	Channels []string `json:"channels"`
	// Profiles are the declared profile names.
	Profiles []string `json:"profiles"`
	// VoiceFiles are the profile files already on disk under the state
	// directory, offered when binding defaults.voice.
	VoiceFiles []string `json:"voice_files"`
	// Packs are the starter profiles a binding can name instead of a file.
	Packs []string `json:"packs"`
}

// RecipeGovernance describes the governance vocabulary of the open project: the
// axes a point can carry, the channels a collection can name, and the profiles
// a voice binding can reach.
//
// The refusal for a structural axis comes from project.DeclarableAxis, the same
// check `kapi apply` runs, so the editor cannot offer an axis the recipe would
// reject or word the refusal differently.
func (a *App) RecipeGovernance(tabID string) (*RecipeGovernanceDTO, error) {
	op := a.getOpenProject(tabID)
	if op == nil {
		return nil, fmt.Errorf("project tab %q not found", tabID)
	}
	if op.Project == nil {
		return nil, errors.New("the tab has no project recipe")
	}
	proj := op.Project

	out := &RecipeGovernanceDTO{
		Channels:   []string{},
		Profiles:   []string{},
		VoiceFiles: []string{},
		Packs:      []string{},
	}

	seen := map[string]bool{}
	addAxis := func(axis string, values []string) {
		if axis == "" || seen[axis] {
			return
		}
		seen[axis] = true
		row := RecipeAxisDTO{Axis: axis, Values: values, Used: proj.Defaults.Coordinates[axis]}
		if err := project.DeclarableAxis(axis); err != nil {
			row.Refusal = err.Error()
		} else {
			row.Declarable = true
		}
		out.Axes = append(out.Axes, row)
	}
	addAxis(project.ProductAxis, nil)
	addAxis(project.ChannelAxis, nil)
	addAxis(project.BrandAxis, nil)
	addAxis(project.ModeAxis, []string{
		project.ModeTutorial, project.ModeHowTo, project.ModeReference, project.ModeExplanation,
	})
	for _, axis := range sortedKeys(proj.Defaults.Coordinates) {
		addAxis(axis, nil)
	}
	for i := range proj.Collections {
		for _, axis := range sortedKeys(proj.Collections[i].Coordinates) {
			addAxis(axis, nil)
		}
	}

	for _, name := range sortedKeys(proj.Profiles) {
		out.Profiles = append(out.Profiles, name)
		for _, ch := range proj.Profiles[name].Channels {
			if ch.ID == "" {
				continue
			}
			out.Channels = append(out.Channels, project.ChannelRef{Profile: name, Channel: ch.ID}.String())
		}
	}

	if op.Path != "" {
		out.VoiceFiles = discoverVoiceFiles(filepath.Dir(op.Path))
	}
	if names, perr := packs.List(); perr == nil {
		out.Packs = names
	}
	return out, nil
}

// discoverVoiceFiles lists the profile files under the project's state
// directory, project-relative, so a binding can be picked rather than typed.
func discoverVoiceFiles(root string) []string {
	stateDir := filepath.Join(root, project.StateDirName)
	var out []string
	_ = filepath.WalkDir(stateDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			// Generated state is not authored source, so nothing under it is a
			// profile a person would bind.
			if d.Name() == "work" {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".yaml" && filepath.Ext(path) != ".yml" {
			return nil
		}
		rel, rerr := filepath.Rel(root, path)
		if rerr != nil {
			return nil
		}
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	sort.Strings(out)
	return out
}

// pointLabel names a point for a reader.
func pointLabel(profile string) string {
	if profile == "" {
		return "project default"
	}
	return profile
}

// profileChannels lists the channels a profile declares, in recipe order.
func profileChannels(proj *project.KapiProject, name string) []string {
	if name == "" {
		return nil
	}
	prof, ok := proj.Profiles[name]
	if !ok {
		return nil
	}
	out := make([]string, 0, len(prof.Channels))
	for _, ch := range prof.Channels {
		if ch.ID == "" {
			continue
		}
		out = append(out, ch.ID)
	}
	return out
}

// collectionsByProfile groups the project's collections by the profile
// governing them at the instant, so each point lists what sits under it.
func collectionsByProfile(proj *project.KapiProject, at time.Time) map[string][]string {
	out := map[string][]string{}
	for _, coll := range proj.Collections {
		if coll.Name == "" {
			continue
		}
		rc, err := proj.ResolveGovernanceFor(project.GovernancePoint{Collection: coll.Name, At: at})
		if err != nil {
			continue
		}
		key := rc.Ref().Profile
		out[key] = append(out[key], coll.Name)
	}
	for key := range out {
		sort.Strings(out[key])
	}
	return out
}

// voiceBinding renders the recipe binding that selected a profile.
func voiceBinding(b *project.VoiceBinding) *VoiceBindingDTO {
	if b == nil {
		return nil
	}
	switch {
	case b.ProfileFile != "":
		return &VoiceBindingDTO{Kind: "profile_file", Value: b.ProfileFile}
	case b.Pack != "":
		return &VoiceBindingDTO{Kind: "pack", Value: b.Pack}
	case b.Profile != "":
		return &VoiceBindingDTO{Kind: "profile", Value: b.Profile}
	}
	return nil
}

// validityDTO renders a declared window and where it stands at the instant.
func validityDTO(v *graph.Validity, at time.Time) *VoiceValidityDTO {
	if v == nil {
		return nil
	}
	out := &VoiceValidityDTO{State: "active"}
	if v.ValidFrom != nil {
		out.From = v.ValidFrom.Format(time.RFC3339)
		if at.Before(*v.ValidFrom) {
			out.State = "upcoming"
		}
	}
	if v.ValidTo != nil {
		out.To = v.ValidTo.Format(time.RFC3339)
		if !at.Before(*v.ValidTo) {
			out.State = "expired"
		}
	}
	return out
}

// fallbackDTO renders a skipped binding and what governs instead.
func fallbackDTO(f *project.GovernanceFallback) *VoiceFallbackDTO {
	if f == nil {
		return nil
	}
	out := &VoiceFallbackDTO{
		Profile:   f.Profile,
		Expired:   f.Expired,
		Governing: f.Governing,
		Message:   f.String(),
	}
	if !f.Boundary.IsZero() {
		out.Boundary = f.Boundary.Format(time.RFC3339)
	}
	return out
}
