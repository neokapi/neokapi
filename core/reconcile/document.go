package reconcile

import (
	"strconv"
	"strings"

	"github.com/neokapi/neokapi/core/model"
)

// Documents have identity too, and for the same reason blocks do.
//
// Block scope must name the document a block was read from, but naming it by
// PATH makes a rename rewrite every block's context at once: identity survives
// on content, but a file that was only renamed reports every block inside it as
// moved. The path is not the document's identity, only its current address.
//
// So the same matching runs one level up. A document is matched on where it
// lives and on what it contains, its resolved key becomes the scope its blocks
// reconcile within, and a rename leaves that key untouched.
//
// The pass order is deliberately the OPPOSITE of the one used for blocks. There,
// content is graded first because a name like "para1" is a weak identifier that
// collides freely. Here, the path is graded first because a path genuinely does
// identify a document: two files do not share one, and a file staying put while
// its contents change is far more ordinary than a file being renamed.

// Document is one file's worth of blocks, as just read.
type Document struct {
	Path   string
	Blocks []*model.Block
}

// DocUnit is a document the project already knows about.
type DocUnit struct {
	Key  string
	Path string
	// Content holds the content hash of each block the document contained, which
	// is what lets a renamed file be recognised by what is inside it.
	Content []string
}

// DocResult is a document's resolved identity. Key is what callers pass to
// Blocks as scope.
type DocResult struct {
	Path string
	Key  string
	Kind Kind
}

// RenameThreshold is how much of a document's content must survive for a file
// at a new path to be recognised as the same document rather than a new one.
//
// Set at half because renaming and editing in the same revision is ordinary —
// a file is moved and its opening paragraph rewritten to match its new home.
// Requiring identical content would call that a new document and orphan every
// block in it. Requiring too little would merge unrelated files that happen to
// share boilerplate.
const RenameThreshold = 0.5

// Identify returns the unit describing a document as just read.
func (d Document) Identify() DocUnit {
	content := make([]string, 0, len(d.Blocks))
	for _, b := range d.Blocks {
		content = append(content, model.ComputeContentHash(b.SourceText()))
	}
	return DocUnit{Path: d.Path, Content: content}
}

// Documents resolves each document to a stable key, in the order given.
//
// A key resolved here is stable across a rename, so passing it to Blocks as
// scope means moving a file costs nothing: its blocks reconcile in exactly the
// context they did before.
func Documents(current []Document, prior []DocUnit) []DocResult {
	units := make([]DocUnit, len(current))
	for i, d := range current {
		units[i] = d.Identify()
	}
	return DocumentUnits(units, prior)
}

// DocumentUnits is Documents for a caller that already holds the units.
//
// The distinction matters where the documents are not in hand. A venue holding
// a project's content knows each document as a path and a list of content
// hashes — which is a DocUnit exactly — and reducing that back into blocks so
// it could be reduced again would mean loading every block of every unchanged
// file to answer a question about names. A producer declaring what it read
// sends the same shape over the wire for the same reason.
//
// Identity is resolved here, on the side that holds the prior, because a
// rename grafts one document's approvals onto another: it is a claim about
// what content IS, and the party that owns the content has to be the one
// making it.
func DocumentUnits(current []DocUnit, prior []DocUnit) []DocResult {
	out := make([]DocResult, len(current))
	units := current
	for i, u := range current {
		out[i] = DocResult{Path: u.Path}
	}

	claimed := map[string]bool{}
	byPath := map[string][]DocUnit{}
	for _, u := range prior {
		byPath[u.Path] = append(byPath[u.Path], u)
	}

	// Same path, same contents.
	for i := range out {
		for _, p := range byPath[units[i].Path] {
			if claimed[p.Key] || !sameContent(p.Content, units[i].Content) {
				continue
			}
			out[i].Key, out[i].Kind, claimed[p.Key] = p.Key, Unchanged, true
			break
		}
	}

	// Same path, contents changed — the ordinary case of editing a file.
	for i := range out {
		if out[i].Key != "" {
			continue
		}
		for _, p := range byPath[units[i].Path] {
			if claimed[p.Key] {
				continue
			}
			out[i].Key, out[i].Kind, claimed[p.Key] = p.Key, Edited, true
			break
		}
	}

	// New path, recognisable contents — a rename, possibly with edits. The best
	// surviving candidate wins, so two similar files cannot both claim one prior.
	for i := range out {
		if out[i].Key != "" {
			continue
		}
		best, bestScore := "", RenameThreshold
		for _, p := range prior {
			if claimed[p.Key] {
				continue
			}
			if s := similarity(p.Content, units[i].Content); s >= bestScore {
				best, bestScore = p.Key, s
			}
		}
		if best != "" {
			out[i].Key, out[i].Kind, claimed[best] = best, Moved, true
		}
	}

	mintDocs(out, units)
	return out
}

// DocumentKeyFor is the key a document gets when nothing prior claims it: a
// function of its path alone, so a project that has recorded nothing yet and one
// that has recorded everything name the same file the same way.
//
// It is exported because a decision is filed against a document key, and a
// surface reading a project before its first extraction has to be able to name
// the document a committed decision was filed under. Deriving it there rather
// than falling back to the path is what stops a fresh clone from reading its own
// committed record as belonging to documents it has never heard of.
func DocumentKeyFor(path string) string {
	return "d-" + model.ComputeContentHash(path)[:16]
}

// IsDocumentKey reports whether a string is already a document key rather than
// the address of a document. It matches the exact shape DocumentKeyFor mints, so
// a file that happens to be called `d-notes.md` is an address.
func IsDocumentKey(s string) bool {
	if len(s) != len("d-")+16 || !strings.HasPrefix(s, "d-") {
		return false
	}
	for _, r := range s[2:] {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}

func mintDocs(out []DocResult, units []DocUnit) {
	seen := map[string]int{}
	for i := range out {
		if out[i].Key != "" {
			continue
		}
		key := DocumentKeyFor(units[i].Path)
		seen[key]++
		if n := seen[key]; n > 1 {
			key += "-" + strconv.Itoa(n)
		}
		out[i].Key, out[i].Kind = key, New
	}
}

func sameContent(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	counts := map[string]int{}
	for _, h := range a {
		counts[h]++
	}
	for _, h := range b {
		counts[h]--
		if counts[h] < 0 {
			return false
		}
	}
	return true
}

// similarity is the share of the larger document's blocks that both hold, so a
// small file cannot score highly against a large one merely by being contained
// in it.
func similarity(a, b []string) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	counts := map[string]int{}
	for _, h := range a {
		counts[h]++
	}
	shared := 0
	for _, h := range b {
		if counts[h] > 0 {
			counts[h]--
			shared++
		}
	}
	return float64(shared) / float64(max(len(a), len(b)))
}
