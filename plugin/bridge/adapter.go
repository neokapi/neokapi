package bridge

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"

	"github.com/asgeirf/gokapi/core/format"
	"github.com/asgeirf/gokapi/core/model"
	"github.com/asgeirf/gokapi/plugin/shared"
)

// BridgeFormatReader implements format.DataFormatReader by delegating to
// a Java bridge subprocess running an Okapi filter.
type BridgeFormatReader struct {
	format.BaseFormatReader
	bridge      *JavaBridge
	filterClass string
	info        *InfoData
	content     []byte // raw document content for base64 encoding
}

var _ format.DataFormatReader = (*BridgeFormatReader)(nil)

// NewBridgeFormatReader creates a reader backed by a Java bridge.
func NewBridgeFormatReader(bridge *JavaBridge, filterClass string) *BridgeFormatReader {
	return &BridgeFormatReader{
		bridge:      bridge,
		filterClass: filterClass,
	}
}

// Signature returns the format detection signature from the Java filter.
func (r *BridgeFormatReader) Signature() format.FormatSignature {
	info, err := r.fetchInfo()
	if err != nil {
		return format.FormatSignature{}
	}
	return format.FormatSignature{
		MIMETypes:  info.MimeTypes,
		Extensions: info.Extensions,
	}
}

// Open reads the document content and sends it to the Java bridge.
func (r *BridgeFormatReader) Open(_ context.Context, doc *model.RawDocument) error {
	r.Doc = doc

	var content []byte
	if doc.Reader != nil {
		var err error
		content, err = io.ReadAll(doc.Reader)
		if err != nil {
			return fmt.Errorf("reading document content: %w", err)
		}
	}
	r.content = content

	return r.bridge.Open(OpenParams{
		FilterClass:   r.filterClass,
		URI:           doc.URI,
		SourceLocale:  string(doc.SourceLocale),
		Encoding:      doc.Encoding,
		ContentBase64: base64.StdEncoding.EncodeToString(content),
		MimeType:      doc.MimeType,
	})
}

// Read sends a read command to the bridge and emits Parts on the returned channel.
func (r *BridgeFormatReader) Read(ctx context.Context) <-chan model.PartResult {
	ch := make(chan model.PartResult)
	go func() {
		defer close(ch)

		rd, err := r.bridge.Read()
		if err != nil {
			ch <- model.PartResult{Error: fmt.Errorf("bridge read: %w", err)}
			return
		}

		var dtos []shared.PartDTO
		if err := json.Unmarshal(rd.Parts, &dtos); err != nil {
			ch <- model.PartResult{Error: fmt.Errorf("parsing parts: %w", err)}
			return
		}

		for _, dto := range dtos {
			part := shared.DTOToPart(dto)
			select {
			case ch <- model.PartResult{Part: part}:
			case <-ctx.Done():
				ch <- model.PartResult{Error: ctx.Err()}
				return
			}
		}
	}()
	return ch
}

// Close releases the filter resources in the Java bridge.
func (r *BridgeFormatReader) Close() error {
	return r.bridge.CloseFilter()
}

// fetchInfo caches and returns the filter's metadata.
func (r *BridgeFormatReader) fetchInfo() (*InfoData, error) {
	if r.info != nil {
		return r.info, nil
	}
	info, err := r.bridge.Info(r.filterClass)
	if err != nil {
		return nil, err
	}
	r.info = info
	r.FormatName = info.Name
	r.FormatDisplayName = info.DisplayName
	return info, nil
}

// BridgeFormatWriter implements format.DataFormatWriter by delegating to
// a Java bridge subprocess running an Okapi filter writer.
type BridgeFormatWriter struct {
	format.BaseFormatWriter
	bridge          *JavaBridge
	filterClass     string
	originalContent []byte // original document needed by Okapi for skeleton
}

var _ format.DataFormatWriter = (*BridgeFormatWriter)(nil)

// NewBridgeFormatWriter creates a writer backed by a Java bridge.
func NewBridgeFormatWriter(bridge *JavaBridge, filterClass string) *BridgeFormatWriter {
	return &BridgeFormatWriter{
		bridge:      bridge,
		filterClass: filterClass,
	}
}

// SetOriginalContent sets the original document content, which Okapi needs
// as a skeleton for the filter writer.
func (w *BridgeFormatWriter) SetOriginalContent(content []byte) {
	w.originalContent = content
}

// Write collects all parts from the channel, sends them to the Java bridge
// with the original content, and writes the reconstructed output.
func (w *BridgeFormatWriter) Write(ctx context.Context, parts <-chan *model.Part) error {
	var collected []*model.Part
	for {
		select {
		case p, ok := <-parts:
			if !ok {
				goto send
			}
			collected = append(collected, p)
		case <-ctx.Done():
			return ctx.Err()
		}
	}

send:
	dtos := shared.PartsToDTO(collected)

	wd, err := w.bridge.Write(WriteParams{
		FilterClass:           w.filterClass,
		Parts:                 dtos,
		Locale:                string(w.Locale),
		Encoding:              w.Encoding,
		OriginalContentBase64: base64.StdEncoding.EncodeToString(w.originalContent),
	})
	if err != nil {
		return fmt.Errorf("bridge write: %w", err)
	}

	output, err := base64.StdEncoding.DecodeString(wd.OutputBase64)
	if err != nil {
		return fmt.Errorf("decoding output: %w", err)
	}

	if w.Output != nil {
		if _, err := io.Copy(w.Output, bytes.NewReader(output)); err != nil {
			return fmt.Errorf("writing output: %w", err)
		}
	}

	return nil
}
