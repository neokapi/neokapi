package klz

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"time"

	"github.com/neokapi/neokapi/core/klf"
	"github.com/neokapi/neokapi/sievepen/klftm"
	"github.com/neokapi/neokapi/termbase/klftb"
)

const (
	// SchemaVersion is the package manifest version (MAJOR.MINOR, same
	// forward-compatibility contract as core/klf).
	SchemaVersion = "1.0"
	// Kind is the magic string on the package manifest.
	Kind = "kapi-localization-package"

	// ManifestPath is the manifest member's path within the archive.
	ManifestPath = "manifest.json"

	ContentTypeBlocks      = "blocks"
	ContentTypeAnnotations = "annotations"
	ContentTypeTM          = "tm"
	ContentTypeTermbase    = "termbase"
	ContentTypeMedia       = "media"

	tmPath       = "tm.klftm"
	termbasePath = "termbase.klftb"
)

// zipEpoch is a fixed modification time so the archive bytes are deterministic
// regardless of when the package was built (the DOS epoch zip uses).
var zipEpoch = time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC)

// Package is the in-memory form of a .klz: the authoritative content of a
// project, grouped by content type. Any section may be empty.
type Package struct {
	Created     string
	Generator   *GeneratorInfo
	Blocks      []BlockDoc
	Annotations []AnnotationDoc
	TM          *klftm.File
	Termbase    *klftb.File
	Media       []Media
}

// GeneratorInfo identifies the tool that produced the package.
type GeneratorInfo struct {
	ID      string `json:"id"`
	Version string `json:"version,omitempty"`
}

// BlockDoc is one KLF document member (blocks + targets).
type BlockDoc struct {
	Path string // archive path under blocks/, e.g. "blocks/app.klf"
	File *klf.File
}

// AnnotationDoc is one .klfl annotation-overlay member.
type AnnotationDoc struct {
	Path string // archive path under annotations/, e.g. "annotations/app.klfl"
	File *klf.AnnotationFile
}

// Media is one opaque blob member (e.g. an image referenced by content).
type Media struct {
	Path string // archive path under media/, e.g. "media/logo.png"
	Data []byte
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
}

// Member is one entry in the manifest inventory.
type Member struct {
	Path        string `json:"path"`
	ContentType string `json:"contentType"`
	SHA256      string `json:"sha256"`
}

type memberBytes struct {
	Member
	data []byte
}

// Marshal serializes the package to deterministic .klz (zip) bytes.
func (p *Package) Marshal() ([]byte, error) {
	members, err := p.serializeMembers()
	if err != nil {
		return nil, err
	}
	sort.Slice(members, func(i, j int) bool { return members[i].Path < members[j].Path })

	manifest := Manifest{
		SchemaVersion: SchemaVersion,
		Kind:          Kind,
		Created:       p.Created,
		Generator:     p.Generator,
		RootHash:      rootHash(members),
	}
	for _, m := range members {
		manifest.Members = append(manifest.Members, m.Member)
	}
	manifestData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("klz: encode manifest: %w", err)
	}
	manifestData = append(manifestData, '\n')

	// Write all entries sorted by path (manifest included) for deterministic
	// archive bytes; store (no compression) so the bytes don't depend on a
	// compressor version.
	all := append([]memberBytes{{Member: Member{Path: ManifestPath}, data: manifestData}}, members...)
	sort.Slice(all, func(i, j int) bool { return all[i].Path < all[j].Path })

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, m := range all {
		w, err := zw.CreateHeader(&zip.FileHeader{Name: m.Path, Method: zip.Store, Modified: zipEpoch})
		if err != nil {
			return nil, fmt.Errorf("klz: create %q: %w", m.Path, err)
		}
		if _, err := w.Write(m.data); err != nil {
			return nil, fmt.Errorf("klz: write %q: %w", m.Path, err)
		}
	}
	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("klz: finalize archive: %w", err)
	}
	return buf.Bytes(), nil
}

// serializeMembers turns each section into its native KLF-family bytes.
func (p *Package) serializeMembers() ([]memberBytes, error) {
	var members []memberBytes
	add := func(path, ct string, data []byte) {
		sum := sha256.Sum256(data)
		members = append(members, memberBytes{Member: Member{Path: path, ContentType: ct, SHA256: hex.EncodeToString(sum[:])}, data: data})
	}

	for _, b := range p.Blocks {
		if b.Path == "" || b.File == nil {
			return nil, errors.New("klz: block doc needs Path and File")
		}
		data, err := klf.Marshal(b.File)
		if err != nil {
			return nil, fmt.Errorf("klz: marshal %q: %w", b.Path, err)
		}
		add(b.Path, ContentTypeBlocks, data)
	}
	for _, a := range p.Annotations {
		if a.Path == "" || a.File == nil {
			return nil, errors.New("klz: annotation doc needs Path and File")
		}
		var ab bytes.Buffer
		if err := klf.EncodeAnnotationFile(&ab, a.File); err != nil {
			return nil, fmt.Errorf("klz: encode %q: %w", a.Path, err)
		}
		add(a.Path, ContentTypeAnnotations, ab.Bytes())
	}
	if p.TM != nil {
		data, err := klftm.Marshal(p.TM)
		if err != nil {
			return nil, fmt.Errorf("klz: marshal tm: %w", err)
		}
		add(tmPath, ContentTypeTM, data)
	}
	if p.Termbase != nil {
		data, err := klftb.Marshal(p.Termbase)
		if err != nil {
			return nil, fmt.Errorf("klz: marshal termbase: %w", err)
		}
		add(termbasePath, ContentTypeTermbase, data)
	}
	for _, m := range p.Media {
		if m.Path == "" {
			return nil, errors.New("klz: media needs Path")
		}
		add(m.Path, ContentTypeMedia, m.Data)
	}
	return members, nil
}

// rootHash is a Merkle digest over the sorted member content hashes.
func rootHash(members []memberBytes) string {
	sorted := make([]Member, len(members))
	for i, m := range members {
		sorted[i] = m.Member
	}
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Path < sorted[j].Path })
	h := sha256.New()
	for _, m := range sorted {
		fmt.Fprintf(h, "%s\x00%s\x00%s\n", m.ContentType, m.Path, m.SHA256)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// Unmarshal parses .klz bytes back into a Package, validating the manifest
// envelope, every member's sha256, and the Merkle root hash.
func Unmarshal(data []byte) (*Package, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("klz: open archive: %w", err)
	}
	files := make(map[string]*zip.File, len(zr.File))
	for _, f := range zr.File {
		files[f.Name] = f
	}

	mf, ok := files[ManifestPath]
	if !ok {
		return nil, errors.New("klz: missing manifest.json")
	}
	manifestData, err := readZipFile(mf)
	if err != nil {
		return nil, err
	}
	var manifest Manifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return nil, fmt.Errorf("klz: decode manifest: %w", err)
	}
	if manifest.Kind != Kind {
		return nil, fmt.Errorf("klz: unexpected kind %q (want %q)", manifest.Kind, Kind)
	}
	major, vok := majorVersion(manifest.SchemaVersion)
	if !vok {
		return nil, fmt.Errorf("klz: invalid schemaVersion %q", manifest.SchemaVersion)
	}
	if wantMajor, _ := majorVersion(SchemaVersion); major != wantMajor {
		return nil, fmt.Errorf("klz: unsupported major schemaVersion %d (this build speaks %s)", major, SchemaVersion)
	}

	pkg := &Package{Created: manifest.Created, Generator: manifest.Generator}
	verify := make([]memberBytes, 0, len(manifest.Members))

	for _, m := range manifest.Members {
		zf, ok := files[m.Path]
		if !ok {
			return nil, fmt.Errorf("klz: manifest references missing member %q", m.Path)
		}
		body, err := readZipFile(zf)
		if err != nil {
			return nil, err
		}
		s := sha256.Sum256(body)
		if hex.EncodeToString(s[:]) != m.SHA256 {
			return nil, fmt.Errorf("klz: member %q checksum mismatch (corrupt package)", m.Path)
		}
		verify = append(verify, memberBytes{Member: m})

		switch m.ContentType {
		case ContentTypeBlocks:
			f, err := klf.Unmarshal(body)
			if err != nil {
				return nil, fmt.Errorf("klz: parse %q: %w", m.Path, err)
			}
			pkg.Blocks = append(pkg.Blocks, BlockDoc{Path: m.Path, File: f})
		case ContentTypeAnnotations:
			f, err := klf.DecodeAnnotationFile(bytes.NewReader(body))
			if err != nil {
				return nil, fmt.Errorf("klz: parse %q: %w", m.Path, err)
			}
			pkg.Annotations = append(pkg.Annotations, AnnotationDoc{Path: m.Path, File: f})
		case ContentTypeTM:
			f, err := klftm.Unmarshal(body)
			if err != nil {
				return nil, fmt.Errorf("klz: parse tm: %w", err)
			}
			pkg.TM = f
		case ContentTypeTermbase:
			f, err := klftb.Unmarshal(body)
			if err != nil {
				return nil, fmt.Errorf("klz: parse termbase: %w", err)
			}
			pkg.Termbase = f
		case ContentTypeMedia:
			pkg.Media = append(pkg.Media, Media{Path: m.Path, Data: body})
		default:
			return nil, fmt.Errorf("klz: unknown content type %q for %q", m.ContentType, m.Path)
		}
	}

	if got := rootHash(verify); got != manifest.RootHash {
		return nil, fmt.Errorf("klz: root hash mismatch (want %s, got %s)", manifest.RootHash, got)
	}
	return pkg, nil
}

func readZipFile(f *zip.File) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, fmt.Errorf("klz: open %q: %w", f.Name, err)
	}
	defer rc.Close()
	data, err := io.ReadAll(rc)
	if err != nil {
		return nil, fmt.Errorf("klz: read %q: %w", f.Name, err)
	}
	return data, nil
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
