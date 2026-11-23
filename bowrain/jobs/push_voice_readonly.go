package jobs

import (
	"context"
	"errors"

	coreprofile "github.com/neokapi/neokapi/core/profile"
)

// errVoiceOutsideTransaction refuses a push's voice profile write that could
// only go through the pool while the push's transaction holds its locks.
var errVoiceOutsideTransaction = errors.New("this server cannot write a voice profile in a push: its voice store cannot join the push's transaction")

// readOnlyVoice is the voice store a push uses when the store cannot join
// the push's transaction. It reads, so a collection still binds the profile
// it names, and refuses every write, as the push refuses terms edits it
// cannot apply in its transaction.
type readOnlyVoice struct{ coreprofile.Store }

func (readOnlyVoice) CreateProfile(context.Context, *coreprofile.VoiceProfile) error {
	return errVoiceOutsideTransaction
}

func (readOnlyVoice) UpdateProfile(context.Context, *coreprofile.VoiceProfile) error {
	return errVoiceOutsideTransaction
}

func (readOnlyVoice) UpdateProfileAt(context.Context, *coreprofile.VoiceProfile, string) error {
	return errVoiceOutsideTransaction
}

func (readOnlyVoice) DeleteProfile(context.Context, string) error {
	return errVoiceOutsideTransaction
}
