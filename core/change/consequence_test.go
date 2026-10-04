package change_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/model"
)

func TestConsequences(t *testing.T) {
	stamp := fixedClock().Format("2006-01-02T15:04:05Z07:00")
	established := model.Edition{Status: "established", Origin: model.Origin{Kind: model.OriginHuman, Timestamp: "earlier"}}
	draft := model.Edition{Status: "draft", Origin: model.Origin{Kind: model.OriginMT, Engine: "deepl"}}
	recase := change.Actor{Kind: change.ActorTool, Name: "case-transform"}
	tests := []struct {
		name    string
		actor   change.Actor
		role    change.Role
		before  model.Edition
		created bool
		want    change.Consequence
	}{
		{"anyone's edit of an established source drops it to written", change.Actor{Kind: change.ActorTool}, change.RoleAuthoritative,
			model.Edition{Status: "established", Origin: model.Origin{Kind: model.OriginOCR}}, false,
			change.Consequence{Status: "written", Origin: model.Origin{Kind: model.OriginOCR}}},
		{"a written source stays written", change.Actor{Kind: change.ActorPerson}, change.RoleAuthoritative,
			model.Edition{Status: "written"}, false, change.Consequence{Status: "written"}},
		{"a source with no status keeps none", change.Actor{Kind: change.ActorAgent}, change.RoleAuthoritative,
			model.Edition{}, false, change.Consequence{}},
		{"a person's edit of a translation", change.Actor{Kind: change.ActorPerson, Name: "ana"}, change.RoleDerived, established, false,
			change.Consequence{Status: "translated", Origin: model.Origin{Kind: model.OriginHuman, Timestamp: stamp}}},
		{"an agent's edit of a translation", change.Actor{Kind: change.ActorAgent, Name: "claude", Session: "s1"}, change.RoleDerived, draft, false,
			change.Consequence{Status: "translated", Origin: model.Origin{Kind: model.OriginAgent, Engine: "claude", Reference: "s1", Timestamp: stamp}}},
		{"a tool's edit of an established translation makes it a draft and keeps who translated it", recase, change.RoleDerived, established, false,
			change.Consequence{Status: "draft", Origin: established.Origin}},
		{"a tool's edit of a translation with no origin records the tool", recase, change.RoleDerived, model.Edition{Status: "translated"}, false,
			change.Consequence{Status: "draft", Origin: model.Origin{Tool: "case-transform"}}},
		{"a tool's new edition takes the tool's origin and no status", recase, change.RoleDerived, model.Edition{}, true,
			change.Consequence{Origin: model.Origin{Tool: "case-transform"}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, change.Consequences(tc.actor, tc.role, tc.before, tc.created, fixedClock()))
		})
	}
}

// ApplyBlock records the consequences on the edition it changes, and an
// unchanged edition keeps its decision.
func TestApplyBlock_RecordsConsequences(t *testing.T) {
	b := guideBlock()
	require.True(t, b.SetEditionStatus(b.Authoritative(model.AuthorityPolicy{}), model.Status(model.SourceStatusEstablished)))
	require.True(t, b.SetEditionStatus(model.Variant("nb"), model.Status(model.TargetStatusEstablished)))

	requireApplied(t, apply(t, b, person, replace("nb", editionRev(b, "nb"), find("Les", "Lees"))))
	assert.Equal(t, model.TargetStatusTranslated, model.TargetStatus(translation(t, b, "nb").Status))
	assert.Equal(t, model.OriginHuman, translation(t, b, "nb").Origin.Kind)
	assert.Equal(t, model.SourceStatusEstablished, sourceStatus(b), "a translation's edit leaves the source alone")

	requireApplied(t, apply(t, b, agent, replace("", sourceRev(b), find("shop guide", "handbook"))))
	assert.Equal(t, model.SourceStatusWritten, sourceStatus(b), "the source approval bound the old wording")
	require.Equal(t, model.TargetStatusTranslated, model.TargetStatus(translation(t, b, "nb").Status), "a source edit stales a translation through its basis, not its status")

	// A tool that rewrites an approved translation leaves wording nobody has
	// read, so the approval does not carry over; one whose write changes
	// nothing leaves the approval alone.
	require.True(t, b.SetEditionStatus(model.Variant("nb"), model.Status(model.TargetStatusEstablished)))
	requireApplied(t, apply(t, b, tool, setRuns("nb", "*", b.TargetRuns("nb"))))
	assert.Equal(t, model.TargetStatusEstablished, model.TargetStatus(translation(t, b, "nb").Status))
	requireApplied(t, apply(t, b, tool, replace("nb", "*", find("Lees", "LEES"))))
	assert.Equal(t, model.TargetStatusDraft, model.TargetStatus(translation(t, b, "nb").Status))
	assert.Equal(t, model.OriginHuman, translation(t, b, "nb").Origin.Kind, "the translation still names who translated it")
}
