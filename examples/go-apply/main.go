// Command go-apply applies a kapi.change/v1 change set to the files under a
// directory with the change service (core/change) and the file home
// (core/change/filehome), and nothing of kapi's host: no project, no recipe,
// no check at commit, no recorder, no providers.
//
// It is the embedder's view of the edit engine that the WP5 evaluation
// measures (docs/internals/evals.md): its dependency closure, whether it needs
// cgo or ICU, and its size.
//
//	go run ./examples/go-apply -dir site change.json
//	kapi inspect -f html site/help.html --jsonl | … | go run ./examples/go-apply -dir site
//
// It prints the result (kapi.change-result/v1) and exits 0 when the change set
// applied, 3 when an operation was refused and nothing was written, and 1 on
// any other failure.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/change/filehome"
	"github.com/neokapi/neokapi/core/formats"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
)

func main() {
	dir := flag.String("dir", ".", "directory the change set's documents are named relative to")
	source := flag.String("source-lang", "en", "language the documents are written in")
	flag.Parse()
	code, err := run(*dir, *source, flag.Args(), os.Stdin, os.Stdout)
	if err != nil {
		fmt.Fprintln(os.Stderr, "go-apply:", err)
	}
	os.Exit(code)
}

func run(dir, source string, args []string, stdin io.Reader, stdout io.Writer) (int, error) {
	in := stdin
	if len(args) == 1 && args[0] != "-" {
		file, err := os.Open(args[0])
		if err != nil {
			return 1, err
		}
		defer file.Close()
		in = file
	}
	set, err := change.Decode(in)
	if err != nil {
		return 2, err
	}
	root, err := filepath.Abs(dir)
	if err != nil {
		return 1, err
	}
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)
	home := filehome.New(filehome.DirLayout{Root: root, Formats: reg, SourceLocale: model.LocaleID(source)}, filehome.Options{})
	service := change.NewService(filehome.Formats{Registry: reg}, change.OneHome(home), change.WithOrigin("go-apply"))
	result, err := service.Apply(context.Background(), set, change.Actor{Kind: change.ActorPerson, Name: "go-apply"})
	if err != nil {
		return 1, err
	}
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(result); err != nil {
		return 1, err
	}
	if result.Status != change.SetApplied {
		return 3, nil
	}
	return 0, nil
}
