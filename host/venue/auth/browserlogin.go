package auth

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// The browser sign-in: authorization code with PKCE and a loopback redirect
// (RFC 8252). The server brokers the identity provider, so the flow runs
// against its native-client routes:
//
//	GET  <server>/api/v1/auth/desktop/login?redirect_uri=http://127.0.0.1:<port>/callback
//	     &code_challenge&code_challenge_method=S256&state&response_mode=code
//	→ identity provider → <server>/api/v1/auth/desktop/callback
//	→ 302 http://127.0.0.1:<port>/callback?code=<one-time code>&state=<state>
//	POST <server>/api/v1/auth/desktop/token {code, code_verifier, redirect_uri}
//
// The redirect carries a one-time code, never a token, and only the holder of
// the PKCE verifier can redeem it.

// browserLoginTimeout bounds the wait for the person to finish in the browser.
const browserLoginTimeout = 5 * time.Minute

// LoginOptions selects how a login reaches the person.
type LoginOptions struct {
	// Progress receives the lines a person acts on: the URL to open when the
	// browser cannot be opened, and the device code in the device flow.
	Progress io.Writer
	// DeviceCode runs the device authorization grant: a code entered on any
	// device, for a machine with no browser of its own.
	DeviceCode bool
	// NoBrowser prints the sign-in URL instead of opening it. The redirect
	// still comes back to this machine, so the URL has to be opened here.
	NoBrowser bool
	// OpenBrowser opens a URL. Nil opens the system browser; tests replace it.
	OpenBrowser func(ctx context.Context, url string) error
	// HTTPClient makes the token request. Nil uses http.DefaultClient.
	HTTPClient *http.Client
}

// browserTokens runs the browser sign-in and returns the tokens.
func browserTokens(ctx context.Context, serverURL string, opts LoginOptions) (*TokenResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, browserLoginTimeout)
	defer cancel()

	verifier, err := GenerateCodeVerifier()
	if err != nil {
		return nil, err
	}
	state, err := randomState()
	if err != nil {
		return nil, err
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("listen for the sign-in redirect: %w", err)
	}
	redirectURI := fmt.Sprintf("http://127.0.0.1:%d/callback", ln.Addr().(*net.TCPAddr).Port)

	codes := make(chan callbackResult, 1)
	srv := &http.Server{
		Handler:           callbackHandler(state, codes),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() { _ = srv.Serve(ln) }()
	defer func() {
		shutdownCtx, done := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		defer done()
		_ = srv.Shutdown(shutdownCtx)
	}()

	signIn := strings.TrimRight(serverURL, "/") + "/api/v1/auth/desktop/login?" + url.Values{
		"redirect_uri":          {redirectURI},
		"code_challenge":        {ComputeCodeChallenge(verifier)},
		"code_challenge_method": {"S256"},
		"state":                 {state},
		"response_mode":         {"code"},
	}.Encode()

	open := opts.OpenBrowser
	if open == nil {
		open = openSystemBrowser
	}
	switch {
	case opts.NoBrowser:
		fmt.Fprintf(opts.Progress, "Open this URL in a browser on this machine to sign in:\n\n  %s\n\n", signIn)
	default:
		fmt.Fprintf(opts.Progress, "Opening your browser to sign in to %s.\n", serverURL)
		if err := open(ctx, signIn); err != nil {
			fmt.Fprintf(opts.Progress, "The browser could not be opened (%v). Open this URL on this machine:\n\n  %s\n\n", err, signIn)
		} else {
			fmt.Fprintf(opts.Progress, "If it does not open, visit:\n\n  %s\n\n", signIn)
		}
	}
	fmt.Fprintln(opts.Progress, "Waiting for the sign-in to finish in the browser...")

	var res callbackResult
	select {
	case res = <-codes:
	case <-ctx.Done():
		return nil, fmt.Errorf("the sign-in did not finish within %s; run the login again, or use --use-device-code on a machine without a browser", browserLoginTimeout)
	}
	if res.err != nil {
		return nil, res.err
	}
	return redeemCode(ctx, opts.HTTPClient, serverURL, res.code, verifier, redirectURI)
}

type callbackResult struct {
	code string
	err  error
}

// callbackHandler receives the redirect. Only a request carrying the state
// this login sent ends the wait; anything else is answered and ignored, so a
// stray request to the port cannot end or steer the login.
func callbackHandler(state string, out chan<- callbackResult) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("state") != state {
			writeCallbackPage(w, http.StatusBadRequest, "This sign-in link does not belong to the login waiting in your terminal. Start the login again.")
			return
		}
		var res callbackResult
		switch {
		case q.Get("error") != "":
			msg := q.Get("error")
			if d := q.Get("error_description"); d != "" {
				msg += ": " + d
			}
			res.err = fmt.Errorf("the sign-in was not completed: %s", msg)
			writeCallbackPage(w, http.StatusBadRequest, "The sign-in was not completed. Return to the terminal for details.")
		case q.Get("code") == "":
			res.err = errors.New("the sign-in redirect carried no code")
			writeCallbackPage(w, http.StatusBadRequest, "The sign-in redirect carried no code. Start the login again.")
		default:
			res.code = q.Get("code")
			writeCallbackPage(w, http.StatusOK, "You are signed in. You can close this tab and return to the terminal.")
		}
		select {
		case out <- res:
		default:
		}
	})
	return mux
}

func writeCallbackPage(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'")
	w.WriteHeader(status)
	fmt.Fprintf(w, `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>kapi sign-in</title>`+
		`<style>body{font:16px system-ui,sans-serif;margin:15vh auto;max-width:32rem;padding:0 1rem;color:#222}`+
		`@media(prefers-color-scheme:dark){body{background:#111;color:#eee}}</style></head>`+
		`<body><p>%s</p></body></html>`, html.EscapeString(message))
}

// redeemCode exchanges the one-time code and the PKCE verifier for tokens.
func redeemCode(ctx context.Context, client *http.Client, serverURL, code, verifier, redirectURI string) (*TokenResponse, error) {
	if client == nil {
		client = http.DefaultClient
	}
	body, err := json.Marshal(map[string]string{
		"code":          code,
		"code_verifier": verifier,
		"redirect_uri":  redirectURI,
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(serverURL, "/")+"/api/v1/auth/desktop/token", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("redeem the sign-in code: %w", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("redeem the sign-in code: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	var tok TokenResponse
	if err := json.Unmarshal(data, &tok); err != nil {
		return nil, fmt.Errorf("redeem the sign-in code: %w", err)
	}
	if tok.AccessToken == "" {
		return nil, errors.New("redeem the sign-in code: the response carried no access token")
	}
	return &tok, nil
}

func randomState() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// CanOpenBrowser reports whether this session has a browser to open: not over
// SSH, not in CI, and on Linux only with a display.
func CanOpenBrowser() bool {
	for _, v := range []string{"SSH_CONNECTION", "SSH_TTY", "CI"} {
		if os.Getenv(v) != "" {
			return false
		}
	}
	if runtime.GOOS == "linux" && os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
		return false
	}
	return true
}

// openSystemBrowser opens url in the default browser.
func openSystemBrowser(ctx context.Context, target string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.CommandContext(ctx, "open", target)
	case "windows":
		cmd = exec.CommandContext(ctx, "rundll32", "url.dll,FileProtocolHandler", target)
	default:
		cmd = exec.CommandContext(ctx, "xdg-open", target)
	}
	return cmd.Run()
}
