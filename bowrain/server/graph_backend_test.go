package server

import "testing"

// TestGraphBackendDefaultsToSQL locks the Epic 014 contract: the graph backend
// defaults to plain SQL (works on standard PostgreSQL/RDS) and only opts into
// Apache AGE when explicitly requested. The AGE ag_catalog AfterConnect hook is
// installed only for the AGE backend, so a stock/RDS Postgres works out of the
// box on the default.
func TestGraphBackendDefaultsToSQL(t *testing.T) {
	cases := []struct {
		name    string
		env     string
		set     bool
		want    string
		wantAGE bool // graphAfterConnect installs the AGE hook
	}{
		{name: "unset", set: false, want: graphBackendSQL, wantAGE: false},
		{name: "empty", env: "", set: true, want: graphBackendSQL, wantAGE: false},
		{name: "sql", env: "sql", set: true, want: graphBackendSQL, wantAGE: false},
		{name: "age", env: "age", set: true, want: graphBackendAGE, wantAGE: true},
		{name: "AGE upper", env: "AGE", set: true, want: graphBackendAGE, wantAGE: true},
		{name: "age padded", env: "  age  ", set: true, want: graphBackendAGE, wantAGE: true},
		{name: "unknown falls back to sql", env: "neo4j", set: true, want: graphBackendSQL, wantAGE: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.set {
				t.Setenv(graphBackendEnv, tc.env)
			} else {
				// Ensure a stray ambient value can't leak in.
				t.Setenv(graphBackendEnv, "")
			}

			if got := graphBackend(); got != tc.want {
				t.Errorf("graphBackend() = %q, want %q", got, tc.want)
			}

			hook := graphAfterConnect()
			if tc.wantAGE {
				if hook == nil {
					t.Errorf("graphAfterConnect() = nil, want the AGE ag_catalog hook")
				}
			} else if hook != nil {
				t.Error("graphAfterConnect() installed a hook for the SQL backend; standard Postgres/RDS must get none")
			}
		})
	}
}
