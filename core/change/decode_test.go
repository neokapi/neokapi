package change_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/model"
)

// decodeCase is one input to Decode. pointer is where the decoder refuses it,
// empty when it decodes. object marks a single envelope object, which the
// schema can validate too; semantic marks a refusal the schema cannot express
// (a value it allows by shape that the decoder refuses by meaning).
type decodeCase struct {
	name     string
	in       string
	object   bool
	pointer  string
	semantic bool
	check    func(t *testing.T, s change.Set)
}

const rev = "r:3f9a1c0e7b2d4a55"

// envelope wraps operations in a change set.
func envelope(ops ...string) string {
	return `{"schema":"kapi.change/v1","note":"examples","ops":[` + strings.Join(ops, ",") + `]}`
}

func decodeCases() []decodeCase {
	at := `"at":{"doc":"docs/guide.html","block":"p"}`
	return []decodeCase{
		// The operations of the contract, as its examples write them.
		{name: "set_content in text form", object: true, in: envelope(`{"op":"set_content",` + at + `,"if_match":"` + rev + `","text":"Read the <x id=\"1\"/>handbook<x id=\"/1\"/> before you <x id=\"2\"/>order<x id=\"/2\"/>."}`),
			check: func(t *testing.T, s change.Set) {
				op := s.Ops[0]
				assert.Equal(t, change.KindSetContent, op.Kind)
				assert.Equal(t, change.Ref{Doc: "docs/guide.html", Block: "p"}, op.At)
				assert.Equal(t, rev, op.IfMatch)
				body := op.Body.(*change.SetContent)
				require.NotNil(t, body.Text)
				assert.Contains(t, *body.Text, "handbook")
				assert.Equal(t, change.ModeApply, s.Mode)
				assert.Equal(t, change.GateEnforce, s.Gate)
			}},
		{name: "replace_text by find", object: true, in: envelope(`{"op":"replace_text",` + at + `,"if_match":"` + rev + `","edits":[{"find":"shop guide","text":"handbook"}]}`)},
		{name: "replace_text by offsets and by range", object: true, in: envelope(
			`{"op":"replace_text",`+at+`,"if_match":"`+rev+`","edits":[{"start":9,"end":19,"text":"handbook"}]}`,
			`{"op":"replace_text",`+at+`,"if_match":"`+rev+`","edits":[{"range":{"start":{"run":2,"offset":0},"end":{"run":2,"offset":10}},"text":"handbook"}]}`)},
		{name: "set_attribute", object: true, in: envelope(`{"op":"set_attribute",` + at + `,"if_match":"` + rev + `","code":"1","name":"href","value":"https://new.example/handbook"}`)},
		{name: "mark", object: true, in: envelope(`{"op":"mark",` + at + `,"if_match":"` + rev + `","range":{"find":"before you"},"type":"fmt:bold"}`)},
		{name: "set_content creating an edition", object: true, in: envelope(`{"op":"set_content","at":{"doc":"docs/guide.html","block":"p","edition":"nb_NO"},"if_match":"absent","basis":"` + rev + `","text":"Les <x id=\"1\"/>håndboka<x id=\"/1\"/>."}`),
			check: func(t *testing.T, s change.Set) {
				assert.Equal(t, model.EditionKey{Locale: "nb-NO"}, s.Ops[0].At.Edition, "the edition key is canonical")
				assert.Equal(t, model.AbsentRevision, s.Ops[0].IfMatch)
				assert.Equal(t, rev, s.Ops[0].Basis)
			}},
		{name: "replace_text in a plural branch", object: true, in: envelope(`{"op":"replace_text","at":{"doc":"locales/en.json","block":"cart.items"},"if_match":"r:77c0a1d2e3f40516","edits":[{"path":[1,{"plural":"one"}],"find":"item","text":"article"}]}`),
			check: func(t *testing.T, s change.Set) {
				e := s.Ops[0].Body.(*change.ReplaceText).Edits[0]
				assert.Equal(t, model.RunPath{{Kind: model.StepIndex, Index: 1}, {Kind: model.StepPlural, PluralForm: model.PluralOne}}, e.Path)
			}},
		{name: "remove_edition", object: true, in: envelope(`{"op":"remove_edition","at":{"doc":"docs/guide.html","block":"p","edition":"de"},"if_match":"r:0c55e1f2a3b4c5d6"}`)},
		{name: "annotate and unannotate", object: true, in: envelope(
			`{"op":"annotate",`+at+`,"type":"note","id":"n1","anchor":{"kind":"range","start":{"run":2,"offset":0},"end":{"run":2,"offset":10}},"value":{"text":"Is it a handbook or a guide?"}}`,
			`{"op":"unannotate",`+at+`,"type":"note","id":"n1"}`)},
		{name: "insert_block and delete_block", object: true, in: envelope(
			`{"op":"insert_block","doc":"locales/en.json","after":"nav.cart","name":"nav.checkout","editions":{"en":{"text":"Checkout"}}}`,
			`{"op":"delete_block","at":{"doc":"locales/en.json","block":"nav.legacy"},"if_match":{"en":"r:5e10a2b3c4d5e6f7"}}`),
			check: func(t *testing.T, s change.Set) {
				assert.Equal(t, change.Ref{Doc: "locales/en.json"}, s.Ops[0].At)
				assert.Equal(t, map[string]string{"en": "r:5e10a2b3c4d5e6f7"}, s.Ops[1].Body.(*change.DeleteBlock).IfMatch)
			}},
		{name: "decide", object: true, in: envelope(
			`{"op":"decide","at":{"doc":"docs/guide.md","block":"install/p","edition":"fr"},"if_match":"r:74dbfac2ed1c9d6e","outcome":"establish"}`,
			`{"op":"decide","at":{"doc":"docs/guide.md","block":"install/p","edition":"fr"},"if_match":"r:74dbfac2ed1c9d6e","outcome":"advise","score":80,"reasons":["terminology"]}`)},
		{name: "term, memory and recipe", object: true, in: envelope(
			`{"op":"term","action":"upsert","term":"handbook","status":"preferred","replaces":"shop guide"}`,
			`{"op":"memory","action":"add","from":{"edition":"en","text":"handbook"},"to":{"edition":"nb_no","text":"håndbok"}}`,
			`{"op":"recipe","path":"defaults.translate_after","value":"written"}`),
			check: func(t *testing.T, s change.Set) {
				assert.Equal(t, "nb-NO", s.Ops[1].Body.(*change.Memory).To.Edition)
			}},
		{name: "native", object: true, in: envelope(`{"op":"native","doc":"report.docx","if_match":"sha256:` + strings.Repeat("9c", 32) + `","name":"docx.append_paragraph","args":{"runs":[{"text":"Results"}],"style":"Heading2"}}`)},
		{name: "set_content with runs", object: true, in: envelope(`{"op":"set_content",` + at + `,"if_match":"*","runs":[{"text":"Read "},{"pcOpen":{"id":"1","type":"link:hyperlink"}},{"text":"docs","noTranslate":true},{"pcClose":{"id":"1"}}]}`),
			check: func(t *testing.T, s change.Set) {
				runs := s.Ops[0].Body.(*change.SetContent).Runs
				require.Len(t, runs, 4)
				assert.True(t, runs[2].Text.NoTranslate)
			}},
		{name: "set_content with empty runs", object: true, in: envelope(`{"op":"set_content",` + at + `,"if_match":"*","runs":[]}`),
			check: func(t *testing.T, s change.Set) {
				runs := s.Ops[0].Body.(*change.SetContent).Runs
				assert.NotNil(t, runs, "an empty edition is content")
				assert.Empty(t, runs)
			}},
		{name: "preview mode and evidence", object: true, in: `{"mode":"preview","gate":"report","require_basis":true,"evidence":[{"path":"docs/a.md","quote":"x"},{"url":"https://example.com/issue/42"}],"ops":[{"op":"remove_edition",` + at[:len(at)-1] + `,"edition":"de"},"if_match":"*"}]}`,
			check: func(t *testing.T, s change.Set) {
				assert.Equal(t, change.ModePreview, s.Mode)
				assert.Equal(t, change.GateReport, s.Gate)
				assert.True(t, s.RequireBasis)
				assert.Len(t, s.Evidence, 2)
			}},

		// The other two input forms.
		{name: "an array of operations", in: `[{"op":"unannotate",` + at + `,"type":"note","id":"n1"}]`,
			check: func(t *testing.T, s change.Set) {
				assert.Equal(t, change.SchemaID, s.Schema)
				assert.Len(t, s.Ops, 1)
			}},
		{name: "JSONL with an envelope line", in: `{"note":"two lines"}` + "\n" + `{"op":"unannotate",` + at + `,"type":"note","id":"n1"}` + "\n" + `{"op":"unannotate",` + at + `,"type":"note","id":"n2"}` + "\n",
			check: func(t *testing.T, s change.Set) {
				assert.Equal(t, "two lines", s.Note)
				assert.Len(t, s.Ops, 2)
			}},
		{name: "JSONL of operations only", in: `{"op":"unannotate",` + at + `,"type":"note","id":"n1"}` + "\n" + `{"op":"unannotate",` + at + `,"type":"note","id":"n2"}`,
			check: func(t *testing.T, s change.Set) { assert.Len(t, s.Ops, 2) }},

		// Refusals, each at the pointer of what is wrong.
		{name: "unknown envelope field", object: true, pointer: "/actor", in: `{"actor":{"kind":"agent"},"ops":[{"op":"unannotate",` + at + `,"type":"note","id":"n1"}]}`},
		{name: "unknown operation", object: true, pointer: "/ops/0/op", in: envelope(`{"op":"set_text",` + at + `}`)},
		{name: "the in-process provenance operation", object: true, pointer: "/ops/0/op", in: envelope(`{"op":"provenance",` + at + `,"status":"draft"}`)},
		{name: "unknown operation field", object: true, pointer: "/ops/1/whatever", in: envelope(`{"op":"unannotate",`+at+`,"type":"note","id":"n1"}`, `{"op":"unannotate",`+at+`,"type":"note","id":"n1","whatever":1}`)},
		{name: "unknown field in a reference", object: true, pointer: "/ops/0/at/locale", in: envelope(`{"op":"unannotate","at":{"doc":"a","block":"b","locale":"fr"},"type":"note","id":"n1"}`)},
		{name: "unknown field in an edit", object: true, pointer: "/ops/0/edits/1/replace", in: envelope(`{"op":"replace_text",` + at + `,"if_match":"*","edits":[{"find":"a","text":"b"},{"find":"c","replace":"d","text":"e"}]}`)},
		{name: "a run carrying native data", object: true, pointer: "/ops/0/runs/1/pcOpen/data", in: envelope(`{"op":"set_content",` + at + `,"if_match":"*","runs":[{"text":"a"},{"pcOpen":{"id":"1","data":"<b>"}}]}`)},
		{name: "a run with two kinds", object: true, pointer: "/ops/0/runs/0", in: envelope(`{"op":"set_content",` + at + `,"if_match":"*","runs":[{"text":"a","ph":{"id":"1"}}]}`)},
		{name: "a plural branch carrying native data", object: true, pointer: "/ops/0/runs/0/plural/forms/one/0/ph/data", in: envelope(`{"op":"set_content",` + at + `,"if_match":"*","runs":[{"plural":{"pivot":"n","forms":{"one":[{"ph":{"id":"n","data":"{n}"}}]}}}]}`)},
		{name: "missing if_match", object: true, pointer: "/ops/0", in: envelope(`{"op":"replace_text",` + at + `,"edits":[{"find":"a","text":"b"}]}`)},
		{name: "an empty if_match", object: true, pointer: "/ops/0/if_match", in: envelope(`{"op":"replace_text",` + at + `,"if_match":"","edits":[{"find":"a","text":"b"}]}`)},
		{name: "a malformed revision", object: true, pointer: "/ops/0/if_match", in: envelope(`{"op":"remove_edition",` + at + `,"if_match":"3f9a1c0e"}`)},
		{name: "a null value", object: true, pointer: "/ops/0/if_match", in: envelope(`{"op":"remove_edition",` + at + `,"if_match":null}`)},
		{name: "both text and runs", object: true, pointer: "/ops/0", in: envelope(`{"op":"set_content",` + at + `,"if_match":"*","text":"a","runs":[]}`)},
		{name: "neither text nor runs", object: true, pointer: "/ops/0", in: envelope(`{"op":"set_content",` + at + `,"if_match":"*"}`)},
		{name: "an edit naming its text twice", object: true, pointer: "/ops/0/edits/0", in: envelope(`{"op":"replace_text",` + at + `,"if_match":"*","edits":[{"find":"a","start":0,"end":1,"text":"b"}]}`)},
		{name: "occurrence without find", object: true, pointer: "/ops/0/edits/0/occurrence", semantic: true, in: envelope(`{"op":"replace_text",` + at + `,"if_match":"*","edits":[{"start":0,"end":1,"occurrence":2,"text":"b"}]}`)},
		{name: "an edition that is not a locale", object: true, pointer: "/ops/0/at/edition", semantic: true, in: envelope(`{"op":"unannotate","at":{"doc":"a","block":"b","edition":"xx-YY"},"type":"note","id":"n1"}`)},
		{name: "an unknown edition dimension", object: true, pointer: "/ops/0/at/edition", semantic: true, in: envelope(`{"op":"unannotate","at":{"doc":"a","block":"b","edition":"fr;product=app"},"type":"note","id":"n1"}`)},
		{name: "an edition on delete_block", object: true, pointer: "/ops/0/at/edition", in: envelope(`{"op":"delete_block","at":{"doc":"a","block":"b","edition":"fr"},"if_match":{"fr":"` + rev + `"}}`)},
		{name: "basis on an operation that has none", object: true, pointer: "/ops/0/basis", in: envelope(`{"op":"replace_text",` + at + `,"if_match":"*","basis":"` + rev + `","edits":[{"find":"a","text":"b"}]}`)},
		{name: "a decide outcome", object: true, pointer: "/ops/0/outcome", in: envelope(`{"op":"decide",` + at + `,"if_match":"` + rev + `","outcome":"approve"}`)},
		{name: "the reserved propose mode", object: true, pointer: "/mode", in: `{"mode":"propose","ops":[{"op":"unannotate",` + at + `,"type":"note","id":"n1"}]}`},
		{name: "another contract version", object: true, pointer: "/schema", in: `{"schema":"kapi.change/v2","ops":[{"op":"unannotate",` + at + `,"type":"note","id":"n1"}]}`},
		{name: "no operations", object: true, pointer: "/ops", in: `{"note":"nothing"}`},
		{name: "ops in the envelope line and after it", pointer: "/ops", in: `{"ops":[]}` + "\n" + `{"op":"unannotate",` + at + `,"type":"note","id":"n1"}`},
		{name: "a wrong type", object: true, pointer: "/ops/0/edits/0/start", in: envelope(`{"op":"replace_text",` + at + `,"if_match":"*","edits":[{"start":"0","end":1,"text":"b"}]}`)},
		{name: "an unknown field in a path step", object: true, pointer: "/ops/0/edits/0/path/1", in: envelope(`{"op":"replace_text",` + at + `,"if_match":"*","edits":[{"path":[1,{"plural":"one","zz":1}],"find":"a","text":"b"}]}`)},
		{name: "an explicit plural selector", object: true, in: envelope(`{"op":"set_content",` + at + `,"if_match":"*","path":[0,{"plural":"=0"}],"runs":[{"text":"none"}]}`)},
		{name: "a path step naming no plural form", object: true, pointer: "/ops/0/edits/0/path/1/plural", in: envelope(`{"op":"replace_text",` + at + `,"if_match":"*","edits":[{"path":[1,{"plural":"onee"}],"find":"a","text":"b"}]}`)},
		{name: "a plural form that is not a plural category", object: true, pointer: "/ops/0/runs/0/plural/forms/onee", in: envelope(`{"op":"set_content",` + at + `,"if_match":"*","runs":[{"plural":{"pivot":"n","forms":{"onee":[{"text":"a"}],"other":[{"text":"b"}]}}}]}`)},
		{name: "an edition named twice on delete_block", object: true, pointer: "/ops/0/if_match/nb_NO", semantic: true, in: envelope(`{"op":"delete_block","at":{"doc":"a","block":"b"},"if_match":{"nb-NO":"` + rev + `","nb_NO":"` + rev + `"}}`)},
		{name: "an edition named twice on insert_block", object: true, pointer: "/ops/0/editions/nb_NO", semantic: true, in: envelope(`{"op":"insert_block","doc":"a","after":"p1","editions":{"nb-NO":{"text":"Hei"},"nb_NO":{"text":"Hallo"}}}`)},
	}
}

func TestDecode(t *testing.T) {
	for _, tc := range decodeCases() {
		t.Run(tc.name, func(t *testing.T) {
			s, err := change.Decode(strings.NewReader(tc.in))
			if tc.pointer == "" {
				require.NoError(t, err)
				if tc.check != nil {
					tc.check(t, s)
				}
				return
			}
			require.Error(t, err)
			var ce *change.Error
			require.ErrorAs(t, err, &ce)
			assert.Equal(t, change.CodeInvalid, ce.Code)
			assert.Equal(t, tc.pointer, ce.Pointer, ce.Message)
		})
	}
}

// What Decode reads, MarshalJSON writes back, and the two agree.
func TestDecode_RoundTripsThroughMarshal(t *testing.T) {
	for _, tc := range decodeCases() {
		if tc.pointer != "" {
			continue
		}
		t.Run(tc.name, func(t *testing.T) {
			s, err := change.Decode(strings.NewReader(tc.in))
			require.NoError(t, err)
			out, err := json.Marshal(s)
			require.NoError(t, err)
			again, err := change.Decode(strings.NewReader(string(out)))
			require.NoError(t, err, "%s", out)
			assert.Equal(t, s, again)
		})
	}
}

func TestOpMarshal_ShapeOnTheWire(t *testing.T) {
	text := "Hi <x id=\"1\"/>there<x id=\"/1\"/>"
	op := change.Op{Kind: change.KindSetContent, At: change.Ref{Doc: "a.html", Block: "p", Edition: model.EditionKey{Locale: "en", Channel: "short"}},
		IfMatch: model.AbsentRevision, Basis: rev, Body: &change.SetContent{Text: &text}}
	b, err := op.MarshalJSON()
	require.NoError(t, err)
	assert.JSONEq(t, `{"op":"set_content","at":{"doc":"a.html","block":"p","edition":"en;channel=short"},"if_match":"absent","basis":"`+rev+`","text":"Hi <x id=\"1\"/>there<x id=\"/1\"/>"}`, string(b))
	assert.Contains(t, string(b), `<x id=`, "markup is not escaped")

	_, err = json.Marshal(change.Op{Kind: change.KindSetContent, Body: &change.ReplaceText{}})
	assert.Error(t, err, "a body of another kind")
}
