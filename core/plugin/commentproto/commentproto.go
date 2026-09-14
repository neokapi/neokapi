// Package commentproto carries what a comment provider located across the
// plugin boundary, as the response of BridgeService.LocateComments. It sits
// apart from protoconvert, whose content-model conversions do not need the
// bridge service's gRPC types.
package commentproto

import (
	"fmt"

	"github.com/neokapi/neokapi/core/comment"
	"github.com/neokapi/neokapi/core/format"
	bridgepb "github.com/neokapi/neokapi/core/plugin/proto/v2"
	"github.com/neokapi/neokapi/core/plugin/protoconvert"
)

// ToProto carries what a comment provider located as a LocateComments response.
func ToProto(f *comment.File) *bridgepb.LocateCommentsResponse {
	resp := &bridgepb.LocateCommentsResponse{}
	if f == nil {
		return resp
	}
	resp.Comments = make([]*bridgepb.CommentSpan, len(f.Comments))
	for i, c := range f.Comments {
		resp.Comments[i] = &bridgepb.CommentSpan{
			Start:      int64(c.Start),
			End:        int64(c.End),
			FirstLine:  int64(c.Lines.First),
			LastLine:   int64(c.Lines.Last),
			Style:      string(c.Style),
			Subject:    c.Subject,
			Doc:        c.Doc,
			Deprecated: c.Deprecated,
			Runs:       protoconvert.RunsToProto(c.Runs),
		}
	}
	resp.Excluded = make([]*bridgepb.CommentExclusion, len(f.Excluded))
	for i, e := range f.Excluded {
		resp.Excluded[i] = &bridgepb.CommentExclusion{
			Start:     int64(e.Start),
			End:       int64(e.End),
			FirstLine: int64(e.Lines.First),
			LastLine:  int64(e.Lines.Last),
			Reason:    string(e.Reason),
			Form:      e.Form,
		}
	}
	return resp
}

// FromProto reads a LocateComments response back into the file a comment
// provider returns, for a source of size bytes. A span outside those bytes, or a
// line range that runs backwards, is an error: every consumer of a comment.File
// slices the source by its spans.
func FromProto(language string, size int, resp *bridgepb.LocateCommentsResponse) (*comment.File, error) {
	f := &comment.File{Language: language}
	for _, c := range resp.GetComments() {
		start, end, lines, err := span(size, c.GetStart(), c.GetEnd(), c.GetFirstLine(), c.GetLastLine())
		if err != nil {
			return nil, fmt.Errorf("comment: %w", err)
		}
		f.Comments = append(f.Comments, comment.Comment{
			Start:      start,
			End:        end,
			Lines:      lines,
			Style:      comment.Style(c.GetStyle()),
			Subject:    c.GetSubject(),
			Doc:        c.GetDoc(),
			Deprecated: c.GetDeprecated(),
			Runs:       protoconvert.ProtoToRuns(c.GetRuns()),
		})
	}
	for _, e := range resp.GetExcluded() {
		start, end, lines, err := span(size, e.GetStart(), e.GetEnd(), e.GetFirstLine(), e.GetLastLine())
		if err != nil {
			return nil, fmt.Errorf("exclusion: %w", err)
		}
		f.Excluded = append(f.Excluded, comment.Excluded{
			Start:  start,
			End:    end,
			Lines:  lines,
			Reason: comment.Reason(e.GetReason()),
			Form:   e.GetForm(),
		})
	}
	return f, nil
}

func span(size int, start, end, first, last int64) (int, int, format.LineRange, error) {
	if start < 0 || end <= start || end > int64(size) {
		return 0, 0, format.LineRange{}, fmt.Errorf("span %d-%d is outside a %d-byte file", start, end, size)
	}
	if first < 1 || last < first || last > int64(size)+1 {
		return 0, 0, format.LineRange{}, fmt.Errorf("span %d-%d reports lines %d-%d", start, end, first, last)
	}
	return int(start), int(end), format.LineRange{First: int(first), Last: int(last)}, nil
}
