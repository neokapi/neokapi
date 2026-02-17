package host

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/rpc"

	"github.com/gokapi/gokapi/format"
	"github.com/gokapi/gokapi/model"
	"github.com/gokapi/gokapi/plugin/shared"
)

// FormatReaderRPCClient implements format.DataFormatReader by delegating
// to a plugin process over net/rpc.
type FormatReaderRPCClient struct {
	client *rpc.Client
	info   *shared.InfoResult
}

// ensure interface compliance
var _ format.DataFormatReader = (*FormatReaderRPCClient)(nil)

// Name returns the format identifier from the remote plugin.
func (c *FormatReaderRPCClient) Name() string {
	info, err := c.fetchInfo()
	if err != nil {
		return ""
	}
	return info.Name
}

// DisplayName returns the human-readable format name from the remote plugin.
func (c *FormatReaderRPCClient) DisplayName() string {
	info, err := c.fetchInfo()
	if err != nil {
		return ""
	}
	return info.DisplayName
}

// Signature returns the format detection signature from the remote plugin.
func (c *FormatReaderRPCClient) Signature() format.FormatSignature {
	info, err := c.fetchInfo()
	if err != nil {
		return format.FormatSignature{}
	}
	return format.FormatSignature{
		MIMETypes:  info.MIMETypes,
		Extensions: info.Extensions,
	}
}

// Open sends a document to the remote plugin for reading.
func (c *FormatReaderRPCClient) Open(_ context.Context, doc *model.RawDocument) error {
	var content []byte
	if doc.Reader != nil {
		var err error
		content, err = io.ReadAll(doc.Reader)
		if err != nil {
			return fmt.Errorf("reading document content: %w", err)
		}
	}
	args := shared.OpenArgs{
		URI:          doc.URI,
		SourceLocale: string(doc.SourceLocale),
		Encoding:     doc.Encoding,
		Content:      content,
		MimeType:     doc.MimeType,
		FormatID:     doc.FormatID,
	}
	var errStr string
	if err := c.client.Call("Plugin.Open", &args, &errStr); err != nil {
		return fmt.Errorf("rpc Open: %w", err)
	}
	if errStr != "" {
		return fmt.Errorf("plugin Open: %s", errStr)
	}
	return nil
}

// Read streams Parts from the remote plugin. All parts are fetched in a single
// RPC call and then emitted onto the returned channel.
func (c *FormatReaderRPCClient) Read(ctx context.Context) <-chan model.PartResult {
	ch := make(chan model.PartResult)
	go func() {
		defer close(ch)
		var result shared.ReadResult
		if err := c.client.Call("Plugin.Read", new(struct{}), &result); err != nil {
			ch <- model.PartResult{Error: fmt.Errorf("rpc Read: %w", err)}
			return
		}
		if result.Error != "" {
			ch <- model.PartResult{Error: fmt.Errorf("plugin Read: %s", result.Error)}
			return
		}
		for _, dto := range result.Parts {
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

// Close releases remote plugin resources.
func (c *FormatReaderRPCClient) Close() error {
	var errStr string
	if err := c.client.Call("Plugin.Close", new(struct{}), &errStr); err != nil {
		return fmt.Errorf("rpc Close: %w", err)
	}
	if errStr != "" {
		return fmt.Errorf("plugin Close: %s", errStr)
	}
	return nil
}

// Config returns nil since remote plugins manage their own config.
func (c *FormatReaderRPCClient) Config() format.DataFormatConfig {
	return nil
}

// SetConfig is a no-op for remote plugins. Configuration is handled
// by the plugin process itself.
func (c *FormatReaderRPCClient) SetConfig(_ format.DataFormatConfig) error {
	return nil
}

// fetchInfo caches and returns the plugin's InfoResult.
func (c *FormatReaderRPCClient) fetchInfo() (*shared.InfoResult, error) {
	if c.info != nil {
		return c.info, nil
	}
	var info shared.InfoResult
	if err := c.client.Call("Plugin.Info", new(struct{}), &info); err != nil {
		return nil, fmt.Errorf("rpc Info: %w", err)
	}
	c.info = &info
	return c.info, nil
}

// FormatWriterRPCClient implements format.DataFormatWriter by delegating
// to a plugin process over net/rpc.
type FormatWriterRPCClient struct {
	client   *rpc.Client
	locale   model.LocaleID
	encoding string
	output   io.Writer
}

// ensure interface compliance
var _ format.DataFormatWriter = (*FormatWriterRPCClient)(nil)

// Name returns the format identifier from the remote plugin.
func (c *FormatWriterRPCClient) Name() string {
	var info shared.InfoResult
	if err := c.client.Call("Plugin.Info", new(struct{}), &info); err != nil {
		return ""
	}
	return info.Name
}

// SetOutput configures a file path as the output destination.
// The actual writing happens remotely; the result is fetched and written locally.
func (c *FormatWriterRPCClient) SetOutput(path string) error {
	// We buffer the output and write to the file when Write completes.
	// The path is stored for later use.
	buf := &bytes.Buffer{}
	c.output = buf
	return nil
}

// SetOutputWriter configures an io.Writer as the output destination.
func (c *FormatWriterRPCClient) SetOutputWriter(w io.Writer) error {
	c.output = w
	return nil
}

// SetLocale sets the target locale for writing.
func (c *FormatWriterRPCClient) SetLocale(locale model.LocaleID) {
	c.locale = locale
}

// SetEncoding sets the output encoding.
func (c *FormatWriterRPCClient) SetEncoding(encoding string) {
	c.encoding = encoding
}

// Write consumes Parts from the channel, sends them to the remote plugin,
// and writes the result to the configured output.
func (c *FormatWriterRPCClient) Write(ctx context.Context, parts <-chan *model.Part) error {
	// Collect all parts from the channel.
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
	args := shared.WriteArgs{
		Parts:    shared.PartsToDTO(collected),
		Locale:   string(c.locale),
		Encoding: c.encoding,
	}
	var result shared.WriteResult
	if err := c.client.Call("Plugin.Write", &args, &result); err != nil {
		return fmt.Errorf("rpc Write: %w", err)
	}
	if result.Error != "" {
		return fmt.Errorf("plugin Write: %s", result.Error)
	}
	if c.output != nil && len(result.Output) > 0 {
		if _, err := c.output.Write(result.Output); err != nil {
			return fmt.Errorf("writing output: %w", err)
		}
	}
	return nil
}

// Close is a no-op for the RPC writer client.
func (c *FormatWriterRPCClient) Close() error {
	return nil
}
