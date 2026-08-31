package kpz

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"time"

	"github.com/neokapi/neokapi/core/kbf"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/safeio"
	"github.com/neokapi/neokapi/memory/kmb"
	"github.com/neokapi/neokapi/terms/ktb"
)

// A .kpz is the profile that exists to cross a trust boundary: it is packed,
// handed to an outside translator or reviewer, and returned to be merged. An
// untrusted package is therefore the expected input, not an edge case, and
// Unmarshal treats every byte and every name in the manifest as content.
//
// Two invariants hold for anything that comes back off the wire:
//
//   - A package may only name paths INSIDE the project it is merged into.
//     Every manifest field that is a path is validated with
//     [safeio.IsLocalPath] before it can reach the filesystem.
//   - Parsing a package is bounded. The archive is checked against
//     [PackageZipLimits] up front, every entry is streamed through one
//     cumulative [safeio.ZipGuard], and whole-document members are hashed
//     without ever being buffered.

// PackageZipLimits bounds parsing a .kpz against the classic archive attack
// classes (oversized entry, decompression bomb, oversized payload, entry
// flood).
//
// The size caps are deliberately looser than [safeio.DefaultZipLimits] and the
// ratio cap identical. A `kapi pack --with-source` of a media-heavy project is a
// legitimate multi-gigabyte archive, so the default 4 GiB cumulative cap would
// reject real work; the guard that actually stops a bomb is the inflate ratio,
// which is enforced on the streamed bytes and is unaffected by how large an
// honest package is.
var PackageZipLimits = safeio.ZipLimits{
	MaxEntrySize:    2 << 30,  // 2 GiB — one source document or media blob
	MaxTotalSize:    16 << 30, // 16 GiB — a media-heavy `pack --with-source`
	MinInflateRatio: safeio.DefaultMinInflateRatio,
	MaxEntries:      safeio.DefaultMaxEntries,
}

// MaxInlineMemberSize bounds a member a caller materializes in memory with
// [ReadAll] — today the round-trip skeleton handed to a reconstructing writer.
// Whole-document and media members are streamed rather than buffered, so this
// applies to derived members only, and 256 MiB is far above any skeleton a real
// document produces.
const MaxInlineMemberSize int64 = 256 << 20

// Content provides a parcel member's bytes on demand. A .kpz member that can
// hold a whole document or media asset — a source document, a media blob, a
// round-trip skeleton stream — carries a Content reference rather than inlining
// the bytes on the public struct, the same reference-not-inline idiom as
// model.Media (BlobKey > URI > Data) and core/blockstore. Marshal reads it to
// hash and to stream into the archive; Unmarshal backs it with a lazy reader
// over the parcel's own ZIP entry, so a whole document is never retained on the
// in-memory Package.
type Content interface {
	// Open returns a reader over the member's bytes. The caller closes it.
	Open() (io.ReadCloser, error)
}

// FileContent references a member by an on-disk path, streamed on demand. The
// preferred producer-side form: the bytes never enter memory until Marshal
// copies them into the archive.
func FileContent(path string) Content { return fileContent{path} }

type fileContent struct{ path string }

func (f fileContent) Open() (io.ReadCloser, error) { return os.Open(f.path) }

// BytesContent wraps already-in-memory bytes — the inline escape hatch for
// genuinely small or freshly-derived members (e.g. a serialized skeleton the
// caller already holds), matching model.Media's small-Data mode.
func BytesContent(b []byte) Content { return bytesContent{b} }

type bytesContent struct{ b []byte }

func (b bytesContent) Open() (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(b.b)), nil
}

// zipContent backs a member by its entry in an already-open parcel archive,
// read lazily so Unmarshal never retains whole documents in memory. The reader
// is guarded: a member's declared size can lie, so the per-entry size and
// inflate-ratio caps are enforced on the bytes the entry actually produces.
type zipContent struct {
	f      *zip.File
	limits safeio.ZipLimits
}

func (z zipContent) Open() (io.ReadCloser, error) { return z.limits.OpenEntry(z.f) }

// ReadAll reads a member's full content into memory, bounded by
// [MaxInlineMemberSize]. Use only at a boundary that genuinely needs the bytes
// (e.g. handing a skeleton stream to a reconstructing writer); prefer streaming
// via Open elsewhere. Whole-document and media members are never read this way —
// they are streamed — so the bound applies to derived members only.
func ReadAll(c Content) ([]byte, error) {
	rc, err := c.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(safeio.NewLimitedReader(rc, MaxInlineMemberSize))
}

// hashContent streams a member through SHA-256 without retaining its bytes.
func hashContent(c Content) (string, error) {
	rc, err := c.Open()
	if err != nil {
		return "", err
	}
	defer rc.Close()
	h := sha256.New()
	if _, err := io.Copy(h, rc); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

const (
	// SchemaVersion is the package manifest version (MAJOR.MINOR, same
	// forward-compatibility contract as core/kbf).
	SchemaVersion = "1.0"
	// KindProject marks a whole-project snapshot .kpz (the pack/unpack
	// profile, AD-025 §7): all locales, the full recipe, content memory, terms,
	// overlays, and source identity + skeletons. The default Kind.
	KindProject = "kapi-project"
	// KindInterchange marks a task-scoped bilingual interchange .kpz (the
	// extract/merge profile, AD-025 §7): one source→target locale pair —
	// blocks, skeleton, target overlays, and the relevant content memory/term context.
	// neokapi's lossless interchange format for a translator or reviewer.
	KindInterchange = "kapi-interchange"

	// ManifestPath is the manifest member's path within the archive.
	ManifestPath = "manifest.json"

	ContentTypeBlocks      = "blocks"
	ContentTypeAnnotations = "annotations"
	ContentTypeMemory      = "memory"
	ContentTypeTerms       = "terms"
	ContentTypeMedia       = "media"
	// ContentTypeSkeleton carries one source document's round-trip skeleton
	// (the derived extract template `merge` reuses), keyed by source. Members
	// live under skeletons/<key> and ARE part of the content RootHash —
	// retaining a source as identity + skeleton is the default; raw bytes are
	// opt-in (AD-025 §6).
	ContentTypeSkeleton = "skeleton"
	// ContentTypeOverlays is the member kind carrying in-progress overlay
	// layers (targets, annotations, segmentation, …) keyed by
	// (kind, blockHash) — the substance of a resumable workspace. See
	// AD-025 §5.
	ContentTypeOverlays = "overlays"
	// ContentTypeSource is the member kind carrying an original input
	// document's bytes, so a `.kpz` written as in-progress run output can
	// reconstruct (resume / finish) the document in its source format. The
	// thing being worked on, not an asset referenced by content (media).
	ContentTypeSource = "source"
	// ContentTypeHistory is the advisory, append-only JSONL log of what ran,
	// when, and by whom (AD-025 §5). It is strictly subordinate to content:
	// deliberately EXCLUDED from the content RootHash, never read by resume
	// or status, and safe to delete with no loss of work. Opt-in.
	ContentTypeHistory = "history"

	// memoryPath and termsPath are the conventional bare bundle names, so
	// unzipping a package by hand yields the same spelling the rest of the
	// tooling teaches. Unpack routes members by their manifest ContentType,
	// never by filename, so these names are presentation only.
	memoryPath = kmb.ConventionalName
	termsPath  = ktb.ConventionalName
	// OverlaysPath is the single overlay-set member's archive path.
	OverlaysPath = "overlays.json"
	// HistoryPath is the advisory history log's archive path.
	HistoryPath = "history.jsonl"
	// SkeletonDir is the archive directory holding per-source skeleton
	// members.
	SkeletonDir = "skeletons/"
	// SourceDir is the archive directory holding raw source members (opt-in,
	// `pack --with-source`). Named here rather than spelled at each call site so
	// the writer and the reader that strips it back off cannot disagree.
	SourceDir = "source/"
)

// zipEpoch is a fixed modification time so the archive bytes are deterministic
// regardless of when the package was built (the DOS epoch zip uses).
var zipEpoch = time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC)

// Package is the in-memory form of a .kpz: the authoritative content of a
// project, grouped by content type. Any section may be empty.
type Package struct {
	// Kind is the package profile discriminator: KindProject (whole-project
	// snapshot, the default) or KindInterchange (bilingual task slice). Empty
	// means KindProject on marshal. See AD-025 §7.
	Kind        string
	Created     string
	Generator   *GeneratorInfo
	Blocks      []BlockDoc
	Annotations []AnnotationDoc
	Memory      *kmb.File
	Terms       *ktb.File
	Media       []Media

	// Skeletons carries each source document's round-trip skeleton (the
	// derived extract template `merge` reuses) — the default source-retention
	// payload (identity + skeleton, raw bytes opt-in; AD-025 §6). Members are
	// content (part of the RootHash).
	Skeletons []SkeletonDoc

	// Overlays carries in-progress overlay layers (targets, annotations, …)
	// when the package snapshots a project's working state rather than a
	// finished at-rest project. Empty for a plain at-rest package.
	// Serialized as a single deterministic overlays.json member.
	Overlays []OverlayDoc
	// Source carries the original input document bytes a `.kpz` was written
	// on (one per input), so reading the package back can re-stream the
	// source through the flow and write the finished output in its source
	// format. Empty for a plain at-rest project snapshot.
	Source []SourceDoc
	// History is the advisory, append-only JSONL log (raw bytes, one JSON
	// object per line). Opt-in and content-subordinate: it rides in the
	// package for hand-off convenience but is excluded from RootHash and
	// ignored by resume/status — deleting it loses no work. Empty by default.
	History []byte
	// Recipe is the FULL project recipe — the same core/project.KapiProject
	// schema a .kapi file uses — so a .kpz is a runnable project in a file
	// (AD-025 §6). nil for a package carrying no intent. Stored in the
	// manifest (metadata), kept out of the content RootHash. Side-effecting
	// Extras (server/hooks/automations) should be stripped via SanitizeRecipe
	// before packing so they travel inert.
	Recipe *project.KapiProject

	// Sources records each source document's identity (logical path, format,
	// content hash) plus whether its skeleton / raw bytes ride in the package.
	// Manifest metadata (not in the RootHash) — the substance is the skeleton
	// (content) and, opt-in, the raw source member.
	Sources []SourceIdentity

	// InterchangeTask, when set, scopes a KindInterchange package to one
	// source→target locale pair (AD-025 §7). nil for a project snapshot.
	// Manifest metadata, not part of the content RootHash.
	InterchangeTask *InterchangeTask
}

// HasContent reports whether the package carries any packable content — blocks,
// annotations, overlays, skeletons, media, raw source, content-memory entries, or terms
// concepts. Recipe, Sources identity, InterchangeTask, and History are metadata,
// not content: a package with only those is empty (nothing worth packing).
// Callers use this to refuse writing a content-less .kpz, the way `git bundle`
// refuses an empty bundle.
func (p *Package) HasContent() bool {
	return len(p.Blocks) > 0 ||
		len(p.Annotations) > 0 ||
		len(p.Overlays) > 0 ||
		len(p.Skeletons) > 0 ||
		len(p.Media) > 0 ||
		len(p.Source) > 0 ||
		(p.Memory != nil && len(p.Memory.Entries) > 0) ||
		(p.Terms != nil && len(p.Terms.Concepts) > 0)
}

// SourceIdentity records one source document's identity so a .kpz can detect
// drift, round-trip via the skeleton, and (opt-in) re-extract from raw bytes.
type SourceIdentity struct {
	// SourcePath is the source's logical path (relative to the project root,
	// or its base name for an ad-hoc workspace).
	SourcePath string `json:"sourcePath"`
	// FormatID is the registry format the source was read with.
	FormatID string `json:"formatId,omitempty"`
	// ContentHash is the source's content hash (sha256:hex) at capture time,
	// the staleness fingerprint.
	ContentHash string `json:"contentHash,omitempty"`
	// SkeletonPath names the skeletons/<key> member carrying this source's
	// round-trip skeleton, when one was captured. Empty when the format has no
	// skeleton emitter (merge re-reads the source instead).
	SkeletonPath string `json:"skeletonPath,omitempty"`
	// HasRawSource is true when the package also embeds this source's raw
	// bytes (source/<name> member) — opt-in via --with-source.
	HasRawSource bool `json:"hasRawSource,omitempty"`
}

// InterchangeTask scopes a KindInterchange package to one source→target locale
// pair handed to a translator or reviewer (AD-025 §7).
type InterchangeTask struct {
	// SourceLocale is the source locale of the blocks.
	SourceLocale string `json:"sourceLocale,omitempty"`
	// TargetLocale is the locale being produced.
	TargetLocale string `json:"targetLocale,omitempty"`
	// SourceFiles lists the source logical paths covered by this task.
	SourceFiles []string `json:"sourceFiles,omitempty"`
}

// GeneratorInfo identifies the tool that produced the package.
type GeneratorInfo struct {
	ID      string `json:"id"`
	Version string `json:"version,omitempty"`
}

// BlockDoc is one KBF document member (blocks + targets).
type BlockDoc struct {
	Path string // archive path under blocks/, e.g. "blocks/app.kbf.json"
	File *kbf.File
}

// AnnotationDoc is one .overlays.jsonl annotation-overlay member.
type AnnotationDoc struct {
	Path string // archive path under annotations/, e.g. "annotations/app.overlays.jsonl"
	File *kbf.AnnotationFile
}

// Media is one opaque blob member (e.g. an image referenced by content). The
// blob can be arbitrarily large, so it travels as a Content reference
// (file-backed when producing, ZIP-entry-backed when read), never inlined.
type Media struct {
	Path    string // archive path under media/, e.g. "media/logo.png"
	Content Content
}

// SourceDoc is one original input document a `.kpz` was written on. The whole
// document (a DOCX/XLSX/PDF/…) rides as a Content reference, not inline bytes —
// the producer streams from the source file, the reader from the parcel's ZIP
// entry.
type SourceDoc struct {
	Path    string // archive path under source/, e.g. "source/report.docx"
	Content Content
}

// SkeletonDoc is one source document's round-trip skeleton member — the
// derived extract template `merge` reuses to rebuild the localized file
// without re-reading the original (AD-025 §6). Content (part of the RootHash).
type SkeletonDoc struct {
	// Path is the archive path under skeletons/, e.g. "skeletons/app.json".
	Path string
	// SourcePath is the logical source path this skeleton derives from.
	SourcePath string
	// FormatID is the format the source was read with (so merge picks the
	// right writer).
	FormatID string
	// ContentHash is the source's content hash at capture time.
	ContentHash string
	// Content is the serialized skeleton stream (format.SkeletonStore bytes),
	// carried as a reference. The skeleton is derived and usually small, so a
	// caller that already holds it may use BytesContent; one materializing it on
	// disk uses FileContent.
	Content Content
}

// OverlayDoc is one append-layer entry, mirroring blockstore.Overlay minus
// the volatile UpdatedAt timestamp: a workspace's content identity is the
// work itself, not when it was recorded (AD-025 §5). Payload is the
// tool-owned JSON body, carried verbatim.
type OverlayDoc struct {
	Kind      string          `json:"kind"`
	BlockHash string          `json:"blockHash"`
	Payload   json.RawMessage `json:"payload"`
	// Source, when set, names the source document this overlay belongs to
	// (its source/<name> member path). It scopes overlays per document so a
	// package carrying several sources keeps each document's block-addressed
	// work isolated — block IDs are only unique within one document, so a
	// shared keyspace would collide. Empty for a project snapshot, whose
	// overlays share one block store.
	Source string `json:"source,omitempty"`
}

// Manifest is the package inventory written as manifest.json. RootHash is a
// Merkle digest over the (sorted) member content hashes, giving the package a
// stable content identity independent of zip framing.
type Manifest struct {
	SchemaVersion string         `json:"schemaVersion"`
	Kind          string         `json:"kind"`
	Created       string         `json:"created,omitempty"`
	Generator     *GeneratorInfo `json:"generator,omitempty"`
	Members       []Member       `json:"members"`
	RootHash      string         `json:"rootHash"`
	// Recipe is the full project recipe (a JSON string holding its YAML
	// encoding). Metadata, kept out of the content RootHash (AD-025 §6).
	Recipe json.RawMessage `json:"recipe,omitempty"`
	// Sources records per-source identity (path, format, hash, skeleton
	// pointer, raw-source flag). Metadata, not in the RootHash.
	Sources []SourceIdentity `json:"sources,omitempty"`
	// Task scopes a KindInterchange package to one locale pair. Metadata,
	// not in the RootHash.
	Task *InterchangeTask `json:"task,omitempty"`
}

// Member is one entry in the manifest inventory.
type Member struct {
	Path        string `json:"path"`
	ContentType string `json:"contentType"`
	SHA256      string `json:"sha256"`
}

// memberContent is one archive member: its manifest identity plus a source for
// its bytes. Exactly one of data / content is set — data for members serialized
// in memory from a bounded native KBF structure, content for a referenced
// whole-document/media member streamed on demand.
type memberContent struct {
	Member
	data    []byte
	content Content
}

// Marshal serializes the package to deterministic .kpz (zip) bytes.
func (p *Package) Marshal() ([]byte, error) {
	members, err := p.serializeMembers()
	if err != nil {
		return nil, err
	}
	sort.Slice(members, func(i, j int) bool { return members[i].Path < members[j].Path })

	recipe, err := marshalRecipe(p.Recipe)
	if err != nil {
		return nil, err
	}
	kind := p.Kind
	if kind == "" {
		kind = KindProject
	}
	manifest := Manifest{
		SchemaVersion: SchemaVersion,
		Kind:          kind,
		Created:       p.Created,
		Generator:     p.Generator,
		RootHash:      rootHash(members),
		Recipe:        recipe,
		Sources:       p.Sources,
		Task:          p.InterchangeTask,
	}
	for _, m := range members {
		manifest.Members = append(manifest.Members, m.Member)
	}
	manifestData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("kpz: encode manifest: %w", err)
	}
	manifestData = append(manifestData, '\n')

	// Write all entries sorted by path (manifest included) for deterministic
	// archive bytes; store (no compression) so the bytes don't depend on a
	// compressor version.
	all := append([]memberContent{{Path: ManifestPath, data: manifestData}}, members...)
	sort.Slice(all, func(i, j int) bool { return all[i].Path < all[j].Path })

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, m := range all {
		w, err := zw.CreateHeader(&zip.FileHeader{Name: m.Path, Method: zip.Store, Modified: zipEpoch})
		if err != nil {
			return nil, fmt.Errorf("kpz: create %q: %w", m.Path, err)
		}
		if m.content != nil {
			// Referenced member: stream from the source (file or ZIP entry)
			// straight into the archive — the whole asset never sits in memory.
			rc, oerr := m.content.Open()
			if oerr != nil {
				return nil, fmt.Errorf("kpz: open %q: %w", m.Path, oerr)
			}
			_, cerr := io.Copy(w, rc)
			_ = rc.Close()
			if cerr != nil {
				return nil, fmt.Errorf("kpz: write %q: %w", m.Path, cerr)
			}
		} else if _, err := w.Write(m.data); err != nil {
			return nil, fmt.Errorf("kpz: write %q: %w", m.Path, err)
		}
	}
	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("kpz: finalize archive: %w", err)
	}
	return buf.Bytes(), nil
}

// RootHash returns the package's Merkle content identity — the same digest
// stored in the manifest, computed over the content members (history
// excluded). Used to detect whether a working cache has diverged from its
// packed .kpz (AD-025 §5).
func (p *Package) RootHash() (string, error) {
	members, err := p.serializeMembers()
	if err != nil {
		return "", err
	}
	return rootHash(members), nil
}

// serializeMembers turns each section into its native Kapi-family bytes.
// Structured members (blocks, annotations, content memory, terms, overlays, history) are
// serialized in memory; whole-document/media members (media, source, skeleton)
// are streamed through SHA-256 from their Content reference without retaining
// the bytes.
func (p *Package) serializeMembers() ([]memberContent, error) {
	var members []memberContent
	addData := func(path, ct string, data []byte) {
		sum := sha256.Sum256(data)
		members = append(members, memberContent{Path: path, ContentType: ct, SHA256: hex.EncodeToString(sum[:]), data: data})
	}
	addContent := func(path, ct string, c Content) error {
		sum, err := hashContent(c)
		if err != nil {
			return fmt.Errorf("kpz: hash %q: %w", path, err)
		}
		members = append(members, memberContent{Path: path, ContentType: ct, SHA256: sum, content: c})
		return nil
	}

	for _, b := range p.Blocks {
		if b.Path == "" || b.File == nil {
			return nil, errors.New("kpz: block doc needs Path and File")
		}
		data, err := kbf.Marshal(b.File)
		if err != nil {
			return nil, fmt.Errorf("kpz: marshal %q: %w", b.Path, err)
		}
		addData(b.Path, ContentTypeBlocks, data)
	}
	for _, a := range p.Annotations {
		if a.Path == "" || a.File == nil {
			return nil, errors.New("kpz: annotation doc needs Path and File")
		}
		var ab bytes.Buffer
		if err := kbf.EncodeAnnotationFile(&ab, a.File); err != nil {
			return nil, fmt.Errorf("kpz: encode %q: %w", a.Path, err)
		}
		addData(a.Path, ContentTypeAnnotations, ab.Bytes())
	}
	if p.Memory != nil {
		data, err := kmb.Marshal(p.Memory)
		if err != nil {
			return nil, fmt.Errorf("kpz: marshal tm: %w", err)
		}
		addData(memoryPath, ContentTypeMemory, data)
	}
	if p.Terms != nil {
		data, err := ktb.Marshal(p.Terms)
		if err != nil {
			return nil, fmt.Errorf("kpz: marshal terms: %w", err)
		}
		addData(termsPath, ContentTypeTerms, data)
	}
	for _, m := range p.Media {
		if m.Path == "" {
			return nil, errors.New("kpz: media needs Path")
		}
		if m.Content == nil {
			return nil, fmt.Errorf("kpz: media %q needs Content", m.Path)
		}
		if err := addContent(m.Path, ContentTypeMedia, m.Content); err != nil {
			return nil, err
		}
	}
	for _, s := range p.Source {
		if s.Path == "" {
			return nil, errors.New("kpz: source needs Path")
		}
		if s.Content == nil {
			return nil, fmt.Errorf("kpz: source %q needs Content", s.Path)
		}
		if err := addContent(s.Path, ContentTypeSource, s.Content); err != nil {
			return nil, err
		}
	}
	for _, s := range p.Skeletons {
		if s.Path == "" {
			return nil, errors.New("kpz: skeleton needs Path")
		}
		if s.Content == nil {
			return nil, fmt.Errorf("kpz: skeleton %q needs Content", s.Path)
		}
		if err := addContent(s.Path, ContentTypeSkeleton, s.Content); err != nil {
			return nil, err
		}
	}
	if len(p.Overlays) > 0 {
		data, err := marshalOverlaySet(p.Overlays)
		if err != nil {
			return nil, fmt.Errorf("kpz: marshal overlays: %w", err)
		}
		addData(OverlaysPath, ContentTypeOverlays, data)
	}
	if len(p.History) > 0 {
		addData(HistoryPath, ContentTypeHistory, p.History)
	}
	return members, nil
}

// rootHash is a Merkle digest over the sorted CONTENT member hashes. The
// advisory history log is excluded: it is subordinate to content (AD-025
// §5), so it must not perturb the package's content identity, and its
// timestamps would otherwise break determinism.
func rootHash(members []memberContent) string {
	sorted := make([]Member, 0, len(members))
	for _, m := range members {
		if m.ContentType == ContentTypeHistory {
			continue
		}
		sorted = append(sorted, m.Member)
	}
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Path < sorted[j].Path })
	h := sha256.New()
	for _, m := range sorted {
		fmt.Fprintf(h, "%s\x00%s\x00%s\n", m.ContentType, m.Path, m.SHA256)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// Unmarshal parses .kpz bytes back into a Package, validating the manifest
// envelope, every member's sha256, and the Merkle root hash.
func Unmarshal(data []byte) (*Package, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("kpz: open archive: %w", err)
	}
	// Bound the archive on its declared headers before a single byte is
	// decompressed, then stream every entry through ONE guard so the cumulative
	// caps apply across the package rather than per member.
	if err := PackageZipLimits.CheckReader(zr); err != nil {
		return nil, fmt.Errorf("kpz: %w", err)
	}
	guard := PackageZipLimits.NewGuard()

	files := make(map[string]*zip.File, len(zr.File))
	for _, f := range zr.File {
		files[f.Name] = f
	}

	mf, ok := files[ManifestPath]
	if !ok {
		return nil, errors.New("kpz: missing manifest.json")
	}
	manifestData, err := readZipEntry(guard, mf)
	if err != nil {
		return nil, err
	}
	var manifest Manifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return nil, fmt.Errorf("kpz: decode manifest: %w", err)
	}
	// Accept the project profile (KindProject) and the interchange profile
	// (KindInterchange); reject any other kind.
	kind := manifest.Kind
	switch kind {
	case KindProject, KindInterchange:
		// keep
	default:
		return nil, fmt.Errorf("kpz: unknown kind %q (want %q or %q)", manifest.Kind, KindProject, KindInterchange)
	}
	major, vok := majorVersion(manifest.SchemaVersion)
	if !vok {
		return nil, fmt.Errorf("kpz: invalid schemaVersion %q", manifest.SchemaVersion)
	}
	if wantMajor, _ := majorVersion(SchemaVersion); major != wantMajor {
		return nil, fmt.Errorf("kpz: unsupported major schemaVersion %d (this build speaks %s)", major, SchemaVersion)
	}

	// Every name in the manifest is content. Validate the whole path surface
	// once, here, so no later stage has to remember to — a package may only
	// name paths inside the project it is merged into.
	if err := validateManifestPaths(&manifest); err != nil {
		return nil, err
	}

	recipe, err := unmarshalRecipe(manifest.Recipe)
	if err != nil {
		return nil, err
	}
	pkg := &Package{
		Kind:            kind,
		Created:         manifest.Created,
		Generator:       manifest.Generator,
		Recipe:          recipe,
		Sources:         manifest.Sources,
		InterchangeTask: manifest.Task,
	}
	// Index source identities by skeleton member path so skeleton members can
	// recover their (sourcePath, formatId, contentHash) metadata on load.
	skelMeta := make(map[string]SourceIdentity, len(manifest.Sources))
	for _, si := range manifest.Sources {
		if si.SkeletonPath != "" {
			skelMeta[si.SkeletonPath] = si
		}
	}
	verify := make([]memberContent, 0, len(manifest.Members))

	for _, m := range manifest.Members {
		zf, ok := files[m.Path]
		if !ok {
			return nil, fmt.Errorf("kpz: manifest references missing member %q", m.Path)
		}
		// Whole-asset members (media, raw source, skeleton) are verified by
		// streaming through SHA-256 and never buffered: the package keeps only a
		// lazy reader over the ZIP entry, so a hostile archive cannot make
		// `merge` or `info` materialize its largest member in memory. Structured
		// members are parsed from bytes, so they are read — through the guard,
		// which bounds them.
		var body []byte
		var sum string
		if opaqueContentType(m.ContentType) {
			sum, err = hashZipEntry(guard, zf)
		} else {
			body, err = readZipEntry(guard, zf)
			if err == nil {
				s := sha256.Sum256(body)
				sum = hex.EncodeToString(s[:])
			}
		}
		if err != nil {
			return nil, err
		}
		if sum != m.SHA256 {
			return nil, fmt.Errorf("kpz: member %q checksum mismatch (corrupt package)", m.Path)
		}
		verify = append(verify, memberContent{Member: m})

		switch m.ContentType {
		case ContentTypeBlocks:
			f, err := kbf.Unmarshal(body)
			if err != nil {
				return nil, fmt.Errorf("kpz: parse %q: %w", m.Path, err)
			}
			pkg.Blocks = append(pkg.Blocks, BlockDoc{Path: m.Path, File: f})
		case ContentTypeAnnotations:
			f, err := kbf.DecodeAnnotationFile(bytes.NewReader(body))
			if err != nil {
				return nil, fmt.Errorf("kpz: parse %q: %w", m.Path, err)
			}
			pkg.Annotations = append(pkg.Annotations, AnnotationDoc{Path: m.Path, File: f})
		case ContentTypeMemory:
			f, err := kmb.Unmarshal(body)
			if err != nil {
				return nil, fmt.Errorf("kpz: parse tm: %w", err)
			}
			pkg.Memory = f
		case ContentTypeTerms:
			f, err := ktb.Unmarshal(body)
			if err != nil {
				return nil, fmt.Errorf("kpz: parse terms: %w", err)
			}
			pkg.Terms = f
		case ContentTypeMedia:
			// Whole-asset members are verified above, then dropped from memory:
			// the public struct keeps only a lazy reader over the ZIP entry.
			pkg.Media = append(pkg.Media, Media{Path: m.Path, Content: zipContent{zf, PackageZipLimits}})
		case ContentTypeSource:
			pkg.Source = append(pkg.Source, SourceDoc{Path: m.Path, Content: zipContent{zf, PackageZipLimits}})
		case ContentTypeSkeleton:
			si := skelMeta[m.Path]
			pkg.Skeletons = append(pkg.Skeletons, SkeletonDoc{
				Path:        m.Path,
				SourcePath:  si.SourcePath,
				FormatID:    si.FormatID,
				ContentHash: si.ContentHash,
				Content:     zipContent{zf, PackageZipLimits},
			})
		case ContentTypeHistory:
			pkg.History = body
		case ContentTypeOverlays:
			overlays, err := unmarshalOverlaySet(body)
			if err != nil {
				return nil, fmt.Errorf("kpz: parse %q: %w", m.Path, err)
			}
			// An overlay's Source names the source/<name> member it scopes to,
			// so it is part of the same path surface as the manifest's.
			for _, ov := range overlays {
				if ov.Source != "" && !safeio.IsLocalPath(ov.Source) {
					return nil, fmt.Errorf("kpz: overlay source %q is not a path inside the project", ov.Source)
				}
			}
			pkg.Overlays = overlays
		default:
			return nil, fmt.Errorf("kpz: unknown content type %q for %q", m.ContentType, m.Path)
		}
	}

	if got := rootHash(verify); got != manifest.RootHash {
		return nil, fmt.Errorf("kpz: root hash mismatch (want %s, got %s)", manifest.RootHash, got)
	}
	return pkg, nil
}

// opaqueContentType reports whether a member is a whole document or asset whose
// bytes Unmarshal never needs — it is verified by streaming and then referenced
// lazily. Structured members (blocks, annotations, memory, terms, overlays,
// history) are parsed, so they must be read.
func opaqueContentType(ct string) bool {
	switch ct {
	case ContentTypeMedia, ContentTypeSource, ContentTypeSkeleton:
		return true
	default:
		return false
	}
}

// readZipEntry reads one member fully through the archive's cumulative guard.
func readZipEntry(g *safeio.ZipGuard, f *zip.File) ([]byte, error) {
	data, err := g.ReadEntry(f)
	if err != nil {
		return nil, fmt.Errorf("kpz: read %q: %w", f.Name, err)
	}
	return data, nil
}

// hashZipEntry streams one member through SHA-256 via the archive's cumulative
// guard, returning the hex digest without ever holding the member in memory.
func hashZipEntry(g *safeio.ZipGuard, f *zip.File) (string, error) {
	rc, err := g.Open(f)
	if err != nil {
		return "", fmt.Errorf("kpz: open %q: %w", f.Name, err)
	}
	defer rc.Close()
	h := sha256.New()
	if _, err := io.Copy(h, rc); err != nil {
		return "", fmt.Errorf("kpz: read %q: %w", f.Name, err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// validateManifestPaths refuses a manifest naming anything outside the project
// it would be merged into. Every field here ends up joined onto a project root,
// a state directory, or the archive's own member index, so a name that is
// absolute or climbs out via ".." is not a package this build will open —
// refusing beats reading and writing somewhere the recipient never named.
//
// Refusal, not repair: a package asking for this is not one to half-apply, the
// same rule restoreSources follows on the unpack side.
func validateManifestPaths(m *Manifest) error {
	check := func(what, name string) error {
		if !safeio.IsLocalPath(name) {
			return fmt.Errorf("kpz: %s %q is not a path inside the project", what, name)
		}
		return nil
	}
	for _, mem := range m.Members {
		if err := check("member path", mem.Path); err != nil {
			return err
		}
	}
	for _, si := range m.Sources {
		if err := check("source path", si.SourcePath); err != nil {
			return err
		}
		if si.SkeletonPath != "" {
			if err := check("skeleton path", si.SkeletonPath); err != nil {
				return err
			}
		}
	}
	if m.Task != nil {
		for _, f := range m.Task.SourceFiles {
			if err := check("interchange source file", f); err != nil {
				return err
			}
		}
	}
	return nil
}

func majorVersion(v string) (int, bool) {
	major := 0
	seen := false
	for _, r := range v {
		if r == '.' {
			return major, seen
		}
		if r < '0' || r > '9' {
			return 0, false
		}
		major = major*10 + int(r-'0')
		seen = true
	}
	return 0, false
}
