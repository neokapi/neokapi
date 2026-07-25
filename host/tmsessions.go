package host

import (
	"fmt"
	"io"
	"sort"
	"strconv"
	"time"

	"github.com/neokapi/neokapi/host/output"
	"github.com/neokapi/neokapi/sievepen"
)

// Import-session reporting.
//
// These three results exist so `kapi tm sessions list/show/delete` go through
// output.Print like every other command: they were the last surfaces writing
// straight to os.Stdout with no structured form, which meant a CI job could not
// read the provenance of its own content memory.

// TMSessionRow is one import session in a listing.
type TMSessionRow struct {
	ID         string `json:"id"`
	FileKey    string `json:"file_key"`
	Tool       string `json:"tool,omitempty"`
	EntryCount int    `json:"entry_count"`
	ImportedAt string `json:"imported_at,omitempty"`
}

// TMSessionListOutput is the structured result of `kapi tm sessions list`.
type TMSessionListOutput struct {
	Sessions []TMSessionRow `json:"sessions"`
}

// FormatText renders the session table.
func (o TMSessionListOutput) FormatText(w io.Writer) error {
	if len(o.Sessions) == 0 {
		fmt.Fprintln(w, "No import sessions.")
		return nil
	}
	t := output.NewTable(w).Accent(0).
		Headers("SESSION", "FILE", "TOOL", "ENTRIES", "IMPORTED")
	for _, s := range o.Sessions {
		t.Rowf(TruncateID(s.ID, 12), s.FileKey, s.Tool, s.EntryCount, s.ImportedAt)
	}
	t.Render()
	return nil
}

// TMSessionDetail is the full record of one import session.
type TMSessionDetail struct {
	ID               string            `json:"id"`
	FileKey          string            `json:"file_key"`
	FileHash         string            `json:"file_hash,omitempty"`
	FileSizeBytes    int64             `json:"file_size_bytes"`
	ImportedAt       string            `json:"imported_at,omitempty"`
	ImportedBy       string            `json:"imported_by,omitempty"`
	ToolName         string            `json:"tool_name,omitempty"`
	ToolVersion      string            `json:"tool_version,omitempty"`
	SegType          string            `json:"seg_type,omitempty"`
	AdminLang        string            `json:"admin_lang,omitempty"`
	SrcLang          string            `json:"src_lang,omitempty"`
	DataType         string            `json:"data_type,omitempty"`
	OriginalFormat   string            `json:"original_format,omitempty"`
	OriginalEncoding string            `json:"original_encoding,omitempty"`
	EntryCount       int               `json:"entry_count"`
	Properties       map[string]string `json:"properties,omitempty"`
}

// FormatText renders the labeled detail block.
func (o TMSessionDetail) FormatText(w io.Writer) error {
	rows := [][2]string{
		{"ID", o.ID},
		{"File key", o.FileKey},
		{"File hash", o.FileHash},
		{"File size", fmt.Sprintf("%d bytes", o.FileSizeBytes)},
		{"Imported at", o.ImportedAt},
		{"Imported by", o.ImportedBy},
		{"Tool", fmt.Sprintf("%s %s", o.ToolName, o.ToolVersion)},
		{"Segment type", o.SegType},
		{"Admin language", o.AdminLang},
		{"Source language", o.SrcLang},
		{"Data type", o.DataType},
		{"Original format", o.OriginalFormat},
		{"Original encoding", o.OriginalEncoding},
		{"Entry count", strconv.Itoa(o.EntryCount)},
	}
	for _, r := range rows {
		fmt.Fprintf(w, "%-18s %s\n", r[0]+":", r[1])
	}
	if len(o.Properties) > 0 {
		fmt.Fprintln(w, "Properties:")
		keys := make([]string, 0, len(o.Properties))
		for k := range o.Properties {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Fprintf(w, "  %s = %s\n", k, o.Properties[k])
		}
	}
	return nil
}

// TMSessionDeleteOutput is the structured result of removing a session record.
type TMSessionDeleteOutput struct {
	ID      string `json:"id"`
	Deleted bool   `json:"deleted"`
}

// FormatText reports the removal.
func (o TMSessionDeleteOutput) FormatText(w io.Writer) error {
	fmt.Fprintf(w, "Deleted session %s.\n", o.ID)
	return nil
}

// TMSessionRows converts store sessions into listing rows.
func TMSessionRows(sessions []sievepen.ImportSession) []TMSessionRow {
	rows := make([]TMSessionRow, 0, len(sessions))
	for _, s := range sessions {
		rows = append(rows, TMSessionRow{
			ID:         s.ID,
			FileKey:    s.FileKey,
			Tool:       s.ToolName,
			EntryCount: s.EntryCount,
			ImportedAt: formatSessionTime(s.ImportedAt, "2006-01-02 15:04"),
		})
	}
	return rows
}

// TMSessionDetailFrom converts a store session into its full report.
func TMSessionDetailFrom(s sievepen.ImportSession) TMSessionDetail {
	return TMSessionDetail{
		ID:               s.ID,
		FileKey:          s.FileKey,
		FileHash:         s.FileHash,
		FileSizeBytes:    s.FileSizeBytes,
		ImportedAt:       formatSessionTime(s.ImportedAt, "2006-01-02 15:04:05 MST"),
		ImportedBy:       s.ImportedBy,
		ToolName:         s.ToolName,
		ToolVersion:      s.ToolVersion,
		SegType:          s.SegType,
		AdminLang:        s.AdminLang,
		SrcLang:          s.SrcLang,
		DataType:         s.DataType,
		OriginalFormat:   s.OriginalFormat,
		OriginalEncoding: s.OriginalEncoding,
		EntryCount:       s.EntryCount,
		Properties:       s.Properties,
	}
}

func formatSessionTime(t time.Time, layout string) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(layout)
}
