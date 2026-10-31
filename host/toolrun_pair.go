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

// WritesFindingsOnly reports whether everything a tool writes to a block is a
// finding: its IO contract declares produced ports, and each one carries a
// check verdict. Such a tool changes no source and no translation, so the file
// it can write is the content it read with its findings attached, never a
// translation (`qa`).
func WritesFindingsOnly(s *schema.ComponentSchema) bool {
	if s == nil || s.ToolMeta == nil || len(s.ToolMeta.Produces) == 0 {
		return false
	}
	for _, p := range s.ToolMeta.Produces {
		if !findingsPorts[p.Type] {
			return false
		}
	}
	return true
}

// ExecTarget is how `kapi exec <tool>` treats the target language of a run.
type ExecTarget struct {
	// PairsTarget: the command takes --target, the translation of the one
	// source file named, kept in a file of its own. The run reads the two
	// files as one bilingual set of blocks and writes no file.
	PairsTarget bool
	// WritesByDefault: a run with no -o and no --output-dir writes a file,
	// the target-language file in the input's locale layout or, with no
	// target language, the input itself. A tool that reads a translation
	// and writes only findings writes a file only where -o or --output-dir
	// names one, so its run never puts the source it read in the place of
	// the translation it checked.
	WritesByDefault bool
}

// ExecTargetFor returns how `kapi exec` treats the target of a run of the tool
// s describes, which writes output files when writesOutput is set.
func ExecTargetFor(s *schema.ComponentSchema, writesOutput bool) ExecTarget {
	var meta *schema.ToolMeta
	if s != nil {
		meta = s.ToolMeta
	}
	findingsOnly := writesOutput && WritesFindingsOnly(s)
	return ExecTarget{
		PairsTarget:     meta.TakesTargetLanguage() && ReadsTargets(s) && (!writesOutput || findingsOnly),
		WritesByDefault: writesOutput && !findingsOnly,
	}
}

// notRunError is the error a run returns once its report is printed: a tool
// that compares a block's source with its translation, run where no block held
// one, checked nothing. The report says what to pass; the exit code is
// ExitNotRun, as for a `kapi check` that checked no content, so a script does
// not read the run as a clean one.
func notRunError(data any) error {
	r, ok := data.(FindingsReport)
	if !ok || r.untranslated == nil {
		return nil
	}
	return fmt.Errorf("%w: %s read no %s translation; pass the translation file with --target <file>",
		ErrCheckNotRun, r.untranslated.tool, r.untranslated.lang)
}

// printToolResult prints a run's collected result and returns notRunError
// for it, or else placeholderGateError.
func printToolResult(cfg ToolRunConfig, data any) error {
	if err := output.FormatCollectorResult(cfg.JSONOutput, cfg.JQ, cfg.Colorize, data); err != nil {
		return err
	}
	if err := notRunError(data); err != nil {
		return err
	}
	return placeholderGateError(data)
}

// runToolOnPair runs cfg's tool over the one source file cfg.Files names,
// each block paired with its translation from cfg.TargetFile in
// cfg.TargetLang, and reports through cfg's collector. It writes no file.
func (a *App) runToolOnPair(ctx context.Context, cfg ToolRunConfig) error {
	if len(cfg.Files) != 1 {
		return errors.New("--target pairs one source file with its translation; pass exactly one positional file")
	}
	if cfg.OutputTemplate != "" {
		return errors.New("a run paired with --target writes no file; drop -o and --output-dir, and read the findings on stdout")
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

	chain, err := a.newToolChain(cfg)
	if err != nil {
		return err
	}
	parts := make([]*model.Part, 0, len(blocks))
	for _, b := range blocks {
		parts = append(parts, &model.Part{Type: model.PartBlock, Resource: b})
	}
	parts, perr := runChain(ctx, chain.tools(chain.main), parts)
	if cfg.AfterTool != nil {
		cfg.AfterTool()
	}
	if perr != nil {
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
	return printToolResult(cfg, result.Data)
}
