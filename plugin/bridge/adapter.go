package bridge

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"

	"github.com/gokapi/gokapi/format"
	"github.com/gokapi/gokapi/model"
	"github.com/gokapi/gokapi/plugin/shared"
)

// BridgeFormatReader implements format.DataFormatReader by delegating to
// a Java bridge subprocess running an Okapi filter. It acquires a bridge
// from the pool on Open and releases it on Close, ensuring exclusive
// access to the stateful JVM filter for the entire lifecycle.
type BridgeFormatReader struct {
	format.BaseFormatReader
	pool         *BridgePool
	cfg          BridgeConfig
	bridge       *JavaBridge // acquired from pool during Open
	filterClass  string
	filterParams map[string]interface{} // optional filter parameters
	info         *InfoData
	content      []byte // raw document content for base64 encoding
}

var _ format.DataFormatReader = (*BridgeFormatReader)(nil)

// NewBridgeFormatReader creates a reader that acquires bridges from the pool.
// The cfg specifies which JAR to use when acquiring a bridge.
func NewBridgeFormatReader(pool *BridgePool, cfg BridgeConfig, filterClass string) *BridgeFormatReader {
	return &BridgeFormatReader{
		pool:        pool,
		cfg:         cfg,
		filterClass: filterClass,
	}
}

// SetFilterParams sets optional filter-specific parameters.
// These are passed to the Java bridge when opening the filter.
func (r *BridgeFormatReader) SetFilterParams(params map[string]interface{}) {
	r.filterParams = params
}

// Signature returns the format detection signature from the Java filter.
// It acquires and releases a bridge just for the info query.
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
// It acquires a bridge from the pool, which is released by Close.
func (r *BridgeFormatReader) Open(_ context.Context, doc *model.RawDocument) error {
	b, err := r.pool.Acquire(r.cfg)
	if err != nil {
		return fmt.Errorf("acquiring bridge: %w", err)
	}

	r.bridge = b
	r.Doc = doc

	var content []byte
	if doc.Reader != nil {
		content, err = io.ReadAll(doc.Reader)
		if err != nil {
			r.pool.Release(b)
			r.bridge = nil
			return fmt.Errorf("reading document content: %w", err)
		}
	}
	r.content = content

	if err := r.bridge.Open(OpenParams{
		FilterClass:   r.filterClass,
		URI:           doc.URI,
		SourceLocale:  string(doc.SourceLocale),
		Encoding:      doc.Encoding,
		ContentBase64: base64.StdEncoding.EncodeToString(content),
		MimeType:      doc.MimeType,
		FilterParams:  r.filterParams,
	}); err != nil {
		r.pool.Release(b)
		r.bridge = nil
		return err
	}
	return nil
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

// Close releases the filter resources in the Java bridge and returns
// the bridge to the pool.
func (r *BridgeFormatReader) Close() error {
	if r.bridge == nil {
		return nil
	}
	err := r.bridge.CloseFilter()
	r.pool.Release(r.bridge)
	r.bridge = nil
	return err
}

// fetchInfo caches and returns the filter's metadata.
// It acquires and releases a bridge from the pool for the info query.
func (r *BridgeFormatReader) fetchInfo() (*InfoData, error) {
	if r.info != nil {
		return r.info, nil
	}
	b, err := r.pool.Acquire(r.cfg)
	if err != nil {
		return nil, fmt.Errorf("acquiring bridge for info: %w", err)
	}
	defer r.pool.Release(b)

	info, err := b.Info(r.filterClass)
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
	pool            *BridgePool
	cfg             BridgeConfig
	filterClass     string
	filterParams    map[string]interface{} // optional filter parameters
	originalContent []byte                 // original document needed by Okapi for skeleton
}

var _ format.DataFormatWriter = (*BridgeFormatWriter)(nil)

// NewBridgeFormatWriter creates a writer that acquires bridges from the pool.
// The cfg specifies which JAR to use when acquiring a bridge.
func NewBridgeFormatWriter(pool *BridgePool, cfg BridgeConfig, filterClass string) *BridgeFormatWriter {
	return &BridgeFormatWriter{
		pool:        pool,
		cfg:         cfg,
		filterClass: filterClass,
	}
}

// SetFilterParams sets optional filter-specific parameters.
// These are passed to the Java bridge when writing.
func (w *BridgeFormatWriter) SetFilterParams(params map[string]interface{}) {
	w.filterParams = params
}

// SetOriginalContent sets the original document content, which Okapi needs
// as a skeleton for the filter writer.
func (w *BridgeFormatWriter) SetOriginalContent(content []byte) {
	w.originalContent = content
}

// Write collects all parts from the channel, acquires a bridge from the pool,
// sends the parts to the Java bridge with the original content, writes the
// reconstructed output, and releases the bridge back to the pool.
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

	b, err := w.pool.Acquire(w.cfg)
	if err != nil {
		return fmt.Errorf("acquiring bridge for write: %w", err)
	}
	defer w.pool.Release(b)

	wd, err := b.Write(WriteParams{
		FilterClass:           w.filterClass,
		Parts:                 dtos,
		Locale:                string(w.Locale),
		Encoding:              w.Encoding,
		OriginalContentBase64: base64.StdEncoding.EncodeToString(w.originalContent),
		FilterParams:          w.filterParams,
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
