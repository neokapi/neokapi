package host

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/neokapi/neokapi/core/contextop"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/project"
)

// What a whole-profile write changed, kept on the edit operation that records
// it (contextop.VoiceEdit): the fields that differ, the profile as it stood,
// and a fingerprint of what the write left, so dropping the edit can put the
// profile back.

// voiceEditOf compares the profile as it stood with the profile the store now
// holds. before is nil when the write created the profile.
func voiceEditOf(before, after *coreprofile.VoiceProfile) (*contextop.VoiceEdit, error) {
	if after == nil {
		return nil, errors.New("voice: no profile to compare")
	}
	return contextop.NewVoiceEdit(before, after)
}

// voiceFingerprint identifies a profile's content (contextop.VoiceFingerprint).
func voiceFingerprint(p *coreprofile.VoiceProfile) (string, error) {
	return contextop.VoiceFingerprint(p)
}

// voiceEditPhrase names what an edit changed for the record's one line.
func voiceEditPhrase(edit *contextop.VoiceEdit) string {
	return edit.Phrase()
}

// RecordedVoiceWrite returns the id of the latest operation on the project's
// context record that wrote prof as it now reads, and "" when none did: the
// profile was written outside kapi's write paths, or the write that left it so
// was never recorded. A push carries the id beside the profile, so the server
// writes the two as one change and records no second operation.
func (a *App) RecordedVoiceWrite(ctx context.Context, root string, prof *coreprofile.VoiceProfile) (string, error) {
	if prof == nil {
		return "", nil
	}
	wrote, err := voiceFingerprint(prof)
	if err != nil {
		return "", err
	}
	s, err := a.contextOps(ctx, filepath.Join(root, project.RecipeFileName))
	if err != nil {
		return "", err
	}
	records, err := s.ledger.Records(ctx, contextop.Filter{Project: s.key, Kinds: []contextop.Kind{contextop.KindEdit}})
	if err != nil {
		return "", err
	}
	for _, r := range records {
		if r.Voice != nil && r.Voice.Profile == prof.ID && r.Voice.Wrote == wrote {
			return r.ID, nil
		}
	}
	return "", nil
}

// restoreVoiceEdit puts back the profile an edit replaced, for dropping the
// edit. It refuses when the profile has been written since, because restoring
// then would undo that later write too, and reports where it restored to.
func (s *contextOpsSession) restoreVoiceEdit(ctx context.Context, r contextop.Record, apply bool) (string, error) {
	edit := r.Voice
	if edit == nil || edit.Previous == "" {
		return "", nil
	}
	w, err := s.writer(ctx, r)
	if err != nil {
		return "", err
	}
	store := voiceWriter(w)
	if store == nil {
		return "", errors.New("voice: this project has no voice store")
	}
	current, err := store.GetProfile(ctx, edit.Profile)
	if err != nil {
		return "", fmt.Errorf("voice profile %s is no longer in the project's store, so dropping this edit cannot restore it", edit.Profile)
	}
	now, err := voiceFingerprint(current)
	if err != nil {
		return "", err
	}
	if now != edit.Wrote {
		return "", fmt.Errorf("voice profile %s was written after operation %s, so dropping it would undo that write too. Drop the later edit first", edit.Profile, contextop.ShortID(r.ID))
	}
	if !apply {
		return "", nil
	}
	prev, err := coreprofile.LoadProfileYAML(strings.NewReader(edit.Previous))
	if err != nil {
		return "", fmt.Errorf("read the earlier voice profile %s: %w", edit.Profile, err)
	}
	prev.ID, prev.Scope, prev.Version = current.ID, current.Scope, current.Version
	if prev.Constraints == nil {
		prev.Constraints = []coreprofile.Constraint{}
	}
	if err := store.UpdateProfile(ctx, prev); err != nil {
		return "", fmt.Errorf("restore voice profile %s: %w", edit.Profile, err)
	}
	s.changed = true
	return "voice profile " + edit.Profile + " restored to the version before", nil
}
