// Package output provides output formatting for the Bowrain CLI.
// Shared formatting (Print, ResolveFormat, AddFlags) is delegated to
// platform/cli/output. This package keeps Bowrain CLI-specific output types.
package output

import (
	shared "github.com/neokapi/neokapi/cli/output"
	"github.com/spf13/cobra"
)

// Re-export shared types.
type Format = shared.Format

const (
	FormatText = shared.FormatText
	FormatJSON = shared.FormatJSON
)

// Re-export shared functions so bowrain commands can use output.Print etc.
func AddFlags(cmd *cobra.Command)                           { shared.AddFlags(cmd) }
func AddPersistentFlags(cmd *cobra.Command)                 { shared.AddPersistentFlags(cmd) }
func ResolveFormat(cmd *cobra.Command) Format               { return shared.ResolveFormat(cmd) }

// Deprecated: GetFormat is an alias for ResolveFormat.
func GetFormat(cmd *cobra.Command) Format                   { return shared.ResolveFormat(cmd) }
func Print(cmd *cobra.Command, data any) error              { return shared.Print(cmd, data) }
func PrintError(cmd *cobra.Command, err error, code string) { shared.PrintError(cmd, err, code) }
