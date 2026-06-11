// Package gen emits the CLI help-string inventory (commands.json) that the
// l10n pipeline reads via the standard JSON filter. It reconstructs the
// full kapi command set from the exported cli factories (cli.KapiCommandSet
// — the same constructor kapi/cmd/kapi uses) and walks the tree, recording
// every non-empty Short/Long/Example under the nested key path
//
//	cli.commands.<full.command.path>.{short,long,example}
//
// plus the cli/output chrome table under cli.output.<key>. The JSON filter
// with its default configuration derives block names from the dotted full
// key path, so those paths become the MO msgctxt — exactly the scopes the
// runtime lookups in cli.LocalizeCommandHelp and cli/output.T use.
//
// Invoked from cli/i18n/doc.go via //go:generate go run ./gen/cmd. The
// generated document is committed so CI can enforce freshness with
// `git diff --exit-code`. Output is deterministic: the command set is built
// from a fixed registration order and json.MarshalIndent sorts map keys.
package gen

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/neokapi/neokapi/cli"
	"github.com/neokapi/neokapi/cli/output"
)

// Generate writes the help-string inventory to outFile (commands.json).
func Generate(outFile string) error {
	doc := BuildDocument()
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal %s: %w", outFile, err)
	}
	data = append(data, '\n')
	return os.WriteFile(outFile, data, 0o644)
}

// BuildDocument constructs the nested inventory document in memory.
func BuildDocument() map[string]any {
	app := &cli.App{}
	app.InitRegistries()

	root := &cobra.Command{
		Use:   "kapi",
		Short: cli.KapiRootShort,
		Long:  cli.KapiRootLong,
	}
	app.AddCommandGroups(root)
	for _, c := range app.KapiCommandSet() {
		root.AddCommand(c)
	}

	doc := map[string]any{}
	walkCommand(root, []string{"cli", "commands", "kapi"}, doc)
	for key, src := range output.Catalog() {
		set(doc, append([]string{"cli", "output"}, strings.Split(key, ".")...), src)
	}
	return doc
}

// walkCommand records the command's help strings and recurses into
// subcommands. Mirrors cli.LocalizeCommandHelp's scope derivation.
func walkCommand(c *cobra.Command, path []string, doc map[string]any) {
	if c.Short != "" {
		set(doc, append(path[:len(path):len(path)], "short"), c.Short)
	}
	if c.Long != "" {
		set(doc, append(path[:len(path):len(path)], "long"), c.Long)
	}
	if c.Example != "" {
		set(doc, append(path[:len(path):len(path)], "example"), c.Example)
	}
	for _, sub := range c.Commands() {
		name := sub.Name()
		if name == "" || strings.ContainsAny(name, ". ") {
			continue
		}
		walkCommand(sub, append(path[:len(path):len(path)], name), doc)
	}
}

// set places val at the nested key path, creating intermediate objects.
func set(doc map[string]any, path []string, val string) {
	cur := doc
	for _, seg := range path[:len(path)-1] {
		next, ok := cur[seg].(map[string]any)
		if !ok {
			next = map[string]any{}
			cur[seg] = next
		}
		cur = next
	}
	cur[path[len(path)-1]] = val
}
