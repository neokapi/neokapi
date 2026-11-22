package auth

import (
	"context"
	"fmt"
	"io"
	"time"

	apiclient "github.com/neokapi/neokapi/host/venue/client"
	"github.com/neokapi/neokapi/host/venue/config"
)

// clientID identifies this client to the venue's device-authorization endpoint.
const clientID = "kapi-cli"

// deviceFlowTimeout bounds the whole grant, including the wait for the person
// to finish in their browser.
const deviceFlowTimeout = 5 * time.Minute

// PerformLogin signs in to serverURL and, on success, stores the credentials
// and returns them. On a machine with a browser it opens the browser sign-in
// (authorization code with PKCE and a loopback redirect); over SSH, in CI, or
// with opts.DeviceCode it runs the device authorization grant, whose code can
// be entered on any device.
//
// It reports the steps a person has to act on to opts.Progress, and returns
// rather than renders the result: what a successful login should print is the
// caller's decision, and the two callers differ (a bare login says so, an
// init folds it into the project it is scaffolding).
func PerformLogin(ctx context.Context, serverURL string, opts LoginOptions) (*config.StoredAuth, error) {
	if opts.Progress == nil {
		opts.Progress = io.Discard
	}
	var token *TokenResponse
	var err error
	if opts.DeviceCode || (!opts.NoBrowser && opts.OpenBrowser == nil && !CanOpenBrowser()) {
		token, err = deviceTokens(ctx, serverURL, opts.Progress)
	} else {
		token, err = browserTokens(ctx, serverURL, opts)
	}
	if err != nil {
		return nil, err
	}

	stored := config.StoredAuth{
		ServerURL:    serverURL,
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		Expiry:       time.Now().Add(time.Duration(token.ExpiresIn) * time.Second),
	}

	// Best-effort identity: a login that cannot name the user is still a login.
	if user, err := apiclient.FetchUser(ctx, serverURL, token.AccessToken); err == nil {
		stored.User = config.StoredUser{ID: user.ID, Email: user.Email, Name: user.Name}
	}

	if err := config.SaveAuth(stored); err != nil {
		return nil, fmt.Errorf("save token: %w", err)
	}
	return &stored, nil
}

// deviceTokens runs the device authorization grant and returns the tokens.
func deviceTokens(ctx context.Context, serverURL string, progress io.Writer) (*TokenResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, deviceFlowTimeout)
	defer cancel()

	client := &DeviceFlowClient{
		DeviceAuthURL: serverURL + "/api/v1/auth/device/start",
		TokenURL:      serverURL + "/api/v1/auth/device/poll",
		ClientID:      clientID,
	}

	fmt.Fprintln(progress, "Starting device authorization flow...")
	resp, err := client.StartDeviceAuth(ctx)
	if err != nil {
		return nil, fmt.Errorf("device auth start failed: %w", err)
	}

	fmt.Fprintf(progress, "\nOpen the following URL in a browser on any device:\n\n  %s\n\n", resp.VerificationURI)
	fmt.Fprintf(progress, "Enter code: %s\n\n", resp.UserCode)
	fmt.Fprintln(progress, "Waiting for authorization...")

	token, err := client.PollForToken(ctx, resp.DeviceCode, resp.Interval)
	if err != nil {
		return nil, fmt.Errorf("authorization failed: %w", err)
	}
	return token, nil
}
