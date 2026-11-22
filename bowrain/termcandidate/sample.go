package termcandidate

import (
	"context"
	"strings"
	"unicode/utf8"
)

// Collection is one place a project's content sits: a collection the recipe
// declares, the coordinates its content occupies, and the items it holds.
// Name is empty for the items no declared collection holds.
type Collection struct {
	Name        string
	Coordinates map[string]string
	Items       []string
}

// FetchFunc reads up to maxRunes of an item's source text.
type FetchFunc func(ctx context.Context, item string, maxRunes int) (string, error)

// Segment is the text one item contributed to a sample.
type Segment struct {
	Collection string
	Item       string
	Text       string
}

// SampleOptions bound a sample.
type SampleOptions struct {
	// TotalRunes is the size of the whole sample.
	TotalRunes int
	// ItemRunes is the most one item contributes, so a long file cannot fill
	// a collection's share on its own.
	ItemRunes int
}

// Default sample bounds: six term-extract chunks of 12,000 runes, filled
// 1,500 runes at a time, which visits about fifty items.
const (
	DefaultTotalRunes = 72000
	DefaultItemRunes  = 1500
)

// Sample draws text from every collection in turn, one item per turn, until
// the sample holds opts.TotalRunes or every item has been read. Within a
// collection the items are visited in spread order (SpreadOrder), so a small
// sample of a large collection reaches its first, middle and last files
// rather than the ones that sort first.
func Sample(ctx context.Context, cols []Collection, fetch FetchFunc, opts SampleOptions) ([]Segment, error) {
	if opts.TotalRunes <= 0 {
		opts.TotalRunes = DefaultTotalRunes
	}
	if opts.ItemRunes <= 0 {
		opts.ItemRunes = DefaultItemRunes
	}
	queues := make([][]string, len(cols))
	for i, col := range cols {
		for _, idx := range SpreadOrder(len(col.Items)) {
			queues[i] = append(queues[i], col.Items[idx])
		}
	}
	var out []Segment
	total := 0
	for {
		progress := false
		for i := range cols {
			for len(queues[i]) > 0 {
				if err := ctx.Err(); err != nil {
					return out, err
				}
				item := queues[i][0]
				queues[i] = queues[i][1:]
				text, err := fetch(ctx, item, min(opts.ItemRunes, opts.TotalRunes-total))
				if err != nil {
					return out, err
				}
				text = strings.TrimSpace(text)
				if text == "" {
					continue
				}
				out = append(out, Segment{Collection: cols[i].Name, Item: item, Text: text})
				total += utf8.RuneCountInString(text)
				progress = true
				break
			}
			if total >= opts.TotalRunes {
				return out, nil
			}
		}
		if !progress {
			return out, nil
		}
	}
}

// SpreadOrder is the indices 0..n-1 in the order a bisection visits them:
// the first, then the middle, then the middle of each half, and so on. Any
// prefix of the order is spread evenly across the range.
func SpreadOrder(n int) []int {
	if n <= 0 {
		return nil
	}
	type span struct{ lo, hi int }
	out := make([]int, 0, n)
	out = append(out, 0)
	queue := []span{{0, n}}
	for len(queue) > 0 {
		s := queue[0]
		queue = queue[1:]
		if s.hi-s.lo <= 1 {
			continue
		}
		mid := (s.lo + s.hi) / 2
		out = append(out, mid)
		queue = append(queue, span{s.lo, mid}, span{mid, s.hi})
	}
	return out
}

// Chunks joins a sample's segments into prompt-sized chunks of at most size
// runes, keeping at most maxChunks. A chunk breaks between segments; a
// segment longer than size is cut to it.
func Chunks(segments []Segment, size, maxChunks int) []string {
	var out []string
	var sb strings.Builder
	runes := 0
	for _, seg := range segments {
		text := seg.Text
		n := utf8.RuneCountInString(text)
		if size > 0 && n > size {
			text, n = truncateRunes(text, size), size
		}
		sep := 0
		if sb.Len() > 0 {
			sep = 2
		}
		if size > 0 && sb.Len() > 0 && runes+sep+n > size {
			out = append(out, sb.String())
			sb.Reset()
			runes, sep = 0, 0
			if maxChunks > 0 && len(out) >= maxChunks {
				return out
			}
		}
		if sep > 0 {
			sb.WriteString("\n\n")
		}
		sb.WriteString(text)
		runes += sep + n
	}
	if sb.Len() > 0 && (maxChunks <= 0 || len(out) < maxChunks) {
		out = append(out, sb.String())
	}
	return out
}

// Text joins a sample's segments, one blank line between them.
func Text(segments []Segment) string {
	parts := make([]string, len(segments))
	for i, s := range segments {
		parts[i] = s.Text
	}
	return strings.Join(parts, "\n\n")
}

// truncateRunes is the longest prefix of s holding at most n runes.
func truncateRunes(s string, n int) string {
	count := 0
	for i := range s {
		if count == n {
			return s[:i]
		}
		count++
	}
	return s
}
