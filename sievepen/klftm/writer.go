package klftm

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/sievepen"
)

// FromModel builds a klftm File from a set of TM entries and the import
// sessions their origins reference. Pass the output of TM.Entries() and
// TM.ListImportSessions().
func FromModel(entries []sievepen.TMEntry, sessions []sievepen.ImportSession) *File {
	f := &File{SchemaVersion: SchemaVersion, Kind: Kind}
	f.Entries = make([]Entry, 0, len(entries))
	for i := range entries {
		f.Entries = append(f.Entries, entryFromModel(&entries[i]))
	}
	f.ImportSessions = make([]ImportSession, 0, len(sessions))
	for i := range sessions {
		f.ImportSessions = append(f.ImportSessions, sessionFromModel(&sessions[i]))
	}
	return f
}

// Marshal encodes a File to deterministic UTF-8 JSON: entries and sessions
// sorted by id, nested slices sorted, HTML escaping off, 2-space indent,
// trailing newline. Deterministic output is what makes klftm hashable and
// diffable, like core/klf.
func Marshal(f *File) ([]byte, error) {
	if f == nil {
		return nil, errors.New("klftm: marshal nil file")
	}
	if f.SchemaVersion == "" {
		f.SchemaVersion = SchemaVersion
	}
	if f.Kind == "" {
		f.Kind = Kind
	}
	canonicalize(f)

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(f); err != nil {
		return nil, fmt.Errorf("klftm: encode: %w", err)
	}
	return buf.Bytes(), nil
}

// canonicalize sorts every order-insensitive slice so Marshal is byte-stable.
func canonicalize(f *File) {
	sort.Slice(f.Entries, func(i, j int) bool { return f.Entries[i].ID < f.Entries[j].ID })
	sort.Slice(f.ImportSessions, func(i, j int) bool { return f.ImportSessions[i].ID < f.ImportSessions[j].ID })
	for i := range f.Entries {
		e := &f.Entries[i]
		sort.Slice(e.Entities, func(a, b int) bool { return e.Entities[a].PlaceholderID < e.Entities[b].PlaceholderID })
		sort.SliceStable(e.Origins, func(a, b int) bool { return originKey(e.Origins[a]) < originKey(e.Origins[b]) })
	}
}

func originKey(o Origin) string {
	return o.SessionID + "\x00" + o.Source + "\x00" + o.Key + "\x00" + o.Reference + "\x00" + o.AddedAt + "\x00" + o.AddedBy
}

func entryFromModel(e *sievepen.TMEntry) Entry {
	out := Entry{
		ID:          e.ID,
		ProjectID:   e.ProjectID,
		HintSrcLang: string(e.HintSrcLang),
		Properties:  e.Properties,
		Note:        e.Note,
		Created:     formatTime(e.CreatedAt),
		Updated:     formatTime(e.UpdatedAt),
	}
	if len(e.Variants) > 0 {
		out.Variants = make(map[string][]model.Run, len(e.Variants))
		for loc, runs := range e.Variants {
			out.Variants[string(loc)] = runs
		}
	}
	for i := range e.Entities {
		out.Entities = append(out.Entities, entityFromModel(&e.Entities[i]))
	}
	for i := range e.Origins {
		out.Origins = append(out.Origins, originFromModel(&e.Origins[i]))
	}
	return out
}

func entityFromModel(m *sievepen.EntityMapping) EntityMapping {
	em := EntityMapping{PlaceholderID: m.PlaceholderID, Type: string(m.Type), ConceptID: m.ConceptID}
	if len(m.Values) > 0 {
		em.Values = make(map[string]EntityValue, len(m.Values))
		for loc, v := range m.Values {
			em.Values[string(loc)] = EntityValue{Text: v.Text, Start: v.Start, End: v.End}
		}
	}
	return em
}

func originFromModel(o *sievepen.Origin) Origin {
	return Origin{
		Source:    o.Source,
		Key:       o.Key,
		Reference: o.Reference,
		AddedAt:   formatTime(o.AddedAt),
		AddedBy:   o.AddedBy,
		SessionID: o.SessionID,
	}
}

func sessionFromModel(s *sievepen.ImportSession) ImportSession {
	return ImportSession{
		ID:               s.ID,
		FileKey:          s.FileKey,
		FileHash:         s.FileHash,
		FileSizeBytes:    s.FileSizeBytes,
		ImportedAt:       formatTime(s.ImportedAt),
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

// formatTime renders a time as RFC 3339 (nanosecond precision, UTC), or "" for
// the zero time so it is omitted from the wire form.
func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}
