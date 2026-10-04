//go:build e2e

package source

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	coreproj "github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/venue"
	bowrainconn "github.com/neokapi/neokapi/core/venue/connector"
	apiclient "github.com/neokapi/neokapi/host/venue/client"
)

// These tests drive a checkout against a live Bowrain server, worker and
// PostgreSQL (the bowrain e2e stack at BOWRAIN_URL): the real change service,
// a real flow, and BowrainSourceConnector's push and pull. The ARB catalog
// declares en and the recipe names en-US, so every basis the checkout records
// is taken under another key than the one the server stamps.

func e2eServerURL() string {
	if u := os.Getenv("BOWRAIN_URL"); u != "" {
		return strings.TrimRight(u, "/")
	}
	return "http://localhost:8080"
}

// e2eSession is a person signed in to the live server, with a project of
// their own written in en-US and translated into French.
type e2eSession struct {
	url, token, ws, pid string
}

func newE2ESession(t *testing.T) e2eSession {
	t.Helper()
	s := e2eSession{url: e2eServerURL()}
	s.token = s.signIn(t)
	s.ws = fmt.Sprintf("basis-e2e-%d", time.Now().UnixNano())
	resp := s.call(t, http.MethodPost, "/api/v1/workspaces", fmt.Sprintf(`{"name":"Basis E2E","slug":%q}`, s.ws))
	require.Equal(t, http.StatusCreated, resp.code, resp.body)
	resp = s.call(t, http.MethodPost, "/api/v1/"+s.ws+"/projects",
		`{"name":"Declared language","default_source_language":"en-US","target_languages":["fr"]}`)
	require.Equal(t, http.StatusCreated, resp.code, resp.body)
	var created struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.Unmarshal([]byte(resp.body), &created))
	s.pid = created.ID
	return s
}

// signIn runs the server's device sign-in for the e2e stack's test user.
func (s e2eSession) signIn(t *testing.T) string {
	t.Helper()
	start := s.form(t, "/api/v1/auth/device/start", url.Values{"client_id": {"e2e-test"}})
	require.Equal(t, http.StatusOK, start.code, start.body)
	var codes struct {
		DeviceCode string `json:"device_code"`
		UserCode   string `json:"user_code"`
	}
	require.NoError(t, json.Unmarshal([]byte(start.body), &codes))
	s.form(t, "/api/v1/auth/device/verify", url.Values{
		"user_code": {codes.UserCode}, "email": {"admin@example.com"}, "name": {"Admin User"},
	})
	for range 10 {
		poll := s.form(t, "/api/v1/auth/device/poll", url.Values{
			"device_code": {codes.DeviceCode}, "grant_type": {"urn:ietf:params:oauth:grant-type:device_code"},
		})
		if poll.code == http.StatusOK {
			var tok struct {
				AccessToken string `json:"access_token"`
			}
			require.NoError(t, json.Unmarshal([]byte(poll.body), &tok))
			return tok.AccessToken
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatal("the device sign-in issued no token")
	return ""
}

type e2eResponse struct {
	code int
	body string
}

func (s e2eSession) form(t *testing.T, path string, values url.Values) e2eResponse {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, s.url+path, strings.NewReader(values.Encode()))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return s.do(t, req)
}

func (s e2eSession) call(t *testing.T, method, path, body string) e2eResponse {
	t.Helper()
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	req, err := http.NewRequestWithContext(t.Context(), method, s.url+path, r)
	require.NoError(t, err)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if s.token != "" {
		req.Header.Set("Authorization", "Bearer "+s.token)
	}
	return s.do(t, req)
}

func (s e2eSession) do(t *testing.T, req *http.Request) e2eResponse {
	t.Helper()
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return e2eResponse{code: resp.StatusCode, body: string(body)}
}

// checkout is a checkout of the session's project holding an ARB catalog
// that declares en, with a placeholder, so a translation carries a code.
func (s e2eSession) checkout(t *testing.T) *BowrainSourceConnector {
	t.Helper()
	return checkoutAt(t, s.url, "/"+s.ws+"/"+s.pid, s.pid, s.token, "en-US", map[string]string{
		arbSource: `{"@@locale": "en", "greeting": "Hello {name}, read the guide", ` +
			`"@greeting": {"placeholders": {"name": {}}}, "farewell": "Goodbye now"}` + "\n",
	}, coreproj.Collection{Name: "app", Path: arbSource, Target: "l10n/app_{lang}.arb"})
}

// seed stores the checkout's source on the server as a client that resolves
// no identity sends it, so the server files each unit under the format's name
// (greeting, farewell). The checkout's ledger keys its decisions by that name,
// and a push resolves its blocks against the units the server holds, so the
// decisions these tests push land on the units they name. The same push turns
// the server's own convergence off, so no draft the server makes on its own
// lands between two steps of a test.
func (s e2eSession) seed(t *testing.T, c *BowrainSourceConnector) {
	t.Helper()
	ctx := context.Background()
	scan, err := c.scanLocal(ctx, nil)
	require.NoError(t, err)
	format := c.detectFormat(filepath.Join(c.project.Root, filepath.FromSlash(arbSource)))
	settings := apiclient.NewPushContext(nil)
	settings.Settings = venue.ProjectSettings{venue.SettingConvergePolicy: "manual"}
	resp, err := c.client.Push(ctx, scan.blocks, []apiclient.ItemMeta{{Name: arbSource, Format: format}},
		settings, nil, apiclient.TransferUnder("en-US"))
	require.NoError(t, err)
	s.settle(t, c, resp.PushID)
}

// settle waits until the worker has applied the push pushID.
func (s e2eSession) settle(t *testing.T, c *BowrainSourceConnector, pushID string) {
	t.Helper()
	if pushID == "" || pushID == apiclient.PushUnchanged {
		return
	}
	for range 60 {
		st, err := c.client.PushStatus(context.Background(), pushID)
		if err == nil && (st.Status == "completed" || st.Status == "failed") {
			require.Equal(t, "completed", st.Status, "the worker applied push %s", pushID)
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("push %s was not applied", pushID)
}

// push pushes the checkout and requires the server to have applied it and
// accepted every verdict it carried.
func (s e2eSession) push(t *testing.T, c *BowrainSourceConnector) {
	t.Helper()
	res, err := c.Push(context.Background(), bowrainconn.PushOptions{})
	require.NoError(t, err)
	s.settle(t, c, res.PushID)
	require.Nil(t, res.Governance, "the server accepted every verdict the push carried")
}

// platformUnit is how the server grades the French translation of key: the
// record it holds, the source revision it stamped, the translation it holds
// and the rung that projects, the review context's stale flag, and the
// dashboard's stale count for French.
type platformUnit struct {
	record      venue.UnitDecision
	translation string
	rung        model.TargetStatus
	flagged     bool
	staleCount  int
}

func (s e2eSession) grade(t *testing.T, c *BowrainSourceConnector, key string) platformUnit {
	t.Helper()
	var u platformUnit
	pr, err := c.client.Pull(context.Background(), 0, nil, 0)
	require.NoError(t, err)
	var bid string
	for _, sb := range pr.Blocks {
		if sb.ItemName != arbSource || sb.Name != key {
			continue
		}
		require.Equal(t, key, sb.Unit, "the server files the unit under the name the checkout's ledger uses")
		bid = sb.ID
		b := apiclient.SyncBlockToBlock(sb)
		u.translation = model.TargetRevision(b, "fr")
		if fr, ok := b.TargetEdition("fr"); ok {
			u.rung = model.TargetStatus(fr.Status)
		}
	}
	require.NotEmpty(t, bid, "the server holds %s", key)
	for _, d := range pr.Decisions {
		if d.ItemName == arbSource && d.Unit == key && d.Variant == "fr" {
			u.record = d
		}
	}

	resp := s.call(t, http.MethodGet, "/api/v1/"+s.ws+"/"+s.pid+"/blocks/main/"+bid+"/review-context?target_locale=fr", "")
	require.Equal(t, http.StatusOK, resp.code, resp.body)
	var review struct {
		Provenance struct {
			ReviewState string `json:"review_state"`
			Stale       bool   `json:"stale"`
		} `json:"provenance"`
	}
	require.NoError(t, json.Unmarshal([]byte(resp.body), &review))
	require.Equal(t, u.record.ReviewState, review.Provenance.ReviewState, "review reads the record the pull carries")
	u.flagged = review.Provenance.Stale

	resp = s.call(t, http.MethodGet, "/api/v1/"+s.ws+"/"+s.pid+"/dashboard/main", "")
	require.Equal(t, http.StatusOK, resp.code, resp.body)
	var stats struct {
		LocaleStats []struct {
			Locale      string `json:"locale"`
			StaleBlocks int    `json:"stale_blocks"`
		} `json:"locale_stats"`
	}
	require.NoError(t, json.Unmarshal([]byte(resp.body), &stats))
	for _, ls := range stats.LocaleStats {
		if ls.Locale == "fr" {
			u.staleCount = ls.StaleBlocks
		}
	}
	return u
}

// A flow on the checkout translates the catalog, and a person approves one
// translation through the change service. Both took the source under en, the
// language the file declares. The push sends the approval with its basis as
// the server's revision of that source, so the server grades it current: the
// review context does not flag it and the dashboard counts nothing stale. The
// translation itself stays in the checkout's file, which a push does not send.
func TestRevisionE2E_ACheckoutApprovalOfADeclaredLanguageFileIsCurrentOnTheServer(t *testing.T) {
	s := newE2ESession(t)
	c := s.checkout(t)
	s.seed(t, c)
	up(t, c)
	record := approveFrench(t, c, arbSource, "greeting")
	b := scannedSource(t, c, arbSource, "greeting")
	require.NotEqual(t, venue.SourceRevision(b, "en-US"), record.Basis,
		"the checkout took the basis under the language the file declares")

	s.push(t, c)

	got := s.grade(t, c, "greeting")
	assert.Equal(t, venue.ReviewStateApproved, got.record.ReviewState)
	assert.Equal(t, record.Revision, got.record.Revision)
	assert.Equal(t, venue.SourceRevision(b, "en-US"), got.record.Basis, "the server holds the basis as it stamps the source")
	assert.False(t, got.flagged, "review reads the approval's source as current")
	assert.Zero(t, got.staleCount, "the dashboard counts nothing stale")
}

// The server drafts a translation, the checkout pulls it into its file, reads
// it back, and a person approves it there. The translation the checkout read
// from its file is the one the server holds, its placeholder included, so the
// approval the push carries lands on it and projects.
func TestRevisionE2E_AServerDraftApprovedOnACheckoutIsCurrentOnTheServer(t *testing.T) {
	s := newE2ESession(t)
	c := s.checkout(t)
	s.seed(t, c)
	up(t, c)

	resp := s.call(t, http.MethodPost, "/api/v1/"+s.ws+"/"+s.pid+"/actions/main/pseudo-translate?item="+url.QueryEscape(arbSource),
		`{"target_locale":"fr"}`)
	require.Equal(t, http.StatusOK, resp.code, resp.body)
	drafted := s.grade(t, c, "greeting")
	require.Equal(t, model.TargetStatusDraft, drafted.rung, "the server holds its draft")

	_, err := c.Pull(context.Background(), bowrainconn.PullOptions{})
	require.NoError(t, err)
	record := approveFrench(t, c, arbSource, "greeting")
	assert.Equal(t, drafted.translation, record.Revision,
		"the translation the checkout read back from its file is the one the server holds")

	s.push(t, c)

	got := s.grade(t, c, "greeting")
	assert.Equal(t, venue.ReviewStateApproved, got.record.ReviewState)
	assert.Equal(t, venue.SourceRevision(scannedSource(t, c, arbSource, "greeting"), "en-US"), got.record.Basis)
	assert.Equal(t, model.TargetStatusEstablished, got.rung, "the server projects the approval onto its draft")
	assert.False(t, got.flagged)
	assert.Zero(t, got.staleCount)
}
