package storeutil

import (
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseStoredTime(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    time.Time
		wantErr string
	}{
		{name: "empty is the zero time", in: ""},
		{
			name: "RFC3339",
			in:   "2026-07-01T10:00:00Z",
			want: time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC),
		},
		{
			name: "RFC3339 with an offset",
			in:   "2026-07-01T12:00:00+02:00",
			want: time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC),
		},
		{
			name: "RFC3339Nano",
			in:   "2026-07-01T10:00:00.123456789Z",
			want: time.Date(2026, 7, 1, 10, 0, 0, 123456789, time.UTC),
		},
		{
			name: "SQLite datetime('now') layout",
			in:   "2026-07-01 10:00:00",
			want: time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC),
		},
		{
			name:    "garbage is an error naming the column",
			in:      "not a time",
			wantErr: "parse created_at",
		},
		{
			name:    "a date alone is an error",
			in:      "2026-07-01",
			wantErr: "parse created_at",
		},
		{
			name:    "the error reports the RFC3339 failure",
			in:      "2026/07/01 10:00",
			wantErr: `as "2006-01-02T15:04:05`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseStoredTime("created_at", tc.in)
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
				assert.True(t, got.IsZero())
				return
			}
			require.NoError(t, err)
			assert.True(t, tc.want.Equal(got), "want %s, got %s", tc.want, got)
		})
	}
}

func TestParseOptionalStoredTime(t *testing.T) {
	got, err := ParseOptionalStoredTime("finished_at", sql.NullString{})
	require.NoError(t, err)
	assert.Nil(t, got)

	got, err = ParseOptionalStoredTime("finished_at", sql.NullString{String: "", Valid: true})
	require.NoError(t, err)
	assert.Nil(t, got)

	got, err = ParseOptionalStoredTime("finished_at", sql.NullString{String: "2026-07-01T10:00:00Z", Valid: true})
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.True(t, time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC).Equal(*got))

	got, err = ParseOptionalStoredTime("finished_at", sql.NullString{String: "yesterday", Valid: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse finished_at")
	assert.Nil(t, got)
}
