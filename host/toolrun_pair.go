package host

import (
	"context"
	"errors"
	"fmt"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/schema"
	"github.com/neokapi/neokapi/host/output"
)

// A check that reads a translation, such as term-check, compares a block's
// source with its target. A file that holds both (XLIFF, a .kbf.json) gives it
// both. A translation kept in a file of its own (fr.json beside en.json) holds
// no source, so the check has nothing to compare. `kapi exec <check> en.json
// --target fr.json` pairs the two files the way `kapi check en.json --target
// fr.json` does (bilingualBlocks), runs the tool over the paired blocks and
// reports what it found.

// ReadsTargets reports whether a tool reads a target it cannot run without: its
// IO contract consumes the target port, and not optionally.
func ReadsTargets(s *schema.ComponentSchema) bool {
	if s == nil || s.ToolMeta == nil {
		return false
	}
	for _, p := range s.ToolMeta.Consumes {
		if p.Type == schema.PortTarget && !p.Optional {
			return true
		}
	}
	return false
}

// runToolOnPair runs cfg's tool over the one source file cfg.Files names,
// each block paired with its translation from cfg.TargetFile in
// cfg.TargetLang, and reports through cfg's collector. It writes no file.
func (a *App) runToolOnPair(ctx context.Context, cfg ToolRunConfig) error {
	if len(cfg.Files) != 1 {
		return errors.New("--target pairs one source file with its translation; pass exactly one positional file")
	}
	if cfg.TargetLang == "" {
		return errors.New("--target needs the language of the translation; pass --target-lang")
	}
	source := cfg.Files[0]
	unit := VerifyUnit{
		SourcePath:  source,
		TargetPath:  cfg.TargetFile,
		Locale:      cfg.TargetLang,
		DisplayPath: cfg.TargetFile,
	}
	if ref := matchFormatMapping(source, cfg.FormatMappings); ref != "" {
		name, fmtCfg, err := a.resolveFormatRef(ref)
		if err != nil {
			return err
		}
		unit.SourceFormat, unit.SourceConfig = name, fmtCfg
		unit.TargetFormat, unit.TargetConfig = name, fmtCfg
	}
	blocks, missing, err := a.bilingualBlocks(ctx, unit)
	if err != nil {
		return err
	}
	if missing {
		return fmt.Errorf("target file %q does not exist", cfg.TargetFile)
	}

	t, err := cfg.NewTool()
	if err != nil {
		return err
	}
	in := make(chan *model.Part, len(blocks))
	for _, b := range blocks {
		in <- &model.Part{Type: model.PartBlock, Resource: b}
	}
	close(in)
	out := make(chan *model.Part)
	errc := make(chan error, 1)
	go func() {
		defer close(out)
		errc <- t.Process(ctx, in, out)
	}()
	var parts []*model.Part
	for p := range out {
		parts = append(parts, p)
	}
	if cfg.AfterTool != nil {
		cfg.AfterTool()
	}
	if perr := <-errc; perr != nil {
		return fmt.Errorf("%s %s: %w", cfg.ToolName, DisplayName(source), perr)
	}

	if cfg.NewCollector == nil {
		return nil
	}
	collector := cfg.NewCollector()
	item := &flow.Item{Input: &model.RawDocument{URI: source}, TargetLocale: model.LocaleID(cfg.TargetLang)}
	if err := collector.Collect(ctx, item, parts); err != nil {
		return err
	}
	result, err := collector.Result()
	if err != nil {
		return fmt.Errorf("collector result: %w", err)
	}
	return output.FormatCollectorResult(cfg.JSONOutput, cfg.JQ, cfg.Colorize, result.Data)
}
