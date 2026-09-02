//go:build !js

package cli

import (
	"errors"

	"github.com/charmbracelet/huh"

	"github.com/neokapi/neokapi/host"
)

// The setup wizard's terminal experience.
//
// The founder asked whether this should move to bubbletea. It should — and the
// repo already had the answer: `kapi init` (bowrain plugin) drives its flow with
// charmbracelet/huh, which is bubbletea underneath. Extending that convention
// rather than hand-rolling a second bubbletea program keeps one interactive idiom
// in the CLI, and huh already solves what a raw Model/Update/View would have to:
// arrow-key selection with a visible cursor, inline validation, masked input for
// a secret, Esc/Ctrl-C cancellation, and a graceful fall back to a plain prompt
// when the terminal cannot support it.
//
// It lives in cli/ (not host/) because it is presentation: host owns detection,
// the ordered choices, the live round-trip and persistence, and stays free of a
// TUI dependency — which matters because Kapi Desktop links host too and has no
// use for a terminal form.
//
// # Why the build tag
//
// This module also compiles to js/wasm — `kapi/cmd/kapi-wasm-cli` is the CLI
// behind the docs playground — and bubbletea does not build for that target (it
// reaches for a TTY, terminal resize signals, and the system clipboard, none of
// which exist there). The tag is not a workaround for that: a browser has no
// terminal, so a terminal form is meaningless in the wasm build, and excluding it
// also keeps huh out of a payload users download. See aisetupform_js.go.

// formPrompter renders the wizard's questions as huh forms.
type formPrompter struct{}

// SelectProvider shows the detected providers as an arrow-key list. The labels
// are host's, so the ordering and the "detected" annotations stay one decision
// made in one place.
func (formPrompter) SelectProvider(choices []host.AISetupChoice, def int) (int, error) {
	if len(choices) == 0 {
		return 0, errors.New("no AI providers available to choose from")
	}
	opts := make([]huh.Option[int], 0, len(choices))
	for i, c := range choices {
		opts = append(opts, huh.NewOption(c.Label(), i))
	}
	selected := 0
	if def >= 0 && def < len(choices) {
		selected = def
	}
	err := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[int]().
				Title("Choose the default AI provider").
				Description("Used when a run names no --provider. Change it later with `kapi models default`.").
				Options(opts...).
				Value(&selected),
		),
	).Run()
	if err != nil {
		return 0, err
	}
	return selected, nil
}

// APIKey asks for a provider key with the input masked — the previous prompt
// echoed it, which is the wrong default for a secret being pasted into a
// terminal that may be shared or recorded.
func (formPrompter) APIKey(providerLabel string) (string, error) {
	var key string
	err := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title(providerLabel + " API key").
				Description("Stored in the OS keychain, never in the recipe.").
				EchoMode(huh.EchoModePassword).
				Value(&key).
				Validate(func(s string) error {
					if s == "" {
						return errors.New("a key is required, or press Esc and set the provider's API-key env var instead")
					}
					return nil
				}),
		),
	).Run()
	if err != nil {
		return "", err
	}
	return key, nil
}

// Confirm asks a yes/no question.
func (formPrompter) Confirm(prompt string, def bool) (bool, error) {
	value := def
	err := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().Title(prompt).Value(&value),
		),
	).Run()
	if err != nil {
		return false, err
	}
	return value, nil
}

// installAISetupPrompter registers the form prompter on the app, so every entry
// point into the wizard — `kapi models setup` and the inline one mid-`kapi up` —
// gets the same experience. Registered through the app-initializer hook rather
// than set per-command, precisely so the two cannot drift.
func installAISetupPrompter(a *App) {
	if a.AISetupPrompter == nil {
		a.AISetupPrompter = formPrompter{}
	}
}

func init() {
	RegisterAppInitializer(installAISetupPrompter)
}
