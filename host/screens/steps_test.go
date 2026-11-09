package screens_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/neokapi/neokapi/host/screens"
)

func TestMapStepsOfANewProject(t *testing.T) {
	steps := screens.MapSteps(screens.MapStepState{Awaiting: 1})
	assert.Equal(t, []screens.MapStepDTO{
		{ID: screens.StepDecide, Count: 1},
		{ID: screens.StepCheck},
		{ID: screens.StepFix},
		{ID: screens.StepConnect},
		{ID: screens.StepShare},
	}, steps)
}

func TestMapStepsFollowTheLastRun(t *testing.T) {
	steps := screens.MapSteps(screens.MapStepState{Checked: true, Failing: 2, Connected: true})
	assert.Equal(t, []screens.MapStepDTO{
		{ID: screens.StepDecide, Done: true},
		{ID: screens.StepCheck, Done: true},
		{ID: screens.StepFix, Count: 2},
		{ID: screens.StepConnect, Done: true},
		{ID: screens.StepShare},
	}, steps)
}

// Once decide, connect and share are done, the project is set up and draws
// no steps, whatever the last run found.
func TestMapStepsEndWhenTheProjectIsSetUp(t *testing.T) {
	assert.Empty(t, screens.MapSteps(screens.MapStepState{Connected: true, Shared: true}))
	assert.Empty(t, screens.MapSteps(screens.MapStepState{Checked: true, Failing: 3, Connected: true, Shared: true}))
}
