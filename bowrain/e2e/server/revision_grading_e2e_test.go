//go:build e2e

package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/state"
	"github.com/neokapi/neokapi/core/venue"
	apiclient "github.com/neokapi/neokapi/host/venue/client"
)

// TestRevisionGradingE2E drives a checkout's push against a live server and
// worker: an approval the checkout made on a source with a link is current on
// the server, and a change to the link alone, pushed as a checkout pushes it,
// retires it there as it does on the checkout.
func TestRevisionGradingE2E(t *testing.T) {
	token := getTestToken(t)
	ctx := context.Background()

	wsSlug := fmt.Sprintf("revision-e2e-%d", time.Now().UnixMilli())
	resp := apiRequest(t, http.MethodPost, "/api/v1/workspaces", token, fmt.Sprintf(`{"name":"Revision E2E","slug":"%s"}`, wsSlug))
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	resp.Body.Close()
	resp = apiRequest(t, http.MethodPost, "/api/v1/"+wsSlug+"/projects", token,
		`{"name":"Revision grading","default_source_language":"en","target_languages":["nb"]}`)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	projectID := readJSON(t, resp)["id"].(string)
	client := apiclient.NewProjectBearerClient(serverURL, projectID, token)

	const item = "docs/guide.json"
	guide := func(href string) *model.Block {
		b := &model.Block{ID: "guide", Name: "guide", Translatable: true, SourceLocale: "en"}
		b.SetSourceRuns([]model.Run{
			model.TextR("Read "),
			model.PcOpenR(model.PcOpenRun{ID: "1", Type: "link", Data: `<a href="` + href + `">`}),
			model.TextR("the guide"),
			model.PcCloseR(model.PcCloseRun{ID: "1", Type: "link", Data: "</a>"}),
		})
		b.SetEdition(model.Variant("nb"), model.Edition{
			Runs: []model.Run{model.TextR("Les veiledningen")}, Status: model.Status(model.TargetStatusTranslated),
		})
		return b
	}
	push := func(b *model.Block, decisions []venue.UnitDecision) *apiclient.SyncPushResponse {
		t.Helper()
		pr, err := client.Push(ctx, map[string][]*model.Block{item: {b}},
			[]apiclient.ItemMeta{{Name: item, Format: "json"}}, nil, decisions, apiclient.TransferUnder("en"))
		require.NoError(t, err)
		if pr.PushID == apiclient.PushUnchanged {
			return pr
		}
		var status string
		for range 60 {
			time.Sleep(500 * time.Millisecond)
			st, err := client.PushStatus(ctx, pr.PushID)
			if err != nil {
				continue
			}
			status = st.Status
			if status == "completed" || status == "failed" {
				require.Nil(t, st.Governance, "the push's verdicts are accepted")
				break
			}
		}
		require.Equal(t, "completed", status, "the worker lands the push")
		return pr
	}
	pulled := func() (state.UnitState, model.TargetStatus) {
		t.Helper()
		pr, err := client.Pull(ctx, 0, nil, 0)
		require.NoError(t, err)
		var record state.UnitState
		for _, d := range pr.Decisions {
			if d.ItemName == item && d.Unit == "guide" && d.Variant == "nb" {
				record = state.UnitState{Unit: d.Unit, Variant: model.Variant("nb"), Status: model.TargetStatus(d.Status),
					Revision: d.Revision, Basis: d.Basis, Decision: state.Decision{ReviewState: d.ReviewState}}
			}
		}
		require.NotEmpty(t, record.Unit, "the pull carries the guide's record")
		var rung model.TargetStatus
		for _, sb := range pr.Blocks {
			if sb.ItemName == item {
				nb, ok := apiclient.SyncBlockToBlock(sb).Edition(model.Variant("nb"))
				require.True(t, ok)
				rung = model.TargetStatus(nb.Status)
			}
		}
		return record, rung
	}

	// The checkout approves the translation of the source linking to v1.
	held := guide("/v1/guide")
	read := state.ReadTarget(held, "nb", "en")
	now := time.Now().UTC().Format(time.RFC3339)
	push(held, []venue.UnitDecision{{
		ItemName: item, Unit: "guide", Variant: "nb", Status: string(model.TargetStatusEstablished),
		Revision: read.Revision, Basis: read.Basis, ReviewState: venue.ReviewStateApproved,
		DecidedAt: now, Updated: now,
	}})
	record, rung := pulled()
	assert.Equal(t, model.TargetStatusEstablished, rung, "the server projects the checkout's approval")
	assert.True(t, record.Fresh(state.ReadTarget(held, "nb", "en")), "and a checkout holding the source reads it current")

	// The link moves; nothing else does.
	moved := guide("/v2/guide")
	require.Equal(t, held.SourceText(), moved.SourceText())
	pr := push(moved, nil)
	assert.Equal(t, 1, pr.BlocksUploaded, "the push carries a change to an inline code alone")
	record, rung = pulled()
	assert.Equal(t, model.TargetStatusTranslated, rung, "the server retires the approval")
	assert.False(t, record.Fresh(state.ReadTarget(moved, "nb", "en")), "as the checkout does")

	resp = apiRequest(t, http.MethodGet, "/api/v1/"+wsSlug+"/"+projectID+"/dashboard/main", token, "")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	var stats struct {
		LocaleStats []struct {
			Locale      string `json:"locale"`
			StaleBlocks int    `json:"stale_blocks"`
		} `json:"locale_stats"`
	}
	require.NoError(t, json.Unmarshal(body, &stats))
	stale := -1
	for _, ls := range stats.LocaleStats {
		if ls.Locale == "nb" {
			stale = ls.StaleBlocks
		}
	}
	assert.Equal(t, 1, stale, "the dashboard counts the approval stale")
}
