package format

import (
	"bufio"
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
	writer     *bufio.Writer
	reader     *bufio.Reader
	persistent bool // when true, Close() does not remove the backing file
}

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

// NewSkeletonStoreAt creates a new skeleton store at a specific path. The
// caller is responsible for directory creation and file cleanup; Close()
// flushes and closes the file but does not remove it. Used by kapi
// extract to persist per-source skeletons under .kapi/extractions/<id>/
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

func (s *SkeletonStore) writeEntry(typ SkeletonEntryType, data []byte) error {
	if err := s.writer.WriteByte(byte(typ)); err != nil {
		return err
	}
	var lenBuf [4]byte
	binary.BigEndian.PutUint32(lenBuf[:], uint32(len(data)))
	if _, err := s.writer.Write(lenBuf[:]); err != nil {
		return err
	}
	_, err := s.writer.Write(data)
	return err
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
