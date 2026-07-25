package cli

import (
	"strings"

	"github.com/spf13/cobra"
)

// UnknownCommandHandler is consulted by Run when execution failed because the
// verb the user typed is not in the command tree. verb is that verb and args
// is everything the user wrote after it.
//
// It returns handled=false to leave cobra's original error in place, or
// handled=true with the error the process should exit on — nil when the
// handler resolved the situation and the command has since run to completion.
type UnknownCommandHandler func(cmd *cobra.Command, verb string, args []string) (handled bool, err error)

// unknownCommandHandler, when set, gets a chance to replace cobra's bare
// "unknown command" failure with something actionable — most usefully, an
// offer to install the plugin that provides the verb. Binaries that never
// call SetUnknownCommandHandler keep cobra's behavior exactly.
var unknownCommandHandler UnknownCommandHandler

// SetUnknownCommandHandler installs the handler Run consults on an unknown
// command. kapi/cmd/kapi wires the missing-plugin offer here.
func SetUnknownCommandHandler(fn UnknownCommandHandler) {
	unknownCommandHandler = fn
}

// unknownCommandVerb extracts the verb from cobra's unknown-command error.
// Cobra formats it as `unknown command "push" for "kapi"`, so the first
// quoted token is the verb the user typed. Returns ok=false for any other
// error, which is what keeps this off the hot path of ordinary failures.
func unknownCommandVerb(err error) (string, bool) {
	if err == nil {
		return "", false
	}
	msg := err.Error()
	if !strings.HasPrefix(msg, "unknown command ") {
		return "", false
	}
	rest := msg[len("unknown command "):]
	if !strings.HasPrefix(rest, `"`) {
		return "", false
	}
	rest = rest[1:]
	end := strings.IndexByte(rest, '"')
	if end <= 0 {
		return "", false
	}
	return rest[:end], true
}

// argsAfterVerb returns everything the user wrote after verb in argv — the
// raw tail a plugin command would have received, since plugin commands parse
// their own flags (pluginattach sets DisableFlagParsing). Global flags written
// before the verb stay with kapi, exactly as they would have.
func argsAfterVerb(argv []string, verb string) []string {
	for i, a := range argv {
		if a == verb {
			return argv[i+1:]
		}
	}
	return nil
}
