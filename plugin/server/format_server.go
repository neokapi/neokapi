// Package server provides helpers for implementing gokapi plugins.
// Plugin authors wrap their format reader/writer or tool implementations
// using these server types, then call the Serve* helpers from main().
package server

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/rpc"

	"github.com/gokapi/gokapi/core/format"
	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/plugin/shared"
	goplugin "github.com/hashicorp/go-plugin"
)

// FormatReaderRPCServer wraps a format.DataFormatReader for serving over net/rpc.
type FormatReaderRPCServer struct {
	Impl format.DataFormatReader
}

// Info returns the plugin's metadata.
func (s *FormatReaderRPCServer) Info(_ struct{}, resp *shared.InfoResult) error {
	sig := s.Impl.Signature()
	*resp = shared.InfoResult{
		Name:        s.Impl.Name(),
		DisplayName: s.Impl.DisplayName(),
		MIMETypes:   sig.MIMETypes,
		Extensions:  sig.Extensions,
	}
	return nil
}

// Open opens a document for reading.
func (s *FormatReaderRPCServer) Open(args *shared.OpenArgs, errStr *string) error {
	doc := &model.RawDocument{
		URI:          args.URI,
		SourceLocale: model.LocaleID(args.SourceLocale),
		Encoding:     args.Encoding,
		MimeType:     args.MimeType,
		FormatID:     args.FormatID,
	}
	if len(args.Content) > 0 {
		doc.Reader = io.NopCloser(bytes.NewReader(args.Content))
	}
	if err := s.Impl.Open(context.Background(), doc); err != nil {
		*errStr = err.Error()
	}
	return nil
}

// Read reads all parts from the opened document.
func (s *FormatReaderRPCServer) Read(_ struct{}, resp *shared.ReadResult) error {
	ctx := context.Background()
	ch := s.Impl.Read(ctx)
	var parts []shared.PartDTO
	for pr := range ch {
		if pr.Error != nil {
			*resp = shared.ReadResult{Error: pr.Error.Error()}
			return nil
		}
		parts = append(parts, shared.PartToDTO(pr.Part))
	}
	*resp = shared.ReadResult{Parts: parts}
	return nil
}

// Close releases resources.
func (s *FormatReaderRPCServer) Close(_ struct{}, errStr *string) error {
	if err := s.Impl.Close(); err != nil {
		*errStr = err.Error()
	}
	return nil
}

// FormatReaderServerPlugin is the go-plugin.Plugin implementation for serving
// a format reader from a plugin process.
type FormatReaderServerPlugin struct {
	Impl format.DataFormatReader
}

// Server returns the RPC server for this plugin.
func (p *FormatReaderServerPlugin) Server(broker *goplugin.MuxBroker) (interface{}, error) {
	return &FormatReaderRPCServer{Impl: p.Impl}, nil
}

// Client is not used on the server side.
func (p *FormatReaderServerPlugin) Client(broker *goplugin.MuxBroker, c *rpc.Client) (interface{}, error) {
	return nil, fmt.Errorf("FormatReaderServerPlugin.Client should not be called on server side")
}

// FormatWriterRPCServer wraps a format.DataFormatWriter for serving over net/rpc.
type FormatWriterRPCServer struct {
	Impl format.DataFormatWriter
}

// Info returns the plugin's metadata.
func (s *FormatWriterRPCServer) Info(_ struct{}, resp *shared.InfoResult) error {
	*resp = shared.InfoResult{
		Name: s.Impl.Name(),
	}
	return nil
}

// Write processes parts and writes the output.
func (s *FormatWriterRPCServer) Write(args *shared.WriteArgs, resp *shared.WriteResult) error {
	// Set locale and encoding.
	s.Impl.SetLocale(model.LocaleID(args.Locale))
	s.Impl.SetEncoding(args.Encoding)

	// Set up a buffer to capture output.
	var buf bytes.Buffer
	if err := s.Impl.SetOutputWriter(&buf); err != nil {
		*resp = shared.WriteResult{Error: fmt.Sprintf("setting output: %v", err)}
		return nil
	}

	// Convert DTOs to parts and feed them through a channel.
	parts := shared.DTOToParts(args.Parts)
	ch := make(chan *model.Part, len(parts))
	for _, p := range parts {
		ch <- p
	}
	close(ch)

	ctx := context.Background()
	if err := s.Impl.Write(ctx, ch); err != nil {
		*resp = shared.WriteResult{Error: fmt.Sprintf("writing: %v", err)}
		return nil
	}

	if err := s.Impl.Close(); err != nil {
		*resp = shared.WriteResult{Error: fmt.Sprintf("closing: %v", err)}
		return nil
	}

	*resp = shared.WriteResult{Output: buf.Bytes()}
	return nil
}

// FormatWriterServerPlugin is the go-plugin.Plugin implementation for serving
// a format writer from a plugin process.
type FormatWriterServerPlugin struct {
	Impl format.DataFormatWriter
}

// Server returns the RPC server for this plugin.
func (p *FormatWriterServerPlugin) Server(broker *goplugin.MuxBroker) (interface{}, error) {
	return &FormatWriterRPCServer{Impl: p.Impl}, nil
}

// Client is not used on the server side.
func (p *FormatWriterServerPlugin) Client(broker *goplugin.MuxBroker, c *rpc.Client) (interface{}, error) {
	return nil, fmt.Errorf("FormatWriterServerPlugin.Client should not be called on server side")
}
