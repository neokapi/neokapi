package backend

// The Share page's preview of a connected project, read from the Bowrain
// server the recipe's `bowrain:` block names: the server's own Map, Reader,
// questions and rules in force, through the sign-in `kapi auth login` keeps.
// A refusal comes back as the server worded it, beside the one action that
// answers it.

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/neokapi/neokapi/host/screens"
	apiclient "github.com/neokapi/neokapi/host/venue/client"
	"github.com/neokapi/neokapi/host/venue/config"
	"github.com/neokapi/neokapi/host/venue/schema"
)

// The parts of a project BowrainPreview reads.
const (
	PreviewMap    = "map"
	PreviewReader = "reader"
	PreviewDecide = "decide"
	PreviewKept   = "kept"
)

// The actions a refusal names.
const (
	// PreviewActionSignIn answers a server that does not take the sign-in,
	// or a server no sign-in on this Mac names.
	PreviewActionSignIn = "sign_in"
	// PreviewActionOpen answers a refusal the person settles on the server:
	// no permission, or no such project.
	PreviewActionOpen = "open"
)

// BowrainPreviewAnswer is one part of the project as the server shows it,
// or the server's refusal. Exactly one of the part fields is set when
// Refusal is nil.
type BowrainPreviewAnswer struct {
	Map     *screens.MapSummaryDTO      `json:"map,omitempty"`
	Reader  *screens.ReaderFileDTO      `json:"reader,omitempty"`
	Decide  *screens.RulesDecideViewDTO `json:"decide,omitempty"`
	Kept    []screens.KeptRuleDTO       `json:"kept,omitempty"`
	Refusal *PreviewRefusal             `json:"refusal,omitempty"`
}

// PreviewRefusal is why the server, or this Mac, read nothing.
type PreviewRefusal struct {
	// Status is the server's HTTP status, 0 when no request was sent.
	Status int `json:"status"`
	// Message is the server's error, word for word. Empty when no request
	// was sent.
	Message string `json:"message,omitempty"`
	// Action is PreviewActionSignIn, PreviewActionOpen, or empty when no
	// action answers the refusal.
	Action string `json:"action,omitempty"`
}

// BowrainPreview reads one part of a tab's project from the Bowrain server
// its recipe connects to: part is "map", "reader" (item names the file),
// "decide" or "kept". The DTOs are the server's own. A tab whose recipe names
// no project in a workspace on a server is an error; a missing sign-in and a
// refusal from the server are a Refusal.
func (a *App) BowrainPreview(tabID, part, item string) (*BowrainPreviewAnswer, error) {
	op := a.getOpenProject(tabID)
	if op == nil {
		return nil, fmt.Errorf("project tab %q not found", tabID)
	}
	switch part {
	case PreviewMap, PreviewDecide, PreviewKept:
	case PreviewReader:
		if item == "" {
			return nil, errors.New("the reader part names a file")
		}
	default:
		return nil, fmt.Errorf("%q is not a part of the preview (map, reader, decide or kept)", part)
	}
	if op.Project == nil {
		return nil, errors.New("this tab has no kapi.yaml to connect to Bowrain")
	}
	venue, ok := op.Project.Venue()
	if !ok || venue.URL == "" {
		return nil, errors.New("kapi.yaml connects to no Bowrain server")
	}
	server, ws, pid, err := previewProject(venue.URL)
	if err != nil {
		return nil, err
	}
	t, refusal := a.bowrain.transport(server)
	if refusal != nil {
		return &BowrainPreviewAnswer{Refusal: refusal}, nil
	}
	c := apiclient.NewWorkspaceClientOn(t, ws, pid)
	c.SetStream(projectStream(op.Project))

	ctx, cancel := context.WithTimeout(context.Background(), sharingTimeout)
	defer cancel()
	out := &BowrainPreviewAnswer{}
	switch part {
	case PreviewMap:
		out.Map, err = c.MapSummary(ctx)
	case PreviewReader:
		out.Reader, err = c.ReaderFile(ctx, item)
	case PreviewDecide:
		out.Decide, err = c.RulesDecideView(ctx)
	case PreviewKept:
		out.Kept, err = c.KeptRules(ctx)
	}
	if err != nil {
		refusal := previewRefusal(err)
		if refusal.Action == PreviewActionSignIn {
			// The sign-in no longer works; the next read takes the one
			// `kapi auth login` stores next.
			a.bowrain.forget(server, t)
		}
		return &BowrainPreviewAnswer{Refusal: refusal}, nil
	}
	return out, nil
}

// previewProject splits the recipe's project URL into the server, the
// workspace and the project. The screen routes sit under a workspace, and
// every request carries the sign-in, so an http:// server is taken for this
// machine alone.
func previewProject(raw string) (server, ws, pid string, err error) {
	info := schema.ParseProjectURL(raw)
	u, perr := url.Parse(info.ServerURL)
	if perr != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", "", "", fmt.Errorf("%q is not a Bowrain project URL", raw)
	}
	if u.Scheme == "http" && !loopbackHost(u.Hostname()) {
		return "", "", "", fmt.Errorf("%s would send the sign-in unencrypted: give bowrain.url an https:// address", raw)
	}
	if info.Workspace == "" || info.ProjectID == "" {
		return "", "", "", fmt.Errorf("%s names no project in a workspace", raw)
	}
	return config.NormalizeServerURL(info.ServerURL), info.Workspace, info.ProjectID, nil
}

// loopbackHost reports whether host names this machine.
func loopbackHost(host string) bool {
	switch host {
	case "localhost", "127.0.0.1", "::1":
		return true
	}
	return false
}

// bowrainSignIns holds one transport per server, built from the stored
// sign-in at the first read. The preview reads several parts at once, and
// they share the transport so an expired access token is refreshed once and
// the keychain is read once per server.
type bowrainSignIns struct {
	mu         sync.Mutex
	transports map[string]*apiclient.Transport
}

// transport answers the server's transport, built with the stored sign-in on
// first use. The sign-in goes only to the server it names: the recipe names
// the server, and anyone who commits to the project writes the recipe.
func (b *bowrainSignIns) transport(server string) (*apiclient.Transport, *PreviewRefusal) {
	b.mu.Lock()
	defer b.mu.Unlock()
	// The metadata names the server without a keychain read, so a project on
	// another server prompts for nothing, and a sign-in removed by `kapi auth
	// logout` ends the cached one.
	if stored, _, ok := config.StoredSignIn(); !ok || stored != server {
		delete(b.transports, server)
		return nil, &PreviewRefusal{Action: PreviewActionSignIn}
	}
	if t, ok := b.transports[server]; ok {
		return t, nil
	}
	auth, ok := loadSignIn(server)
	if !ok {
		return nil, &PreviewRefusal{Action: PreviewActionSignIn}
	}
	t := apiclient.NewTransport(server, auth.AccessToken)
	t.RefuseRedirectsOffServer()
	// kapi refreshes the same sign-in in other processes, so each refresh
	// starts from the pair the keychain holds then.
	t.SetTokenSource(func() (string, string, bool) {
		if now, ok := loadSignIn(server); ok {
			return now.AccessToken, now.RefreshToken, true
		}
		return "", "", false
	})
	if auth.RefreshToken != "" {
		t.SetRefreshToken(auth.RefreshToken, func(newAccess, newRefresh string) {
			saved := *auth
			saved.AccessToken, saved.RefreshToken = newAccess, newRefresh
			_ = config.SaveAuth(saved)
		})
	}
	if b.transports == nil {
		b.transports = map[string]*apiclient.Transport{}
	}
	b.transports[server] = t
	return t, nil
}

// loadSignIn reads the stored sign-in from the keychain when it names server.
func loadSignIn(server string) (*config.StoredAuth, bool) {
	auth, err := config.LoadAuth()
	if err != nil || config.NormalizeServerURL(auth.ServerURL) != server {
		return nil, false
	}
	return auth, true
}

// forget drops the server's transport when it is still t.
func (b *bowrainSignIns) forget(server string, t *apiclient.Transport) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.transports[server] == t {
		delete(b.transports, server)
	}
}

// previewRefusal is the server's error as the server worded it, with the
// action that answers its status. A refresh of the sign-in the server
// refused asks for a sign-in and carries no message; one that reached no
// server keeps its own words. Any other error that carries no
// status, such as a server that cannot be reached, keeps its own words and
// names no action.
func previewRefusal(err error) *PreviewRefusal {
	if re, ok := errors.AsType[*apiclient.RefreshError](err); ok {
		// A refresh that reached no server leaves the sign-in as it was.
		if re.StatusCode == 0 {
			return &PreviewRefusal{Message: re.Error()}
		}
		return &PreviewRefusal{Status: re.StatusCode, Action: PreviewActionSignIn}
	}
	var se *apiclient.StatusError
	if !errors.As(err, &se) {
		return &PreviewRefusal{Message: err.Error()}
	}
	msg := se.Message
	if msg == "" {
		msg = se.Code
	}
	if msg == "" {
		msg = plainBody(se.Body)
	}
	if msg == "" {
		msg = http.StatusText(se.StatusCode)
	}
	out := &PreviewRefusal{Status: se.StatusCode, Message: msg}
	switch se.StatusCode {
	case http.StatusUnauthorized:
		out.Action = PreviewActionSignIn
	case http.StatusForbidden, http.StatusNotFound:
		out.Action = PreviewActionOpen
	}
	return out
}

// maxPlainBody bounds a response body read as the server's words.
const maxPlainBody = 200

// plainBody is a body short enough to read as a sentence and holding no
// markup, such as a proxy's HTML error page; empty otherwise.
func plainBody(body string) string {
	body = strings.TrimSpace(body)
	if len(body) > maxPlainBody || strings.Contains(body, "<") {
		return ""
	}
	return body
}
