package host

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/projectdb"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/safeio"
	"github.com/neokapi/neokapi/core/tool"
)

// WriteBackOptions selects one source file of a project and one locale.
type WriteBackOptions struct {
	// Project is the loaded recipe; ProjectPath is where it was loaded from.
	Project     *project.KapiProject
	ProjectPath string
	// SourcePath is the absolute path of the source file to write back. It has
	// to be one of the project's own content files: the recipe's configuration
	// for that item is what makes the round trip faithful.
	SourcePath string
	// Locale is the target to apply. Empty copies the source file itself, which
	// is what a reader asks for when no locale is in view.
	Locale model.LocaleID
}

// WriteBack writes one of a project's source files to out, as the format's
// writer emits it for a locale, with the targets the project block store holds
// applied to it.
//
// This is the materialize path without a file at the end of it. `kapi merge`
// reads a source, hydrates the stored `targets/<locale>` overlays onto it, and
// writes the result through the format's skeleton round trip to the output
// template; a surface showing a reader what the file will look like wants the
// same bytes and no file. The reader and writer are configured from the recipe
// exactly as merge configures them, so the document splits into the same blocks
// the extraction produced and the stored targets land where they belong.
//
// Nothing is written to disk, and nothing is recorded: the content memory
// absorption merge does belongs to a materialize pass, not to a reading of one.
func (a *App) WriteBack(ctx context.Context, opts WriteBackOptions, out io.Writer) error {
	if opts.Project == nil {
		return errors.New("write back: no project")
	}
	if opts.SourcePath == "" {
		return errors.New("write back: no source file")
	}

	pctx := project.NewProjectContext(opts.Project, opts.ProjectPath)
	files, err := pctx.ResolveContent(a.FormatReg)
	if err != nil {
		return fmt.Errorf("write back: resolve project content: %w", err)
	}

	var file *project.ResolvedFile
	for i := range files {
		if files[i].Path == opts.SourcePath {
			file = &files[i]
			break
		}
	}
	if file == nil {
		return fmt.Errorf("write back: %s is not one of this project's content files", opts.SourcePath)
	}

	srcFormat := file.Format
	if srcFormat == "" {
		srcFormat = detectSourceFormat(a.FormatReg, pctx, file.Relative, file.Path)
	}
	if srcFormat == "" {
		return fmt.Errorf("write back: cannot detect the format of %s", file.Relative)
	}

	// No locale in view means the source file, which is already on disk and is
	// the thing itself. Sending it through the round trip instead would show a
	// reader the engine's rendering of the file they can open.
	if opts.Locale == "" {
		in, oerr := os.Open(file.Path)
		if oerr != nil {
			return fmt.Errorf("write back: open %s: %w", file.Relative, oerr)
		}
		defer in.Close()
		if _, cerr := io.Copy(out, safeio.DefaultBudget().Reader(in)); cerr != nil {
			return fmt.Errorf("write back: read %s: %w", file.Relative, cerr)
		}
		return nil
	}

	layout, err := project.LayoutFor(opts.ProjectPath)
	if err != nil {
		return fmt.Errorf("write back: resolve project layout: %w", err)
	}
	db, err := a.ProjectDB(ctx, layout.Root)
	if err != nil {
		return fmt.Errorf("write back: open project store: %w", err)
	}
	store := db.BlocksAutocommit()
	if store == nil {
		return fmt.Errorf("write back: read the block cache: %w", projectdb.ErrNoStore)
	}

	cfg := flow.FileRunnerConfig{
		Store:        store,
		FormatReg:    a.FormatReg,
		SourceLocale: pctx.SourceLocale,
		Encoding:     pctx.Encoding,
		DetectFormat: func(string) registry.FormatID { return registry.FormatID(srcFormat) },
		ConfigureReader: func(reader format.DataFormatReader, detected registry.FormatID) error {
			return pctx.ConfigureReaderFor(reader, string(detected), file.Item)
		},
		ConfigureWriter: func(writer format.DataFormatWriter, fmtName registry.FormatID) error {
			return pctx.ConfigureWriterFor(writer, string(fmtName), file.Item)
		},
	}

	// The overlays are addressed by the same source-file-namespaced key the run
	// wrote them under (blockstore.StoreKey).
	ctx = blockstore.WithSourceRel(ctx, file.Relative)
	tools := []tool.Tool{newHydrateTargetsTool(opts.Locale)}

	in, err := os.Open(file.Path)
	if err != nil {
		return fmt.Errorf("write back: open %s: %w", file.Relative, err)
	}
	defer in.Close()

	runner := flow.NewFileRunner(cfg)
	if rerr := runner.RunStream(ctx, "write-back", tools, in, file.Path, registry.FormatID(srcFormat), out, string(opts.Locale)); rerr != nil {
		return fmt.Errorf("write back %s: %w", file.Relative, rerr)
	}
	return nil
}
