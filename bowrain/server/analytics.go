package server

import (
	"net/url"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/neokapi/neokapi/bowrain/analytics"
)

// acquisitionCookie carries the channel a visitor arrived through — the
// campaign the landing stamped, or the site that referred them. The app writes
// it on first load; it is SameSite=Lax, so it rides the top-level GET back from
// the identity provider, which is the only request on which the server learns
// that a signup just happened.
const acquisitionCookie = "bowrain_acquisition"

// maxAcquisitionSource bounds the label. The cookie is written by a browser and
// can be written by anyone, so the property it feeds has to stay a label rather
// than become a free-text field in the analytics store.
const maxAcquisitionSource = 64

// trackEvent captures a PostHog product analytics event if the client is configured.
func (s *Server) trackEvent(userID, event string, properties map[string]any) {
	if s.PostHogClient == nil {
		return
	}
	s.PostHogClient.CaptureEvent(userID, event, properties)
}

// trackUserLogin captures a login or signup event.
// If the user was created within the last 10 seconds, it's treated as a signup.
func (s *Server) trackUserLogin(c echo.Context, userID, email string, createdAt time.Time) {
	if s.PostHogClient == nil {
		return
	}

	if time.Since(createdAt) >= 10*time.Second {
		s.trackEvent(userID, analytics.EventUserLogin, map[string]any{
			"email": email,
		})
		return
	}

	s.PostHogClient.Identify(userID, map[string]any{
		"email": email,
	})
	props := map[string]any{"email": email}
	if src := acquisitionSource(c); src != "" {
		props[analytics.PropAcquisitionSource] = src
	}
	s.trackEvent(userID, analytics.EventUserSignup, props)
}

// acquisitionSource reads the acquisition label off the request and normalizes
// it to the shape an analytics property can be grouped on: lowercase, and only
// the characters a campaign name or a host is made of. A value outside that
// shape is dropped rather than repaired — a label that had to be cleaned up is
// not one to group by.
func acquisitionSource(c echo.Context) string {
	if c == nil {
		return ""
	}
	ck, err := c.Cookie(acquisitionCookie)
	if err != nil || ck == nil {
		return ""
	}
	raw, err := url.QueryUnescape(ck.Value)
	if err != nil {
		return ""
	}
	raw = strings.ToLower(strings.TrimSpace(raw))
	if raw == "" || len(raw) > maxAcquisitionSource {
		return ""
	}
	for _, r := range raw {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '.', r == '-', r == '_':
		default:
			return ""
		}
	}
	return raw
}
