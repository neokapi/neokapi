// Bookmark and comment-range markers: the zero-width w:bookmarkStart /
// w:bookmarkEnd and w:commentRangeStart / w:commentRangeEnd elements captured
// as sentinel runs so their position inside the text survives the round trip.

package openxml

import (
	"encoding/xml"
)

// skippableBookmarkName is the well-known Word internal bookmark
// generated to track the user's last edit position. ECMA-376 doesn't
// reserve the name, but every modern Word build emits it on save (and
// expects it to round-trip as a no-op). Upstream Okapi's
// SkippableElements.BookmarkCrossStructure.SKIPPABLE_BOOKMARK_NAME
// (SkippableElements.java line 304) hard-codes it to `_GoBack` and
// drops both the start and the matching end (by id) silently — we
// mirror that policy exactly. The matching is by id, not by name,
// because the end element only carries an id attribute (ECMA-376
// Part 1 §17.13.6.2 — `<w:bookmarkEnd>` has only the `w:id` attribute).
const skippableBookmarkName = "_GoBack"

// bookmarkSkipState tracks the id of the most recent skipped
// bookmarkStart so the matching bookmarkEnd can also be dropped.
// Mirrors the `identifier` field on
// SkippableElements.CrossStructure (SkippableElements.java line 231)
// and the conditional id check on canBeSkipped (lines 277-281).
type bookmarkSkipState struct {
	skippedID string // id of the last skipped bookmarkStart, "" when no pending skip
}

// captureBookmark serializes a `<w:bookmarkStart>` or `<w:bookmarkEnd>`
// element verbatim (preserving every attribute and namespace prefix)
// and returns it as a sentinel textRun. The boolean second result is
// false when the bookmark should be silently dropped (matching upstream
// Okapi's `_GoBack` skip policy — see skippableBookmarkName for the
// citation). The decoder is advanced past the matching end token in
// every case so the caller can continue draining sibling tokens.
//
// ECMA-376 Part 1 §17.13.6.1 — `<w:bookmarkStart>` has `w:id`,
// `w:name`, plus optional revision-tracking attributes (`w:colFirst`,
// `w:colLast`, `w:displacedByCustomXml`). We preserve ALL of them.
//
// ECMA-376 Part 1 §17.13.6.2 — `<w:bookmarkEnd>` has only `w:id` plus
// the optional `w:displacedByCustomXml`.
func (p *wmlParser) captureBookmark(d *xml.Decoder, start xml.StartElement, bms *bookmarkSkipState) (textRun, bool, error) {
	id := attrVal(start, "id")
	if start.Name.Local == "bookmarkStart" {
		name := attrVal(start, "name")
		if name == skippableBookmarkName {
			bms.skippedID = id
			if err := skipElement(d); err != nil {
				return textRun{}, false, err
			}
			return textRun{}, false, nil
		}
	} else if start.Name.Local == "bookmarkEnd" {
		// A bookmarkEnd whose id matches the last skipped start is
		// the closing half of a skipped `_GoBack` and is dropped
		// silently; once consumed the tracking id is cleared so a
		// later bookmarkEnd with the same id (uncommon but legal
		// when ids are recycled) is preserved.
		if bms.skippedID != "" && bms.skippedID == id {
			bms.skippedID = ""
			if err := skipElement(d); err != nil {
				return textRun{}, false, err
			}
			return textRun{}, false, nil
		}
	}

	raw, err := captureRawElement(d, start)
	if err != nil {
		return textRun{}, false, err
	}

	var sentinel string
	if start.Name.Local == "bookmarkStart" {
		sentinel = ":" + id
	} else {
		sentinel = ":" + id
	}
	return textRun{text: sentinel, data: raw}, true, nil
}

// captureCommentRangeMarker serializes a <w:commentRangeStart/> or
// <w:commentRangeEnd/> element verbatim and returns it as a sentinel
// textRun. ECMA-376 Part 1 §17.13.4.3 (CT_MarkupRangeStart) /
// §17.13.4.4 (CT_MarkupRange) define both as direct children of <w:p>
// carrying a required w:id attribute that ties the range to the
// matching <w:commentReference w:id="N"/> in a sibling run.
//
// Mirrors the bookmark capture path (captureBookmark): the marker
// has no inner content (empty element), so a single self-closing tag
// captures its complete representation. The sentinel uses a distinct
// PUA char ( for start, for end) so the writer can tell
// comment-range markers apart from bookmarks and dispatch the
// appropriate SubType on the resulting Run.Ph.
func (p *wmlParser) captureCommentRangeMarker(d *xml.Decoder, start xml.StartElement) (textRun, error) {
	raw, err := captureRawElement(d, start)
	if err != nil {
		return textRun{}, err
	}
	id := attrVal(start, "id")
	var sentinel string
	if start.Name.Local == "commentRangeStart" {
		sentinel = ":" + id
	} else {
		sentinel = ":" + id
	}
	return textRun{text: sentinel, data: raw}, nil
}
