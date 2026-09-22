package host

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/neokapi/neokapi/core/contextop"
	coreprofile "github.com/neokapi/neokapi/core/profile"
)

// Hand-authoring a voice profile that lives in a store.
//
// A profile is easiest to write as YAML and easiest to read as one document,
// which is what `kapi voice edit` gives a person: the stored profile written
// out as YAML, opened in their editor, checked the way `kapi voice validate`
// checks a file, and read back into the store as one recorded operation. The
// file is scratch. Everything that survives the command is in the store and on
// the project's context record.

// VoiceEditRequest names the profile to edit.
type VoiceEditRequest struct {
	// Profile is the profile's id or name in the project's voice store. Empty
	// edits the one the recipe binds at the project's default point.
	Profile string
}

// VoiceEdit reports what an edit did.
type VoiceEdit struct {
	// ID identifies the profile that was opened.
	ID string `json:"id"`
	// Name is the profile's label.
	Name string `json:"name,omitempty"`
	// Changed reports that the edited document differed from the one opened
	// and was read back into the store.
	Changed bool `json:"changed"`
	// Recorded is the id of the context operation the edit left on the
	// project's record, empty when nothing changed.
	Recorded string `json:"recorded,omitempty"`
}

// FormatText renders the edit for a reader.
func (r VoiceEdit) FormatText(w io.Writer) error {
	if !r.Changed {
		_, err := fmt.Fprintf(w, "Voice profile %s is unchanged.\n", r.ID)
		return err
	}
	_, err := fmt.Fprintf(w, "Read voice profile %s back into the project's store.\n", r.ID)
	return err
}

// ErrNoEditor reports an environment naming no editor to open.
var ErrNoEditor = errors.New("no editor: set $VISUAL or $EDITOR to the command that opens a file")

// EditVoiceProfile writes a stored voice profile out as YAML, opens it in the
// person's editor, and reads what they wrote back into the store.
//
// The whole document is read back, so a key the person deleted is gone from the
// profile: a full-profile edit states the profile entire, including its
// constraints, which is why an edit that carries no `constraints:` clears the
// ones the store held.
//
// An editor that exits non-zero, and a document that comes back as it went out,
// both leave the store and the record where they were.
func (a *App) EditVoiceProfile(ctx context.Context, cmd Command, req VoiceEditRequest) (VoiceEdit, error) {
	var res VoiceEdit

	recipePath, root, err := a.resolveProjectRoot(cmd)
	if err != nil {
		return res, err
	}
	db, err := a.ProjectDB(ctx, root)
	if err != nil {
		return res, err
	}
	store := db.Voice()
	if store == nil {
		return res, errors.New("voice: this project has no voice store")
	}

	var prof *coreprofile.VoiceProfile
	if name := strings.TrimSpace(req.Profile); name != "" {
		prof, err = lookupProfileIn(ctx, store, name)
		if err != nil {
			return res, fmt.Errorf("voice: this project's store holds no profile %q. `kapi voice profiles` lists the ones it holds", name)
		}
	} else if prof, err = a.boundVoiceProfileForWrite(ctx, db, recipePath, root); err != nil {
		return res, err
	}
	res.ID, res.Name = prof.ID, prof.Name

	opened, err := renderSnapshotProfile(nil, prof)
	if err != nil {
		return res, fmt.Errorf("render voice profile %s: %w", prof.ID, err)
	}
	path, err := writeEditScratch(prof.ID, opened)
	if err != nil {
		return res, err
	}
	keep := false
	defer func() {
		if !keep {
			_ = os.Remove(path)
		}
	}()

	if err := openInEditor(ctx, cmd, path); err != nil {
		return res, err
	}

	edited, err := os.ReadFile(path)
	if err != nil {
		return res, fmt.Errorf("read the edited profile: %w", err)
	}
	if bytes.Equal(edited, opened) {
		return res, nil
	}

	updated, err := readEditedProfile(edited)
	if err != nil {
		keep = true
		return res, fmt.Errorf("%w\nYour edit is at %s", err, path)
	}
	updated.ID, updated.Scope = prof.ID, prof.Scope
	if err := store.UpdateProfile(ctx, updated); err != nil {
		keep = true
		return res, fmt.Errorf("write voice profile %s: %w\nYour edit is at %s", prof.ID, err, path)
	}
	res.Changed, res.Name = true, updated.Name

	op, err := a.recordVoiceEdit(ctx, recipePath, updated)
	if err != nil {
		return res, err
	}
	res.Recorded = op
	return res, nil
}

// readEditedProfile parses and checks an edited document the way `kapi voice
// validate` checks a file, so a profile that cannot be read never reaches the
// store.
//
// Constraints come back explicit. The store reads a profile carrying none as
// "leave the ones you have", which is right for a partial write and wrong for a
// document that states the profile entire.
func readEditedProfile(data []byte) (*coreprofile.VoiceProfile, error) {
	prof, err := coreprofile.LoadProfileYAML(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("the edited profile does not parse: %w", err)
	}
	var problems []coreprofile.ProfileProblem
	if _, serr := coreprofile.DecodeProfileStrict(bytes.NewReader(data)); serr != nil {
		problems = append(problems, StrictDecodeProblems(serr)...)
	}
	problems = append(problems, coreprofile.ValidateProfile(prof)...)
	if len(problems) > 0 {
		lines := make([]string, 0, len(problems)+1)
		lines = append(lines, "the edited profile has problems:")
		for _, p := range problems {
			if p.Field != "" {
				lines = append(lines, "  "+p.Field+": "+p.Message)
				continue
			}
			lines = append(lines, "  "+p.Message)
		}
		return nil, errors.New(strings.Join(lines, "\n"))
	}
	if prof.Constraints == nil {
		prof.Constraints = []coreprofile.Constraint{}
	}
	return prof, nil
}

// recordVoiceEdit puts the edit on the project's context record, as the person
// who made it.
func (a *App) recordVoiceEdit(ctx context.Context, recipePath string, prof *coreprofile.VoiceProfile) (string, error) {
	return a.recordVoiceWrite(ctx, recipePath, contextop.Actor{}, prof,
		"edited whole", "edited with `kapi voice edit`")
}

// recordVoiceWrite puts a whole-profile write on the project's context record.
//
// actor says who wrote it. An empty kind leaves the environment to answer, the
// way a command line does; a surface a person drives states the person. what
// says what happened to the profile and how says where it came from.
func (a *App) recordVoiceWrite(
	ctx context.Context,
	recipePath string,
	actor contextop.Actor,
	prof *coreprofile.VoiceProfile,
	what, how string,
) (string, error) {
	s, err := a.contextOps(ctx, recipePath)
	if err != nil {
		return "", err
	}
	actor, note, err := s.actorFor(ctx, actor, how)
	if err != nil {
		return "", err
	}
	record, err := s.ledger.Append(ctx, s.stamp(contextop.Record{
		Actor: actor,
		Kind:  contextop.KindConfirm,
		Subject: contextop.Subject{
			Kind: contextop.SubjectNote,
			Text: "voice profile " + prof.ID + " " + what,
		},
		Note: note,
	}, nil))
	if err != nil {
		return "", teachRefusal(err)
	}
	return record.ID, nil
}

// writeEditScratch puts the document somewhere an editor can open it. It sits
// outside the project, because a file under `.kapi/` that an editor is holding
// open is a context file in the checkout for as long as the edit lasts.
func writeEditScratch(id string, data []byte) (string, error) {
	f, err := os.CreateTemp("", "kapi-voice-"+slugify(id)+"-*.yaml")
	if err != nil {
		return "", fmt.Errorf("open a file to edit: %w", err)
	}
	path := f.Name()
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return "", fmt.Errorf("write the profile to edit: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(path)
		return "", fmt.Errorf("write the profile to edit: %w", err)
	}
	return path, nil
}

// openInEditor runs the person's editor over path and waits for it.
//
// $VISUAL is asked first and $EDITOR second, the order every other command that
// opens an editor uses. The value is a command line rather than a bare program,
// so "code --wait" works.
func openInEditor(ctx context.Context, cmd Command, path string) error {
	editor := strings.TrimSpace(os.Getenv("VISUAL"))
	if editor == "" {
		editor = strings.TrimSpace(os.Getenv("EDITOR"))
	}
	if editor == "" {
		return ErrNoEditor
	}
	fields := strings.Fields(editor)
	run := exec.CommandContext(ctx, fields[0], append(fields[1:], path)...) //nolint:gosec // the editor is the person's own choice
	run.Stdin, run.Stdout, run.Stderr = os.Stdin, cmd.OutOrStdout(), cmd.ErrOrStderr()
	if err := run.Run(); err != nil {
		return fmt.Errorf("%s left the profile unedited: %w", fields[0], err)
	}
	return nil
}
