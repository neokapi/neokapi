package server

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPushStatus(t *testing.T) {
	cases := []struct {
		name                                string
		total, completed, failed, inProcess int
		want                                string
	}{
		{"a push with no jobs", 0, 0, 0, 0, "no_jobs"},
		{"a job still running", 2, 1, 0, 1, "in_progress"},
		{"every job failed", 2, 0, 2, 0, "failed"},
		{"some jobs completed", 2, 1, 1, 0, "completed"},
		{"every job completed", 2, 2, 0, 0, "completed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, pushStatus(tc.total, tc.completed, tc.failed, tc.inProcess))
		})
	}
}
