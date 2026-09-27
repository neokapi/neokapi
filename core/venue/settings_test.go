package venue

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestProjectSettingsHash(t *testing.T) {
	a := ProjectSettings{SettingConvergePolicy: "manual", SettingTranslateAfter: "none"}
	b := ProjectSettings{SettingTranslateAfter: "none", SettingConvergePolicy: "manual"}
	assert.Equal(t, a.Hash(), b.Hash(), "the hash does not depend on map order")
	assert.NotEqual(t, a.Hash(),
		ProjectSettings{SettingConvergePolicy: "manual", SettingTranslateAfter: "written"}.Hash())
	assert.Empty(t, ProjectSettings(nil).Hash())
}

func TestProjectSettingsDiffering(t *testing.T) {
	ours := ProjectSettings{SettingConvergePolicy: "on-push", SettingTranslateAfter: "established"}

	assert.Nil(t, ours.Differing(ProjectSettings{
		SettingConvergePolicy: "on-push", SettingTranslateAfter: "established",
	}), "every setting matches")

	assert.Equal(t, ProjectSettings{SettingTranslateAfter: "established"},
		ours.Differing(ProjectSettings{SettingConvergePolicy: "on-push", SettingTranslateAfter: "written"}))

	assert.Equal(t, ProjectSettings{SettingTranslateAfter: "established"},
		ours.Differing(ProjectSettings{SettingConvergePolicy: "on-push"}),
		"a setting the venue did not report is sent")
}

func TestSettingRefusalNamesWhatWouldApplyIt(t *testing.T) {
	loosens := SettingRefusal{
		Setting: SettingTranslateAfter, Requested: "none", InForce: "established",
		Reason: SettingLoosens, Requires: "manage_project",
	}
	assert.Contains(t, loosens.String(), "translate_after stays established")
	assert.Contains(t, loosens.String(), "owner or admin (manage_project)")

	offStream := SettingRefusal{
		Setting: SettingConvergePolicy, Requested: "manual", InForce: "on-push",
		Reason: SettingNotDefaultStream, DefaultStream: "main",
	}
	assert.Contains(t, offStream.String(), `default stream "main"`)
}
