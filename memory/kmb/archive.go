package kmb

import (
	"github.com/neokapi/neokapi/internal/bundlezip"
)

const (
	// Ext is the single-file Kapi Memory Bundle extension.
	Ext = ".kmb"
	// ArchiveExt is the Kapi Memory Archive extension: a zip container around
	// one .kmb member. The bundle is the readable, git-diffable form; the
	// archive is the compressed at-rest/handoff form of the same bytes.
	ArchiveExt = ".kmz"
	// ArchiveMember is the member name a .kmz carries its bundle under.
	ArchiveMember = "tm" + Ext
)

// MarshalArchive encodes a File to .kmz bytes: the deterministic .kmb
// serialization wrapped in a single-member zip container.
func MarshalArchive(f *File) ([]byte, error) {
	data, err := Marshal(f)
	if err != nil {
		return nil, err
	}
	return bundlezip.Marshal(ArchiveMember, data)
}

// UnmarshalArchive decodes .kmz bytes: unwraps the container and decodes the
// .kmb member, applying the same envelope checks as Unmarshal.
func UnmarshalArchive(data []byte) (*File, error) {
	member, err := bundlezip.Unmarshal(data, ArchiveMember)
	if err != nil {
		return nil, err
	}
	return Unmarshal(member)
}
