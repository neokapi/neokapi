package klftm

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/sievepen"
)

// Unmarshal decodes a klftm payload into a File, rejecting an unknown kind or
// major schema version (unknown minors of a known major are accepted).
func Unmarshal(data []byte) (*File, error) {
	var f File
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("klftm: decode: %w", err)
	}
	if err := checkEnvelope(&f); err != nil {
		return nil, err
	}
	return &f, nil
}

// Decode streams a klftm payload from r.
func Decode(r io.Reader) (*File, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("klftm: read: %w", err)
	}
	return Unmarshal(data)
}

func checkEnvelope(f *File) error {
	if f.Kind != Kind {
		return fmt.Errorf("klftm: unexpected kind %q (want %q)", f.Kind, Kind)
	}
	major, ok := majorVersion(f.SchemaVersion)
	if !ok {
		return fmt.Errorf("klftm: invalid schemaVersion %q", f.SchemaVersion)
	}
	wantMajor, _ := majorVersion(SchemaVersion)
	if major != wantMajor {
		return fmt.Errorf("klftm: unsupported major schemaVersion %d (this build speaks %s)", major, SchemaVersion)
	}
	return nil
}

// majorVersion parses the MAJOR of a MAJOR.MINOR string.
func majorVersion(v string) (int, bool) {
	major := 0
	seen := false
	for _, r := range v {
		if r == '.' {
			return major, seen
		}
		if r < '0' || r > '9' {
			return 0, false
		}
		major = major*10 + int(r-'0')
		seen = true
	}
	return 0, false // no dot → invalid MAJOR.MINOR
}

// ModelEntries converts the file's wire entries back to sievepen.TMEntry values.
func (f *File) ModelEntries() []sievepen.TMEntry {
	out := make([]sievepen.TMEntry, 0, len(f.Entries))
	for i := range f.Entries {
		out = append(out, entryToModel(&f.Entries[i]))
	}
	return out
}

// ModelImportSessions converts the file's wire import sessions back to model.
func (f *File) ModelImportSessions() []sievepen.ImportSession {
	out := make([]sievepen.ImportSession, 0, len(f.ImportSessions))
	for i := range f.ImportSessions {
		out = append(out, sessionToModel(&f.ImportSessions[i]))
	}
	return out
}

func entryToModel(e *Entry) sievepen.TMEntry {
	out := sievepen.TMEntry{
		ID:          e.ID,
		ProjectID:   e.ProjectID,
		HintSrcLang: model.LocaleID(e.HintSrcLang),
		Properties:  e.Properties,
		Note:        e.Note,
		CreatedAt:   parseTime(e.Created),
		UpdatedAt:   parseTime(e.Updated),
	}
	if len(e.Variants) > 0 {
		out.Variants = make(map[model.LocaleID][]model.Run, len(e.Variants))
		for loc, runs := range e.Variants {
			out.Variants[model.LocaleID(loc)] = runs
		}
	}
	for i := range e.Entities {
		out.Entities = append(out.Entities, entityToModel(&e.Entities[i]))
	}
	for i := range e.Origins {
		out.Origins = append(out.Origins, originToModel(&e.Origins[i]))
	}
	return out
}

func entityToModel(em *EntityMapping) sievepen.EntityMapping {
	out := sievepen.EntityMapping{
		PlaceholderID: em.PlaceholderID,
		Type:          model.EntityType(em.Type),
		ConceptID:     em.ConceptID,
	}
	if len(em.Values) > 0 {
		out.Values = make(map[model.LocaleID]sievepen.EntityValue, len(em.Values))
		for loc, v := range em.Values {
			out.Values[model.LocaleID(loc)] = sievepen.EntityValue{Text: v.Text, Start: v.Start, End: v.End}
		}
	}
	return out
}

func originToModel(o *Origin) sievepen.Origin {
	return sievepen.Origin{
		Source:    o.Source,
		Key:       o.Key,
		Reference: o.Reference,
		AddedAt:   parseTime(o.AddedAt),
		AddedBy:   o.AddedBy,
		SessionID: o.SessionID,
	}
}

func sessionToModel(s *ImportSession) sievepen.ImportSession {
	return sievepen.ImportSession{
		ID:               s.ID,
		FileKey:          s.FileKey,
		FileHash:         s.FileHash,
		FileSizeBytes:    s.FileSizeBytes,
		ImportedAt:       parseTime(s.ImportedAt),
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

// parseTime parses an RFC 3339 timestamp, returning the zero time for "" or an
// unparseable value (the wire form is always UTC RFC 3339 from formatTime).
func parseTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}
	}
	return t.UTC()
}
