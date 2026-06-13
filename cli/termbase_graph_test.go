package cli

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/neokapi/neokapi/cli/output"
	"github.com/neokapi/neokapi/core/graph"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/termbase"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// seedGraphTermbase creates a fresh SQLite termbase with three concepts to
// relate — a retired name, its replacement, and a competitor — and returns
// the database path.
func seedGraphTermbase(t *testing.T) string {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "termbase.db")
	tb, err := termbase.NewSQLiteTermBase(dbPath)
	require.NoError(t, err)
	defer tb.Close()

	require.NoError(t, tb.AddConcept(t.Context(), termbase.Concept{
		ID:         "old-name",
		Domain:     "software",
		Definition: "The retired product name",
		Source:     termbase.TermSourceTerminology,
		Terms: []termbase.Term{
			{Text: "Legacy Sync", Locale: model.LocaleEnglish, Status: model.TermDeprecated},
			{Text: "Alt-Sync", Locale: model.LocaleGerman, Status: model.TermApproved,
				Validity: &graph.Validity{Tags: map[string]string{"market": "dach"}}},
		},
	}))
	require.NoError(t, tb.AddConcept(t.Context(), termbase.Concept{
		ID:     "new-name",
		Domain: "software",
		Terms: []termbase.Term{
			{Text: "Cloud Sync", Locale: model.LocaleEnglish, Status: model.TermPreferred},
		},
	}))
	require.NoError(t, tb.AddConcept(t.Context(), termbase.Concept{
		ID:     "rival",
		Domain: "software",
		Terms: []termbase.Term{
			{Text: "RivalSync", Locale: model.LocaleEnglish, Status: model.TermApproved},
		},
	}))
	return dbPath
}

// newGraphTestCmd builds one of the termbase graph subcommands bound to the
// SQLite file at dbPath, with the resource and output flags the real command
// group registers. Calling RunE directly bypasses cobra's Execute, which is
// what normally seeds cmd.Context(); set it here so the ctx-aware termbase
// path has a real context instead of nil.
func newGraphTestCmd(t *testing.T, build func() *cobra.Command, dbPath string) (*cobra.Command, *bytes.Buffer) {
	t.Helper()
	cmd := build()
	AddResourceFlags(cmd)
	output.AddFlags(cmd)
	require.NoError(t, cmd.Flags().Set("file", dbPath))
	cmd.SetContext(t.Context())
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	return cmd, &buf
}

// listGraphRelations reopens the termbase and returns all persisted relations.
func listGraphRelations(t *testing.T, dbPath string) []termbase.ConceptRelation {
	t.Helper()
	tb, err := termbase.NewSQLiteTermBase(dbPath)
	require.NoError(t, err)
	defer tb.Close()
	rels, err := tb.ListRelations(t.Context(), nil)
	require.NoError(t, err)
	return rels
}

// addGraphRelation reopens the termbase and persists one relation directly.
func addGraphRelation(t *testing.T, dbPath string, rel termbase.ConceptRelation) {
	t.Helper()
	tb, err := termbase.NewSQLiteTermBase(dbPath)
	require.NoError(t, err)
	defer tb.Close()
	require.NoError(t, tb.AddRelation(t.Context(), rel))
}

func TestTermbaseRelate_AddsRelation(t *testing.T) {
	dbPath := seedGraphTermbase(t)
	a := &App{}

	cmd, buf := newGraphTestCmd(t, a.newTermbaseRelateCmd, dbPath)
	require.NoError(t, cmd.Flags().Set("note", "renamed at launch"))
	require.NoError(t, cmd.RunE(cmd, []string{"old-name", "replaced-by", "new-name"}))

	rels := listGraphRelations(t, dbPath)
	require.Len(t, rels, 1)
	assert.NotEmpty(t, rels[0].ID, "the CLI must generate a relation ID")
	assert.Equal(t, "old-name", rels[0].SourceID)
	assert.Equal(t, "new-name", rels[0].TargetID)
	assert.Equal(t, graph.LabelReplacedBy, rels[0].RelationType)
	assert.Equal(t, "renamed at launch", rels[0].Note)
	assert.Nil(t, rels[0].Validity, "no validity flags must persist a nil validity")
	assert.False(t, rels[0].CreatedAt.IsZero())

	out := buf.String()
	assert.Contains(t, out, "Added relation")
	assert.Contains(t, out, "old-name -[REPLACED_BY]-> new-name")
	assert.Contains(t, out, "renamed at launch")
}

func TestTermbaseRelate_JSONOutput(t *testing.T) {
	dbPath := seedGraphTermbase(t)
	a := &App{}

	cmd, buf := newGraphTestCmd(t, a.newTermbaseRelateCmd, dbPath)
	require.NoError(t, cmd.Flags().Set("json", "true"))
	require.NoError(t, cmd.RunE(cmd, []string{"old-name", "use-instead", "new-name"}))

	var parsed output.TermbaseRelateOutput
	require.NoError(t, json.Unmarshal(buf.Bytes(), &parsed), "relate must emit valid JSON: %s", buf.String())
	assert.NotEmpty(t, parsed.Relation.ID)
	assert.Equal(t, "old-name", parsed.Relation.SourceID)
	assert.Equal(t, "new-name", parsed.Relation.TargetID)
	assert.Equal(t, graph.LabelUseInstead, parsed.Relation.RelationType)
	assert.Equal(t, dbPath, parsed.DBPath)
}

func TestTermbaseRelate_ValidityFlags(t *testing.T) {
	dbPath := seedGraphTermbase(t)
	a := &App{}

	cmd, _ := newGraphTestCmd(t, a.newTermbaseRelateCmd, dbPath)
	require.NoError(t, cmd.Flags().Set("valid-from", "2026-01-01"))
	require.NoError(t, cmd.Flags().Set("valid-to", "2026-06-01T12:30:00Z"))
	require.NoError(t, cmd.Flags().Set("tag", "market=dach"))
	require.NoError(t, cmd.Flags().Set("tag", "channel=web"))
	require.NoError(t, cmd.RunE(cmd, []string{"old-name", "use-instead", "new-name"}))

	rels := listGraphRelations(t, dbPath)
	require.Len(t, rels, 1)
	v := rels[0].Validity
	require.NotNil(t, v)
	require.NotNil(t, v.ValidFrom)
	assert.True(t, v.ValidFrom.Equal(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)),
		"a plain date must parse as midnight UTC")
	require.NotNil(t, v.ValidTo)
	assert.True(t, v.ValidTo.Equal(time.Date(2026, 6, 1, 12, 30, 0, 0, time.UTC)))
	assert.Equal(t, map[string]string{"market": "dach", "channel": "web"}, v.Tags)
}

func TestTermbaseRelate_Errors(t *testing.T) {
	dbPath := seedGraphTermbase(t)
	a := &App{}

	tests := []struct {
		name    string
		args    []string
		flags   map[string]string
		wantErr string
	}{
		{
			name:    "unknown relation",
			args:    []string{"old-name", "friends-with", "new-name"},
			wantErr: "unknown relation",
		},
		{
			name:    "concept-to-term label rejected",
			args:    []string{"old-name", "has-term", "new-name"},
			wantErr: "unknown relation",
		},
		{
			name:    "invalid valid-from",
			args:    []string{"old-name", "related", "new-name"},
			flags:   map[string]string{"valid-from": "yesterday"},
			wantErr: "--valid-from",
		},
		{
			name:    "tag without value",
			args:    []string{"old-name", "related", "new-name"},
			flags:   map[string]string{"tag": "dach"},
			wantErr: "--tag",
		},
		{
			name:    "target concept not found",
			args:    []string{"old-name", "related", "ghost"},
			wantErr: "target concept not found",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, _ := newGraphTestCmd(t, a.newTermbaseRelateCmd, dbPath)
			for k, v := range tt.flags {
				require.NoError(t, cmd.Flags().Set(k, v))
			}
			err := cmd.RunE(cmd, tt.args)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
	assert.Empty(t, listGraphRelations(t, dbPath), "failed relate calls must not persist relations")
}

func TestRelationTypeArg(t *testing.T) {
	tests := []struct {
		arg     string
		want    string
		wantErr bool
	}{
		{arg: "broader", want: graph.LabelBroader},
		{arg: "narrower", want: graph.LabelNarrower},
		{arg: "part-of", want: graph.LabelPartOf},
		{arg: "has-part", want: graph.LabelHasPart},
		{arg: "related", want: graph.LabelRelated},
		{arg: "replaced-by", want: graph.LabelReplacedBy},
		{arg: "use-instead", want: graph.LabelUseInstead},
		{arg: "exact-match", want: graph.LabelExactMatch},
		{arg: "close-match", want: graph.LabelCloseMatch},
		{arg: "competitor", want: graph.LabelCompetitor},
		{arg: "USE_INSTEAD", want: graph.LabelUseInstead},
		{arg: "REPLACED_BY", want: graph.LabelReplacedBy},
		{arg: "friends-with", wantErr: true},
		{arg: "has-term", wantErr: true}, // concept-to-term labels are statuses, not relations
		{arg: "forbidden", wantErr: true},
		{arg: "preferred", wantErr: true},
		{arg: "", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.arg, func(t *testing.T) {
			got, err := relationTypeArg(tt.arg)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// seedScopedRelations persists three edges with distinct validity for the
// relations listing tests: one unbounded, one market-tagged, one retired.
func seedScopedRelations(t *testing.T, dbPath string, past time.Time) {
	t.Helper()
	addGraphRelation(t, dbPath, termbase.ConceptRelation{
		ID: "r-always", SourceID: "old-name", TargetID: "new-name",
		RelationType: graph.LabelReplacedBy,
	})
	addGraphRelation(t, dbPath, termbase.ConceptRelation{
		ID: "r-dach", SourceID: "old-name", TargetID: "new-name",
		RelationType: graph.LabelUseInstead,
		Validity:     &graph.Validity{Tags: map[string]string{"market": "dach"}},
	})
	addGraphRelation(t, dbPath, termbase.ConceptRelation{
		ID: "r-retired", SourceID: "rival", TargetID: "old-name",
		RelationType: graph.LabelCompetitor,
		Validity:     &graph.Validity{ValidTo: &past},
	})
}

// runRelationsJSON runs `termbase relations` with the given args and flags
// and returns the parsed JSON output.
func runRelationsJSON(t *testing.T, dbPath string, args []string, flags map[string]string) output.TermbaseRelationsOutput {
	t.Helper()
	a := &App{}
	cmd, buf := newGraphTestCmd(t, a.newTermbaseRelationsCmd, dbPath)
	require.NoError(t, cmd.Flags().Set("json", "true"))
	for k, v := range flags {
		require.NoError(t, cmd.Flags().Set(k, v))
	}
	require.NoError(t, cmd.RunE(cmd, args))

	var parsed output.TermbaseRelationsOutput
	require.NoError(t, json.Unmarshal(buf.Bytes(), &parsed), "relations must emit valid JSON: %s", buf.String())
	return parsed
}

func relationOutputIDs(o output.TermbaseRelationsOutput) []string {
	ids := make([]string, len(o.Relations))
	for i, r := range o.Relations {
		ids[i] = r.ID
	}
	return ids
}

func TestTermbaseRelations_ListAndFilter(t *testing.T) {
	dbPath := seedGraphTermbase(t)
	past := time.Now().UTC().Add(-24 * time.Hour).Truncate(time.Second)
	seedScopedRelations(t, dbPath, past)

	// No args, no scope flags: every relation, no validity filtering.
	all := runRelationsJSON(t, dbPath, nil, nil)
	assert.Equal(t, []string{"r-always", "r-dach", "r-retired"}, relationOutputIDs(all))
	assert.Equal(t, 3, all.Total)

	// Concept-scoped: both directions — old-name is the source of r-always
	// and r-dach and the target of r-retired.
	ofOld := runRelationsJSON(t, dbPath, []string{"old-name"}, nil)
	assert.Equal(t, []string{"r-always", "r-dach", "r-retired"}, relationOutputIDs(ofOld))
	ofNew := runRelationsJSON(t, dbPath, []string{"new-name"}, nil)
	assert.Equal(t, []string{"r-always", "r-dach"}, relationOutputIDs(ofNew))

	// A tag scope evaluates at the current time: the retired edge drops out,
	// and the dach edge only holds in its market.
	dach := runRelationsJSON(t, dbPath, nil, map[string]string{"tag": "market=dach"})
	assert.Equal(t, []string{"r-always", "r-dach"}, relationOutputIDs(dach))
	us := runRelationsJSON(t, dbPath, nil, map[string]string{"tag": "market=us"})
	assert.Equal(t, []string{"r-always"}, relationOutputIDs(us))

	// As-of a date before the retirement, the retired edge is active again.
	asOf := runRelationsJSON(t, dbPath, nil, map[string]string{
		"as-of": past.Add(-time.Hour).Format(time.RFC3339),
	})
	assert.Equal(t, []string{"r-always", "r-dach", "r-retired"}, relationOutputIDs(asOf))
}

func TestTermbaseRelations_TextTable(t *testing.T) {
	dbPath := seedGraphTermbase(t)
	addGraphRelation(t, dbPath, termbase.ConceptRelation{
		ID: "r1", SourceID: "old-name", TargetID: "new-name",
		RelationType: graph.LabelReplacedBy, Note: "renamed at launch",
		Validity: &graph.Validity{Tags: map[string]string{"market": "dach"}},
	})

	a := &App{}
	cmd, buf := newGraphTestCmd(t, a.newTermbaseRelationsCmd, dbPath)
	require.NoError(t, cmd.RunE(cmd, nil))

	out := buf.String()
	assert.Contains(t, out, "RELATION")
	assert.Contains(t, out, "REPLACED_BY")
	assert.Contains(t, out, "market=dach")
	assert.Contains(t, out, "renamed at launch")
	assert.Contains(t, out, "Total: 1 relation(s)")
}

func TestTermbaseUnrelate(t *testing.T) {
	dbPath := seedGraphTermbase(t)
	addGraphRelation(t, dbPath, termbase.ConceptRelation{
		ID: "r1", SourceID: "old-name", TargetID: "new-name",
		RelationType: graph.LabelReplacedBy,
	})

	a := &App{}
	cmd, buf := newGraphTestCmd(t, a.newTermbaseUnrelateCmd, dbPath)
	require.NoError(t, cmd.RunE(cmd, []string{"r1"}))
	assert.Contains(t, buf.String(), "Removed relation r1")
	assert.Empty(t, listGraphRelations(t, dbPath))

	// Removing it again reports the missing relation.
	again, _ := newGraphTestCmd(t, a.newTermbaseUnrelateCmd, dbPath)
	err := again.RunE(again, []string{"r1"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "relation not found: r1")
}

func TestTermbaseShow_FullConceptDetail(t *testing.T) {
	dbPath := seedGraphTermbase(t)
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	addGraphRelation(t, dbPath, termbase.ConceptRelation{
		ID: "rel-competitor", SourceID: "rival", TargetID: "old-name",
		RelationType: graph.LabelCompetitor,
	})
	addGraphRelation(t, dbPath, termbase.ConceptRelation{
		ID: "rel-replaced", SourceID: "old-name", TargetID: "new-name",
		RelationType: graph.LabelReplacedBy, Note: "renamed at launch",
		Validity: &graph.Validity{ValidFrom: &from},
	})

	a := &App{}
	cmd, buf := newGraphTestCmd(t, a.newTermbaseShowCmd, dbPath)
	require.NoError(t, cmd.Flags().Set("json", "true"))
	require.NoError(t, cmd.RunE(cmd, []string{"old-name"}))

	var parsed output.TermbaseShowOutput
	require.NoError(t, json.Unmarshal(buf.Bytes(), &parsed), "show must emit valid JSON: %s", buf.String())

	assert.Equal(t, "old-name", parsed.ID)
	assert.Equal(t, "software", parsed.Domain)
	assert.Equal(t, "The retired product name", parsed.Definition)
	assert.Equal(t, string(termbase.TermSourceTerminology), parsed.Source)

	// Terms per locale with status and validity.
	require.Len(t, parsed.Terms, 2)
	byLocale := make(map[string]output.TermbaseShowTerm, len(parsed.Terms))
	for _, term := range parsed.Terms {
		byLocale[term.Locale] = term
	}
	assert.Equal(t, "Legacy Sync", byLocale["en"].Text)
	assert.Equal(t, string(model.TermDeprecated), byLocale["en"].Status)
	assert.Nil(t, byLocale["en"].Validity)
	assert.Equal(t, "Alt-Sync", byLocale["de"].Text)
	require.NotNil(t, byLocale["de"].Validity)
	assert.Equal(t, map[string]string{"market": "dach"}, byLocale["de"].Validity.Tags)

	// Relations in both directions, sorted by ID, with the other concept's
	// preferred term resolved for display.
	require.Len(t, parsed.Relations, 2)
	incoming := parsed.Relations[0]
	assert.Equal(t, "rel-competitor", incoming.ID)
	assert.Equal(t, "incoming", incoming.Direction)
	assert.Equal(t, graph.LabelCompetitor, incoming.RelationType)
	assert.Equal(t, "rival", incoming.ConceptID)
	assert.Equal(t, "RivalSync", incoming.ConceptTerm)

	outgoing := parsed.Relations[1]
	assert.Equal(t, "rel-replaced", outgoing.ID)
	assert.Equal(t, "outgoing", outgoing.Direction)
	assert.Equal(t, graph.LabelReplacedBy, outgoing.RelationType)
	assert.Equal(t, "new-name", outgoing.ConceptID)
	assert.Equal(t, "Cloud Sync", outgoing.ConceptTerm, "the replacement's preferred term must label it")
	assert.Equal(t, "renamed at launch", outgoing.Note)
	require.NotNil(t, outgoing.Validity)
	require.NotNil(t, outgoing.Validity.ValidFrom)
	assert.True(t, outgoing.Validity.ValidFrom.Equal(from))
}

func TestTermbaseShow_TextRendering(t *testing.T) {
	dbPath := seedGraphTermbase(t)
	addGraphRelation(t, dbPath, termbase.ConceptRelation{
		ID: "rel-replaced", SourceID: "old-name", TargetID: "new-name",
		RelationType: graph.LabelReplacedBy,
	})
	addGraphRelation(t, dbPath, termbase.ConceptRelation{
		ID: "rel-competitor", SourceID: "rival", TargetID: "old-name",
		RelationType: graph.LabelCompetitor,
	})

	a := &App{}
	cmd, buf := newGraphTestCmd(t, a.newTermbaseShowCmd, dbPath)
	require.NoError(t, cmd.RunE(cmd, []string{"old-name"}))

	out := buf.String()
	assert.Contains(t, out, "Concept: old-name")
	assert.Contains(t, out, "The retired product name")
	assert.Contains(t, out, "Legacy Sync")
	assert.Contains(t, out, "deprecated")
	assert.Contains(t, out, "market=dach")
	assert.Contains(t, out, "-[REPLACED_BY]->")
	assert.Contains(t, out, "new-name (Cloud Sync)")
	assert.Contains(t, out, "<-[COMPETITOR]-")
	assert.Contains(t, out, "rival (RivalSync)")
}

func TestTermbaseShow_NotFound(t *testing.T) {
	dbPath := seedGraphTermbase(t)
	a := &App{}
	cmd, _ := newGraphTestCmd(t, a.newTermbaseShowCmd, dbPath)
	err := cmd.RunE(cmd, []string{"ghost"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "concept not found: ghost")
}

// TestTermbaseCmd_GraphSubcommandsThroughExecute drives the graph subcommands
// through the real termbase command group (cobra Execute seeds the context),
// asserting they are registered and carry the shared resource flags.
func TestTermbaseCmd_GraphSubcommandsThroughExecute(t *testing.T) {
	dbPath := seedGraphTermbase(t)
	a := &App{}

	run := func(args ...string) string {
		t.Helper()
		cmd := a.NewTermbaseCmd()
		cmd.SetArgs(append(args, "--file", dbPath))
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)
		require.NoError(t, cmd.Execute())
		return buf.String()
	}

	out := run("relate", "old-name", "replaced-by", "new-name", "--tag", "market=dach")
	assert.Contains(t, out, "old-name -[REPLACED_BY]-> new-name")

	rels := listGraphRelations(t, dbPath)
	require.Len(t, rels, 1)

	assert.Contains(t, run("relations"), "REPLACED_BY")
	assert.Contains(t, run("show", "old-name"), "Concept: old-name")
	assert.Contains(t, run("unrelate", rels[0].ID), "Removed relation")
	assert.Empty(t, listGraphRelations(t, dbPath))
}

func TestScopeFromFlags(t *testing.T) {
	newCmd := func(t *testing.T, flags map[string]string) *cobra.Command {
		t.Helper()
		a := &App{}
		cmd := a.newTermbaseRelationsCmd()
		for k, v := range flags {
			require.NoError(t, cmd.Flags().Set(k, v))
		}
		return cmd
	}

	t.Run("no flags yields nil scope", func(t *testing.T) {
		scope, err := scopeFromFlags(newCmd(t, nil))
		require.NoError(t, err)
		assert.Nil(t, scope, "no --as-of and no --tag must mean no validity filtering")
	})

	t.Run("tags alone evaluate at now", func(t *testing.T) {
		before := time.Now()
		scope, err := scopeFromFlags(newCmd(t, map[string]string{"tag": "market=dach"}))
		require.NoError(t, err)
		require.NotNil(t, scope)
		assert.Equal(t, map[string]string{"market": "dach"}, scope.Tags)
		assert.False(t, scope.At.Before(before), "a tag-only scope must evaluate at the current time")
	})

	t.Run("as-of date sets the instant", func(t *testing.T) {
		scope, err := scopeFromFlags(newCmd(t, map[string]string{"as-of": "2025-12-01"}))
		require.NoError(t, err)
		require.NotNil(t, scope)
		assert.True(t, scope.At.Equal(time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC)))
	})

	t.Run("invalid as-of errors", func(t *testing.T) {
		_, err := scopeFromFlags(newCmd(t, map[string]string{"as-of": "last week"}))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "--as-of")
	})
}
