package formats_test

import (
	"context"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/formats"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
)

// The structure cells of the operations matrix prove, for every fixture that
// holds plurals or selects, what a caller meets through the change service
// over the file home, the path every surface takes:
//
//   - the read lists each plural and select, nested ones included, with the
//     path that reaches it and the text of each branch, and the branches are
//     the ones the format's reader read;
//   - replace_text and set_content naming one branch by its path change that
//     branch and nothing else: the document written differs from the one read
//     in that one word, and every other block reads back at its revision;
//   - set_content with a block's edit text, which shows one branch, is refused
//     as a guard that would flatten the structure, and the document is left
//     as it was.

// structureDoc is a fixture's untouched document written to a directory, with
// the change service that serves it.
type structureDoc struct {
	path, name string
	input      []byte
	svc        *change.Service
}

func newStructureDoc(t *testing.T, reg *registry.FormatRegistry, fx opsFixture) structureDoc {
	t.Helper()
	doc := fx.render(t, nil)
	dir := t.TempDir()
	name := "doc" + filepath.Ext(strings.TrimSuffix(fx.template, ".tmpl"))
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, doc.input, 0o644))
	return structureDoc{path: path, name: name, input: doc.input, svc: structuralService(t, dir, reg, fx.format)}
}

// branchStep is the path step that names a branch of a structure read
// reports.
func branchStep(st change.StructureRead, key string) model.RunPathStep {
	if st.Kind == "select" {
		return model.RunPathStep{Kind: model.StepSelect, SelectValue: key}
	}
	return model.RunPathStep{Kind: model.StepPlural, PluralForm: model.PluralForm(key)}
}

// structureTarget is one branch a cell edits: the block, the branch's path
// and its text as read.
type structureTarget struct {
	block  change.BlockRead
	path   model.RunPath
	text   string
	nested bool
}

func (s structureTarget) String() string {
	return fmt.Sprintf("%s %v", s.block.Ref.Block, s.path)
}

// structureTargets are, for each structure every read block lists, the first
// branch (by key) whose text holds the source word, and no other branch
// holds the word in a structure inside it.
func structureTargets(blocks []change.BlockRead) []structureTarget {
	var out []structureTarget
	for _, b := range blocks {
		for _, st := range b.Structures {
			for _, key := range slices.Sorted(maps.Keys(st.Branches)) {
				text := st.Branches[key]
				if !strings.Contains(text, sourceWord.from) {
					continue
				}
				path := append(slices.Clone(st.Path), branchStep(st, key))
				// A branch that holds a structure shows one branch of it; its
				// words are reached through the inner structure's own entry.
				inner := slices.ContainsFunc(b.Structures, func(o change.StructureRead) bool {
					return len(o.Path) > len(path) && slices.Equal(o.Path[:len(path)], path)
				})
				if inner {
					continue
				}
				out = append(out, structureTarget{block: b, path: path, text: text, nested: len(st.Path) > 1})
				break
			}
		}
	}
	return out
}

func TestOperationsMatrixStructures(t *testing.T) {
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)
	ctx := context.Background()
	person := change.Actor{Kind: change.ActorPerson, Name: "matrix"}
	for _, fx := range opsMatrix() {
		if !fx.structures {
			continue
		}
		t.Run(fx.id()+"/read", func(t *testing.T) {
			d := newStructureDoc(t, reg, fx)
			blocks := readAllBlocks(t, d.svc, d.name)
			native := map[string]*model.Block{}
			for _, b := range fx.readEditable(t, d.input, "") {
				native[change.BlockKey(b)] = b
			}
			kinds := map[string]bool{}
			for _, b := range blocks {
				src := native[b.Ref.Block]
				require.NotNil(t, src, "block %s", b.Ref.Block)
				var want []change.StructureRead
				collectStructures(src.Source, nil, &want)
				assert.Equal(t, want, b.Structures, "block %s lists the structures its reader read", b.Ref.Block)
				for _, st := range b.Structures {
					kinds[st.Kind] = true
					if len(st.Path) > 1 {
						kinds["nested"] = true
					}
				}
			}
			assert.Equal(t, map[string]bool{"plural": true, "select": true, "nested": true}, kinds,
				"the fixture holds a plural, a select and a structure inside a branch")
		})

		d := newStructureDoc(t, reg, fx)
		targets := structureTargets(readAllBlocks(t, d.svc, d.name))
		require.NotEmpty(t, targets, "%s: no branch holds the word", fx.id())
		require.True(t, slices.ContainsFunc(targets, func(s structureTarget) bool { return s.nested }),
			"%s: no branch of a nested structure holds the word", fx.id())
		for _, target := range targets {
			edited := strings.Replace(target.text, sourceWord.from, sourceWord.to, 1)
			for _, kind := range []change.Kind{change.KindReplaceText, change.KindSetContent} {
				t.Run(fmt.Sprintf("%s/%s %s", fx.id(), kind, target), func(t *testing.T) {
					d := newStructureDoc(t, reg, fx)
					before := readAllBlocks(t, d.svc, d.name)
					at := target.block.Ref
					op := change.Op{Kind: kind, At: at, IfMatch: target.block.Rev}
					find := sourceWord.from
					if kind == change.KindReplaceText {
						op.Body = &change.ReplaceText{Edits: []change.TextEdit{{Path: target.path, Find: &find, Text: sourceWord.to}}}
					} else {
						op.Body = &change.SetContent{Path: target.path, Text: &edited}
					}
					res, err := d.svc.Apply(ctx, change.Set{Ops: []change.Op{op}}, person)
					require.NoError(t, err)
					require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops[0].Error)

					out, err := os.ReadFile(d.path)
					require.NoError(t, err)
					assertOneWordReplaced(t, d.input, out)

					after := readAllBlocks(t, d.svc, d.name)
					require.Len(t, after, len(before))
					for i, b := range after {
						if b.Ref.Block != at.Block {
							assert.Equal(t, before[i].Rev, b.Rev, "block %s reads back as it was read", b.Ref.Block)
							continue
						}
						// The edited branch holds the edit, and every branch but
						// it and those that hold it reads as it was read.
						require.Len(t, b.Structures, len(before[i].Structures))
						for j, st := range b.Structures {
							was := before[i].Structures[j]
							require.Equal(t, was.Path, st.Path)
							for key, text := range st.Branches {
								branch := append(slices.Clone(st.Path), branchStep(st, key))
								switch {
								case slices.Equal(branch, target.path):
									assert.Equal(t, edited, text, "the edited branch %v", branch)
								case len(branch) < len(target.path) && slices.Equal(branch, target.path[:len(branch)]):
									// It holds the edited branch.
								default:
									assert.Equal(t, was.Branches[key], text, "branch %v", branch)
								}
							}
						}
					}
				})
			}
		}

		t.Run(fx.id()+"/set_content flattening a structure", func(t *testing.T) {
			d := newStructureDoc(t, reg, fx)
			for _, b := range readAllBlocks(t, d.svc, d.name) {
				if len(b.Structures) == 0 {
					continue
				}
				text := strings.ReplaceAll(b.Text, sourceWord.from, sourceWord.to)
				res, err := d.svc.Apply(ctx, change.Set{Ops: []change.Op{{Kind: change.KindSetContent, At: b.Ref, IfMatch: b.Rev,
					Body: &change.SetContent{Text: &text}}}}, person)
				require.NoError(t, err)
				require.Equal(t, change.SetRefused, res.Status, "block %s", b.Ref.Block)
				require.NotNil(t, res.Ops[0].Error)
				assert.Equal(t, change.CodeGuard, res.Ops[0].Error.Code, res.Ops[0].Error.Message)
				assert.Equal(t, change.SubcodeStructureLost, res.Ops[0].Error.Subcode)
			}
			out, err := os.ReadFile(d.path)
			require.NoError(t, err)
			assert.Equal(t, string(d.input), string(out), "a refused change set writes nothing")
		})
	}
}

// collectStructures lists the structures of runs as a read reports them:
// each with its path and the edit text of each branch, a nested one after
// the one that holds it.
func collectStructures(runs []model.Run, prefix model.RunPath, out *[]change.StructureRead) {
	for i, r := range runs {
		path := append(slices.Clone(prefix), model.RunPathStep{Kind: model.StepIndex, Index: i})
		switch {
		case r.Plural != nil:
			st := change.StructureRead{Path: path, Kind: "plural", Pivot: r.Plural.Pivot, Branches: map[string]string{}}
			for form, rs := range r.Plural.Forms {
				st.Branches[string(form)] = model.RunsEditText(rs)
			}
			*out = append(*out, st)
			for _, form := range slices.Sorted(maps.Keys(r.Plural.Forms)) {
				collectStructures(r.Plural.Forms[form], append(slices.Clone(path), model.RunPathStep{Kind: model.StepPlural, PluralForm: form}), out)
			}
		case r.Select != nil:
			st := change.StructureRead{Path: path, Kind: "select", Pivot: r.Select.Pivot, Branches: map[string]string{}}
			for c, rs := range r.Select.Cases {
				st.Branches[c] = model.RunsEditText(rs)
			}
			*out = append(*out, st)
			for _, c := range slices.Sorted(maps.Keys(r.Select.Cases)) {
				collectStructures(r.Select.Cases[c], append(slices.Clone(path), model.RunPathStep{Kind: model.StepSelect, SelectValue: c}), out)
			}
		}
	}
}

// assertOneWordReplaced asserts out is in with exactly one occurrence of the
// source word replaced, every other byte as it was.
func assertOneWordReplaced(t *testing.T, in, out []byte) {
	t.Helper()
	from, to := sourceWord.from, sourceWord.to
	for i := 0; ; {
		j := strings.Index(string(in[i:]), from)
		if j < 0 {
			break
		}
		at := i + j
		if string(in[:at])+to+string(in[at+len(from):]) == string(out) {
			return
		}
		i = at + 1
	}
	assert.Fail(t, "the written document differs from the one read in more than one word", "read:\n%s\nwritten:\n%s", in, out)
}
