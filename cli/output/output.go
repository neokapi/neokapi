// Package output provides consistent output formatting for neokapi CLI tools.
package output

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
)

// Format represents the output format for CLI commands.
type Format string

const (
	// FormatText outputs human-readable text (default).
	FormatText Format = "text"
	// FormatJSON outputs machine-readable JSON.
	FormatJSON Format = "json"
)

// TextFormatter is implemented by types that can render themselves as text.
// Types that implement this interface will have their FormatText method called
// when output format is text. Types without this interface fall back to JSON.
type TextFormatter interface {
	FormatText(w io.Writer) error
}

// TableFormattable is implemented by collector result data that can
// render itself as an aligned text table.
type TableFormattable interface {
	FormatTable(w io.Writer)
}

// AddFlags registers output format flags on the given command.
// This adds --json, --text, and --output-format flags.
func AddFlags(cmd *cobra.Command) {
	cmd.Flags().Bool("json", false, "Output in JSON format")
	cmd.Flags().Bool("text", false, "Output in text format (default)")
	cmd.Flags().String("output-format", "", "Output format: json, text")
}

// AddPersistentFlags registers output format flags as persistent flags.
// Use this on the root command to make flags available to all subcommands.
func AddPersistentFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().Bool("json", false, "Output in JSON format")
	cmd.PersistentFlags().Bool("text", false, "Output in text format (default)")
	cmd.PersistentFlags().String("output-format", "", "Output format: json, text")
}

// Format resolves the output format from command flags.
// Precedence: --json > --text > --output-format > default (text)
//
// Deprecated: GetFormat is an alias for Format kept for backward compatibility.
func GetFormat(cmd *cobra.Command) Format { return ResolveFormat(cmd) }

// ResolveFormat resolves the output format from command flags.
// Precedence: --json > --text > --output-format > default (text)
func ResolveFormat(cmd *cobra.Command) Format {
	if jsonFlag, _ := cmd.Flags().GetBool("json"); jsonFlag {
		return FormatJSON
	}
	if textFlag, _ := cmd.Flags().GetBool("text"); textFlag {
		return FormatText
	}
	if format, _ := cmd.Flags().GetString("output-format"); format != "" {
		switch format {
		case "json":
			return FormatJSON
		case "text":
			return FormatText
		}
	}
	return FormatText
}

// Print outputs data in the format specified by command flags.
func Print(cmd *cobra.Command, data any) error {
	return PrintTo(os.Stdout, ResolveFormat(cmd), data)
}

// PrintTo outputs data in the specified format to the given writer.
func PrintTo(w io.Writer, format Format, data any) error {
	switch format {
	case FormatJSON:
		return printJSON(w, data)
	default:
		return printText(w, data)
	}
}

// PrintJSON outputs data as JSON regardless of flags.
func PrintJSON(data any) error {
	return printJSON(os.Stdout, data)
}

// PrintText outputs data as text regardless of flags.
func PrintText(data any) error {
	return printText(os.Stdout, data)
}

func printJSON(w io.Writer, data any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(data)
}

func printText(w io.Writer, data any) error {
	if tf, ok := data.(TextFormatter); ok {
		return tf.FormatText(w)
	}
	return printJSON(w, data)
}

// Error represents a structured error for JSON output.
type Error struct {
	Error string `json:"error"`
	Code  string `json:"code,omitempty"`
}

// PrintError outputs an error in the appropriate format.
func PrintError(cmd *cobra.Command, err error, code string) {
	format := ResolveFormat(cmd)
	if format == FormatJSON {
		e := Error{Error: err.Error(), Code: code}
		_ = printJSON(os.Stderr, e)
	} else {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	}
}

// FormatCollectorResult writes a collector result to stdout.
// In JSON mode it marshals result data; in text mode it calls FormatTable
// if available, falling back to JSON.
func FormatCollectorResult(jsonMode bool, data any) error {
	if jsonMode {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(data)
	}

	if ft, ok := data.(TableFormattable); ok {
		ft.FormatTable(os.Stdout)
		return nil
	}

	out, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal result: %w", err)
	}
	fmt.Println(string(out))
	return nil
}
