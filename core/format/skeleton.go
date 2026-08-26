package format

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
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
	// SkeletonOriginal carries the source bytes that the NEXT SkeletonRef
	// stands in for, paired with the text that ref renders to while nothing
	// has edited its block. A reader emits it when extraction normalized the
	// block — collapsing source line breaks inside the text, dropping edge
	// whitespace — so the bytes it hands the writer differ from the bytes it
	// read. A writer that understands it replays the original bytes in place
	// of the ref when the block still renders to the paired text, and takes
	// its ordinary ref path otherwise; a document nothing edited therefore
	// round-trips byte-for-byte while an edited block keeps the normalized
	// join that extraction parity depends on.
	//
	// Payload: EncodeSkeletonPair(rendered, original).
	SkeletonOriginal SkeletonEntryType = 3
	// SkeletonTrimmed carries source bytes a reader trimmed off the trailing
	// edge of the text unit named by the PRECEDING SkeletonRef — whitespace
	// that belongs to neither the extracted text nor any other skeleton entry,
	// paired with the text that ref renders to while nothing has edited its
	// block. A writer that understands it emits the bytes when the preceding
	// block still renders to the paired text, restoring source formatting the
	// extraction dropped, and suppresses them otherwise.
	//
	// Payload: EncodeSkeletonPair(rendered, trimmed).
	SkeletonTrimmed SkeletonEntryType = 4
)

// EncodeSkeletonPair packs two byte strings into one length-prefixed payload:
// a 4-byte big-endian length for a, then a, then b. Both SkeletonOriginal and
// SkeletonTrimmed carry a pair whose first half is a block's rendered text —
// content that can hold any byte — so a separator would need escaping and a
// length prefix does not.
func EncodeSkeletonPair(a, b []byte) []byte {
	out := make([]byte, 4, 4+len(a)+len(b))
	binary.BigEndian.PutUint32(out, uint32(len(a)))
	out = append(out, a...)
	out = append(out, b...)
	return out
}

// DecodeSkeletonPair splits a payload written by EncodeSkeletonPair. ok is
// false for a truncated payload, which a writer treats as an entry it cannot
// act on rather than an error — the SkeletonRef beside it still carries the
// content, so the round-trip degrades to the un-restored output instead of
// losing text.
func DecodeSkeletonPair(data []byte) (a, b []byte, ok bool) {
	if len(data) < 4 {
		return nil, nil, false
	}
	n := binary.BigEndian.Uint32(data[:4])
	if uint64(n) > uint64(len(data)-4) {
		return nil, nil, false
	}
	return data[4 : 4+n], data[4+n:], true
}

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

	// originFormat names the format that produced this skeleton. A skeleton is
	// the typed scaffolding of ONE format — it is meaningful only to that
	// format's writer. WireSkeleton stamps it and refuses to feed the skeleton
	// to a writer of a different format, so a cross-format conversion can never
	// consume a foreign skeleton (it reconstructs from the content model
	// instead). Empty = untyped (legacy callers).
	originFormat string

	// stream, when non-nil, switches the store into concurrent streaming mode:
	// the reader appends entries while the writer pops them, so a streaming
	// round-trip never materialises the whole skeleton in memory. In this mode
	// the file/buf backings are unused; Flush is a no-op and Bytes is
	// unsupported. See NewStreamingSkeletonStore.
	stream *skeletonStream

	// writeErr holds the first error a buffered write hit. WriteText/WriteRef/
	// WriteLang do not return an error: a memory- or streaming-backed store
	// cannot fail a write, and a file-backed store buffers through a bufio.Writer
	// whose I/O only reaches disk on an internal flush — an error there is
	// retained here (and by bufio) and surfaced at Flush/Bytes, which every
	// consumer already checks before replaying the skeleton. Recording it once,
	// stickily, keeps that single surface honest without a per-write error return
	// that all ~113 reader call sites would otherwise discard.
	writeErr error
}

// skeletonStream is the unbounded, concurrency-safe queue backing a streaming
// SkeletonStore. The reader (one goroutine) appends entries; the writer (one
// goroutine) pops them, blocking until an entry is available or the write side
// is closed. It is unbounded on purpose: the reader must never block writing a
// skeleton entry (it would stall the Part stream that the writer is draining,
// deadlocking the pipeline). With an order-preserving tool chain the writer
// keeps pace and the queue holds only a small window; a fully-buffering tool
// makes it grow to O(n), trading the memory win for guaranteed liveness.
type skeletonStream struct {
	mu     sync.Mutex
	cond   *sync.Cond
	queue  []SkeletonEntry
	closed bool
}

// NewStreamingSkeletonStore creates a skeleton store in concurrent streaming
// mode. The reader writes entries (WriteText/WriteRef/WriteLang) from its read
// goroutine and calls CloseWrite when done; the writer reads them via Next from
// its write goroutine, interleaved with the arriving Part stream (see
// StreamSkeletonWrite). Output is byte-identical to the buffered skeleton path;
// peak memory is bounded to a small window for an order-preserving flow.
func NewStreamingSkeletonStore() *SkeletonStore {
	st := &skeletonStream{}
	st.cond = sync.NewCond(&st.mu)
	return &SkeletonStore{stream: st}
}

// IsStreaming reports whether the store is in concurrent streaming mode. A
// StreamingWriter checks this to choose its interleaved write path.
func (s *SkeletonStore) IsStreaming() bool { return s.stream != nil }

// CloseWrite signals that no more entries will be written, so a blocked Next
// returns io.EOF once the queue drains. Streaming mode only; a no-op otherwise.
// The reader's feeder calls this after the read channel closes (which is
// strictly after the reader's final skeleton write).
func (s *SkeletonStore) CloseWrite() {
	if s.stream == nil {
		return
	}
	s.stream.mu.Lock()
	s.stream.closed = true
	s.stream.cond.Broadcast()
	s.stream.mu.Unlock()
}

// OriginFormat returns the format id that produced this skeleton, or "".
func (s *SkeletonStore) OriginFormat() string { return s.originFormat }

// SetOriginFormat records the format that produced this skeleton.
func (s *SkeletonStore) SetOriginFormat(name string) { s.originFormat = name }

// EntriesWritten returns the number of skeleton entries written so far.
// Useful for callers that wire SetSkeletonStore on both reader and writer
// but want to avoid the writer's skeleton path when the reader produced
// nothing — typically a partial implementation that satisfies
// SkeletonStoreEmitter without actually emitting.
// In streaming mode the counter is incremented by the reader's goroutine while
// the writer's goroutine may be asking, so the read takes the same lock the
// increment does.
func (s *SkeletonStore) EntriesWritten() int {
	if s.stream == nil {
		return s.entries
	}
	s.stream.mu.Lock()
	defer s.stream.mu.Unlock()
	return s.entries
}

// Bytes returns a copy of the serialized skeleton stream written so far. It
// flushes pending writes first, so it is valid to call right after the last
// WriteText/WriteRef/WriteLang without a separate Flush. For a file-backed
// store the bytes are read back from the backing file; for a memory-backed
// store they come straight from the buffer. The returned slice is a copy the
// caller owns, so embedding it (e.g. as a .kpz skeleton member) is safe even
// after the store is closed. Used by kapi to capture a source's skeleton into
// a portable package (AD-025 §6).
func (s *SkeletonStore) Bytes() ([]byte, error) {
	if s.stream != nil {
		return nil, errors.New("skeleton store: Bytes is unsupported in streaming mode")
	}
	if s.writeErr != nil {
		return nil, fmt.Errorf("skeleton store: deferred write: %w", s.writeErr)
	}
	if s.writer != nil {
		if err := s.writer.Flush(); err != nil {
			return nil, err
		}
	}
	if s.buf != nil {
		out := make([]byte, s.buf.Len())
		copy(out, s.buf.Bytes())
		return out, nil
	}
	if s.file == nil {
		return nil, errors.New("skeleton store: no backing storage")
	}
	// File-backed: read the whole file back from the start without disturbing
	// any reader position the caller may rely on later.
	data, err := os.ReadFile(s.file.Name())
	if err != nil {
		return nil, fmt.Errorf("skeleton store: read bytes: %w", err)
	}
	return data, nil
}

// NewSkeletonStore creates a new skeleton store backed by a temporary file.
// The file is removed when Close is called — use NewSkeletonStoreAt for a
// store that survives Close() for later reuse (e.g. kapi extract capturing
// a source-file skeleton for kapi merge).
//
// On a platform with no temp filesystem at all (js/wasm — see
// tempFilesystemAvailable) the backing is in-memory *by design*: there is no
// alternative there, and browser documents are small. On every platform that
// does have one, a temp file that cannot be created is an operational fault and
// is returned as an error. It used to fall back to memory unconditionally, which
// silently substituted an unbounded in-memory buffer for a spill-to-disk store —
// and, worse, made this error unreachable, so the callers that discarded it (all
// seven of them, silently degrading a faithful round-trip into a reconstruction)
// could never be caught. Callers must propagate: see NewWiredSkeleton.
func NewSkeletonStore() (*SkeletonStore, error) {
	if !tempFilesystemAvailable {
		return NewMemorySkeletonStore(), nil
	}
	f, err := os.CreateTemp("", "neokapi-skeleton-*")
	if err != nil {
		return nil, fmt.Errorf("skeleton store: cannot create a temporary file in %s (set TMPDIR to a writable directory with free space): %w", os.TempDir(), err)
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
// extract to persist per-source skeletons under .kapi/work/cache/extractions/<id>/
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

// NewSkeletonStoreFromBytes returns a read-mode skeleton store backed by the
// given serialized stream (the bytes a prior Bytes() call produced). Use it to
// consume a skeleton carried inline rather than on the filesystem — e.g. a
// .kpz package that embeds the round-trip skeleton as a member (AD-025 §6).
// The store is ready to read via Next(); Close() is a no-op (no backing file).
func NewSkeletonStoreFromBytes(data []byte) *SkeletonStore {
	buf := bytes.NewBuffer(append([]byte(nil), data...))
	return &SkeletonStore{
		buf:    buf,
		reader: bufio.NewReader(buf),
	}
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

// WriteText writes a non-translatable text entry to the store. It does not
// return an error: writes buffer (memory/streaming backings cannot fail; a
// file backing goes through a bufio.Writer), so any I/O error is retained on
// the store and surfaced at Flush/Bytes, which every consumer checks before
// replaying the skeleton.
func (s *SkeletonStore) WriteText(data []byte) {
	if len(data) == 0 {
		return
	}
	s.writeEntry(SkeletonText, data)
}

// WriteRef writes a block ID reference entry to the store. Like WriteText it
// does not return an error; see WriteText for why.
func (s *SkeletonStore) WriteRef(blockID string) {
	s.writeEntry(SkeletonRef, []byte(blockID))
}

// WriteLang writes a language-attribute value entry to the store. The value
// is the raw source-locale lang value spliced out of a lang=/xml:lang=
// attribute, so the writer can retarget it structurally instead of
// rewriting serialized bytes. See SkeletonLang for the consumption contract.
// Like WriteText it does not return an error; see WriteText for why.
func (s *SkeletonStore) WriteLang(value string) {
	s.writeEntry(SkeletonLang, []byte(value))
}

// WriteOriginal writes the source bytes the next block ref stands in for,
// paired with the text that ref renders to unedited. See SkeletonOriginal for
// the consumption contract. Like WriteText it does not return an error.
func (s *SkeletonStore) WriteOriginal(rendered, original []byte) {
	s.writeEntry(SkeletonOriginal, EncodeSkeletonPair(rendered, original))
}

// WriteTrimmed writes source bytes trimmed off the trailing edge of the
// preceding block's text unit, paired with the text that block renders to
// unedited. See SkeletonTrimmed for the consumption contract. Like WriteText
// it does not return an error.
func (s *SkeletonStore) WriteTrimmed(rendered, trimmed []byte) {
	if len(trimmed) == 0 {
		return
	}
	s.writeEntry(SkeletonTrimmed, EncodeSkeletonPair(rendered, trimmed))
}

func (s *SkeletonStore) writeEntry(typ SkeletonEntryType, data []byte) {
	if s.stream != nil {
		// Copy: callers reuse their buffer (e.g. skelBuf.Reset()) right after
		// WriteText, so the queued slice must own its bytes.
		cp := make([]byte, len(data))
		copy(cp, data)
		s.stream.mu.Lock()
		s.stream.queue = append(s.stream.queue, SkeletonEntry{Type: typ, Data: cp})
		s.entries++
		s.stream.cond.Signal()
		s.stream.mu.Unlock()
		return
	}
	if s.writeErr != nil {
		return // stay sticky: a prior write already failed
	}
	if err := s.writer.WriteByte(byte(typ)); err != nil {
		s.writeErr = err
		return
	}
	var lenBuf [4]byte
	binary.BigEndian.PutUint32(lenBuf[:], uint32(len(data)))
	if _, err := s.writer.Write(lenBuf[:]); err != nil {
		s.writeErr = err
		return
	}
	if _, err := s.writer.Write(data); err != nil {
		s.writeErr = err
		return
	}
	s.entries++
}

// Flush finishes writing and prepares the store for reading. On stores
// opened via OpenSkeletonStore (already in read mode) this is a no-op.
func (s *SkeletonStore) Flush() error {
	if s.stream != nil {
		// Streaming stores are read concurrently via Next; there is nothing to
		// flush or seek.
		return nil
	}
	if s.writeErr != nil {
		// A buffered write failed earlier (WriteText/WriteRef/WriteLang do not
		// return an error); surface it here, the single point every consumer
		// checks before replaying the skeleton.
		return fmt.Errorf("skeleton store: deferred write: %w", s.writeErr)
	}
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
	if s.stream != nil {
		s.stream.mu.Lock()
		defer s.stream.mu.Unlock()
		for len(s.stream.queue) == 0 && !s.stream.closed {
			s.stream.cond.Wait()
		}
		if len(s.stream.queue) == 0 {
			return SkeletonEntry{}, io.EOF
		}
		entry := s.stream.queue[0]
		s.stream.queue[0] = SkeletonEntry{}
		s.stream.queue = s.stream.queue[1:]
		return entry, nil
	}
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

// Close flushes any pending writes and closes the backing file, reporting a
// deferred write error or a failed flush as well as a failed close. For stores
// created with NewSkeletonStore the temp file is removed; stores created with
// NewSkeletonStoreAt / OpenSkeletonStore leave the file in place for later
// reuse. It always closes the file, even when it returns an error.
//
// Close is a surfacing point for s.writeErr, not just Flush/Bytes: the
// *persistent* producers (kapi extract, the project document cache) only ever
// call Close, so dropping the deferred write error here left a TRUNCATED
// skeleton file that stats clean. A later replay then stops at the short record
// boundary and reads it as end-of-document — a truncated deliverable reported as
// success, the same family of defect as #1449.
func (s *SkeletonStore) Close() error {
	if s.stream != nil {
		// Unblock any reader still parked in Next so the pipeline can unwind on
		// an early error.
		s.CloseWrite()
		return nil
	}
	var writeErr error
	if s.writeErr != nil {
		writeErr = fmt.Errorf("skeleton store: deferred write: %w", s.writeErr)
	}
	if s.writer != nil {
		if ferr := s.writer.Flush(); ferr != nil && writeErr == nil {
			writeErr = fmt.Errorf("skeleton store: flush: %w", ferr)
		}
	}
	if s.buf != nil {
		// Memory-backed: no file to close or remove.
		return writeErr
	}
	name := s.file.Name()
	err := s.file.Close()
	if !s.persistent {
		_ = os.Remove(name)
	}
	// The write failure is the root cause; a close error is secondary noise.
	if writeErr != nil {
		return writeErr
	}
	return err
}

// SkeletonStoreEmitter is implemented by readers that produce skeleton data.
type SkeletonStoreEmitter interface {
	SetSkeletonStore(store *SkeletonStore)
}

// SkeletonStoreConsumer is implemented by writers that consume skeleton data.
// Consuming a skeleton (for byte-exact same-format fidelity) is ORTHOGONAL to
// being generative: HTML both consumes a skeleton when given one and writes a
// clean document without one. So this interface does not say a writer needs a
// skeleton — only that it will use one if offered (see GenerativeWriter).
type SkeletonStoreConsumer interface {
	SetSkeletonStore(store *SkeletonStore)
}

// SkeletonPairEligible reports whether a reader→writer pair has a skeleton path
// at all: the reader emits one, the writer consumes one, and they are the same
// format (a skeleton is meaningless to a foreign writer — see WireSkeleton).
// When this is false the writer legitimately reconstructs from the content
// model, which is the documented lower-fidelity path; when it is true, the
// skeleton is the only thing that carries the structure the reader could not
// model, so failing to obtain a store is a fault, not a fallback.
func SkeletonPairEligible(reader DataFormatReader, writer DataFormatWriter) bool {
	if reader == nil || writer == nil {
		return false
	}
	if _, ok := reader.(SkeletonStoreEmitter); !ok {
		return false
	}
	if _, ok := writer.(SkeletonStoreConsumer); !ok {
		return false
	}
	return reader.Name() == writer.Name()
}

// NewWiredSkeleton creates the temp-file-backed skeleton store a same-format
// reader→writer pair needs and wires both ends. It is the single seam for "give
// this round-trip its skeleton", so the decision cannot drift across the call
// sites that make it (core/flow's file and stream runners, the toolbox
// edit/convert/archive paths, the tool runner, and the desktop's in-place
// rewrite).
//
// The backing is deliberately NOT chosen from the pair's streaming capability.
// A streaming store blocks the writer inside Next() until the reader's feeder
// calls CloseWrite, which only the concurrent read↔write protocol in core/flow
// does; a caller that buffers the whole part stream and then writes would
// deadlock. Callers that drive that protocol use NewWiredStreamingSkeleton.
//
// Returns:
//
//   - (nil, nil) — the pair has no skeleton path (SkeletonPairEligible is
//     false). The writer reconstructs from the content model; that is the
//     documented behaviour, not a degradation.
//   - (nil, err) — a skeleton path exists but the store could not be created.
//     This must never be swallowed. Discarding it silently turns a byte-exact
//     same-format pass into a re-serialization from the content model: the
//     structure the reader could not model (WordprocessingML fragments, unknown
//     elements, comment and whitespace layout, attribute spacing) is dropped
//     while the command still reports success.
func NewWiredSkeleton(reader DataFormatReader, writer DataFormatWriter) (*SkeletonStore, error) {
	if !SkeletonPairEligible(reader, writer) {
		return nil, nil
	}
	store, err := NewSkeletonStore()
	if err != nil {
		return nil, fmt.Errorf("the %s round-trip needs a skeleton store to preserve the original formatting, and one could not be created — without it the document would be rewritten from the content model and its exact formatting lost: %w", reader.Name(), err)
	}
	WireSkeleton(store, reader, writer)
	return store, nil
}

// NewWiredStreamingSkeleton is NewWiredSkeleton's counterpart for a caller that
// drives the concurrent streaming protocol (the reader feeds entries from its
// read goroutine and calls CloseWrite; the writer pops them interleaved with the
// Part stream). The store is channel-backed, so it needs no temp file and cannot
// fail — the error return exists on NewWiredSkeleton alone. Returns nil when the
// pair has no skeleton path.
func NewWiredStreamingSkeleton(reader DataFormatReader, writer DataFormatWriter) *SkeletonStore {
	if !SkeletonPairEligible(reader, writer) {
		return nil
	}
	store := NewStreamingSkeletonStore()
	WireSkeleton(store, reader, writer)
	return store
}

// WireSkeleton connects a reader's skeleton emission to a writer — but only when
// they are the SAME format. A skeleton is the typed scaffolding of one format
// (origin-tagged on the store), foreign to any other writer, so a cross-format
// conversion must never feed it to the target writer; that writer reconstructs
// from the content model via its generative path instead. WireSkeleton stamps
// the store's origin with the reader's format and wires the consumer only on a
// matching format. It returns true when the writer was wired (same-format
// round-trip). Callers pass a store with the backing they need (file in the CLI,
// memory in wasm).
func WireSkeleton(store *SkeletonStore, reader DataFormatReader, writer DataFormatWriter) bool {
	if store == nil {
		return false
	}
	store.SetOriginFormat(reader.Name())
	emitter, eok := reader.(SkeletonStoreEmitter)
	if !eok {
		return false
	}
	emitter.SetSkeletonStore(store)
	consumer, cok := writer.(SkeletonStoreConsumer)
	if !cok || reader.Name() != writer.Name() {
		return false // foreign (or non-consuming) writer: skeleton stays unread
	}
	consumer.SetSkeletonStore(store)
	return true
}

// GenerativeWriter is implemented by writers that can declare whether they
// serialize a complete document from the content model alone. BaseFormatWriter
// implements it (via the RequiresSkeleton declaration), so every writer that
// embeds the base exposes it. The registry probes this once at registration to
// record FormatInfo.Generative — the declarative, no-plugin-load source of truth
// for whether a format is a valid cross-format conversion target.
type GenerativeWriter interface {
	Generative() bool
}

// InterchangeWriter is implemented by bilingual translation-interchange writers
// (XLIFF, PO, TMX, …). The registry records it on FormatInfo.Interchange so
// `convert` can exclude them — they belong to the extract/merge loop, not to
// document conversion. BaseFormatWriter implements it via the Interchange field.
type InterchangeWriter interface {
	IsInterchange() bool
}

// InlineAnnotationWriter is implemented by writers that can project a block's
// stand-off annotations into the document as inline marks: a located term as an
// XLIFF <mrk>, as an HTML <span>. It names the annotation types the writer
// knows how to draw, which the registry records on
// FormatInfo.InlineAnnotations.
//
// The capability is what a format can carry, not what it will. A recipe narrows
// the set under `defaults.annotations.write` and never widens it, so a format
// that gains the capability starts projecting without a recipe edit, and a
// project that wants a clean export can still ask for one.
//
// A writer that implements nothing here carries no marks, and the block's
// annotations stay stand-off. That is the default, and it is the right one:
// content leaves as content, and a governance layer travels beside it rather
// than inside it unless someone asked for the marks.
type InlineAnnotationWriter interface {
	InlineAnnotations() []string
}
