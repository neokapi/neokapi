package host

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/neokapi/neokapi/core/contextop"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/projector"
)

// TakeServerVoice writes a venue's version of a voice profile over the one
// the project's voice store holds, and records the write on the project's
// context record, as `kapi voice edit` records an edit. local is the profile
// as the project resolves it and server the venue's version; the fields a
// push carries are copied, so the next push sends the venue's version back
// unchanged.
//
// A voice that does not come from the project's store, such as a starter
// pack, is refused: the store is not what resolves it, so writing there would
// change nothing a run reads.
func (a *App) TakeServerVoice(ctx context.Context, recipePath, root, source string, local, server *coreprofile.VoiceProfile) (string, error) {
	if local == nil || server == nil {
		return "", errors.New("voice: no profile to take")
	}
	if !strings.HasPrefix(source, "store:") {
		return "", fmt.Errorf("voice: the profile %q comes from %s, not the project's voice store, so the server's version cannot be taken here; bind a stored profile, or change the source", local.Name, source)
	}
	if err := a.mayWriteVoice(contextop.Actor{}); err != nil {
		return "", err
	}
	w, err := a.Projector(ctx, root)
	if err != nil {
		return "", err
	}
	store := voiceWriter(w.With(projector.Origin{By: "pull --take-server"}))
	if store == nil {
		return "", errors.New("voice: this project has no voice store")
	}
	before, err := lookupProfileIn(ctx, store, local.ID)
	if err != nil {
		return "", err
	}
	taken := *before
	taken.Description = server.Description
	taken.Tone = server.Tone
	taken.Style = server.Style
	taken.Examples = server.Examples
	taken.Locales = server.Locales
	taken.Channels = server.Channels
	taken.Personas = server.Personas
	taken.MinScore = server.MinScore
	taken.Constraints = server.Constraints
	if taken.Constraints == nil {
		taken.Constraints = []coreprofile.Constraint{}
	}
	if err := store.UpdateProfile(ctx, &taken); err != nil {
		return "", fmt.Errorf("write voice profile %s: %w", before.ID, err)
	}
	stored, err := store.GetProfile(ctx, before.ID)
	if err != nil {
		return "", fmt.Errorf("read voice profile %s back: %w", before.ID, err)
	}
	edit, err := voiceEditOf(before, stored)
	if err != nil {
		return "", err
	}
	return a.recordVoiceWrite(ctx, recipePath, contextop.Actor{}, edit,
		"taken from the server", "taken from the server with `kapi pull --take-server`")
}
