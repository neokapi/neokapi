package backend

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// governance.go is the desktop's REST proxy for the bowrain-server governance
// surfaces that have no gRPC EditorService equivalent: workspace members and
// the brand correction-learning loop (AD-019). The desktop frontend reaches the
// server only through Wails bindings, and the keychain auth token is never
// exposed to it, so these *App methods do the authenticated HTTP calls here and
// return decoded JSON. Paths mirror bowrain/packages/ui/src/api/rest-adapter.ts.

// governanceTimeout bounds each proxied REST call.
const governanceTimeout = 15 * time.Second

// govRequest performs an authenticated JSON request against the connected
// bowrain server and decodes the response body into out (when out != nil).
// body, when non-nil, is JSON-encoded as the request payload. It returns
// errNotConnected when there is no active server connection or token.
func (a *App) govRequest(method, path string, body, out any) error {
	a.mu.RLock()
	serverURL := a.serverURL
	auth := a.authInfo
	connected := a.connState == StateConnected && a.remote != nil
	a.mu.RUnlock()

	if !connected {
		return errNotConnected
	}
	if auth == nil || auth.AccessToken == "" {
		return fmt.Errorf("no auth token available")
	}

	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode request: %w", err)
		}
		reqBody = bytes.NewReader(data)
	}

	ctx, cancel := context.WithTimeout(context.Background(), governanceTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, method, serverURL+path, reqBody)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+auth.AccessToken)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("%s %s: server returned %d: %s", method, path, resp.StatusCode, bytes.TrimSpace(b))
	}

	if out == nil {
		return nil
	}
	// Tolerate empty 200/204 bodies for endpoints that return nothing.
	dec := json.NewDecoder(resp.Body)
	if err := dec.Decode(out); err != nil && err != io.EOF {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

// --- Members ---

// MemberInfo mirrors the server's workspace Membership JSON.
type MemberInfo struct {
	UserID      string `json:"user_id"`
	WorkspaceID string `json:"workspace_id"`
	Role        string `json:"role"`
	User        struct {
		ID        string `json:"id"`
		Email     string `json:"email"`
		Name      string `json:"name"`
		AvatarURL string `json:"avatar_url"`
	} `json:"user"`
}

// ListMembers returns the members of a workspace.
func (a *App) ListMembers(workspaceSlug string) ([]MemberInfo, error) {
	var out []MemberInfo
	if err := a.govRequest(http.MethodGet, "/api/v1/"+url.PathEscape(workspaceSlug)+"/members", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// AddMember adds a user to a workspace with the given role.
func (a *App) AddMember(workspaceSlug, userID, role string) error {
	body := map[string]string{"user_id": userID, "role": role}
	return a.govRequest(http.MethodPost, "/api/v1/"+url.PathEscape(workspaceSlug)+"/members", body, nil)
}

// UpdateMemberRole changes a member's role.
func (a *App) UpdateMemberRole(workspaceSlug, userID, role string) error {
	body := map[string]string{"role": role}
	path := "/api/v1/" + url.PathEscape(workspaceSlug) + "/members/" + url.PathEscape(userID) + "/role"
	return a.govRequest(http.MethodPut, path, body, nil)
}

// RemoveMember removes a member from a workspace.
func (a *App) RemoveMember(workspaceSlug, userID string) error {
	path := "/api/v1/" + url.PathEscape(workspaceSlug) + "/members/" + url.PathEscape(userID)
	return a.govRequest(http.MethodDelete, path, nil, nil)
}

// --- Invites ---

// InviteInfo mirrors the server's Invite JSON.
type InviteInfo struct {
	ID        string `json:"id"`
	Code      string `json:"code"`
	Email     string `json:"email"`
	Role      string `json:"role"`
	MaxUses   int    `json:"max_uses"`
	UseCount  int    `json:"use_count"`
	ExpiresAt string `json:"expires_at"`
	CreatedAt string `json:"created_at"`
}

// ListInvites returns pending invitations for a workspace.
func (a *App) ListInvites(workspaceSlug string) ([]InviteInfo, error) {
	var out []InviteInfo
	if err := a.govRequest(http.MethodGet, "/api/v1/"+url.PathEscape(workspaceSlug)+"/invites", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// CreateInvite creates a workspace invitation.
func (a *App) CreateInvite(workspaceSlug, email, role string, maxUses int) (InviteInfo, error) {
	body := map[string]any{"email": email, "role": role, "max_uses": maxUses}
	var out InviteInfo
	if err := a.govRequest(http.MethodPost, "/api/v1/"+url.PathEscape(workspaceSlug)+"/invites", body, &out); err != nil {
		return InviteInfo{}, err
	}
	return out, nil
}

// DeleteInvite revokes a workspace invitation.
func (a *App) DeleteInvite(workspaceSlug, inviteID string) error {
	path := "/api/v1/" + url.PathEscape(workspaceSlug) + "/invites/" + url.PathEscape(inviteID)
	return a.govRequest(http.MethodDelete, path, nil, nil)
}

// --- Brand profiles (read) ---

// brandProfilesPath returns the workspace brand-profiles collection path.
func brandProfilesPath(ws string) string {
	return "/api/v1/" + url.PathEscape(ws) + "/brand-profiles"
}

// ListBrandProfiles returns the brand voice profiles for a workspace.
// The shape is opaque to the proxy; the frontend has the VoiceProfile type.
func (a *App) ListBrandProfiles(workspaceSlug string) (json.RawMessage, error) {
	var out json.RawMessage
	if err := a.govRequest(http.MethodGet, brandProfilesPath(workspaceSlug), nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetBrandProfile returns a single brand voice profile.
func (a *App) GetBrandProfile(workspaceSlug, profileID string) (json.RawMessage, error) {
	var out json.RawMessage
	path := brandProfilesPath(workspaceSlug) + "/" + url.PathEscape(profileID)
	if err := a.govRequest(http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetBrandScores returns stored brand-compliance scores for a project.
func (a *App) GetBrandScores(workspaceSlug, projectID string) (json.RawMessage, error) {
	var out json.RawMessage
	path := "/api/v1/" + url.PathEscape(workspaceSlug) + "/" + url.PathEscape(projectID) + "/brand-voice/main/scores"
	if err := a.govRequest(http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetBrandTrends returns brand-compliance score trends for a project.
func (a *App) GetBrandTrends(workspaceSlug, projectID string) (json.RawMessage, error) {
	var out json.RawMessage
	path := "/api/v1/" + url.PathEscape(workspaceSlug) + "/" + url.PathEscape(projectID) + "/brand-voice/main/trends"
	if err := a.govRequest(http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// --- Correction-learning loop (AD-019) ---

// CandidateRuleArgs is the body for promote/reject candidate-rule actions.
type CandidateRuleArgs struct {
	Term            string `json:"term"`
	Replacement     string `json:"replacement,omitempty"`
	CorrectionCount int    `json:"correction_count,omitempty"`
}

// EvaluateRuleArgs is the body for the blast-radius evaluation.
type EvaluateRuleArgs struct {
	Term        string `json:"term"`
	Replacement string `json:"replacement,omitempty"`
	ProjectID   string `json:"project_id"`
	Stream      string `json:"stream,omitempty"`
}

// GetSuggestedRules returns the candidate rules a profile's corrections have
// produced. minCount filters by correction count (0 = server default); all,
// when true, includes already-decided candidates (promoted/rejected) as history.
func (a *App) GetSuggestedRules(workspaceSlug, profileID string, minCount int, all bool) (json.RawMessage, error) {
	q := url.Values{}
	if minCount > 0 {
		q.Set("min_count", strconv.Itoa(minCount))
	}
	if all {
		q.Set("all", "true")
	}
	path := brandProfilesPath(workspaceSlug) + "/" + url.PathEscape(profileID) + "/candidates"
	if qs := q.Encode(); qs != "" {
		path += "?" + qs
	}
	var out json.RawMessage
	if err := a.govRequest(http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// PromoteRule hardens a candidate rule into an enforced check on the profile.
func (a *App) PromoteRule(workspaceSlug, profileID string, rule CandidateRuleArgs) (json.RawMessage, error) {
	path := brandProfilesPath(workspaceSlug) + "/" + url.PathEscape(profileID) + "/promote-rule"
	var out json.RawMessage
	if err := a.govRequest(http.MethodPost, path, rule, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// RejectRule rejects a candidate rule so it stops re-surfacing.
func (a *App) RejectRule(workspaceSlug, profileID string, rule CandidateRuleArgs) error {
	path := brandProfilesPath(workspaceSlug) + "/" + url.PathEscape(profileID) + "/reject-rule"
	return a.govRequest(http.MethodPost, path, rule, nil)
}

// EvaluateRule computes the blast radius of promoting a candidate rule across a
// project's content (how many blocks it newly flags / resolves, per collection).
func (a *App) EvaluateRule(workspaceSlug, profileID string, req EvaluateRuleArgs) (json.RawMessage, error) {
	path := brandProfilesPath(workspaceSlug) + "/" + url.PathEscape(profileID) + "/evaluate-rule"
	var out json.RawMessage
	if err := a.govRequest(http.MethodPost, path, req, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetBrandDrift returns the brand-compliance drift analysis for a project.
func (a *App) GetBrandDrift(workspaceSlug, projectID string, recentDays, minScore, dropPoints int) (json.RawMessage, error) {
	q := url.Values{}
	if recentDays > 0 {
		q.Set("recent_days", strconv.Itoa(recentDays))
	}
	if minScore > 0 {
		q.Set("min_score", strconv.Itoa(minScore))
	}
	if dropPoints > 0 {
		q.Set("drop_points", strconv.Itoa(dropPoints))
	}
	path := "/api/v1/" + url.PathEscape(workspaceSlug) + "/" + url.PathEscape(projectID) + "/brand-voice/main/drift"
	if qs := q.Encode(); qs != "" {
		path += "?" + qs
	}
	var out json.RawMessage
	if err := a.govRequest(http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// ListStarterPacks returns the available brand-voice starter packs.
func (a *App) ListStarterPacks() (json.RawMessage, error) {
	var out json.RawMessage
	if err := a.govRequest(http.MethodGet, "/api/v1/brand-voice/starter-packs", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}
