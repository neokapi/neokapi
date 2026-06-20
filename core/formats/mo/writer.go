// Package mo implements a gettext MO (Machine Object) binary catalog writer.
// MO is the compiled runtime form of PO. It stores one entry per (msgctxt,
// msgid) pair with a msgstr translation, indexable by gettext libraries
// (github.com/leonelquinteros/gotext, for one) via binary search.
//
// The writer consumes Block Parts and emits a MO file for a single target
// locale. msgctxt is resolved from (in order): Block.Properties["context"],
// Block.Name, Block.ID — whichever is first non-empty. This lets blocks
// produced by the PO filter (which uses Properties["context"]) and blocks
// produced by the JSON filter with useFullKeyPath (which sets Block.Name to
// the key path) both round-trip through MO without re-configuration.
//
// Untranslated entries (no target for the configured locale) are dropped —
// MO catalogs are expected to contain only translations. The required
// empty-msgid metadata header entry is always emitted.
package mo

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"slices"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// MO format constants.
const (
	moMagic   uint32 = 0x950412de
	moVersion uint32 = 0
	moHeader         = "Content-Type: text/plain; charset=UTF-8\n"
	eot              = "\x04"
)

// Writer implements DataFormatWriter for gettext MO files.
type Writer struct {
	format.BaseFormatWriter
	cfg *Config
}

// NewWriter creates a new MO writer.
func NewWriter() *Writer {
	return &Writer{
		BaseFormatWriter: format.BaseFormatWriter{FormatName: "mo", Interchange: true},
		cfg:              &Config{},
	}
}

// Config returns the writer's configuration.
func (w *Writer) Config() format.DataFormatConfig { return w.cfg }

// SetConfig applies a new configuration after validation.
func (w *Writer) SetConfig(cfg format.DataFormatConfig) error {
	if cfg == nil {
		return nil
	}
	c, ok := cfg.(*Config)
	if !ok {
		return fmt.Errorf("mo: expected *Config, got %T", cfg)
	}
	w.cfg = c
	return nil
}

// Write consumes Parts and emits a gettext MO binary catalog.
func (w *Writer) Write(ctx context.Context, parts <-chan *model.Part) error {
	if w.Output == nil {
		return errors.New("mo writer: no output configured")
	}

	entries := make(map[string]string) // msgid (possibly with ctxt prefix) → msgstr

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case part, ok := <-parts:
			if !ok {
				return w.flush(entries)
			}
			if part.Type != model.PartBlock {
				continue
			}
			block, ok := part.Resource.(*model.Block)
			if !ok {
				continue
			}
			src := block.SourceText()
			if src == "" {
				continue
			}
			if w.Locale.IsEmpty() || !block.HasTarget(w.Locale) {
				continue
			}
			tgt := block.TargetText(w.Locale)
			if tgt == "" {
				continue
			}
			key := msgidWithContext(blockContext(block), src)
			// First write wins for duplicate keys — stable output in the face of
			// stream ordering quirks.
			if _, exists := entries[key]; !exists {
				entries[key] = tgt
			}
		}
	}
}

// blockContext picks the string that becomes msgctxt for this block.
// Properties["context"] takes precedence (for PO → MO round-trips that
// already carry the msgctxt verbatim), then Block.Name (the full key path
// from the JSON filter when useFullKeyPath is set), then Block.ID.
func blockContext(b *model.Block) string {
	if ctx, ok := b.Properties["context"]; ok && ctx != "" {
		return ctx
	}
	if b.Name != "" {
		return b.Name
	}
	return b.ID
}

// msgidWithContext encodes the (msgctxt, msgid) pair the way gettext does
// internally — ctxt + EOT (0x04) + msgid — so binary search against the
// sorted table works with a single string.
func msgidWithContext(ctx, msgid string) string {
	if ctx == "" {
		return msgid
	}
	return ctx + eot + msgid
}

// flush assembles the collected entries into a gettext MO binary and
// writes it to w.Output.
func (w *Writer) flush(entries map[string]string) error {
	// Always emit the metadata header entry (msgid="").
	if _, ok := entries[""]; !ok {
		entries[""] = moHeader
	}

	keys := make([]string, 0, len(entries))
	for k := range entries {
		keys = append(keys, k)
	}
	// Gettext requires entries sorted by the (contextualized) msgid.
	slices.Sort(keys)

	n := uint32(len(keys))
	const headerSize uint32 = 7 * 4

	origTabOff := headerSize
	transTabOff := origTabOff + 8*n
	stringsStart := transTabOff + 8*n

	// Build the string blobs. Each string is NUL-terminated in the file,
	// but the length field excludes the NUL.
	var origBuf, transBuf bytes.Buffer
	origOff := make([]uint32, n)
	origLen := make([]uint32, n)
	transOff := make([]uint32, n)
	transLen := make([]uint32, n)

	for i, k := range keys {
		origOff[i] = stringsStart + uint32(origBuf.Len())
		origLen[i] = uint32(len(k))
		origBuf.WriteString(k)
		origBuf.WriteByte(0)
	}
	transStart := stringsStart + uint32(origBuf.Len())
	for i, k := range keys {
		v := entries[k]
		transOff[i] = transStart + uint32(transBuf.Len())
		transLen[i] = uint32(len(v))
		transBuf.WriteString(v)
		transBuf.WriteByte(0)
	}

	le := binary.LittleEndian
	var hdr [7 * 4]byte
	le.PutUint32(hdr[0:4], moMagic)
	le.PutUint32(hdr[4:8], moVersion)
	le.PutUint32(hdr[8:12], n)
	le.PutUint32(hdr[12:16], origTabOff)
	le.PutUint32(hdr[16:20], transTabOff)
	le.PutUint32(hdr[20:24], 0) // no hash table
	le.PutUint32(hdr[24:28], transStart+uint32(transBuf.Len()))

	if _, err := w.Output.Write(hdr[:]); err != nil {
		return err
	}
	var buf [8]byte
	for i := range keys {
		le.PutUint32(buf[0:4], origLen[i])
		le.PutUint32(buf[4:8], origOff[i])
		if _, err := w.Output.Write(buf[:]); err != nil {
			return err
		}
	}
	for i := range keys {
		le.PutUint32(buf[0:4], transLen[i])
		le.PutUint32(buf[4:8], transOff[i])
		if _, err := w.Output.Write(buf[:]); err != nil {
			return err
		}
	}
	if _, err := w.Output.Write(origBuf.Bytes()); err != nil {
		return err
	}
	if _, err := w.Output.Write(transBuf.Bytes()); err != nil {
		return err
	}
	return nil
}
