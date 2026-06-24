// Package output provides consistent output formatting for neokapi CLI tools.
package output

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/itchyny/gojq"
	"github.com/mattn/go-isatty"
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
	cmd.Flags().String("jq", "", "filter JSON output through a jq expression (implies --json)")
	cmd.Flags().String("color", "auto", "colorize JSON output: auto, always, never")
}

// AddPersistentFlags registers output format flags as persistent flags.
// Use this on the root command to make flags available to all subcommands.
func AddPersistentFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().Bool("json", false, "Output in JSON format")
	cmd.PersistentFlags().Bool("text", false, "Output in text format (default)")
	cmd.PersistentFlags().String("output-format", "", "Output format: json, text")
	cmd.PersistentFlags().String("jq", "", "filter JSON output through a jq expression (implies --json)")
	cmd.PersistentFlags().String("color", "auto", "colorize JSON output: auto, always, never")
}

// Format resolves the output format from command flags.
// Precedence: --json > --text > --output-format > default (text)
//
// Deprecated: GetFormat is an alias for Format kept for backward compatibility.
func GetFormat(cmd *cobra.Command) Format { return ResolveFormat(cmd) }

// ResolveFormat resolves the output format from command flags.
// Precedence: --json > --text > --output-format > default (text)
func ResolveFormat(cmd *cobra.Command) Format {
	// --jq filters JSON, so it implies JSON output.
	if jq, _ := cmd.Flags().GetString("jq"); jq != "" {
		return FormatJSON
	}
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

// Print outputs data in the format specified by command flags. It writes to
// cmd.OutOrStdout() / cmd.ErrOrStderr() so tests that call cmd.SetOut/SetErr
// can capture structured output. In JSON mode it honors --jq (filter) and
// --color.
func Print(cmd *cobra.Command, data any) error {
	w := cmd.OutOrStdout()
	if ResolveFormat(cmd) == FormatJSON {
		filter, _ := cmd.Flags().GetString("jq")
		return RenderJSON(w, data, filter, Colorize(cmd, w))
	}
	return printText(w, data)
}

// Colorize decides whether JSON output should be ANSI-colored. Precedence:
// --color always/never, then NO_COLOR / CLICOLOR_FORCE env, then whether
// the resolved writer w is a terminal.
//
// Pass cmd.OutOrStdout() as w so the check reflects the actual destination,
// not the process stdout (they differ when a test calls cmd.SetOut).
func Colorize(cmd *cobra.Command, w io.Writer) bool {
	switch c, _ := cmd.Flags().GetString("color"); c {
	case "always", "force":
		return true
	case "never", "none":
		return false
	}
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	if os.Getenv("CLICOLOR_FORCE") != "" {
		return true
	}
	if f, ok := w.(*os.File); ok {
		return isatty.IsTerminal(f.Fd())
	}
	return false
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

// truncate shortens s to at most n runes, appending an ellipsis when cut.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
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

// PrintError outputs an error in the appropriate format to cmd.ErrOrStderr()
// so tests using cmd.SetErr can capture error output.
func PrintError(cmd *cobra.Command, err error, code string) {
	w := cmd.ErrOrStderr()
	format := ResolveFormat(cmd)
	if format == FormatJSON {
		e := Error{Error: err.Error(), Code: code}
		_ = printJSON(w, e)
	} else {
		fmt.Fprintf(w, "Error: %v\n", err)
	}
}

// FormatCollectorResult writes a collector result to stdout. In JSON mode (or
// when a --jq filter is given) it renders JSON, honoring the filter and color;
// in text mode it calls FormatTable if available, falling back to JSON.
func FormatCollectorResult(jsonMode bool, filter string, color bool, data any) error {
	if jsonMode || filter != "" {
		return RenderJSON(os.Stdout, data, filter, color)
	}
	if ft, ok := data.(TableFormattable); ok {
		ft.FormatTable(os.Stdout)
		return nil
	}
	return RenderJSON(os.Stdout, data, "", color)
}

// RenderJSON writes data as JSON to w. When filter is non-empty it is applied
// as a jq expression (gojq); when color is true the output is ANSI-colored.
// With no filter the original key order is preserved.
func RenderJSON(w io.Writer, data any, filter string, color bool) error {
	if filter == "" || filter == "." {
		raw, err := json.MarshalIndent(data, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal result: %w", err)
		}
		return writeJSONResult(w, raw, color)
	}

	raw, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal result: %w", err)
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return err
	}
	query, err := gojq.Parse(filter)
	if err != nil {
		return fmt.Errorf("invalid --jq filter %q: %w", filter, err)
	}
	iter := query.Run(v)
	for {
		out, ok := iter.Next()
		if !ok {
			break
		}
		if e, ok := out.(error); ok {
			return fmt.Errorf("jq: %w", e)
		}
		rb, err := json.MarshalIndent(out, "", "  ")
		if err != nil {
			return err
		}
		if err := writeJSONResult(w, rb, color); err != nil {
			return err
		}
	}
	return nil
}

func writeJSONResult(w io.Writer, raw []byte, color bool) error {
	if !color {
		_, err := w.Write(append(raw, '\n'))
		return err
	}
	s, err := colorizeJSON(raw)
	if err != nil { // fall back to plain on any tokenizer hiccup
		_, werr := w.Write(append(raw, '\n'))
		return werr
	}
	_, err = io.WriteString(w, s+"\n")
	return err
}

// jq-like ANSI colors: bold-blue keys, green strings, yellow numbers, cyan
// booleans, gray null.
const (
	cKey   = "\x1b[1;34m"
	cStr   = "\x1b[32m"
	cNum   = "\x1b[33m"
	cBool  = "\x1b[36m"
	cNull  = "\x1b[90m"
	cReset = "\x1b[0m"
)

// colorizeJSON re-emits already-marshaled JSON with ANSI colors and 2-space
// indentation, preserving the original key order via the streaming tokenizer.
func colorizeJSON(raw []byte) (string, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var b strings.Builder
	if err := writeColoredValue(&b, dec, 0); err != nil {
		return "", err
	}
	return b.String(), nil
}

func col(b *strings.Builder, code, s string) {
	b.WriteString(code)
	b.WriteString(s)
	b.WriteString(cReset)
}

func jsonIndent(n int) string { return strings.Repeat("  ", n) }

func jsonQuote(s string) string {
	bs, _ := json.Marshal(s)
	return string(bs)
}

func writeColoredValue(b *strings.Builder, dec *json.Decoder, depth int) error {
	t, err := dec.Token()
	if err != nil {
		return err
	}
	switch v := t.(type) {
	case json.Delim:
		if v == '{' {
			return writeColoredObject(b, dec, depth)
		}
		if v == '[' {
			return writeColoredArray(b, dec, depth)
		}
	case string:
		col(b, cStr, jsonQuote(v))
	case json.Number:
		col(b, cNum, v.String())
	case bool:
		col(b, cBool, strconv.FormatBool(v))
	case nil:
		col(b, cNull, "null")
	}
	return nil
}

func writeColoredObject(b *strings.Builder, dec *json.Decoder, depth int) error {
	if !dec.More() {
		_, _ = dec.Token() // consume '}'
		b.WriteString("{}")
		return nil
	}
	b.WriteString("{\n")
	first := true
	for dec.More() {
		if !first {
			b.WriteString(",\n")
		}
		first = false
		b.WriteString(jsonIndent(depth + 1))
		kt, err := dec.Token()
		if err != nil {
			return err
		}
		col(b, cKey, jsonQuote(kt.(string)))
		b.WriteString(": ")
		if err := writeColoredValue(b, dec, depth+1); err != nil {
			return err
		}
	}
	_, _ = dec.Token() // consume '}'
	b.WriteString("\n")
	b.WriteString(jsonIndent(depth))
	b.WriteString("}")
	return nil
}

func writeColoredArray(b *strings.Builder, dec *json.Decoder, depth int) error {
	if !dec.More() {
		_, _ = dec.Token() // consume ']'
		b.WriteString("[]")
		return nil
	}
	b.WriteString("[\n")
	first := true
	for dec.More() {
		if !first {
			b.WriteString(",\n")
		}
		first = false
		b.WriteString(jsonIndent(depth + 1))
		if err := writeColoredValue(b, dec, depth+1); err != nil {
			return err
		}
	}
	_, _ = dec.Token() // consume ']'
	b.WriteString("\n")
	b.WriteString(jsonIndent(depth))
	b.WriteString("]")
	return nil
}
