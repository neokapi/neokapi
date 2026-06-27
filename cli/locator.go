package cli

import (
	"os"
	"strings"

	"github.com/neokapi/neokapi/core/container"
	"github.com/neokapi/neokapi/core/model"
)

// EntryLocator addresses a single entry inside an archive container, written
// with the JAR-style bang separator: `bundle.zip!locales/en.json` (AD-026 §6).
type EntryLocator struct {
	Archive string // path to the container file
	Entry   string // slash-separated entry path inside it
}

// ParseEntryLocator splits a `container!entry` locator. It returns ok=false for
// a plain path so callers can fall through to normal handling. To avoid mistaking
// a real filename that contains '!' for a locator, it only matches when the part
// before a '!' has a container extension AND exists as a regular file. Splitting
// is single-level: the part before the first qualifying '!' is the archive and
// the remainder (which may itself contain '/') is the entry; nested-archive
// addressing is not supported.
func ParseEntryLocator(s string) (EntryLocator, bool) {
	for i := range len(s) {
		if s[i] != '!' {
			continue
		}
		left, right := s[:i], s[i+1:]
		if right == "" || !container.IsContainerPath(left) {
			continue
		}
		if fi, err := os.Stat(left); err != nil || fi.IsDir() {
			continue
		}
		return EntryLocator{Archive: left, Entry: strings.TrimPrefix(right, "/")}, true
	}
	return EntryLocator{}, false
}

// HasEntryLocator reports whether s is a `container!entry` locator.
func HasEntryLocator(s string) bool {
	_, ok := ParseEntryLocator(s)
	return ok
}

// anyContainerInput reports whether any input is an archive container or a
// `container!entry` locator — i.e. a single argument that expands to multiple
// inner files, so per-entry filenames should be shown.
func anyContainerInput(files []string) bool {
	for _, f := range files {
		if container.IsContainerPath(f) || HasEntryLocator(f) {
			return true
		}
	}
	return false
}

// entryLabel renders the display path for a block: when the block was read from
// inside a container (the archive reader stamped its entry), it returns the bang
// locator `<displayFile>!<entry>`; otherwise the file path unchanged. For a block
// already read via an explicit `archive!entry` locator, displayFile is that
// locator and the block carries no stamp, so it is returned as-is.
func entryLabel(displayFile string, b *model.Block) string {
	if b != nil {
		if e := b.Properties[model.PropContainerEntry]; e != "" {
			return displayFile + "!" + e
		}
	}
	return displayFile
}
