package backend

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	aiprovider "github.com/neokapi/neokapi/providers/ai"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/host"
	"github.com/neokapi/neokapi/host/credentials"
)

// A run failure reaches the UI as a chain of wrapping prefixes:
//
//	converge nb: flow execution error: notes.md: execute flow: tool translate:
//	  translate: ollama: model "gemma4:e2b" is not installed — pull it first …
//
// Every layer of that is true and none of it is what a person needs. What they
// need is: what broke ("Model gemma4:e2b isn't installed"), what fixes it
// (`ollama pull gemma4:e2b`), where it happened (which file, which locale), and
// — only if they ask — the raw chain.
//
// RunError is that decomposition. It is derived by *unwrapping typed errors*
// (errors.As against the provider's own error types), never by matching on the
// rendered prose: the sentence is UI copy and may be reworded or translated,
// while the type is a contract. The locale/file metadata is peeled off the
// wrapping layers, which is the one place a string is unavoidable — those
// prefixes are formatting, not classification, and a miss only costs a metadata
// field, never a misclassification.

// RunErrorKind classifies a run failure. Values are stable wire strings.
type RunErrorKind string

const (
	// RunErrOllamaUnreachable — no Ollama server at the configured URL.
	RunErrOllamaUnreachable RunErrorKind = "ollama-unreachable"
	// RunErrModelNotInstalled — the local model has not been pulled.
	RunErrModelNotInstalled RunErrorKind = "model-not-installed"
	// RunErrAmbiguousCredential — several AI credentials match; none is default.
	RunErrAmbiguousCredential RunErrorKind = "ambiguous-credential"
	// RunErrMissingCredential — the provider needs an API key and none is set.
	RunErrMissingCredential RunErrorKind = "missing-credential"
	// RunErrCanceled — the user stopped the run.
	RunErrCanceled RunErrorKind = "canceled"
	// RunErrBlockedTargetPath — a target path cannot be written (a directory
	// occupies it, the directory denies writing, the filesystem is read-only).
	// It is an obstruction to clear on disk, not a translation problem, so it
	// must never be presented as pending target-language work (#1449).
	RunErrBlockedTargetPath RunErrorKind = "blocked-target-path"
	// RunErrUnknown — no confident classification; headline is the raw message.
	RunErrUnknown RunErrorKind = "unknown"
)

// RunErrorActionKind tells the UI what affordance to render for a remedy.
type RunErrorActionKind string

const (
	// ActionCommand — a shell command to run; the UI offers it copyable.
	ActionCommand RunErrorActionKind = "command"
	// ActionOpenSettings — navigate to an in-app view (Target is a View id from
	// the frontend's sidebar union, e.g. "settings", which hosts AI Models).
	ActionOpenSettings RunErrorActionKind = "open-settings"
	// ActionOpenURL — open an external page (install docs, downloads).
	ActionOpenURL RunErrorActionKind = "open-url"
)

// RunErrorAction is one thing the user can do about the failure.
type RunErrorAction struct {
	Kind RunErrorActionKind `json:"kind"`
	// Label is the button text.
	Label string `json:"label"`
	// Command is the shell command for ActionCommand.
	Command string `json:"command,omitempty"`
	// URL is the destination for ActionOpenURL.
	URL string `json:"url,omitempty"`
	// Target is the in-app view id for ActionOpenSettings.
	Target string `json:"target,omitempty"`
}

// RunError is a run failure presented as structure rather than prose.
type RunError struct {
	Kind RunErrorKind `json:"kind"`
	// Headline is the short human sentence — the primary line in the UI.
	Headline string `json:"headline"`
	// Remediation is one sentence saying what to do about it ("" when the
	// failure has no known remedy).
	Remediation string `json:"remediation,omitempty"`
	// Actions are the concrete affordances for the remediation.
	Actions []RunErrorAction `json:"actions,omitempty"`
	// Locale / File / Provider / Model are the affected scope, when known.
	Locale   string `json:"locale,omitempty"`
	File     string `json:"file,omitempty"`
	Provider string `json:"provider,omitempty"`
	Model    string `json:"model,omitempty"`
	// Raw is the full wrapped error chain, for the "Details" disclosure.
	Raw string `json:"raw"`
}

// classifyRunError decomposes a run failure into a RunError. It never returns
// nil for a non-nil error: an unclassified failure still gets its raw message
// as the headline, so nothing is ever swallowed.
func classifyRunError(err error) *RunError {
	if err == nil {
		return nil
	}
	re := &RunError{Kind: RunErrUnknown, Raw: err.Error()}
	re.Locale, re.File = runErrorScope(err)

	if errors.Is(err, context.Canceled) {
		re.Kind = RunErrCanceled
		re.Headline = "Run canceled"
		return re
	}

	// A destination the filesystem refuses (#1449). Classified BEFORE the
	// provider errors because it is not a translation failure at all: the run
	// never got as far as producing content, and the remedy is on disk. #1442
	// made a reported failure accurate; this makes it legible — without it the
	// blocked path lands in RunErrUnknown and the UI has no remedy to offer.
	var ope *flow.OutputPathError
	if errors.As(err, &ope) {
		re.Kind = RunErrBlockedTargetPath
		re.File = ope.Path
		re.Headline = blockedPathHeadline(ope)
		re.Remediation = "Clear whatever occupies the path (or point the collection's target at a different one), then run again."
		re.Actions = []RunErrorAction{
			{Kind: ActionOpenSettings, Label: "Open project settings", Target: "settings"},
		}
		return re
	}

	var oe *aiprovider.OllamaError
	if errors.As(err, &oe) {
		re.Provider = string(aiprovider.Ollama)
		re.Model = oe.Model
		switch oe.Kind {
		case aiprovider.OllamaErrUnreachable:
			re.Kind = RunErrOllamaUnreachable
			re.Headline = "Ollama isn't running"
			re.Remediation = fmt.Sprintf("Start Ollama, then run again. Kapi expects it at %s.", oe.BaseURL)
			re.Actions = []RunErrorAction{
				{Kind: ActionCommand, Label: "Start Ollama", Command: "ollama serve"},
				{Kind: ActionOpenURL, Label: "Install Ollama", URL: "https://ollama.com"},
			}
			return re
		case aiprovider.OllamaErrModelNotInstalled:
			re.Kind = RunErrModelNotInstalled
			re.Headline = fmt.Sprintf("Model %s isn't installed", oe.Model)
			re.Remediation = "Pull the model, then run again, or pick a different one in Settings → AI Models."
			re.Actions = []RunErrorAction{
				{Kind: ActionCommand, Label: "Pull the model", Command: oe.Remediation()},
				{Kind: ActionOpenSettings, Label: "Choose another model", Target: "settings"},
			}
			return re
		}
		// OllamaErrOther — the server answered, but not usefully.
		re.Headline = oe.Error()
		return re
	}

	var amb *credentials.AmbiguousCredentialError
	if errors.As(err, &amb) {
		re.Kind = RunErrAmbiguousCredential
		re.Headline = "More than one AI credential matches"
		re.Remediation = fmt.Sprintf(
			"Set a default in Settings → AI Models, or pick one on this flow step. Candidates: %s.",
			strings.Join(amb.Candidates, ", "))
		re.Actions = []RunErrorAction{
			{Kind: ActionOpenSettings, Label: "Open AI Models", Target: "settings"},
		}
		return re
	}

	var tb *host.FlowToolBuildError
	if errors.As(err, &tb) {
		if re.Locale == "" {
			re.Locale = tb.Locale
		}
		// A tool that could not be built for want of a key is the other common
		// setup failure. The resolver has no typed error for it, so this is the
		// one place a phrase is consulted — and only to *upgrade* an already
		// unknown classification, never to override a typed one.
		if isMissingCredentialMessage(tb.Err) {
			re.Kind = RunErrMissingCredential
			re.Headline = fmt.Sprintf("No API key for the %s step", tb.Tool)
			re.Remediation = "Add a credential in Settings → AI Models, or switch to a local model."
			re.Actions = []RunErrorAction{
				{Kind: ActionOpenSettings, Label: "Open AI Models", Target: "settings"},
			}
			return re
		}
		re.Headline = fmt.Sprintf("The %s step could not start", tb.Tool)
		re.Remediation = tb.Err.Error()
		return re
	}

	re.Headline = firstLine(err.Error())
	return re
}

// blockedPathHeadline is the one-line headline for a refused destination. It
// switches on the typed Kind, never on the error's prose, and names the file
// rather than the whole path so the headline stays one short sentence (the full
// path rides in File, and the full chain in Raw).
func blockedPathHeadline(ope *flow.OutputPathError) string {
	name := filepath.Base(ope.Path)
	switch ope.Kind {
	case flow.OutputPathIsDir:
		return "A directory occupies " + name
	case flow.OutputPathIrregular:
		return name + " is not a regular file"
	case flow.OutputPathParentNotDir:
		return name + " cannot be created: its folder is blocked"
	case flow.OutputPathPermission:
		return "No permission to write " + name
	case flow.OutputPathReadOnly:
		return name + " is on a read-only filesystem"
	default:
		return name + " could not be written"
	}
}

// isMissingCredentialMessage reports the "no saved credentials" family the
// credential resolver emits untyped.
func isMissingCredentialMessage(err error) bool {
	if err == nil {
		return false
	}
	l := strings.ToLower(err.Error())
	return strings.Contains(l, "no saved credential") ||
		strings.Contains(l, "no credential") ||
		strings.Contains(l, "api key") && strings.Contains(l, "not")
}

// runErrorScope peels the locale and file out of the orchestrator's wrapping
// prefixes: `converge <locale>: ` from the convergence engine, `<file>: ` from
// the multi-file runner, and `<file> [<locale>]: ` from the desktop flow runner.
func runErrorScope(err error) (locale, file string) {
	msg := err.Error()
	if rest, ok := strings.CutPrefix(msg, "converge "); ok {
		if i := strings.Index(rest, ":"); i > 0 {
			locale = strings.TrimSpace(rest[:i])
			msg = rest[i+1:]
		}
	}
	if _, after, found := strings.Cut(msg, "flow execution error:"); found {
		rest := strings.TrimSpace(after)
		if before, _, ok := strings.Cut(rest, ": execute flow:"); ok {
			file = strings.TrimSpace(before)
		}
	}
	// The desktop flow runner's shape: "<file> [<locale>]: <cause>".
	if file == "" {
		if j := strings.Index(msg, " ["); j > 0 {
			if k := strings.Index(msg[j:], "]:"); k > 0 {
				file = strings.TrimSpace(msg[:j])
				if locale == "" {
					locale = strings.TrimSpace(msg[j+2 : j+k])
				}
			}
		}
	}
	return locale, file
}

// firstLine trims a multi-line error down to its first line — a headline is one
// sentence; the rest stays in Raw.
func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if first, _, ok := strings.Cut(s, "\n"); ok {
		return strings.TrimSpace(first)
	}
	return s
}
