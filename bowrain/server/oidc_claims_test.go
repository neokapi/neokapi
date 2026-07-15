package server

import (
	"encoding/json"
	"errors"
	"testing"
)

// TestFlexBoolUnmarshal covers the encodings different IdPs use for
// email_verified: Keycloak emits a JSON bool, some Cognito variants emit a
// string. Absent/null/garbage must be fail-safe false (unverified).
func TestFlexBoolUnmarshal(t *testing.T) {
	cases := []struct {
		name string
		json string
		want bool
	}{
		{"bool true", `{"email_verified": true}`, true},
		{"bool false", `{"email_verified": false}`, false},
		{"string true", `{"email_verified": "true"}`, true},
		{"string false", `{"email_verified": "false"}`, false},
		{"absent", `{}`, false},
		{"null", `{"email_verified": null}`, false},
		{"one", `{"email_verified": "1"}`, true},
		{"zero", `{"email_verified": "0"}`, false},
		{"garbage string", `{"email_verified": "yes-please"}`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var claims oidcIdentityClaims
			if err := json.Unmarshal([]byte(tc.json), &claims); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if got := bool(claims.EmailVerified); got != tc.want {
				t.Errorf("EmailVerified = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestEnforceVerified covers the verified-email gate matrix. The fail-safe cell
// is (unverified, required) → reject, which blocks account creation AND
// linking-by-email with an unverified address.
func TestEnforceVerified(t *testing.T) {
	cases := []struct {
		name            string
		verified        bool
		requireVerified bool
		wantErr         bool
	}{
		{"verified + required", true, true, false},
		{"unverified + required", false, true, true},
		{"unverified + not required", false, false, false},
		{"verified + not required", true, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			claims := oidcIdentityClaims{Email: "a@b.com", EmailVerified: flexBool(tc.verified)}
			err := enforceVerified(claims, tc.requireVerified)
			if tc.wantErr {
				if !errors.Is(err, errEmailNotVerified) {
					t.Errorf("want errEmailNotVerified, got %v", err)
				}
			} else if err != nil {
				t.Errorf("want nil, got %v", err)
			}
		})
	}
}
