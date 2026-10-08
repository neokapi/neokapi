package host

import (
	"testing"

	"github.com/neokapi/neokapi/host/output"
	"github.com/stretchr/testify/assert"
)

func TestServerLsArgs(t *testing.T) {
	listed := &output.LsOutput{Files: []output.LsEntry{{Path: "src/a.json"}, {Path: "src/b.json"}}}
	tests := []struct {
		name  string
		out   *output.LsOutput
		paths []string
		max   int
		want  []string
	}{
		{"hands over the listed files", listed, []string{"src/"}, 1 << 10, []string{"src/a.json", "src/b.json"}},
		{"nothing listed keeps the arguments", &output.LsOutput{}, []string{"src/"}, 1 << 10, []string{"src/"}},
		{"too long for a command line keeps the arguments", listed, []string{"src/"}, 12, []string{"src/"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			saved := serverLsArgMax
			serverLsArgMax = tt.max
			t.Cleanup(func() { serverLsArgMax = saved })
			assert.Equal(t, tt.want, serverLsArgs(tt.out, tt.paths))
		})
	}
}
