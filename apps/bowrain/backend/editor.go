package backend

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"runtime"

	"github.com/gokapi/gokapi/ai/provider"
	"github.com/gokapi/gokapi/ai/tools"
	"github.com/gokapi/gokapi/core/credentials"
	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/tool"
	"github.com/gokapi/gokapi/lib/pensieve"
)

// GetItemBlocks returns all blocks for an item in the project.
func (a *App) GetItemBlocks(projectID, itemName string) ([]BlockInfo, error) {
	p, err := a.projects.get(projectID)
	if err != nil {
		return nil, err
	}

	id, ok := p.items[itemName]
	if !ok {
		return nil, fmt.Errorf("item %q not found in project", itemName)
	}

	// Build a lookup from block ID → model.Block for span enrichment
	blockMap := make(map[string]*model.Block)
	for _, pt := range id.parts {
		if pt.Type != model.PartBlock {
			continue
		}
		if block, ok := pt.Resource.(*model.Block); ok {
			blockMap[block.ID] = block
		}
	}

	// Use blockIndex if available (O(1) per block)
	if id.blockIndex != nil {
		var blocks []BlockInfo
		for _, b := range id.blockIndex.Blocks {
			bi := BlockInfo{
				ID:           b.ID,
				Source:       b.Source,
				Targets:      copyTargets(b.Targets),
				Translatable: b.Translatable,
				HasSpans:     b.SourceHTML != b.Source,
				Properties:   copyStringMap(b.Properties),
			}
			// Enrich with coded text and spans from the Part stream
			if mb, ok := blockMap[b.ID]; ok {
				enrichBlockInfo(&bi, mb, p.info.TargetLocales)
			}
			blocks = append(blocks, bi)
		}
		return blocks, nil
	}

	// Fallback: iterate Part stream
	var blocks []BlockInfo
	for _, pt := range id.parts {
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

		bi := BlockInfo{
			ID:           block.ID,
			Source:       block.SourceText(),
			Targets:      targets,
			Translatable: block.Translatable,
			HasSpans:     hasSpans,
			Properties:   props,
		}
		enrichBlockInfo(&bi, block, p.info.TargetLocales)
		blocks = append(blocks, bi)
	}

	return blocks, nil
}

// enrichBlockInfo populates coded text and span metadata on a BlockInfo
// from the underlying model.Block, if spans are present.
func enrichBlockInfo(bi *BlockInfo, block *model.Block, targetLocales []string) {
	if len(block.Source) == 0 || block.Source[0].Content == nil {
		return
	}
	frag := block.Source[0].Content
	if !frag.HasSpans() {
		return
	}

	bi.HasSpans = true
	bi.SourceCoded = frag.CodedText
	bi.SourceSpans = make([]SpanInfo, len(frag.Spans))
	for i, s := range frag.Spans {
		bi.SourceSpans[i] = spanToInfo(s)
	}

	// Extract target coded text per locale
	bi.TargetsCoded = make(map[string]string)
	for _, locale := range targetLocales {
		segs, ok := block.Targets[model.LocaleID(locale)]
		if !ok || len(segs) == 0 {
			continue
		}
		if segs[0].Content != nil {
			bi.TargetsCoded[locale] = segs[0].Content.CodedText
		}
	}
}

// spanToInfo converts a model.Span to a SpanInfo for frontend serialization.
func spanToInfo(s *model.Span) SpanInfo {
	var spanType string
	switch s.SpanType {
	case model.SpanOpening:
		spanType = "opening"
	case model.SpanClosing:
		spanType = "closing"
	case model.SpanPlaceholder:
		spanType = "placeholder"
	}
	return SpanInfo{
		SpanType: spanType,
		Type:     s.Type,
		ID:       s.ID,
		Data:     s.Data,
	}
}

// GetFileBlocks is an alias for GetItemBlocks for backward compatibility.
func (a *App) GetFileBlocks(projectID, fileName string) ([]BlockInfo, error) {
	return a.GetItemBlocks(projectID, fileName)
}

// UpdateBlockTarget updates the target text for a specific block.
func (a *App) UpdateBlockTarget(req UpdateBlockRequest) error {
	p, err := a.projects.get(req.ProjectID)
	if err != nil {
		return err
	}

	// Resolve item name (support both ItemName and legacy FileName field)
	itemName := req.ItemName
	id, ok := p.items[itemName]
	if !ok {
		return fmt.Errorf("item %q not found in project", itemName)
	}

	// Update block index if available
	if id.blockIndex != nil {
		if err := id.blockIndex.UpdateTarget(req.BlockID, req.TargetLocale, req.Text); err != nil {
			return err
		}
	}

	// Also update the Part stream for export/reconstruction
	for _, pt := range id.parts {
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

	return fmt.Errorf("block %q not found in item %q", req.BlockID, itemName)
}

// UpdateBlockTargetCoded updates the target for a block using coded text with span data.
func (a *App) UpdateBlockTargetCoded(req UpdateBlockTargetCodedRequest) error {
	p, err := a.projects.get(req.ProjectID)
	if err != nil {
		return err
	}

	itemName := req.ItemName
	id, ok := p.items[itemName]
	if !ok {
		return fmt.Errorf("item %q not found in project", itemName)
	}

	// Update block index with plain text
	plainText := stripMarkers(req.CodedText)
	if id.blockIndex != nil {
		if err := id.blockIndex.UpdateTarget(req.BlockID, req.TargetLocale, plainText); err != nil {
			return err
		}
	}

	// Update the Part stream with full coded text + spans
	for _, pt := range id.parts {
		if pt.Type != model.PartBlock {
			continue
		}
		block, ok := pt.Resource.(*model.Block)
		if !ok {
			continue
		}
		if block.ID == req.BlockID {
			frag := &model.Fragment{
				CodedText: req.CodedText,
			}
			for _, si := range req.Spans {
				frag.Spans = append(frag.Spans, infoToSpan(si))
			}
			block.SetTargetFragment(model.LocaleID(req.TargetLocale), frag)
			p.dirty = true
			return nil
		}
	}

	return fmt.Errorf("block %q not found in item %q", req.BlockID, itemName)
}

// stripMarkers removes Unicode span markers from coded text, returning plain text.
func stripMarkers(coded string) string {
	var buf []byte
	for _, r := range coded {
		if r < '\uE001' || r > '\uE003' {
			buf = append(buf, []byte(string(r))...)
		}
	}
	return string(buf)
}

// infoToSpan converts a SpanInfo from the frontend back to a model.Span.
func infoToSpan(si SpanInfo) *model.Span {
	var st model.SpanType
	switch si.SpanType {
	case "opening":
		st = model.SpanOpening
	case "closing":
		st = model.SpanClosing
	case "placeholder":
		st = model.SpanPlaceholder
	}
	return &model.Span{
		SpanType: st,
		Type:     si.Type,
		ID:       si.ID,
		Data:     si.Data,
	}
}

// PseudoTranslateItem pseudo-translates all blocks in an item.
func (a *App) PseudoTranslateItem(projectID, itemName, targetLocale string) (*TranslationStats, error) {
	p, err := a.projects.get(projectID)
	if err != nil {
		return nil, err
	}

	id, ok := p.items[itemName]
	if !ok {
		return nil, fmt.Errorf("item %q not found in project", itemName)
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
	in := make(chan *model.Part, len(id.parts))
	out := make(chan *model.Part, len(id.parts))
	for _, pt := range id.parts {
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
	id.parts = newParts

	// Sync block index from parts
	a.syncBlockIndexFromParts(id, p.info.SourceLocale)

	stats := computeStats(id.parts, targetLocale)
	p.dirty = true
	return stats, nil
}

// PseudoTranslateFile is an alias for PseudoTranslateItem for backward compatibility.
func (a *App) PseudoTranslateFile(projectID, fileName, targetLocale string) (*TranslationStats, error) {
	return a.PseudoTranslateItem(projectID, fileName, targetLocale)
}

// AITranslateItem translates all blocks using an AI provider.
func (a *App) AITranslateItem(req AITranslateFileRequest) (*TranslationStats, error) {
	p, err := a.projects.get(req.ProjectID)
	if err != nil {
		return nil, err
	}

	id, ok := p.items[req.ItemName]
	if !ok {
		return nil, fmt.Errorf("item %q not found in project", req.ItemName)
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
	in := make(chan *model.Part, len(id.parts))
	out := make(chan *model.Part, len(id.parts))
	for _, pt := range id.parts {
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
	id.parts = newParts

	// Sync block index from parts
	a.syncBlockIndexFromParts(id, p.info.SourceLocale)

	stats := computeStats(id.parts, req.TargetLocale)
	p.dirty = true
	return stats, nil
}

// AITranslateFile is an alias for AITranslateItem for backward compatibility.
func (a *App) AITranslateFile(req AITranslateFileRequest) (*TranslationStats, error) {
	return a.AITranslateItem(req)
}

// TMTranslateItem leverages translation memory to translate blocks.
func (a *App) TMTranslateItem(projectID, itemName, targetLocale string) (*TranslationStats, error) {
	p, err := a.projects.get(projectID)
	if err != nil {
		return nil, err
	}

	id, ok := p.items[itemName]
	if !ok {
		return nil, fmt.Errorf("item %q not found in project", itemName)
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
	in := make(chan *model.Part, len(id.parts))
	out := make(chan *model.Part, len(id.parts))
	for _, pt := range id.parts {
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
	id.parts = newParts

	// Sync block index from parts
	a.syncBlockIndexFromParts(id, p.info.SourceLocale)

	stats := computeStats(id.parts, targetLocale)
	p.dirty = true
	return stats, nil
}

// TMTranslateFile is an alias for TMTranslateItem for backward compatibility.
func (a *App) TMTranslateFile(projectID, fileName, targetLocale string) (*TranslationStats, error) {
	return a.TMTranslateItem(projectID, fileName, targetLocale)
}

// GetWordCount returns word and character counts for an item.
func (a *App) GetWordCount(projectID, itemName string) (*WordCountResult, error) {
	p, err := a.projects.get(projectID)
	if err != nil {
		return nil, err
	}

	id, ok := p.items[itemName]
	if !ok {
		return nil, fmt.Errorf("item %q not found in project", itemName)
	}

	result := &WordCountResult{
		TargetWords: make(map[string]int),
		TargetChars: make(map[string]int),
	}

	for _, pt := range id.parts {
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

// ExportTranslatedItem writes the translated item to disk and returns the output path.
func (a *App) ExportTranslatedItem(projectID, itemName, targetLocale string) (string, error) {
	p, err := a.projects.get(projectID)
	if err != nil {
		return "", err
	}

	id, ok := p.items[itemName]
	if !ok {
		return "", fmt.Errorf("item %q not found in project", itemName)
	}

	// Determine output path
	ext := fileExtension(itemName)
	baseName := itemName
	if ext != "" {
		baseName = itemName[:len(itemName)-len(ext)-1]
	}
	outputPath := fmt.Sprintf("%s_%s.%s", baseName, targetLocale, ext)

	// If project has a path, put output next to it
	if p.info.Path != "" {
		dir := fileDir(p.info.Path)
		outputPath = dir + "/" + outputPath
	}

	writer, err := a.formatReg.NewWriter(id.format)
	if err != nil {
		return "", fmt.Errorf("no writer for %q: %w", id.format, err)
	}

	if err := writer.SetOutput(outputPath); err != nil {
		return "", fmt.Errorf("set output: %w", err)
	}
	writer.SetLocale(model.LocaleID(targetLocale))

	ch := make(chan *model.Part, len(id.parts))
	for _, pt := range id.parts {
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

// ExportTranslatedFile is an alias for ExportTranslatedItem for backward compatibility.
func (a *App) ExportTranslatedFile(projectID, fileName, targetLocale string) (string, error) {
	return a.ExportTranslatedItem(projectID, fileName, targetLocale)
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

// syncBlockIndexFromParts rebuilds the block index targets from the Part stream.
func (a *App) syncBlockIndexFromParts(id *projectItemData, sourceLocale string) {
	if id.blockIndex == nil {
		return
	}
	// Update targets in block index from parts
	for _, pt := range id.parts {
		if pt.Type != model.PartBlock {
			continue
		}
		block, ok := pt.Resource.(*model.Block)
		if !ok {
			continue
		}
		b := id.blockIndex.BlockByID(block.ID)
		if b == nil {
			continue
		}
		b.Targets = make(map[string]string)
		for locale, segs := range block.Targets {
			if len(segs) > 0 {
				b.Targets[string(locale)] = block.TargetText(locale)
			}
		}
	}
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

// copyTargets creates a shallow copy of a string map.
func copyTargets(m map[string]string) map[string]string {
	if m == nil {
		return map[string]string{}
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// copyStringMap creates a shallow copy of a string map.
func copyStringMap(m map[string]string) map[string]string {
	if m == nil {
		return map[string]string{}
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
