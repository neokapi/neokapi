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
	tests := []struct {
		name   string
		actor  change.Actor
		role   change.Role
		before model.Edition
		want   change.Consequence
	}{
		{"anyone's edit of an established source drops it to written", change.Actor{Kind: change.ActorTool}, change.RoleAuthoritative,
			model.Edition{Status: "established", Origin: model.Origin{Kind: model.OriginOCR}},
			change.Consequence{Status: "written", Origin: model.Origin{Kind: model.OriginOCR}}},
		{"a written source stays written", change.Actor{Kind: change.ActorPerson}, change.RoleAuthoritative,
			model.Edition{Status: "written"}, change.Consequence{Status: "written"}},
		{"a source with no status keeps none", change.Actor{Kind: change.ActorAgent}, change.RoleAuthoritative,
			model.Edition{}, change.Consequence{}},
		{"a person's edit of a translation", change.Actor{Kind: change.ActorPerson, Name: "ana"}, change.RoleDerived, established,
			change.Consequence{Status: "translated", Origin: model.Origin{Kind: model.OriginHuman, Timestamp: stamp}}},
		{"an agent's edit of a translation", change.Actor{Kind: change.ActorAgent, Name: "claude", Session: "s1"}, change.RoleDerived, draft,
			change.Consequence{Status: "translated", Origin: model.Origin{Kind: model.OriginAgent, Engine: "claude", Reference: "s1", Timestamp: stamp}}},
		{"a tool's edit leaves status and origin to its stamp", change.Actor{Kind: change.ActorTool, Name: "translate"}, change.RoleDerived, established,
			change.Consequence{Status: "established", Origin: established.Origin}},
		{"a tool's new edition starts with none", change.Actor{Kind: change.ActorTool}, change.RoleDerived, model.Edition{},
			change.Consequence{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, change.Consequences(tc.actor, tc.role, tc.before, fixedClock()))
		})
	}
}

// ApplyBlock records the consequences on the edition it changes, and an
// unchanged edition keeps its decision.
func TestApplyBlock_RecordsConsequences(t *testing.T) {
	b := guideBlock()
	b.SourceStatus = model.SourceStatusEstablished
	b.Target("nb").Status = model.TargetStatusEstablished

	requireApplied(t, apply(t, b, person, replace("nb", editionRev(b, "nb"), find("Les", "Lees"))))
	assert.Equal(t, model.TargetStatusTranslated, b.Target("nb").Status)
	assert.Equal(t, model.OriginHuman, b.Target("nb").Origin.Kind)
	assert.Equal(t, model.SourceStatusEstablished, b.SourceStatus, "a translation's edit leaves the source alone")

	requireApplied(t, apply(t, b, agent, replace("", sourceRev(b), find("shop guide", "handbook"))))
	assert.Equal(t, model.SourceStatusWritten, b.SourceStatus, "the source approval bound the old wording")
	require.Equal(t, model.TargetStatusTranslated, b.Target("nb").Status, "a source edit stales a translation through its basis, not its status")
}
