package server

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNextPlanLevel_SkipsALevelThatOnlyWaits(t *testing.T) {
	none := 0
	base := func() reviewPlanResponse {
		return reviewPlanResponse{
			Coordinates: planCoordinatesDTO{Confirmed: true},
			Context: planContextDTO{
				ScanWaiting:   scanWaitingQuota,
				QuotaResetsAt: "2026-12-01T00:00:00Z",
				Untranslated:  map[string]int{},
			},
			Exceptions: planExceptionsDTO{SourceFindings: &none},
			Languages:  []planLanguageDTO{{Locale: "nb", Waiting: 30, Pages: 4}},
			Policy:     planPolicyDTO{Settle: settlePolicy},
		}
	}
	quota := planWaitDTO{Level: planContext, Reason: scanWaitingQuota, QuotaResetsAt: "2026-12-01T00:00:00Z"}

	tests := []struct {
		name    string
		edit    func(*reviewPlanResponse)
		next    string
		waiting []planWaitDTO
	}{
		{
			name:    "a scan waiting for the AI limit is skipped for the next level",
			next:    planInterfaces,
			waiting: []planWaitDTO{quota},
		},
		{
			name: "every other level done leaves no next step and the reason",
			edit: func(p *reviewPlanResponse) {
				p.Languages = []planLanguageDTO{{Locale: "nb"}}
			},
			next:    "",
			waiting: []planWaitDTO{quota},
		},
		{
			name: "a running scan waits too",
			edit: func(p *reviewPlanResponse) {
				p.Context.ScanWaiting, p.Context.QuotaResetsAt = "", ""
				p.Context.Scan = &planScanDTO{Status: "processing"}
			},
			next:    planInterfaces,
			waiting: []planWaitDTO{{Level: planContext, Reason: "processing"}},
		},
		{
			name: "a scan that starts with the next push is the next step",
			edit: func(p *reviewPlanResponse) {
				p.Context.ScanWaiting, p.Context.QuotaResetsAt = scanWaitingPush, ""
			},
			next:    planContext,
			waiting: []planWaitDTO{},
		},
		{
			name: "a context card waiting is the next step whatever the scan waits for",
			edit: func(p *reviewPlanResponse) {
				p.Context.Waiting = 3
			},
			next:    planContext,
			waiting: []planWaitDTO{},
		},
		{
			name: "a missing term in a language not followed is not the caller's step",
			edit: func(p *reviewPlanResponse) {
				p.Context = planContextDTO{Concepts: 4, Untranslated: map[string]int{"de": 2}}
			},
			next:    planInterfaces,
			waiting: []planWaitDTO{},
		},
		{
			name: "unconfirmed coordinates come first",
			edit: func(p *reviewPlanResponse) {
				p.Coordinates.Confirmed = false
			},
			next:    planCoordinates,
			waiting: []planWaitDTO{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := base()
			if tt.edit != nil {
				tt.edit(&p)
			}
			next, waiting := nextPlanLevel(p)
			assert.Equal(t, tt.next, next)
			assert.Equal(t, tt.waiting, waiting)

			p.Next, p.Waiting = next, waiting
			brief := summarizePlan(p)
			assert.Equal(t, tt.next, brief.Next, "the plan in brief names the same next step")
			assert.Equal(t, tt.waiting, brief.Waiting)
		})
	}
}
