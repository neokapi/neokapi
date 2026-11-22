package projector

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/workspace"
)

// VoiceWrite returns the voice.write operation of project key that puts each
// profile of put into the project's voice store and removes the profiles
// remove names, as recorded at at. A profile is stored under its ID, which a
// checkout spells from its name (coreprofile.Slug).
//
// It is for a writer outside the project's checkouts, which records the
// operation in the log a checkout merges. Such a writer cannot know which
// profiles a checkout holds, so a put creates a profile the checkout lacks and
// updates the one it holds, and removing a profile the checkout lacks is not
// an error. The operation has no ID; the writer names it when recording it.
func VoiceWrite(key workspace.ProjectKey, at time.Time, origin Origin, put []*coreprofile.VoiceProfile, remove []string) (workspace.Op, error) {
	if len(put) == 0 && len(remove) == 0 {
		return workspace.Op{}, errors.New("projector: a voice write puts or removes a profile")
	}
	s := step{DeleteProfiles: remove}
	for _, prof := range put {
		if prof == nil || prof.ID == "" {
			return workspace.Op{}, errors.New("projector: a voice write puts a profile with an id")
		}
		copied := *prof
		s.CreateProfiles = append(s.CreateProfiles, &copied)
	}
	at = at.UTC()
	s.stamp(at)
	body, err := json.Marshal(payload{Origin: origin, Steps: []step{s}})
	if err != nil {
		return workspace.Op{}, fmt.Errorf("projector: encode %s: %w", KindVoice, err)
	}
	return workspace.Op{Project: key, Kind: KindVoice, Payload: body, At: at}, nil
}
