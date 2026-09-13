package host

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/neokapi/neokapi/core/check"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/profile/packs"
	"github.com/neokapi/neokapi/host/output"
)

// VoiceProfileWarnings reports the configuration warnings of the voice profile a
// resolver loaded from source: each key its YAML carries that the profile model
// does not define, and each advisory problem ValidateProfile finds in it.
//
// source is the string the voice resolvers return beside a profile: a file
// path, "pack:<name>" or "store:<name>". The profile is read again from there,
// as authored, because a resolver hands back the profile with its locale,
// channel and persona overrides applied, and a warning about an override names
// the profile as it was written. A store profile is looked up in store; with a
// nil store there is nowhere to look, and it reports none.
func (a *App) VoiceProfileWarnings(ctx context.Context, store coreprofile.Store, source string) ([]check.Warning, error) {
	var (
		problems []coreprofile.ProfileProblem
		authored *coreprofile.VoiceProfile
		err      error
	)
	shown := source
	switch {
	case source == "":
		return nil, nil
	case strings.HasPrefix(source, "pack:"):
		authored, err = packs.Load(strings.TrimPrefix(source, "pack:"))
	case strings.HasPrefix(source, "store:"):
		if store == nil {
			return nil, nil
		}
		authored, err = lookupProfileIn(ctx, store, strings.TrimPrefix(source, "store:"))
	default:
		shown = displayRelative(source)
		data, rerr := os.ReadFile(source)
		if rerr != nil {
			return nil, fmt.Errorf("read voice profile %s: %w", shown, rerr)
		}
		if problems, err = coreprofile.UnknownKeys(data); err == nil {
			authored, err = coreprofile.LoadProfileYAML(bytes.NewReader(data))
		}
	}
	if err != nil {
		return nil, fmt.Errorf("voice profile %s: %w", shown, err)
	}
	problems = append(problems, coreprofile.Advisory(coreprofile.ValidateProfile(authored))...)
	warnings := make([]check.Warning, 0, len(problems))
	for _, p := range problems {
		warnings = append(warnings, check.Warning{Code: p.Code, Message: p.Message, Source: shown, Key: p.Field})
	}
	return warnings, nil
}

// voiceWarnings gathers the warnings of every voice profile one run loads,
// reading each source once however many files it governs.
type voiceWarnings struct {
	seen map[string]bool
	list []check.Warning
}

// note collects the warnings of the profile loaded from source, unless the run
// has collected them already. A nil collector collects nothing.
func (w *voiceWarnings) note(ctx context.Context, app *App, store coreprofile.Store, source string) error {
	if w == nil || source == "" || w.seen[source] {
		return nil
	}
	found, err := app.VoiceProfileWarnings(ctx, store, source)
	if err != nil {
		return err
	}
	if w.seen == nil {
		w.seen = map[string]bool{}
	}
	w.seen[source] = true
	w.list = append(w.list, found...)
	return nil
}

// merged is what the run reports: each warning once, in a stable order.
func (w *voiceWarnings) merged() []check.Warning {
	if w == nil {
		return nil
	}
	return check.MergeWarnings(w.list)
}

// note collects the warnings of the profile the resolver loaded from source. A
// profile named from the store outside a project is looked up in the store the
// command selects.
func (v *checkVoice) note(ctx context.Context, source string) error {
	if v.warnings == nil {
		return nil
	}
	store := v.store
	if store == nil && strings.HasPrefix(source, "store:") && v.cmd != nil {
		lookup, release, err := v.app.VoiceLookupStore(v.cmd)
		if err != nil {
			return err
		}
		defer release()
		store = lookup
	}
	return v.warnings.note(ctx, v.app, store, source)
}

// writeWarnings lists configuration warnings under their own heading after the
// verdict, so none reads as a finding. Each names a profile, and the key in it,
// to fix.
func writeWarnings(w io.Writer, warnings []check.Warning) {
	if len(warnings) == 0 {
		return
	}
	fmt.Fprintln(w)
	output.Title(w, fmt.Sprintf("Configuration warnings (%d):", len(warnings)))
	t := output.NewTable(w).Headers("code", "location", "message")
	s := t.Styles()
	for _, warning := range warnings {
		location := warning.Source
		if warning.Key != "" {
			location += ": " + warning.Key
		}
		t.Row(s.Warn.Render(warning.Code), s.Dim(location), warning.Message)
	}
	t.Render()
	fmt.Fprintln(w, "  Warnings are separate from findings: they never change the score, the gate or the verdict.")
}
