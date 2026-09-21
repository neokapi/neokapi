package workspace

import (
	"fmt"
	"path/filepath"
	"strings"
)

// A file-synchronizing client and a SQLite database in WAL mode disagree about
// what a file is. SQLite keeps a database, a write-ahead log and a shared-memory
// index consistent with each other through byte-range locks the operating system
// enforces; a sync client copies each of the three whenever it notices a change,
// takes no lock, and replaces a file under an open handle when a second machine
// writes. The result is a database whose log describes a state the main file is
// not in. It is reported as corruption long after the copy, in a process that did
// nothing wrong.
//
// So a workspace refuses to live inside a synchronized folder. The refusal is by
// path pattern, which is what can be recognised without asking the sync client
// anything, and it costs a false positive on a directory that merely carries one
// of these names.

// cloudSyncRoots are the directory names the desktop sync clients use, matched
// against one path segment at a time, case-insensitively.
//
// OneDrive and Google Drive name a segment after the account or organization it
// belongs to ("OneDrive - Contoso", "Google Drive Sync"), so those two match on a
// prefix. The rest are exact.
var cloudSyncRoots = []struct {
	name    string
	prefix  bool
	product string
}{
	{name: "Mobile Documents", product: "iCloud Drive"},
	{name: "com~apple~CloudDocs", product: "iCloud Drive"},
	{name: "iCloud Drive", product: "iCloud Drive"},
	{name: "iCloudDrive", product: "iCloud Drive"},
	{name: "Dropbox", product: "Dropbox"},
	{name: "OneDrive", prefix: true, product: "OneDrive"},
	{name: "Google Drive", prefix: true, product: "Google Drive"},
	{name: "GoogleDrive", prefix: true, product: "Google Drive"},
}

// CloudSyncedDir reports whether path sits inside a folder a desktop sync client
// keeps, naming the segment that matched and the product it belongs to.
//
// The path is examined as text. Nothing is read from disk, so the answer is the
// same for a directory that does not exist yet as for one that does.
func CloudSyncedDir(path string) (segment, product string, found bool) {
	if path == "" {
		return "", "", false
	}
	cleaned := filepath.Clean(path)
	for seg := range strings.SplitSeq(filepath.ToSlash(cleaned), "/") {
		if seg == "" || seg == "." || seg == ".." {
			continue
		}
		for _, root := range cloudSyncRoots {
			if matchesSegment(seg, root.name, root.prefix) {
				return seg, root.product, true
			}
		}
	}
	return "", "", false
}

// matchesSegment compares one path segment against a root name, exactly or by
// prefix, ignoring case.
func matchesSegment(seg, name string, prefix bool) bool {
	if strings.EqualFold(seg, name) {
		return true
	}
	if !prefix {
		return false
	}
	// A prefix match has to end at a word boundary, so "OneDriveProjects" is not
	// OneDrive while "OneDrive - Contoso" is.
	if len(seg) <= len(name) || !strings.EqualFold(seg[:len(name)], name) {
		return false
	}
	switch seg[len(name)] {
	case ' ', '-', '_':
		return true
	default:
		return false
	}
}

// errCloudSynced builds the refusal, naming the folder, the product and the way
// out.
func errCloudSynced(path, segment, product string) error {
	return fmt.Errorf(
		"workspace: %s sits inside %q, which %s keeps in sync. A synchronized copy of a "+
			"SQLite database and its write-ahead log is reported as corruption later, in a "+
			"process that did nothing wrong. Set KAPI_DATA_DIR to a directory outside the "+
			"synchronized folder",
		path, segment, product)
}
