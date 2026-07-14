// Package output provides output formatting for the Bowrain CLI.
// Shared formatting (Print, ResolveFormat, AddFlags) is delegated to
// platform/cli/output. This package keeps Bowrain CLI-specific output types.
package output

import (
	"io"

	shared "github.com/neokapi/neokapi/host/output"
	"github.com/spf13/cobra"
)

// Re-export shared types.
type (
	Format = shared.Format
	Table  = shared.Table
	Styles = shared.Styles
)

const (
	FormatText = shared.FormatText
	FormatJSON = shared.FormatJSON
)

// Re-export the shared text primitives so bowrain's FormatText implementations
// render with the same table, theme and spacing as the kapi ones.
func NewTable(w io.Writer) *Table               { return shared.NewTable(w) }
func Theme(w io.Writer) *Styles                 { return shared.Theme(w) }
func Title(w io.Writer, text string)            { shared.Title(w, text) }
func Note(w io.Writer, format string, a ...any) { shared.Note(w, format, a...) }

// Re-export shared functions so bowrain commands can use output.Print etc.
func AddFlags(cmd *cobra.Command)             { shared.AddFlags(cmd.Flags()) }
func AddPersistentFlags(cmd *cobra.Command)   { shared.AddPersistentFlags(cmd.PersistentFlags()) }
func ResolveFormat(cmd *cobra.Command) Format { return shared.ResolveFormat(cmd) }

// Deprecated: GetFormat is an alias for ResolveFormat.
func GetFormat(cmd *cobra.Command) Format                   { return shared.ResolveFormat(cmd) }
func Print(cmd *cobra.Command, data any) error              { return shared.Print(cmd, data) }
func PrintError(cmd *cobra.Command, err error, code string) { shared.PrintError(cmd, err, code) }
