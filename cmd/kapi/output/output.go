// Package output provides consistent output formatting for the kapi CLI.
// All commands use this package to output results in either text or JSON format.
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

// AddFlags registers output format flags on the given command.
// This adds --json, --text, and --output-format/-o flags.
func AddFlags(cmd *cobra.Command) {
	cmd.Flags().Bool("json", false, "Output in JSON format")
	cmd.Flags().Bool("text", false, "Output in text format (default)")
	cmd.Flags().StringP("output-format", "o", "", "Output format: json, text")
}

// AddPersistentFlags registers output format flags as persistent flags.
// Use this on the root command to make flags available to all subcommands.
func AddPersistentFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().Bool("json", false, "Output in JSON format")
	cmd.PersistentFlags().Bool("text", false, "Output in text format (default)")
	cmd.PersistentFlags().StringP("output-format", "o", "", "Output format: json, text")
}

// GetFormat resolves the output format from command flags.
// Precedence: --json > --text > --output-format > default (text)
func GetFormat(cmd *cobra.Command) Format {
	// Check --json flag first (convenience shorthand)
	if jsonFlag, _ := cmd.Flags().GetBool("json"); jsonFlag {
		return FormatJSON
	}

	// Check --text flag
	if textFlag, _ := cmd.Flags().GetBool("text"); textFlag {
		return FormatText
	}

	// Check --output-format flag
	if format, _ := cmd.Flags().GetString("output-format"); format != "" {
		switch format {
		case "json":
			return FormatJSON
		case "text":
			return FormatText
		default:
			// Invalid format, fall through to default
		}
	}

	return FormatText
}

// Print outputs data in the format specified by command flags.
func Print(cmd *cobra.Command, data any) error {
	return PrintTo(os.Stdout, GetFormat(cmd), data)
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
	// If the data implements TextFormatter, use it
	if tf, ok := data.(TextFormatter); ok {
		return tf.FormatText(w)
	}

	// Fallback: pretty-print as JSON
	return printJSON(w, data)
}

// Error represents a structured error for JSON output.
type Error struct {
	Error string `json:"error"`
	Code  string `json:"code,omitempty"`
}

// PrintError outputs an error in the appropriate format.
func PrintError(cmd *cobra.Command, err error, code string) {
	format := GetFormat(cmd)
	if format == FormatJSON {
		e := Error{Error: err.Error(), Code: code}
		printJSON(os.Stderr, e)
	} else {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	}
}
