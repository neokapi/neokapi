package contextgraph

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The id scheme has one job: two different (kind, scope, local) tuples must
// never render to one id, and an id must read back as what produced it.
func TestNodeIDRoundTrip(t *testing.T) {
	tests := map[string]struct {
		id   NodeID
		want string
	}{
		"local project, no workspace": {
			id:   NodeID{Label: NodeBlock, Scope: Scope{Project: "neokapi"}, Local: "k1_abc"},
			want: "block:/neokapi/:k1_abc",
		},
		"fully qualified": {
			id:   NodeID{Label: NodeBlock, Scope: Scope{Workspace: "ws1", Project: "pr7", Stream: "main"}, Local: "k1_abc"},
			want: "block:ws1/pr7/main:k1_abc",
		},
		"workspace vocabulary": {
			id:   NodeID{Label: NodeConcept, Scope: Scope{Workspace: "ws1"}, Local: "c-memory"},
			want: "concept:ws1//:c-memory",
		},
		"disconnected vocabulary": {
			id:   NodeID{Label: NodeConcept, Scope: Scope{}, Local: "c-memory"},
			want: "concept://:c-memory",
		},
		"separators in values are escaped": {
			id:   NodeID{Label: NodeCollection, Scope: Scope{Project: "a/b"}, Local: "docs:en|x"},
			want: "collection:/a%2Fb/:docs%3Aen%7Cx",
		},
		"the escape character is itself escaped": {
			id:   NodeID{Label: NodeCollection, Scope: Scope{Project: "a%2Fb"}, Local: "x"},
			want: "collection:/a%252Fb/:x",
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got := tt.id.String()
			assert.Equal(t, tt.want, got)
			back, ok := ParseNodeID(got)
			require.True(t, ok, "the id parses")
			assert.Equal(t, tt.id, back, "the id reads back as what produced it")
		})
	}
}

// Escaping is not cosmetic: without it a project named "a/b" and a workspace
// named "a" with project "b" would be one node.
func TestNodeIDsDoNotCollide(t *testing.T) {
	a := BlockNodeID(Scope{Project: "a/b"}, "k")
	b := BlockNodeID(Scope{Workspace: "a", Project: "b"}, "k")
	assert.NotEqual(t, a, b)

	c := CollectionNodeID(Scope{Project: "p"}, "docs")
	d := CollectionNodeID(Scope{Project: "p", Stream: "docs"}, "")
	assert.NotEqual(t, c, d)
}

func TestParseNodeIDRejectsForeignIDs(t *testing.T) {
	for _, id := range []string{"", "block", "block:", "block:only-two:parts/x", "Concept-legacy-id"} {
		_, ok := ParseNodeID(id)
		assert.False(t, ok, "%q is not this vocabulary", id)
	}
}

// Vocabulary nodes drop the instance dimensions, which is the whole mechanism
// behind cross-project questions: two projects reach one concept node.
func TestVocabularyIsWorkspaceScoped(t *testing.T) {
	one := Scope{Workspace: "ws", Project: "alpha", Stream: "main"}
	two := Scope{Workspace: "ws", Project: "beta", Stream: "release"}
	assert.Equal(t, ConceptNodeID(one, "c-1"), ConceptNodeID(two, "c-1"))
	assert.Equal(t, CoordinateNodeID(one, "bowrain", "web"), CoordinateNodeID(two, "bowrain", "web"))

	// Instance nodes do not.
	assert.NotEqual(t, BlockNodeID(one, "k"), BlockNodeID(two, "k"))
	assert.NotEqual(t, CollectionNodeID(one, "docs"), CollectionNodeID(two, "docs"))

	// And a different workspace is a different vocabulary.
	other := Scope{Workspace: "other", Project: "alpha"}
	assert.NotEqual(t, ConceptNodeID(one, "c-1"), ConceptNodeID(other, "c-1"))
}

func TestScopeContains(t *testing.T) {
	full := Scope{Workspace: "ws", Project: "alpha", Stream: "main"}
	tests := map[string]struct {
		filter Scope
		want   bool
	}{
		"empty filter is any":       {Scope{}, true},
		"matching project":          {Scope{Project: "alpha"}, true},
		"other project":             {Scope{Project: "beta"}, false},
		"matching workspace":        {Scope{Workspace: "ws"}, true},
		"other workspace":           {Scope{Workspace: "other"}, false},
		"matching stream":           {Scope{Stream: "main"}, true},
		"other stream":              {Scope{Stream: "release"}, false},
		"every dimension pinned":    {full, true},
		"one dimension disagreeing": {Scope{Workspace: "ws", Project: "alpha", Stream: "release"}, false},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.filter.Contains(full))
		})
	}
}

// An empty dimension is an absent property, not an empty one: a property filter
// compares against a value, and absence is not the empty string.
func TestScopePropertiesOmitEmptyDimensions(t *testing.T) {
	assert.Equal(t, map[string]string{"project": "alpha"}, Scope{Project: "alpha"}.Properties())
	assert.Equal(t, map[string]string{"workspace": "ws", "project": "alpha", "stream": "main"},
		Scope{Workspace: "ws", Project: "alpha", Stream: "main"}.Properties())
	assert.Empty(t, Scope{}.Properties())
}

// Edges key on node ids, which contain no "|", so the join is unambiguous.
func TestEdgeIDsAreDistinct(t *testing.T) {
	s := Scope{Project: "p"}
	source := BlockNodeID(s, "k1")
	target := ConceptNodeID(s, "c1")
	assert.NotEqual(t, EdgeID(EdgeUsesTerm, source, target, ""), EdgeID(EdgeUsesTerm, source, target, "nb"))
	assert.NotEqual(t, EdgeID(EdgeUsesTerm, source, target), EdgeID(EdgeInCollection, source, target))
	assert.Equal(t, "uses_term|"+source+"|"+target+"|nb", EdgeID(EdgeUsesTerm, source, target, "nb"),
		"the id is the join of the nodes it relates, which contain no separator of its own")
}

// A block node carries its content key as a property as well as in its id: the
// id places the instance, the property is what a cross-project "same wording"
// query compares.
func TestBlockNodeCarriesContentKeyBothWays(t *testing.T) {
	s := Scope{Workspace: "ws", Project: "alpha", Stream: "main"}
	n := BlockNode(s, Block{ContentKey: "k1_abc", BlockID: "intro", Document: "docs/a.md", Collection: "docs"})
	assert.Equal(t, BlockNodeID(s, "k1_abc"), n.ID)
	assert.Equal(t, "k1_abc", n.Properties[PropContentKey])
	assert.Equal(t, "alpha", n.Properties[PropProject])
	assert.Equal(t, "main", n.Properties[PropStream])
	assert.Equal(t, "docs/a.md", n.Properties[PropDocument])
}

// A use with no explicit count is still one use.
func TestUsesTermEdgeCountFloorsAtOne(t *testing.T) {
	e := UsesTermEdge(Scope{Project: "p"}, UsesTerm{ContentKey: "k", ConceptID: "c"})
	assert.Equal(t, "1", e.Properties[PropCount])
	e = UsesTermEdge(Scope{Project: "p"}, UsesTerm{ContentKey: "k", ConceptID: "c", Count: 3})
	assert.Equal(t, "3", e.Properties[PropCount])
}
