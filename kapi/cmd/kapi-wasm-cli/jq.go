//go:build js && wasm

package main

import (
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/itchyny/gojq"
	"github.com/spf13/cobra"
)

// newJqCmd is a browser-only `kapi jq` backed by gojq: query JSON files with a
// jq filter and print colored, indented output. With no filter it pretty-prints
// the input (identity "."), which doubles as a "colorize this JSON" command.
//
// Argument shape mirrors jq but is forgiving: if the first arg is an existing
// file, every arg is treated as a file and the filter defaults to "."; otherwise
// the first arg is the filter and the rest are files.
func newJqCmd() *cobra.Command {
	var compact, raw, noColor bool
	cmd := &cobra.Command{
		Use:   "jq [filter] [files...]",
		Short: "Query and pretty-print JSON (colored), powered by gojq",
		Long: "Run a jq filter over JSON files and print colored, indented output.\n" +
			"With no filter, pretty-prints the input (identity '.').\n\n" +
			"  kapi jq out.json                 # colorize / pretty-print\n" +
			"  kapi jq '.greeting' out.json     # apply a filter\n" +
			"  kapi jq '.[].text' blocks.json   # iterate",
		RunE: func(cmd *cobra.Command, args []string) error {
			filter := "."
			files := args
			if len(args) > 0 && !fileExists(args[0]) {
				filter = args[0]
				files = args[1:]
			}
			if len(files) == 0 {
				return fmt.Errorf("no input files")
			}
			query, err := gojq.Parse(filter)
			if err != nil {
				return fmt.Errorf("invalid filter %q: %w", filter, err)
			}

			enc := &jsonEnc{compact: compact, color: !noColor}
			out := cmd.OutOrStdout()
			for _, f := range files {
				data, err := os.ReadFile(f)
				if err != nil {
					return fmt.Errorf("read %s: %w", f, err)
				}
				var input any
				if err := json.Unmarshal(data, &input); err != nil {
					return fmt.Errorf("%s: invalid JSON: %w", f, err)
				}
				iter := query.Run(input)
				for {
					v, ok := iter.Next()
					if !ok {
						break
					}
					if e, ok := v.(error); ok {
						return fmt.Errorf("jq: %w", e)
					}
					if raw {
						if s, ok := v.(string); ok {
							fmt.Fprintln(out, s)
							continue
						}
					}
					var b strings.Builder
					enc.encode(&b, v, 0)
					fmt.Fprintln(out, b.String())
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&compact, "compact", false, "compact output (no pretty-printing)")
	cmd.Flags().BoolVarP(&raw, "raw-output", "r", false, "output strings without quotes")
	cmd.Flags().BoolVar(&noColor, "no-color", false, "disable ANSI colors")
	return cmd
}

func fileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}

// Colors roughly follow jq's defaults: bold-blue keys, green strings, yellow
// numbers, cyan booleans, gray null.
const (
	cKey   = "\x1b[1;34m"
	cStr   = "\x1b[32m"
	cNum   = "\x1b[33m"
	cBool  = "\x1b[36m"
	cNull  = "\x1b[90m"
	cReset = "\x1b[0m"
)

type jsonEnc struct {
	compact bool
	color   bool
}

func (e *jsonEnc) col(b *strings.Builder, code, s string) {
	if e.color {
		b.WriteString(code)
		b.WriteString(s)
		b.WriteString(cReset)
	} else {
		b.WriteString(s)
	}
}

func indent(n int) string { return strings.Repeat("  ", n) }

func quote(s string) string {
	bs, _ := json.Marshal(s)
	return string(bs)
}

func (e *jsonEnc) encode(b *strings.Builder, v any, depth int) {
	switch x := v.(type) {
	case nil:
		e.col(b, cNull, "null")
	case bool:
		e.col(b, cBool, strconv.FormatBool(x))
	case string:
		e.col(b, cStr, quote(x))
	case int:
		e.col(b, cNum, strconv.Itoa(x))
	case float64:
		e.col(b, cNum, strconv.FormatFloat(x, 'g', -1, 64))
	case *big.Int:
		e.col(b, cNum, x.String())
	case []any:
		e.encodeArray(b, x, depth)
	case map[string]any:
		e.encodeObject(b, x, depth)
	default:
		bs, _ := json.Marshal(x)
		b.Write(bs)
	}
}

func (e *jsonEnc) encodeArray(b *strings.Builder, arr []any, depth int) {
	if len(arr) == 0 {
		b.WriteString("[]")
		return
	}
	if e.compact {
		b.WriteByte('[')
		for i, el := range arr {
			if i > 0 {
				b.WriteByte(',')
			}
			e.encode(b, el, depth)
		}
		b.WriteByte(']')
		return
	}
	b.WriteString("[\n")
	for i, el := range arr {
		b.WriteString(indent(depth + 1))
		e.encode(b, el, depth+1)
		if i < len(arr)-1 {
			b.WriteByte(',')
		}
		b.WriteByte('\n')
	}
	b.WriteString(indent(depth))
	b.WriteByte(']')
}

func (e *jsonEnc) encodeObject(b *strings.Builder, m map[string]any, depth int) {
	if len(m) == 0 {
		b.WriteString("{}")
		return
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys) // gojq maps are unordered; sort for deterministic output
	if e.compact {
		b.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				b.WriteByte(',')
			}
			e.col(b, cKey, quote(k))
			b.WriteByte(':')
			e.encode(b, m[k], depth)
		}
		b.WriteByte('}')
		return
	}
	b.WriteString("{\n")
	for i, k := range keys {
		b.WriteString(indent(depth + 1))
		e.col(b, cKey, quote(k))
		b.WriteString(": ")
		e.encode(b, m[k], depth+1)
		if i < len(keys)-1 {
			b.WriteByte(',')
		}
		b.WriteByte('\n')
	}
	b.WriteString(indent(depth))
	b.WriteByte('}')
}
