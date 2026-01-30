package backend

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"runtime"

	"github.com/asgeirf/gokapi/ai/provider"
	"github.com/asgeirf/gokapi/ai/tools"
	"github.com/asgeirf/gokapi/core/credentials"
	"github.com/asgeirf/gokapi/core/model"
	"github.com/asgeirf/gokapi/core/tool"
	"github.com/asgeirf/gokapi/lib/pensieve"
)

// GetFileBlocks returns all blocks for a file in the project.
func (a *App) GetFileBlocks(projectID, fileName string) ([]BlockInfo, error) {
	p, err := a.projects.get(projectID)
	if err != nil {
		return nil, err
	}

	fd, ok := p.files[fileName]
	if !ok {
		return nil, fmt.Errorf("file %q not found in project", fileName)
	}

	var blocks []BlockInfo
	for _, pt := range fd.parts {
		if pt.Type != model.PartBlock {
			continue
		}
		block, ok := pt.Resource.(*model.Block)
		if !ok {
			continue
		}

		targets := make(map[string]string)
		for _, locale := range p.info.TargetLocales {
			if t := block.TargetText(model.LocaleID(locale)); t != "" {
				targets[locale] = t
			}
		}

		props := make(map[string]string)
		for k, v := range block.Properties {
			props[k] = v
		}

		hasSpans := false
		if len(block.Source) > 0 && block.Source[0].Content != nil {
			hasSpans = block.Source[0].Content.HasSpans()
		}

		blocks = append(blocks, BlockInfo{
			ID:           block.ID,
			Source:       block.SourceText(),
			Targets:      targets,
			Translatable: block.Translatable,
			HasSpans:     hasSpans,
			Properties:   props,
		})
	}

	return blocks, nil
}

// UpdateBlockTarget updates the target text for a specific block.
func (a *App) UpdateBlockTarget(req UpdateBlockRequest) error {
	p, err := a.projects.get(req.ProjectID)
	if err != nil {
		return err
	}

	fd, ok := p.files[req.FileName]
	if !ok {
		return fmt.Errorf("file %q not found in project", req.FileName)
	}

	for _, pt := range fd.parts {
		if pt.Type != model.PartBlock {
			continue
		}
		block, ok := pt.Resource.(*model.Block)
		if !ok {
			continue
		}
		if block.ID == req.BlockID {
			block.SetTargetText(model.LocaleID(req.TargetLocale), req.Text)
			p.dirty = true
			return nil
		}
	}

	return fmt.Errorf("block %q not found in file %q", req.BlockID, req.FileName)
}

// PseudoTranslateFile pseudo-translates all blocks in a file.
func (a *App) PseudoTranslateFile(projectID, fileName, targetLocale string) (*TranslationStats, error) {
	p, err := a.projects.get(projectID)
	if err != nil {
		return nil, err
	}

	fd, ok := p.files[fileName]
	if !ok {
		return nil, fmt.Errorf("file %q not found in project", fileName)
	}

	// Build pseudo-translate tool
	pseudoTool := &tool.BaseTool{
		ToolName:        "pseudo-translate",
		ToolDescription: "Pseudo-translates blocks",
	}
	pseudoTool.HandleBlockFn = func(part *model.Part) (*model.Part, error) {
		block, ok := part.Resource.(*model.Block)
		if !ok || !block.Translatable {
			return part, nil
		}
		src := block.SourceText()
		pseudo := "[" + pseudoAccent(src) + "]"
		block.SetTargetText(model.LocaleID(targetLocale), pseudo)
		return part, nil
	}

	// Process parts through tool
	ctx := context.Background()
	in := make(chan *model.Part, len(fd.parts))
	out := make(chan *model.Part, len(fd.parts))
	for _, pt := range fd.parts {
		in <- pt
	}
	close(in)

	if err := pseudoTool.Process(ctx, in, out); err != nil {
		return nil, fmt.Errorf("pseudo-translate: %w", err)
	}
	close(out)

	// Collect results back
	var newParts []*model.Part
	for pt := range out {
		newParts = append(newParts, pt)
	}
	fd.parts = newParts

	stats := computeStats(fd.parts, targetLocale)
	p.dirty = true
	return stats, nil
}

// AITranslateFile translates all blocks using an AI provider.
func (a *App) AITranslateFile(req AITranslateFileRequest) (*TranslationStats, error) {
	p, err := a.projects.get(req.ProjectID)
	if err != nil {
		return nil, err
	}

	fd, ok := p.files[req.FileName]
	if !ok {
		return nil, fmt.Errorf("file %q not found in project", req.FileName)
	}

	var prov provider.LLMProvider
	if req.ProviderConfigID != "" {
		var err error
		prov, err = credentials.NewProvider(a.credentials, req.ProviderConfigID)
		if err != nil {
			return nil, fmt.Errorf("resolve provider config: %w", err)
		}
	} else {
		prov = createProvider(req.Provider, req.APIKey, req.Model)
	}
	translateTool := tools.NewAITranslateTool(prov, tools.AITranslateConfig{
		SourceLocale: model.LocaleID(p.info.SourceLocale),
		TargetLocale: model.LocaleID(req.TargetLocale),
	})

	ctx := context.Background()
	in := make(chan *model.Part, len(fd.parts))
	out := make(chan *model.Part, len(fd.parts))
	for _, pt := range fd.parts {
		in <- pt
	}
	close(in)

	if err := translateTool.Process(ctx, in, out); err != nil {
		return nil, fmt.Errorf("AI translate: %w", err)
	}
	close(out)

	var newParts []*model.Part
	for pt := range out {
		newParts = append(newParts, pt)
	}
	fd.parts = newParts

	stats := computeStats(fd.parts, req.TargetLocale)
	p.dirty = true
	return stats, nil
}

// TMTranslateFile leverages translation memory to translate blocks.
func (a *App) TMTranslateFile(projectID, fileName, targetLocale string) (*TranslationStats, error) {
	p, err := a.projects.get(projectID)
	if err != nil {
		return nil, err
	}

	fd, ok := p.files[fileName]
	if !ok {
		return nil, fmt.Errorf("file %q not found in project", fileName)
	}

	// Create in-memory TM and leverage tool
	tm := pensieve.NewInMemoryTM()
	tmTool := pensieve.NewTMLeverageTool(tm, pensieve.TMLeverageConfig{
		MinScore:     0.7,
		MaxResults:   5,
		SourceLocale: model.LocaleID(p.info.SourceLocale),
		TargetLocale: model.LocaleID(targetLocale),
	})

	ctx := context.Background()
	in := make(chan *model.Part, len(fd.parts))
	out := make(chan *model.Part, len(fd.parts))
	for _, pt := range fd.parts {
		in <- pt
	}
	close(in)

	if err := tmTool.Process(ctx, in, out); err != nil {
		return nil, fmt.Errorf("TM translate: %w", err)
	}
	close(out)

	var newParts []*model.Part
	for pt := range out {
		newParts = append(newParts, pt)
	}
	fd.parts = newParts

	stats := computeStats(fd.parts, targetLocale)
	p.dirty = true
	return stats, nil
}

// GetWordCount returns word and character counts for a file.
func (a *App) GetWordCount(projectID, fileName string) (*WordCountResult, error) {
	p, err := a.projects.get(projectID)
	if err != nil {
		return nil, err
	}

	fd, ok := p.files[fileName]
	if !ok {
		return nil, fmt.Errorf("file %q not found in project", fileName)
	}

	result := &WordCountResult{
		TargetWords: make(map[string]int),
		TargetChars: make(map[string]int),
	}

	for _, pt := range fd.parts {
		if pt.Type != model.PartBlock {
			continue
		}
		block, ok := pt.Resource.(*model.Block)
		if !ok || !block.Translatable {
			continue
		}

		src := block.SourceText()
		result.SourceWords += countWords(src)
		result.SourceChars += countChars(src)

		for _, locale := range p.info.TargetLocales {
			t := block.TargetText(model.LocaleID(locale))
			if t != "" {
				result.TargetWords[locale] += countWords(t)
				result.TargetChars[locale] += countChars(t)
			}
		}
	}

	return result, nil
}

// ExportTranslatedFile writes the translated file to disk and returns the output path.
func (a *App) ExportTranslatedFile(projectID, fileName, targetLocale string) (string, error) {
	p, err := a.projects.get(projectID)
	if err != nil {
		return "", err
	}

	fd, ok := p.files[fileName]
	if !ok {
		return "", fmt.Errorf("file %q not found in project", fileName)
	}

	// Determine output path
	ext := fileExtension(fileName)
	baseName := fileName
	if ext != "" {
		baseName = fileName[:len(fileName)-len(ext)-1]
	}
	outputPath := fmt.Sprintf("%s_%s.%s", baseName, targetLocale, ext)

	// If project has a path, put output next to it
	if p.info.Path != "" {
		dir := fileDir(p.info.Path)
		outputPath = dir + "/" + outputPath
	}

	writer, err := a.formatReg.NewWriter(fd.format)
	if err != nil {
		return "", fmt.Errorf("no writer for %q: %w", fd.format, err)
	}

	if err := writer.SetOutput(outputPath); err != nil {
		return "", fmt.Errorf("set output: %w", err)
	}
	writer.SetLocale(model.LocaleID(targetLocale))

	ch := make(chan *model.Part, len(fd.parts))
	for _, pt := range fd.parts {
		ch <- pt
	}
	close(ch)

	ctx := context.Background()
	if err := writer.Write(ctx, ch); err != nil {
		return "", fmt.Errorf("write: %w", err)
	}
	writer.Close()

	return outputPath, nil
}

// OpenFileInOS opens a file using the OS default application.
func (a *App) OpenFileInOS(filePath string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", filePath)
	case "linux":
		cmd = exec.Command("xdg-open", filePath)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", filePath)
	default:
		return fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
	return cmd.Start()
}

// computeStats calculates translation statistics from parts.
func computeStats(parts []*model.Part, targetLocale string) *TranslationStats {
	stats := &TranslationStats{}
	for _, pt := range parts {
		if pt.Type != model.PartBlock {
			continue
		}
		block, ok := pt.Resource.(*model.Block)
		if !ok || !block.Translatable {
			continue
		}
		stats.TotalBlocks++
		stats.WordCount += countWords(block.SourceText())
		if block.TargetText(model.LocaleID(targetLocale)) != "" {
			stats.TranslatedBlocks++
		}
	}
	return stats
}

// pseudoAccent applies accent characters to ASCII text for pseudo-translation.
func pseudoAccent(text string) string {
	var buf bytes.Buffer
	for _, r := range text {
		if replacement, ok := accentMap[r]; ok {
			buf.WriteRune(replacement)
		} else {
			buf.WriteRune(r)
		}
	}
	return buf.String()
}

var accentMap = map[rune]rune{
	'a': '\u00e0', 'b': '\u0180', 'c': '\u00e7', 'd': '\u00f0',
	'e': '\u00e9', 'f': '\u0192', 'g': '\u011d', 'h': '\u0125',
	'i': '\u00ee', 'j': '\u0135', 'k': '\u0137', 'l': '\u013c',
	'm': '\u1e3f', 'n': '\u00f1', 'o': '\u00f6', 'p': '\u00fe',
	'q': '\u01eb', 'r': '\u0155', 's': '\u0161', 't': '\u0163',
	'u': '\u00fb', 'v': '\u1e7d', 'w': '\u0175', 'x': '\u1e8b',
	'y': '\u00fd', 'z': '\u017e',
	'A': '\u00c0', 'B': '\u0181', 'C': '\u00c7', 'D': '\u00d0',
	'E': '\u00c9', 'F': '\u0191', 'G': '\u011c', 'H': '\u0124',
	'I': '\u00ce', 'J': '\u0134', 'K': '\u0136', 'L': '\u013b',
	'M': '\u1e3e', 'N': '\u00d1', 'O': '\u00d6', 'P': '\u00de',
	'Q': '\u01ea', 'R': '\u0154', 'S': '\u0160', 'T': '\u0162',
	'U': '\u00db', 'V': '\u1e7c', 'W': '\u0174', 'X': '\u1e8a',
	'Y': '\u00dd', 'Z': '\u017d',
}

// fileDir returns the directory of a file path.
func fileDir(path string) string {
	dir := path
	for i := len(dir) - 1; i >= 0; i-- {
		if dir[i] == '/' || dir[i] == '\\' {
			return dir[:i]
		}
	}
	return "."
}
