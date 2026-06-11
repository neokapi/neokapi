package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/neokapi/neokapi/core/model"
)

// Kapi uses `#.` (extracted) PO comments to carry its bookkeeping. The
// exact strings are part of the file contract between kapi extract and
// kapi merge, and are also documented in AD-017 (#422).
const (
	// poBatchIDCommentPrefix marks the file-level batch id. Emitted on
	// the PO header entry so kapi merge can find the extraction manifest
	// even after the file has travelled through a CAT tool.
	poBatchIDCommentPrefix = "kapi-batch: "

	// poSourceFileCommentPrefix records the source file path (relative
	// to the project root) — analogous to the XLIFF file-note.
	poSourceFileCommentPrefix = "kapi-source-file: "

	// poSourceHashCommentPrefix records the sha256 hex of the source
	// content at extract time, for per-file staleness detection.
	poSourceHashCommentPrefix = "kapi-source-hash: "

	// poBlockCommentPrefix carries the block id on each entry, so merge
	// can match a returning msgid back to its source block. Format:
	// "kapi-block: <block-id>".
	poBlockCommentPrefix = "kapi-block: "
)

// writePOExtract emits a minimal but spec-correct PO file carrying the
// pre-filled translations and kapi's bookkeeping comments. It deliberately
// does not reuse core/formats/po/Writer — that writer is designed for
// byte-exact round-trip via a skeleton store, which doesn't fit the
// "emit a fresh file" extract path. The output round-trips cleanly with
// both the core PO reader and any gettext tool.
//
// One entry per translatable block. Today that matches the "one segment
// per block" case (segmentation off); when segmentation is on, PO
// support is deferred to a follow-up — kapi extract errors out early in
// that combination rather than emit an ambiguous file.
func writePOExtract(out io.Writer, target model.LocaleID, batchID, sourceRel, sourceHash string, blocks []*model.Block) error {
	bw := bufio.NewWriter(out)

	// PO header: msgid "" / msgstr "" with Content-Type and file-level
	// kapi bookkeeping carried as extracted comments above it.
	fmt.Fprintf(bw, "#. %s%s\n", poBatchIDCommentPrefix, batchID)
	fmt.Fprintf(bw, "#. %s%s\n", poSourceFileCommentPrefix, sourceRel)
	fmt.Fprintf(bw, "#. %s%s\n", poSourceHashCommentPrefix, sourceHash)
	fmt.Fprintln(bw, `msgid ""`)
	fmt.Fprintln(bw, `msgstr ""`)
	fmt.Fprintln(bw, `"MIME-Version: 1.0\n"`)
	fmt.Fprintln(bw, `"Content-Type: text/plain; charset=UTF-8\n"`)
	fmt.Fprintln(bw, `"Content-Transfer-Encoding: 8bit\n"`)
	if target != "" {
		fmt.Fprintf(bw, `"Language: %s\n"`+"\n", target)
	}

	for _, b := range blocks {
		if !b.Translatable {
			continue
		}
		if len(b.Source) == 0 {
			continue
		}
		// One msgid per block (segmentation-off case). Skip multi-segment
		// blocks for v1 PO — extract fails earlier for segmentation-on.
		if b.SourceSegmentCount() > 1 {
			return errors.New("po extract: segmentation-on output is tracked as a follow-up; emit XLIFF for this project or unset defaults.segmentation.source")
		}
		fmt.Fprintln(bw)
		if b.ID != "" {
			fmt.Fprintf(bw, "#. %s%s\n", poBlockCommentPrefix, b.ID)
		}

		// Fuzzy flag when TM pre-fill populated a fuzzy match for this
		// block. We piggyback on block.Properties set by applyTMPrefill
		// (see extract.go) — missing means no pre-fill or exact.
		if b.Properties["kapi-tm-match"] == "fuzzy" {
			fmt.Fprintln(bw, "#, fuzzy")
		}

		if ctxt := b.Properties["context"]; ctxt != "" {
			fmt.Fprintf(bw, "msgctxt %s\n", poQuote(ctxt))
		}
		fmt.Fprintf(bw, "msgid %s\n", poQuote(b.SourceText()))

		tgt := ""
		if target != "" && b.HasTarget(target) {
			tgt = b.TargetText(target)
		}
		fmt.Fprintf(bw, "msgstr %s\n", poQuote(tgt))
	}

	return bw.Flush()
}

// poMergeBlock is a returning PO entry parsed by readPOForMerge. Each
// corresponds to one block in the translator's PO.
type poMergeBlock struct {
	BlockID string // kapi-block: <id> comment; empty if missing
	MsgID   string // source text as carried in the PO
	MsgStr  string // translator's target text (empty if skipped)
	Fuzzy   bool   // true if the #, fuzzy flag is present
}

// poMergeFile is everything readPOForMerge returns for a single input.
type poMergeFile struct {
	BatchID    string // from #. kapi-batch:
	SourceFile string // from #. kapi-source-file:
	SourceHash string // from #. kapi-source-hash:
	Blocks     []poMergeBlock
}

// readPOForMerge is the counterpart to writePOExtract — a minimal,
// forgiving PO reader targeted at merging a translator's return. It
// doesn't try to be a general-purpose PO parser (core/formats/po
// handles that); it only reads what merge needs: the kapi bookkeeping
// comments and msgid/msgstr pairs keyed by block id.
func readPOForMerge(path string) (*poMergeFile, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return parsePOForMerge(f)
}

// parsePOForMerge is the string-based engine behind readPOForMerge. Split
// out for test injection.
func parsePOForMerge(r io.Reader) (*poMergeFile, error) {
	scanner := bufio.NewScanner(r)
	// Allow long lines — 1 MiB is plenty for even pathological PO entries
	// without being unbounded.
	buf := make([]byte, 0, 1<<16)
	scanner.Buffer(buf, 1<<20)

	out := &poMergeFile{}
	var cur poMergeBlock
	var (
		inMsgID, inMsgStr bool
		seenHeader        bool
	)

	flush := func() {
		if cur.MsgID == "" && cur.MsgStr == "" && cur.BlockID == "" && !cur.Fuzzy {
			return
		}
		// Skip the header (msgid "") — it lives on out, not in .Blocks.
		if cur.MsgID == "" {
			seenHeader = true
			cur = poMergeBlock{}
			return
		}
		out.Blocks = append(out.Blocks, cur)
		cur = poMergeBlock{}
	}

	for scanner.Scan() {
		raw := scanner.Text()
		line := strings.TrimSpace(raw)

		// Blank line terminates the current entry.
		if line == "" {
			flush()
			inMsgID, inMsgStr = false, false
			continue
		}

		// Extracted comments `#. <text>` — carry kapi bookkeeping.
		if after, ok := strings.CutPrefix(line, "#."); ok {
			content := strings.TrimSpace(after)
			switch {
			case strings.HasPrefix(content, poBatchIDCommentPrefix):
				out.BatchID = strings.TrimPrefix(content, poBatchIDCommentPrefix)
			case strings.HasPrefix(content, poSourceFileCommentPrefix):
				out.SourceFile = strings.TrimPrefix(content, poSourceFileCommentPrefix)
			case strings.HasPrefix(content, poSourceHashCommentPrefix):
				out.SourceHash = strings.TrimPrefix(content, poSourceHashCommentPrefix)
			case strings.HasPrefix(content, poBlockCommentPrefix):
				cur.BlockID = strings.TrimPrefix(content, poBlockCommentPrefix)
			}
			continue
		}
		// Flag comments `#, fuzzy, ...`
		if after, ok := strings.CutPrefix(line, "#,"); ok {
			flags := strings.TrimSpace(after)
			for f := range strings.SplitSeq(flags, ",") {
				if strings.TrimSpace(f) == "fuzzy" {
					cur.Fuzzy = true
				}
			}
			continue
		}
		// Translator comments `#` / references `#:` / prev-msgid `#|`:
		// silently skip — irrelevant for merge.
		if strings.HasPrefix(line, "#") {
			continue
		}

		// Field lines.
		switch {
		case strings.HasPrefix(line, "msgid "):
			inMsgID, inMsgStr = true, false
			cur.MsgID = poUnquote(strings.TrimPrefix(line, "msgid "))
		case strings.HasPrefix(line, "msgstr "):
			inMsgID, inMsgStr = false, true
			cur.MsgStr = poUnquote(strings.TrimPrefix(line, "msgstr "))
		case strings.HasPrefix(line, `"`) && (inMsgID || inMsgStr):
			// Continuation line.
			val := poUnquote(line)
			if inMsgID {
				cur.MsgID += val
			} else if inMsgStr {
				cur.MsgStr += val
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	flush()

	// If we never saw a header entry, the file is technically malformed
	// but we still accept it — merging what's there beats failing hard.
	_ = seenHeader
	return out, nil
}

// poQuote wraps a string in PO-style double quotes with escaping.
func poQuote(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\t", "\\t")
	return "\"" + s + "\""
}

// poUnquoteRE matches a PO-quoted string, capturing the unescaped body.
var poUnquoteRE = regexp.MustCompile(`^"(.*)"$`)

// poUnquote strips surrounding quotes and unescapes a PO string. Leading
// whitespace is tolerated so callers don't need to pre-trim.
func poUnquote(s string) string {
	s = strings.TrimSpace(s)
	m := poUnquoteRE.FindStringSubmatch(s)
	if m == nil {
		return s
	}
	body := m[1]
	body = strings.ReplaceAll(body, "\\n", "\n")
	body = strings.ReplaceAll(body, "\\t", "\t")
	body = strings.ReplaceAll(body, `\"`, `"`)
	body = strings.ReplaceAll(body, `\\`, `\`)
	return body
}
