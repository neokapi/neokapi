package json

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gokapi/gokapi/core/format"
	"github.com/gokapi/gokapi/core/model"
)

// Reader implements DataFormatReader for JSON files.
type Reader struct {
	format.BaseFormatReader
	cfg           *Config
	resolver      format.SubfilterResolver
	skeletonStore *format.SkeletonStore
	skelBuf       bytes.Buffer // coalesces skeleton text between refs
	layerSeq      int          // counter for generating unique child layer IDs
}

// Ensure Reader implements SubfilterAware and SkeletonStoreEmitter.
var _ format.SubfilterAware = (*Reader)(nil)
var _ format.SkeletonStoreEmitter = (*Reader)(nil)

// NewReader creates a new JSON reader.
func NewReader() *Reader {
	cfg := &Config{}
	cfg.Reset() // sets defaults
	return &Reader{
		BaseFormatReader: format.BaseFormatReader{
			FormatName:        "json",
			FormatDisplayName: "JSON",
			FormatMimeType:    "application/json",
			FormatExtensions:  []string{".json"},
			Cfg:               cfg,
		},
		cfg: cfg,
	}
}

// SetSubfilterResolver sets the resolver for creating sub-format readers.
func (r *Reader) SetSubfilterResolver(resolver format.SubfilterResolver) {
	r.resolver = resolver
}

// SetSkeletonStore sets the skeleton store for streaming skeleton output.
func (r *Reader) SetSkeletonStore(store *format.SkeletonStore) {
	r.skeletonStore = store
}

// skelText appends text to the skeleton buffer if active.
func (r *Reader) skelText(s string) {
	if r.skeletonStore != nil {
		r.skelBuf.WriteString(s)
	}
}

// skelRef flushes buffered text and writes a block reference to the skeleton store.
func (r *Reader) skelRef(id string) {
	if r.skeletonStore != nil {
		if r.skelBuf.Len() > 0 {
			_ = r.skeletonStore.WriteText(r.skelBuf.Bytes())
			r.skelBuf.Reset()
		}
		_ = r.skeletonStore.WriteRef(id)
	}
}

// skelToken appends a token's prefix and raw bytes to the skeleton buffer.
func (r *Reader) skelToken(tok token) {
	if r.skeletonStore != nil {
		r.skelBuf.WriteString(tok.prefix)
		r.skelBuf.WriteString(tok.raw)
	}
}

// skelFlush writes any remaining buffered text to the skeleton store.
func (r *Reader) skelFlush() {
	if r.skeletonStore != nil && r.skelBuf.Len() > 0 {
		_ = r.skeletonStore.WriteText(r.skelBuf.Bytes())
		r.skelBuf.Reset()
	}
}

// Signature returns detection metadata for this format.
func (r *Reader) Signature() format.FormatSignature {
	return format.FormatSignature{
		MIMETypes:  []string{"application/json"},
		Extensions: []string{".json"},
	}
}

// Open opens a RawDocument for reading.
func (r *Reader) Open(ctx context.Context, doc *model.RawDocument) error {
	if doc == nil || doc.Reader == nil {
		return fmt.Errorf("json: nil document or reader")
	}
	r.Doc = doc
	return nil
}

// Read returns a channel of PartResults.
func (r *Reader) Read(ctx context.Context) <-chan model.PartResult {
	ch := make(chan model.PartResult, 64)
	go func() {
		defer close(ch)
		r.readContent(ctx, ch)
	}()
	return ch
}

// readState tracks metadata accumulated while walking the JSON tree.
// Note/ID/meta values attach to the next translatable block within the
// same object scope.
type readState struct {
	pendingNote     string // note text to attach to next block
	pendingID       string // ID to use as name for next block
	pendingMeta     map[string]string
	pendingMaxwidth int    // -1 = not set
	idStack         []string
}

func (r *Reader) readContent(ctx context.Context, ch chan<- model.PartResult) {
	locale := r.Doc.SourceLocale
	if locale.IsEmpty() {
		locale = model.LocaleEnglish
	}

	// Read all content
	content, err := io.ReadAll(r.Doc.Reader)
	if err != nil {
		ch <- model.PartResult{Error: fmt.Errorf("json: reading: %w", err)}
		return
	}

	// Emit layer start
	layer := &model.Layer{
		ID:         "doc1",
		Name:       r.Doc.URI,
		Format:     "json",
		Locale:     locale,
		Encoding:   r.Doc.Encoding,
		MimeType:   "application/json",
		Properties: make(map[string]string),
	}
	// Store original JSON for non-skeleton roundtrip
	if r.skeletonStore == nil {
		layer.Properties["json.original"] = string(content)
	}
	if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerStart, Resource: layer}) {
		return
	}

	// Tokenize
	sc := newScanner(content)
	tokens, err := sc.scan()
	if err != nil {
		ch <- model.PartResult{Error: fmt.Errorf("json: parsing: %w", err)}
		return
	}

	blockCounter := 0
	dataCounter := 0
	state := &readState{pendingMaxwidth: -1}
	pos := 0
	r.walkTokenValue(ctx, ch, tokens, &pos, "", "", layer.ID, &blockCounter, &dataCounter, state)

	// Write trailing whitespace/comments to skeleton store
	if r.skeletonStore != nil && pos < len(tokens) && tokens[pos].typ == tokenEOF {
		r.skelText(tokens[pos].prefix)
	}
	r.skelFlush()

	// Emit layer end
	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
}

// walkTokenValue reads a JSON value from the token stream starting at pos.
func (r *Reader) walkTokenValue(ctx context.Context, ch chan<- model.PartResult,
	tokens []token, pos *int, keyName, path, parentLayerID string,
	blockCounter, dataCounter *int, state *readState) {

	if *pos >= len(tokens) {
		return
	}
	tok := tokens[*pos]

	switch tok.typ {
	case tokenObjectStart:
		r.walkTokenObject(ctx, ch, tokens, pos, path, parentLayerID, blockCounter, dataCounter, state)
	case tokenArrayStart:
		r.walkTokenArray(ctx, ch, tokens, pos, keyName, path, parentLayerID, blockCounter, dataCounter, state)
	case tokenString:
		*pos++
		r.handleStringValue(ctx, ch, tok, keyName, path, parentLayerID, blockCounter, dataCounter, state)
	case tokenNumber, tokenTrue, tokenFalse, tokenNull:
		*pos++
		r.handleNonStringValue(ctx, ch, tok, keyName, path, parentLayerID, blockCounter, dataCounter, state)
	default:
		r.skelToken(tok)
		*pos++ // skip unexpected tokens
	}
}

// walkTokenObject reads a JSON object { key: value, ... }.
func (r *Reader) walkTokenObject(ctx context.Context, ch chan<- model.PartResult,
	tokens []token, pos *int, parentPath, parentLayerID string,
	blockCounter, dataCounter *int, state *readState) {

	r.skelToken(tokens[*pos]) // {
	*pos++
	// Save and reset pending state for this object scope
	savedState := *state
	state.pendingNote = ""
	state.pendingID = ""
	state.pendingMeta = nil
	state.pendingMaxwidth = -1

	for *pos < len(tokens) {
		tok := tokens[*pos]
		if tok.typ == tokenObjectEnd {
			r.skelToken(tok)
			*pos++
			break
		}
		if tok.typ == tokenComma {
			r.skelToken(tok)
			*pos++
			continue
		}
		if tok.typ != tokenString {
			r.skelToken(tok)
			*pos++
			continue
		}

		key := tok.value
		r.skelToken(tok) // key string
		*pos++
		// skip colon
		if *pos < len(tokens) && tokens[*pos].typ == tokenColon {
			r.skelToken(tokens[*pos])
			*pos++
		}

		childPath := r.buildPath(key, parentPath)

		// Walk the value (skeleton writing handled by value/handle functions)
		r.walkTokenValue(ctx, ch, tokens, pos, key, childPath, parentLayerID, blockCounter, dataCounter, state)
	}

	// Restore parent state
	*state = savedState
}

// walkTokenArray reads a JSON array [ value, ... ].
func (r *Reader) walkTokenArray(ctx context.Context, ch chan<- model.PartResult,
	tokens []token, pos *int, keyName, parentPath, parentLayerID string,
	blockCounter, dataCounter *int, state *readState) {

	r.skelToken(tokens[*pos]) // [
	*pos++
	index := 0

	for *pos < len(tokens) {
		tok := tokens[*pos]
		if tok.typ == tokenArrayEnd {
			r.skelToken(tok)
			*pos++
			break
		}
		if tok.typ == tokenComma {
			r.skelToken(tok)
			*pos++
			continue
		}

		childPath := parentPath + "[" + strconv.Itoa(index) + "]"

		if tok.typ == tokenString && !r.cfg.ExtractIsolatedStrings {
			// Standalone string in array — skip extraction
			r.skelToken(tok)
			*pos++
			*dataCounter++
			data := &model.Data{
				ID:   fmt.Sprintf("d%d", *dataCounter),
				Name: childPath,
			}
			r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data})
		} else {
			elemKey := keyName
			if tok.typ == tokenObjectStart {
				elemKey = ""
			}
			r.walkTokenValue(ctx, ch, tokens, pos, elemKey, childPath, parentLayerID, blockCounter, dataCounter, state)
		}
		index++
	}
}

// handleStringValue processes a string value found at the given key path.
func (r *Reader) handleStringValue(ctx context.Context, ch chan<- model.PartResult,
	tok token, keyName, path, parentLayerID string,
	blockCounter, dataCounter *int, state *readState) {

	value := tok.value
	fullPath := r.fullKeyPath(path)

	// Check metadata rules first — these consume the value without emitting a block
	if r.cfg.isNote(keyName, fullPath) {
		state.pendingNote = value
		r.skelToken(tok)
		return
	}
	if r.cfg.isID(keyName, fullPath) {
		state.pendingID = value
		if r.cfg.UseIDStack {
			state.idStack = append(state.idStack, value)
		}
		r.skelToken(tok)
		return
	}
	if r.cfg.isGenericMeta(keyName, fullPath) {
		if state.pendingMeta == nil {
			state.pendingMeta = make(map[string]string)
		}
		state.pendingMeta[keyName] = value
		r.skelToken(tok)
		return
	}

	// Check extraction rules
	if !r.cfg.shouldExtract(keyName, fullPath) {
		r.skelToken(tok)
		*dataCounter++
		data := &model.Data{
			ID:   fmt.Sprintf("d%d", *dataCounter),
			Name: path,
		}
		r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data})
		return
	}

	// Check for subfilter (pattern-based or global)
	if mapping := r.matchSubfilter(path); mapping != nil && r.resolver != nil {
		r.skelText(tok.prefix)
		r.skelRef("layer:" + path)
		r.emitSubfiltered(ctx, ch, value, path, parentLayerID, mapping, blockCounter, dataCounter)
		r.consumePendingState(state, nil)
		return
	}
	if r.cfg.shouldSubfilter(keyName, fullPath) && r.resolver != nil {
		r.skelText(tok.prefix)
		r.skelRef("layer:" + path)
		mapping := &format.SubfilterMapping{Format: r.cfg.SubfilterFormat}
		r.emitSubfiltered(ctx, ch, value, path, parentLayerID, mapping, blockCounter, dataCounter)
		r.consumePendingState(state, nil)
		return
	}

	// Emit as a translatable block
	*blockCounter++
	blockID := fmt.Sprintf("tu%d", *blockCounter)
	r.skelText(tok.prefix)
	r.skelRef(blockID)

	block := model.NewBlock(blockID, value)

	// Apply block name
	block.Name = r.blockName(keyName, path, state)

	// Store the raw key path for non-skeleton roundtrip.
	// The block Name may differ from the path (e.g. UseFullKeyPath, idRules),
	// so the token-based writer needs the original path to match tokens.
	if block.Name != path {
		block.Properties["json.keypath"] = path
	}

	// Apply code finder if enabled
	if r.cfg.UseCodeFinder {
		r.applyCodeFinder(block)
	}

	// Apply pending metadata
	r.consumePendingState(state, block)

	r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})
}

// handleNonStringValue processes a non-string value (number, bool, null).
func (r *Reader) handleNonStringValue(ctx context.Context, ch chan<- model.PartResult,
	tok token, keyName, path, parentLayerID string,
	blockCounter, dataCounter *int, state *readState) {

	fullPath := r.fullKeyPath(path)

	// Check maxwidth rules — numeric values set max width
	if tok.typ == tokenNumber && r.cfg.isMaxwidth(keyName, fullPath) {
		if v, err := strconv.ParseFloat(tok.value, 64); err == nil {
			state.pendingMaxwidth = int(v)
		}
		r.skelToken(tok)
		return
	}

	r.skelToken(tok)
	*dataCounter++
	data := &model.Data{
		ID:   fmt.Sprintf("d%d", *dataCounter),
		Name: path,
	}
	r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data})
}

// buildPath constructs the key path for a child key.
func (r *Reader) buildPath(key, parentPath string) string {
	if parentPath == "" {
		return key
	}
	return parentPath + "." + key
}

// fullKeyPath converts a dotted path to the full key path format.
func (r *Reader) fullKeyPath(path string) string {
	if !r.cfg.UseFullKeyPath {
		return path
	}
	// Convert dots to slashes for full path
	p := "/" + strings.ReplaceAll(path, ".", "/")
	return p
}

// blockName determines the name for a block.
func (r *Reader) blockName(keyName, path string, state *readState) string {
	// If there's a pending ID, use it
	if state.pendingID != "" {
		return state.pendingID
	}
	// If using ID stack, join IDs
	if r.cfg.UseIDStack && len(state.idStack) > 0 {
		return strings.Join(state.idStack, "/")
	}

	if !r.cfg.UseKeyAsName {
		return path
	}
	if r.cfg.UseFullKeyPath {
		fullPath := "/" + strings.ReplaceAll(path, ".", "/")
		if !r.cfg.UseLeadingSlashOnKeyPath {
			fullPath = strings.TrimPrefix(fullPath, "/")
		}
		return fullPath
	}
	return path
}

// consumePendingState applies pending notes/meta/maxwidth to a block and resets state.
func (r *Reader) consumePendingState(state *readState, block *model.Block) {
	if block != nil {
		if state.pendingNote != "" {
			block.Annotations["note"] = &model.NoteAnnotation{
				Text: state.pendingNote,
				From: "json",
			}
		}
		if state.pendingMeta != nil {
			for k, v := range state.pendingMeta {
				block.Properties[k] = v
			}
		}
		if state.pendingMaxwidth >= 0 {
			block.Properties["maxwidth"] = strconv.Itoa(state.pendingMaxwidth)
			if r.cfg.MaxwidthSizeUnit != "" {
				block.Properties["maxwidthSizeUnit"] = r.cfg.MaxwidthSizeUnit
			}
		}
	}
	state.pendingNote = ""
	state.pendingID = ""
	state.pendingMeta = nil
	state.pendingMaxwidth = -1
}

// applyCodeFinder applies code finder patterns to a block's fragments.
// It rebuilds the CodedText with markers for matched patterns.
func (r *Reader) applyCodeFinder(block *model.Block) {
	patterns := r.cfg.GetCodeFinderPatterns()
	if len(patterns) == 0 {
		return
	}

	for _, seg := range block.Source {
		if seg.Content == nil {
			continue
		}
		text := seg.Content.Text()

		// Collect all match ranges
		type matchRange struct {
			start, end int
		}
		var matches []matchRange
		for _, re := range patterns {
			for _, loc := range re.FindAllStringIndex(text, -1) {
				matches = append(matches, matchRange{loc[0], loc[1]})
			}
		}
		if len(matches) == 0 {
			continue
		}

		// Sort matches by start position
		for i := 1; i < len(matches); i++ {
			for j := i; j > 0 && matches[j].start < matches[j-1].start; j-- {
				matches[j], matches[j-1] = matches[j-1], matches[j]
			}
		}

		// Rebuild fragment with coded text markers
		newFrag := &model.Fragment{}
		lastEnd := 0
		spanID := 1
		for _, m := range matches {
			if m.start > lastEnd {
				newFrag.AppendText(text[lastEnd:m.start])
			}
			newFrag.AppendSpan(&model.Span{
				ID:       fmt.Sprintf("c%d", spanID),
				SpanType: model.SpanPlaceholder,
				Type:     "code",
				Data:     text[m.start:m.end],
			})
			lastEnd = m.end
			spanID++
		}
		if lastEnd < len(text) {
			newFrag.AppendText(text[lastEnd:])
		}
		seg.Content = newFrag
	}
}

// matchSubfilter checks if the given key path matches any configured subfilter mapping.
func (r *Reader) matchSubfilter(path string) *format.SubfilterMapping {
	for i := range r.cfg.Subfilters {
		sf := &r.cfg.Subfilters[i]
		if matchGlob(sf.Pattern, path) {
			return sf
		}
	}
	return nil
}

// emitSubfiltered emits a child layer with content parsed by the subfilter format reader.
func (r *Reader) emitSubfiltered(ctx context.Context, ch chan<- model.PartResult, content, path, parentLayerID string, mapping *format.SubfilterMapping, blockCounter, dataCounter *int) {
	subReader, err := r.resolver.ResolveReader(mapping.Format)
	if err != nil {
		// Fall back to plain block if subfilter reader is unavailable
		*blockCounter++
		block := model.NewBlock(fmt.Sprintf("tu%d", *blockCounter), content)
		block.Name = path
		r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})
		return
	}

	r.layerSeq++
	childLayerID := fmt.Sprintf("sf%d", r.layerSeq)

	locale := r.Doc.SourceLocale
	if locale.IsEmpty() {
		locale = model.LocaleEnglish
	}

	childLayer := &model.Layer{
		ID:       childLayerID,
		Name:     path,
		Format:   mapping.Format,
		Locale:   locale,
		ParentID: parentLayerID,
		Properties: map[string]string{
			"subfilter.source":  "json",
			"subfilter.keyPath": path,
		},
	}

	// Emit child layer start
	if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerStart, Resource: childLayer}) {
		return
	}

	// Open sub-reader and emit its parts
	subDoc := &model.RawDocument{
		URI:          path,
		SourceLocale: locale,
		Encoding:     "UTF-8",
		Reader:       io.NopCloser(bytes.NewReader([]byte(content))),
	}
	if err := subReader.Open(ctx, subDoc); err != nil {
		ch <- model.PartResult{Error: fmt.Errorf("json: subfilter open for %s: %w", path, err)}
		r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: childLayer})
		return
	}

	// Read sub-reader parts, skipping the sub-reader's own layer start/end
	for pr := range subReader.Read(ctx) {
		if pr.Error != nil {
			ch <- model.PartResult{Error: fmt.Errorf("json: subfilter read for %s: %w", path, pr.Error)}
			break
		}
		if pr.Part.Type == model.PartLayerStart || pr.Part.Type == model.PartLayerEnd {
			if layer, ok := pr.Part.Resource.(*model.Layer); ok && layer.IsRoot() {
				continue
			}
		}
		r.emit(ctx, ch, pr.Part)
	}
	subReader.Close()

	// Emit child layer end
	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: childLayer})
}

func (r *Reader) emit(ctx context.Context, ch chan<- model.PartResult, part *model.Part) bool {
	select {
	case ch <- model.PartResult{Part: part}:
		return true
	case <-ctx.Done():
		return false
	}
}

// Close releases resources.
func (r *Reader) Close() error {
	if r.Doc != nil && r.Doc.Reader != nil {
		return r.Doc.Reader.Close()
	}
	return nil
}

// matchGlob matches a path against a glob pattern.
func matchGlob(pattern, path string) bool {
	patternNorm := strings.ReplaceAll(pattern, ".", "/")
	pathNorm := strings.ReplaceAll(path, ".", "/")
	matched, _ := filepath.Match(patternNorm, pathNorm)
	return matched
}
