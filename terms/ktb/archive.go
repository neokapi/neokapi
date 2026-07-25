package ktb

import (
	"github.com/neokapi/neokapi/internal/bundlezip"
)

const (
	// Ext is the single-file Kapi Term Bundle extension.
	Ext = ".ktb"
	// ArchiveExt is the Kapi Term Archive extension: a zip container around one
	// .ktb member. The bundle is the readable, git-diffable form; the archive is
	// the compressed at-rest/handoff form of the same bytes.
	ArchiveExt = ".ktz"
	// ArchiveMember is the member name a .ktz carries its bundle under. It keeps
	// the legacy stem so it stays paired with kmb's "tm" + Ext: the archive member
	// names are a format surface and move together, in one migration.
	ArchiveMember = "termbase" + Ext
)

// MarshalArchive encodes a File to .ktz bytes: the deterministic .ktb
// serialization wrapped in a single-member zip container.
func MarshalArchive(f *File) ([]byte, error) {
	data, err := Marshal(f)
	if err != nil {
		return nil, err
	}
	return bundlezip.Marshal(ArchiveMember, data)
}

// UnmarshalArchive decodes .ktz bytes: unwraps the container and decodes the
// .ktb member, applying the same envelope checks as Unmarshal.
func UnmarshalArchive(data []byte) (*File, error) {
	member, err := bundlezip.Unmarshal(data, ArchiveMember)
	if err != nil {
		return nil, err
	}
	return Unmarshal(member)
}
