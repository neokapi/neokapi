package cli

import "github.com/spf13/cobra"

// AnnotationReference is the cobra annotation that keeps a command hidden from
// the root help and still gives it a page in the command reference. The
// reference generator reads it from the command; it keeps no list of its own.
const AnnotationReference = "kapi.reference"

// referenceListed is the value of AnnotationReference on a listed command.
const referenceListed = "listed"

// ListInReference marks a hidden command for the command reference, and
// returns it.
func ListInReference(cmd *cobra.Command) *cobra.Command {
	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}
	cmd.Annotations[AnnotationReference] = referenceListed
	return cmd
}

// InReference reports whether cmd gets a page in the command reference: every
// visible command, and a hidden one marked with ListInReference.
func InReference(cmd *cobra.Command) bool {
	return !cmd.Hidden || cmd.Annotations[AnnotationReference] == referenceListed
}
