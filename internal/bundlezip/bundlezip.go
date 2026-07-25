// Package bundlezip implements the single-member zip container shared by the
// compressed Kapi bundle archives: `.kmz` (Kapi Memory Archive, wrapping a
// `tm.kmb`) and `.ktz` (Kapi Term Archive, wrapping a `termbase.ktb`).
//
// It follows the same container discipline as the multi-member `.kpz` project
// archive (package kpz): one member per payload, a fixed DOS-epoch modification
// time, and no extra fields, so the archive bytes depend only on the member name
// and its payload — never on when or where the archive was built.
//
// It differs from `.kpz` in one respect. `.kpz` stores its members
// uncompressed so the archive bytes cannot depend on a compressor version; the
// memory and term archives exist *for* compression (a project TM serialized as
// `.kmb` JSON is far larger than its deflated form, and these archives are the
// at-rest/handoff form), so their single member is deflated. The content
// identity that matters — the member payload, which is the deterministic
// `.kmb` / `.ktb` serialization — stays byte-exact either way.
package bundlezip

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"time"
)

// epoch is a fixed modification time (the DOS epoch zip uses) so the archive
// bytes are independent of when it was built.
var epoch = time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC)

// Marshal wraps payload in a zip archive holding exactly one deflated member
// named name.
func Marshal(name string, payload []byte) ([]byte, error) {
	if name == "" {
		return nil, errors.New("bundlezip: member name required")
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate, Modified: epoch})
	if err != nil {
		return nil, fmt.Errorf("bundlezip: create %q: %w", name, err)
	}
	if _, err := w.Write(payload); err != nil {
		return nil, fmt.Errorf("bundlezip: write %q: %w", name, err)
	}
	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("bundlezip: finalize archive: %w", err)
	}
	return buf.Bytes(), nil
}

// Unmarshal returns the payload of the member named name. When no member
// carries that name but the archive holds exactly one file, that file is
// returned instead — a hand-zipped bundle keeps working regardless of what the
// member was called inside.
func Unmarshal(data []byte, name string) ([]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("bundlezip: open archive: %w", err)
	}
	var files []*zip.File
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		if f.Name == name {
			return readMember(f)
		}
		files = append(files, f)
	}
	switch len(files) {
	case 0:
		return nil, fmt.Errorf("bundlezip: archive has no %q member and no other file", name)
	case 1:
		return readMember(files[0])
	default:
		names := make([]string, 0, len(files))
		for _, f := range files {
			names = append(names, f.Name)
		}
		return nil, fmt.Errorf("bundlezip: archive has no %q member; found %d files (%v)", name, len(files), names)
	}
}

func readMember(f *zip.File) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, fmt.Errorf("bundlezip: open %q: %w", f.Name, err)
	}
	defer rc.Close()
	data, err := io.ReadAll(rc)
	if err != nil {
		return nil, fmt.Errorf("bundlezip: read %q: %w", f.Name, err)
	}
	return data, nil
}
