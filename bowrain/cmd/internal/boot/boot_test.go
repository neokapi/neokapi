package boot

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidatePostgresURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr string
	}{
		{name: "empty", url: "", wantErr: "is required"},
		{name: "postgres scheme", url: "postgres://u:p@localhost:5432/db"},
		{name: "postgresql scheme", url: "postgresql://u:p@localhost:5432/db"},
		{name: "sqlite path", url: "/data/bowrain.db", wantErr: "must start with postgres://"},
		{name: "mysql scheme", url: "mysql://u:p@localhost/db", wantErr: "must start with postgres://"},
		{name: "typo scheme", url: "postgre://u:p@localhost/db", wantErr: "must start with postgres://"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePostgresURL("BOWRAIN_DATABASE_URL", tt.url)
			if tt.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), "BOWRAIN_DATABASE_URL")
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestMissingServerConfig(t *testing.T) {
	tests := []struct {
		name        string
		jwtSecret   string
		databaseURL string
		wantCount   int
	}{
		{name: "both set", jwtSecret: "s3cret", databaseURL: "postgres://localhost/db", wantCount: 0},
		{name: "missing jwt", jwtSecret: "", databaseURL: "postgres://localhost/db", wantCount: 1},
		{name: "missing db", jwtSecret: "s3cret", databaseURL: "", wantCount: 1},
		{name: "both missing", jwtSecret: "", databaseURL: "", wantCount: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			missing := MissingServerConfig(tt.jwtSecret, tt.databaseURL)
			require.Len(t, missing, tt.wantCount)
			if tt.jwtSecret == "" {
				assert.Contains(t, missing[0].Error(), "BOWRAIN_JWT_SECRET")
			}
			if tt.databaseURL == "" {
				assert.Contains(t, missing[len(missing)-1].Error(), "BOWRAIN_DATABASE_URL")
			}
		})
	}
}

func TestAllowInsecureDevFromEnv(t *testing.T) {
	tests := []struct {
		value string
		want  bool
	}{
		{"", false},
		{"0", false},
		{"false", false},
		{"no", false},
		{"nonsense", false},
		{"1", true},
		{"true", true},
		{"TRUE", true},
		{"yes", true},
		{"on", true},
		{" 1 ", true},
	}
	for _, tt := range tests {
		t.Run("value="+tt.value, func(t *testing.T) {
			t.Setenv(InsecureDevEnv, tt.value)
			assert.Equal(t, tt.want, AllowInsecureDevFromEnv())
		})
	}
}
