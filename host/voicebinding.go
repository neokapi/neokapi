package host

import (
	"errors"
	"fmt"
	"slices"

	"github.com/neokapi/neokapi/core/profile/packs"
)

// ErrVoiceProfileNotFound reports a voice profile the store holds under no id,
// slug or name matching the one asked for.
var ErrVoiceProfileNotFound = errors.New("voice profile not found")

// voiceProfileNotFound is the error for a name the voice store does not hold.
//
// It points at what can put a profile there. `kapi voice pack <name>` installs
// a built-in starter pack, so it is suggested only when a pack carries that
// name; for any other name it would install something else entirely.
func voiceProfileNotFound(name string) error {
	hint := "`kapi voice profiles` lists what it holds"
	if names, err := packs.List(); err == nil && slices.Contains(names, name) {
		hint = fmt.Sprintf("`kapi voice pack %s` installs the starter pack of that name, and %s", name, hint)
	}
	return fmt.Errorf("%w: the store holds no profile %q (%s)", ErrVoiceProfileNotFound, name, hint)
}

// boundVoiceNotHeld is the error for a recipe that binds a voice by name on a
// machine whose store does not hold it.
//
// The recipe travels with the checkout and the store does not, so the ordinary
// cause is a project whose context has not been brought to this machine: a
// fresh clone, a new data root, another person's laptop. The fix is to bring
// the context, not to install something under the same name.
func boundVoiceNotHeld(field, name string, err error) error {
	if !errors.Is(err, ErrVoiceProfileNotFound) {
		return err
	}
	return fmt.Errorf("%s binds voice profile %q, and this machine's store for the project does not hold it: "+
		"the project's context has not been imported or restored here. "+
		"Run `kapi context import` in a checkout that carries the profile, or `kapi context restore` from an export "+
		"(`kapi voice profiles` lists what the store holds): %w",
		field, name, ErrVoiceProfileNotFound)
}
