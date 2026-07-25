package host

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/neokapi/neokapi/core/formats"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/structrec"
)

func (a *App) RunInspect(ctx context.Context, cmd Command, args []string, outFormat string, project []string) error {
	hadError := false
	files, err := a.ResolveInputs(cmd, args, InputOptions{
		Command: "kapi inspect",
		OnSkip: func(path string, err error) {
			hadError = true
			fmt.Fprintf(cmd.ErrOrStderr(), "kapi inspect: %s: %v\n", path, err)
		},
	})
	if err != nil {
		return err
	}

	streaming := outFormat == "jsonl"
	enc := json.NewEncoder(cmd.OutOrStdout())
	var recs []structrec.Record // accumulated for the array (json/yaml) forms
	n := 0

	prog := a.NewProgress(cmd, "reading", len(files))
	defer prog.Done()

	for _, file := range files {
		prog.Step(DisplayName(file))
		_, ferr := a.StreamBlocks(ctx, file, func(_ int, b *model.Block) error {
			if b.SourceText() == "" {
				return nil
			}
			n++
			rec := structrec.FromBlock(n, b, b.SourceRuns())
			// Attribute blocks read from inside an archive to `<archive>!<entry>`.
			rec.File = entryLabel(DisplayName(file), b)
			// Per-block projection: render the block to each requested target
			// format (faithful markup, not the flattened Text anchor).
			if len(project) > 0 {
				rec.Projected = make(map[string]string, len(project))
				for _, p := range project {
					if frag, ok := formats.RenderBlockFragment(b, p); ok {
						rec.Projected[p] = frag
					}
				}
			}
			if streaming {
				return enc.Encode(rec) // Encode writes one object + newline = JSONL
			}
			recs = append(recs, rec)
			return nil
		})
		if ferr != nil {
			// A cancelled context (Ctrl-C) is a global interrupt; stop now. Any
			// other error is per-file: report it and carry on, matching kcat.
			if errors.Is(ferr, context.Canceled) {
				return ferr
			}
			hadError = true
			fmt.Fprintf(cmd.ErrOrStderr(), "kapi inspect: %s: %v\n", DisplayName(file), ferr)
			continue
		}
	}

	if !streaming {
		var (
			out  []byte
			mErr error
		)
		if outFormat == "yaml" {
			out, mErr = structrec.MarshalYAML(recs)
		} else {
			out, mErr = structrec.MarshalJSONArray(recs)
		}
		if mErr != nil {
			return mErr
		}
		if _, wErr := cmd.OutOrStdout().Write(out); wErr != nil {
			return wErr
		}
	}
	if hadError {
		return WithExitCode(ExitUsage, ErrSilentExit)
	}
	return nil
}
