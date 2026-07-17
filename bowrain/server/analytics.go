package server

import (
	"time"

	"github.com/neokapi/neokapi/bowrain/analytics"
)

// trackEvent captures a PostHog product analytics event if the client is configured.
func (s *Server) trackEvent(userID, event string, properties map[string]any) {
	if s.PostHogClient == nil {
		return
	}
	s.PostHogClient.CaptureEvent(userID, event, properties)
}

// trackUserLogin captures a login or signup event.
// If the user was created within the last 10 seconds, it's treated as a signup.
func (s *Server) trackUserLogin(userID, email string, createdAt time.Time) {
	if s.PostHogClient == nil {
		return
	}

	isNew := time.Since(createdAt) < 10*time.Second
	if isNew {
		s.PostHogClient.Identify(userID, map[string]any{
			"email": email,
		})
		s.trackEvent(userID, analytics.EventUserSignup, map[string]any{
			"email": email,
		})
	} else {
		s.trackEvent(userID, analytics.EventUserLogin, map[string]any{
			"email": email,
		})
	}
}
