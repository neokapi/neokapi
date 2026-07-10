package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateWorkerDBURL(t *testing.T) {
	tests := []struct {
		name        string
		dbURL       string
		insecureDev bool
		wantErr     string
	}{
		{
			name:    "missing without escape hatch is fatal",
			dbURL:   "",
			wantErr: "BOWRAIN_DATABASE_URL is required",
		},
		{
			name:        "missing with escape hatch is allowed",
			dbURL:       "",
			insecureDev: true,
		},
		{
			name:  "valid postgres url",
			dbURL: "postgres://u:p@localhost:5432/bowrain?sslmode=disable",
		},
		{
			name:  "valid postgresql url",
			dbURL: "postgresql://u:p@localhost:5432/bowrain",
		},
		{
			name:    "malformed scheme is always fatal",
			dbURL:   "mysql://u:p@localhost/db",
			wantErr: "must start with postgres://",
		},
		{
			name:        "malformed scheme stays fatal even in insecure-dev",
			dbURL:       "sqlite:///data/bowrain.db",
			insecureDev: true,
			wantErr:     "must start with postgres://",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateWorkerDBURL(tt.dbURL, tt.insecureDev)
			if tt.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}
