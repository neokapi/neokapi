package format

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
)

// SkeletonEntryType identifies the type of a skeleton entry.
type SkeletonEntryType byte

const (
	// SkeletonText is a non-translatable raw bytes entry.
	SkeletonText SkeletonEntryType = 0
	// SkeletonRef is a block ID placeholder entry.
	SkeletonRef SkeletonEntryType = 1
	// SkeletonLang is a language-attribute value entry. The payload is the
	// SOURCE-locale lang value as it appeared in the document (the raw bytes
	// between the surrounding quotes of a lang=/xml:lang= attribute). Writers
	// that retarget the document language emit the target locale when the
	// stored value matches the document's source locale, otherwise they emit
	// the stored value verbatim. Writers that do not understand this entry
	// type must treat it as inert — emitting nothing for it would drop the
	// attribute value. Only the HTML reader emits SkeletonLang today; other
	// formats never see it, and their entry-type switches ignore unknown
	// types (no default case), so this addition is purely additive.
	SkeletonLang SkeletonEntryType = 2
)

// SkeletonEntry is a single entry in the skeleton stream.
type SkeletonEntry struct {
	Type SkeletonEntryType
	Data []byte
}

// SkeletonStore streams document skeleton data through temporary or
// persistent storage. Readers write text/ref entries as they parse;
// writers read them to reconstruct.
//
// Binary format: [type:1byte] [length:4bytes big-endian] [data:N bytes]
type SkeletonStore struct {
	file       *os.File
	buf        *bytes.Buffer // non-nil for memory-backed stores (no filesystem, e.g. wasm)
	writer     *bufio.Writer
	reader     *bufio.Reader
	persistent bool // when true, Close() does not remove the backing file
	entries    int  // count of entries written; lets callers detect a
	// reader that registered for skeleton emission but never actually
	// wrote any (a stubbed implementation) so they can fall back to
	// the writer's no-skeleton path instead of producing empty output.
}

// EntriesWritten returns the number of skeleton entries written so far.
// Useful for callers that wire SetSkeletonStore on both reader and writer
// but want to avoid the writer's skeleton path when the reader produced
// nothing — typically a partial implementation that satisfies
// SkeletonStoreEmitter without actually emitting.
func (s *SkeletonStore) EntriesWritten() int { return s.entries }

// NewSkeletonStore creates a new skeleton store backed by a temporary file.
// The file is removed when Close is called — use NewSkeletonStoreAt for a
// store that survives Close() for later reuse (e.g. kapi extract capturing
// a source-file skeleton for kapi merge).
func NewSkeletonStore() (*SkeletonStore, error) {
	f, err := os.CreateTemp("", "neokapi-skeleton-*")
	if err != nil {
		return nil, fmt.Errorf("skeleton store: create temp: %w", err)
	}
	return &SkeletonStore{
		file:   f,
		writer: bufio.NewWriter(f),
	}, nil
}

// NewMemorySkeletonStore creates a skeleton store backed entirely by an
// in-memory buffer. Use it where no filesystem is available (e.g. the
// js/wasm browser build, where os.CreateTemp fails). Close() is a no-op
// beyond flushing; there is no backing file to remove.
func NewMemorySkeletonStore() *SkeletonStore {
	buf := &bytes.Buffer{}
	return &SkeletonStore{
		buf:    buf,
		writer: bufio.NewWriter(buf),
	}
}

// NewSkeletonStoreAt creates a new skeleton store at a specific path. The
// caller is responsible for directory creation and file cleanup; Close()
// flushes and closes the file but does not remove it. Used by kapi
// extract to persist per-source skeletons under .kapi/cache/extractions/<id>/
// for later use by kapi merge.
func NewSkeletonStoreAt(path string) (*SkeletonStore, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("skeleton store: create %s: %w", path, err)
	}
	return &SkeletonStore{
		file:       f,
		writer:     bufio.NewWriter(f),
		persistent: true,
	}, nil
}

// OpenSkeletonStore opens an existing persisted skeleton file for reading.
// Used by kapi merge to load the source-file skeleton captured by a prior
// extract run. Callers do not need to call Flush — the store is returned
// in read mode. Close() will not remove the backing file.
func OpenSkeletonStore(path string) (*SkeletonStore, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("skeleton store: open %s: %w", path, err)
	}
	return &SkeletonStore{
		file:       f,
		reader:     bufio.NewReader(f),
		persistent: true,
	}, nil
}

// WriteText writes a non-translatable text entry to the store.
func (s *SkeletonStore) WriteText(data []byte) error {
	if len(data) == 0 {
		return nil
	}
	return s.writeEntry(SkeletonText, data)
}

// WriteRef writes a block ID reference entry to the store.
func (s *SkeletonStore) WriteRef(blockID string) error {
	return s.writeEntry(SkeletonRef, []byte(blockID))
}

// WriteLang writes a language-attribute value entry to the store. The value
// is the raw source-locale lang value spliced out of a lang=/xml:lang=
// attribute, so the writer can retarget it structurally instead of
// rewriting serialized bytes. See SkeletonLang for the consumption contract.
func (s *SkeletonStore) WriteLang(value string) error {
	return s.writeEntry(SkeletonLang, []byte(value))
}

func (s *SkeletonStore) writeEntry(typ SkeletonEntryType, data []byte) error {
	if err := s.writer.WriteByte(byte(typ)); err != nil {
		return err
	}
	var lenBuf [4]byte
	binary.BigEndian.PutUint32(lenBuf[:], uint32(len(data)))
	if _, err := s.writer.Write(lenBuf[:]); err != nil {
		return err
	}
	if _, err := s.writer.Write(data); err != nil {
		return err
	}
	s.entries++
	return nil
}

// Flush finishes writing and prepares the store for reading. On stores
// opened via OpenSkeletonStore (already in read mode) this is a no-op.
func (s *SkeletonStore) Flush() error {
	if s.reader != nil && s.writer == nil {
		// Already prepared for reading by OpenSkeletonStore — nothing to
		// flush. The seek-to-start guarantee still holds since the file
		// hasn't been read yet.
		return nil
	}
	if s.writer != nil {
		if err := s.writer.Flush(); err != nil {
			return err
		}
	}
	if s.buf != nil {
		// Memory-backed: the buffer already holds every written byte and
		// bytes.Buffer reads from the front, so there's nothing to seek.
		s.reader = bufio.NewReader(s.buf)
		return nil
	}
	if _, err := s.file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	s.reader = bufio.NewReader(s.file)
	return nil
}

// Next reads the next skeleton entry. Returns io.EOF when done.
func (s *SkeletonStore) Next() (SkeletonEntry, error) {
	if s.reader == nil {
		return SkeletonEntry{}, errors.New("skeleton store: must call Flush before reading")
	}
	typByte, err := s.reader.ReadByte()
	if err != nil {
		return SkeletonEntry{}, err
	}
	var lenBuf [4]byte
	if _, err := io.ReadFull(s.reader, lenBuf[:]); err != nil {
		return SkeletonEntry{}, fmt.Errorf("skeleton store: read length: %w", err)
	}
	length := binary.BigEndian.Uint32(lenBuf[:])
	data := make([]byte, length)
	if _, err := io.ReadFull(s.reader, data); err != nil {
		return SkeletonEntry{}, fmt.Errorf("skeleton store: read data: %w", err)
	}
	return SkeletonEntry{
		Type: SkeletonEntryType(typByte),
		Data: data,
	}, nil
}

// Close flushes any pending writes and closes the backing file. For
// stores created with NewSkeletonStore the temp file is removed; stores
// created with NewSkeletonStoreAt / OpenSkeletonStore leave the file in
// place for later reuse.
func (s *SkeletonStore) Close() error {
	if s.writer != nil {
		_ = s.writer.Flush()
	}
	if s.buf != nil {
		// Memory-backed: no file to close or remove.
		return nil
	}
	name := s.file.Name()
	err := s.file.Close()
	if !s.persistent {
		_ = os.Remove(name)
	}
	return err
}

// SkeletonStoreEmitter is implemented by readers that produce skeleton data.
type SkeletonStoreEmitter interface {
	SetSkeletonStore(store *SkeletonStore)
}

// SkeletonStoreConsumer is implemented by writers that consume skeleton data.
type SkeletonStoreConsumer interface {
	SetSkeletonStore(store *SkeletonStore)
}
