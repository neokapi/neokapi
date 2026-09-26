package host

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/neokapi/neokapi/core/contextop"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/projectdb"
	"github.com/neokapi/neokapi/core/projector"
)

// Writing a whole voice profile at a point, for a surface that edits one as a
// document.
//
// The desktop's voice editor holds the profile in a form and saves it entire,
// the way `kapi voice edit` saves what came back from the editor. Both land in
// the project's voice store and leave one operation on the project's context
// record, so the two surfaces write the same thing to the same place.

// VoiceProfileSave reports a write into a project's voice store.
type VoiceProfileSave struct {
	// ID is the profile the write landed on, in the project's voice store.
	ID string `json:"id"`
	// Name is the profile's label.
	Name string `json:"name,omitempty"`
	// Created is true when the store had no profile under this id.
	Created bool `json:"created,omitempty"`
	// Changed is false when the store already said this.
	Changed bool `json:"changed"`
	// Recorded is the id of the context operation the write left on the
	// project's record, empty when nothing changed.
	Recorded string `json:"recorded,omitempty"`
}

// VoiceProfileTarget is the profile a save at a point writes, and whether one
// can.
type VoiceProfileTarget struct {
	// ID is the profile in the project's voice store a save lands on.
	ID string `json:"id,omitempty"`
	// Writable is false when the point binds something a save cannot reach: a
	// starter pack.
	Writable bool `json:"writable"`
	// Exists is false when a save creates the profile.
	Exists bool `json:"exists"`
	// Inherited is true when the point has no voice of its own and reads the
	// one bound coarser. A save here gives the point a profile of its own.
	Inherited bool `json:"inherited"`
	// Reason states why a save cannot land, when Writable is false.
	Reason string `json:"reason,omitempty"`
}

// VoiceProfileTargetAt reports which profile a save at a point would write,
// without writing anything.
//
// A point that binds a profile by name answers with that profile. A point that
// binds nothing answers with the profile a save would open for it, and says the
// point reads its voice from coarser up.
func (a *App) VoiceProfileTargetAt(ctx context.Context, root string, point project.GovernancePoint) (VoiceProfileTarget, error) {
	recipePath := filepath.Join(root, project.RecipeFileName)
	proj, err := project.LoadWithOptions(recipePath, project.LoadOptions{SkipRequiresCheck: true})
	if err != nil {
		return VoiceProfileTarget{}, fmt.Errorf("load project: %w", err)
	}
	db, err := a.ProjectDB(ctx, root)
	if err != nil {
		return VoiceProfileTarget{}, err
	}
	store := projector.VoiceView(db)
	bound, own := voiceBindingAt(proj, point.Profile)

	// A profile that binds no voice of its own is answered by the stored
	// profile named after it, ahead of the project's, which is the ladder a
	// resolution takes (profileVoiceByName). A save at that point lands on the
	// profile it reads.
	if !own && store != nil {
		if p, gerr := lookupProfileIn(ctx, store, point.Profile); gerr == nil {
			return VoiceProfileTarget{ID: p.ID, Writable: true, Exists: true}, nil
		}
	}

	target := VoiceProfileTarget{Writable: true, Inherited: !own}
	switch {
	case bound == nil || !own:
		target.ID = newVoiceProfileID(root, point.Profile)
	case bound.Pack != "":
		return VoiceProfileTarget{
			Reason: fmt.Sprintf("%s binds the %q starter pack, which is read-only. Bind a profile of this project's own to edit the voice here.",
				voiceFieldName(point.Profile, own), bound.Pack),
		}, nil
	case bound.Profile != "":
		target.ID = bound.Profile
	}
	if store != nil && target.ID != "" {
		if _, gerr := lookupProfileIn(ctx, store, target.ID); gerr == nil {
			target.Exists = true
		}
	}
	return target, nil
}

// SaveVoiceProfileAt writes a whole voice profile into the project's voice
// store at a point, and records the write on the project's context record.
//
// The profile is stated entire, so a list the caller left out is left out of
// the stored profile. Constraints are the exception the store already makes:
// a profile carrying none keeps the ones the store holds, which is what a
// caller that never saw them needs.
//
// actor says who is writing. A surface a person drives states the person; an
// empty kind leaves the environment to answer, the way a command line does.
func (a *App) SaveVoiceProfileAt(
	ctx context.Context,
	root string,
	point project.GovernancePoint,
	actor contextop.Actor,
	prof *coreprofile.VoiceProfile,
) (VoiceProfileSave, error) {
	var res VoiceProfileSave
	if prof == nil {
		return res, errors.New("voice: no profile to save")
	}
	recipePath := filepath.Join(root, project.RecipeFileName)
	w, err := a.Projector(ctx, root)
	if err != nil {
		return res, err
	}
	store := voiceWriter(w.With(projector.Origin{By: "voice save"}))
	if store == nil {
		return res, projectdb.ErrNoStore
	}

	target, err := a.VoiceProfileTargetAt(ctx, root, point)
	if err != nil {
		return res, err
	}
	if !target.Writable {
		return res, errors.New(target.Reason)
	}

	held, herr := lookupProfileIn(ctx, store, target.ID)
	prof.ID, prof.Scope = target.ID, LocalScope
	if herr != nil {
		if err := store.CreateProfile(ctx, prof); err != nil {
			return res, fmt.Errorf("create voice profile %s: %w", prof.ID, err)
		}
		res.Created, res.Changed = true, true
	} else {
		// The store owns the version, so a caller holding an older copy of the
		// profile is saying what to write rather than which version to write.
		prof.Version = held.Version
		same, cerr := sameAuthoredVoice(held, prof)
		if cerr != nil {
			return res, cerr
		}
		if !same {
			if err := store.UpdateProfile(ctx, prof); err != nil {
				return res, fmt.Errorf("write voice profile %s: %w", prof.ID, err)
			}
			res.Changed = true
		}
	}
	res.ID, res.Name = prof.ID, prof.Name

	// A point that had no voice of its own has one from here on, and the recipe
	// is where that is declared.
	if target.Inherited || !target.Exists {
		if err := bindVoiceProfile(recipePath, point.Profile, prof.ID); err != nil {
			return res, err
		}
	}
	if !res.Changed {
		return res, nil
	}
	op, err := a.recordVoiceWrite(ctx, recipePath, actor, prof,
		"saved whole", "saved in the voice editor")
	if err != nil {
		return res, err
	}
	res.Recorded = op
	return res, nil
}

// voiceBindingAt returns the voice binding in force at a point and whether the
// point declares it itself. A profile that declares none reads the project's.
func voiceBindingAt(proj *project.KapiProject, profile string) (*project.VoiceBinding, bool) {
	if profile == "" {
		return proj.Defaults.Voice, true
	}
	if pr, ok := proj.Profiles[profile]; ok && pr.Voice != nil {
		return pr.Voice, true
	}
	return proj.Defaults.Voice, false
}

// voiceFieldName names the recipe key a binding was declared on, for a message
// a person reads beside their recipe.
func voiceFieldName(profile string, own bool) string {
	if profile == "" || !own {
		return project.DefaultVoiceField
	}
	return "profiles." + profile + ".voice"
}

// newVoiceProfileID settles the id a save opens for a point that binds nothing:
// the profile's own name, or the directory the project sits in, which is the
// name boundVoiceProfileForWrite opens a project's first profile under.
func newVoiceProfileID(root, profile string) string {
	name := profile
	if name == "" {
		name = filepath.Base(root)
	}
	id := slugify(name)
	if id == "" || id == "." || id == string(filepath.Separator) {
		id = "voice"
	}
	return id
}

// bindVoiceProfile declares in the recipe which stored profile governs a point,
// leaving a binding that already names it alone.
//
// A point that binds nothing gains the two lines of the binding and nothing
// else changes in the file (project.BindVoice). A point that binds another
// voice has that binding replaced, through project.Save.
func bindVoiceProfile(recipePath, profile, id string) error {
	proj, err := project.LoadWithOptions(recipePath, project.LoadOptions{SkipRequiresCheck: true})
	if err != nil {
		return fmt.Errorf("load project: %w", err)
	}
	current := proj.Defaults.Voice
	if profile != "" {
		pr, ok := proj.Profiles[profile]
		if !ok {
			return fmt.Errorf("voice: this project declares no profile %q", profile)
		}
		current = pr.Voice
	}
	if current != nil && current.Profile == id {
		return nil
	}
	if current == nil {
		if err := project.BindVoice(recipePath, profile, id); err != nil {
			return fmt.Errorf("bind voice profile: %w", err)
		}
		return nil
	}
	binding := &project.VoiceBinding{Profile: id}
	if profile == "" {
		proj.Defaults.Voice = binding
	} else {
		pr := proj.Profiles[profile]
		pr.Voice = binding
		proj.Profiles[profile] = pr
	}
	if err := project.Save(recipePath, proj); err != nil {
		return fmt.Errorf("bind voice profile: %w", err)
	}
	return nil
}
