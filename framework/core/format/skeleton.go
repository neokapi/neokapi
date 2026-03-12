package format

import (
	"bufio"
	"encoding/binary"
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

// SkeletonStore streams document skeleton data through temporary storage.
// Readers write text/ref entries as they parse; writers read them to reconstruct.
//
// Binary format: [type:1byte] [length:4bytes big-endian] [data:N bytes]
type SkeletonStore struct {
	file   *os.File
	writer *bufio.Writer
	reader *bufio.Reader
}

// NewSkeletonStore creates a new skeleton store backed by a temporary file.
func NewSkeletonStore() (*SkeletonStore, error) {
	f, err := os.CreateTemp("", "gokapi-skeleton-*")
	if err != nil {
		return nil, fmt.Errorf("skeleton store: create temp: %w", err)
	}
	return &SkeletonStore{
		file:   f,
		writer: bufio.NewWriter(f),
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

// Flush finishes writing and prepares the store for reading.
func (s *SkeletonStore) Flush() error {
	if err := s.writer.Flush(); err != nil {
		return err
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
		return SkeletonEntry{}, fmt.Errorf("skeleton store: must call Flush before reading")
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

// Close removes the temporary file and releases resources.
func (s *SkeletonStore) Close() error {
	name := s.file.Name()
	s.file.Close()
	return os.Remove(name)
}

// SkeletonStoreEmitter is implemented by readers that produce skeleton data.
type SkeletonStoreEmitter interface {
	SetSkeletonStore(store *SkeletonStore)
}

// SkeletonStoreConsumer is implemented by writers that consume skeleton data.
type SkeletonStoreConsumer interface {
	SetSkeletonStore(store *SkeletonStore)
}
