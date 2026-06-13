package output

import (
	"testing"
	"time"

	"github.com/neokapi/neokapi/core/graph"
	"github.com/stretchr/testify/assert"
)

func TestFormatValidity(t *testing.T) {
	date := func(y int, m time.Month, d int) *time.Time {
		t := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
		return &t
	}
	instant := time.Date(2026, 6, 1, 12, 30, 0, 0, time.UTC)

	tests := []struct {
		name string
		v    *graph.Validity
		want string
	}{
		{"nil validity is empty", nil, ""},
		{"empty validity is empty", &graph.Validity{}, ""},
		{"from date only", &graph.Validity{ValidFrom: date(2026, 1, 1)}, "from 2026-01-01"},
		{"to date only", &graph.Validity{ValidTo: date(2026, 6, 1)}, "to 2026-06-01"},
		{
			"full interval",
			&graph.Validity{ValidFrom: date(2026, 1, 1), ValidTo: date(2026, 6, 1)},
			"from 2026-01-01 to 2026-06-01",
		},
		{
			"non-midnight instant keeps RFC3339",
			&graph.Validity{ValidTo: &instant},
			"to 2026-06-01T12:30:00Z",
		},
		{
			"tags sorted by key",
			&graph.Validity{Tags: map[string]string{"market": "dach", "channel": "web"}},
			"channel=web market=dach",
		},
		{
			"interval and tags",
			&graph.Validity{ValidFrom: date(2026, 1, 1), Tags: map[string]string{"market": "dach"}},
			"from 2026-01-01; market=dach",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, FormatValidity(tt.v))
		})
	}
}
