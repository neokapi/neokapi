package host

import (
	"errors"
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeTree reads a process tree stated as a map, so the walk is exercised on
// every platform rather than on the one the test happens to run on.
func fakeTree(procs map[int]processInfo) func(int) (processInfo, error) {
	return func(pid int) (processInfo, error) {
		parent, ok := procs[pid]
		if !ok {
			return processInfo{}, errors.New("no such process")
		}
		return parent, nil
	}
}

func TestIsShellProcess(t *testing.T) {
	for _, name := range []string{"zsh", "-zsh", "/bin/bash", "SH", "fish", "pwsh.exe", "cmd.exe"} {
		assert.True(t, isShellProcess(name), name)
	}
	for _, name := range []string{"claude", "codex", "node", "make", "kapi", "", "shell"} {
		assert.False(t, isShellProcess(name), name)
	}
}

// TestWalkClimbsOutOfTheShells is the rule the session rests on: the shell an
// agent host starts for one command is passed over, and the host above it is
// the answer.
func TestWalkClimbsOutOfTheShells(t *testing.T) {
	cases := []struct {
		name    string
		from    int
		tree    map[int]processInfo
		want    int
		wantErr bool
	}{
		{
			name: "one shell between kapi and the host",
			from: 101,
			tree: map[int]processInfo{
				101: {PID: 100, Name: "zsh", Start: 11},
				100: {PID: 50, Name: "claude", Start: 7},
			},
			want: 50,
		},
		{
			name: "the shell replaced itself with kapi",
			from: 100,
			tree: map[int]processInfo{100: {PID: 50, Name: "claude", Start: 7}},
			want: 50,
		},
		{
			name: "a login shell under the host's shell",
			from: 101,
			tree: map[int]processInfo{
				101: {PID: 100, Name: "sh", Start: 11},
				100: {PID: 90, Name: "-zsh", Start: 10},
				90:  {PID: 50, Name: "codex", Start: 7},
			},
			want: 50,
		},
		{
			name: "shells all the way to the top",
			from: 101,
			tree: map[int]processInfo{
				101: {PID: 100, Name: "zsh", Start: 11},
				100: {PID: 1, Name: "zsh", Start: 2},
			},
			wantErr: true,
		},
		{
			name:    "the tree cannot be read",
			from:    101,
			tree:    map[int]processInfo{},
			wantErr: true,
		},
		{
			name: "a cycle is bounded",
			from: 101,
			tree: map[int]processInfo{
				101: {PID: 102, Name: "zsh", Start: 11},
				102: {PID: 101, Name: "zsh", Start: 12},
			},
			wantErr: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := walkToAgentHost(tc.from, fakeTree(tc.tree))
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got.PID)
		})
	}
}

// TestOneRunIsOneSession is what a session promises: two kapi processes started
// by one agent run record under the same id, and a second run records under
// another.
func TestOneRunIsOneSession(t *testing.T) {
	host := processInfo{PID: 50, Name: "claude", Start: 7}
	first, err := walkToAgentHost(101, fakeTree(map[int]processInfo{
		101: {PID: 100, Name: "zsh", Start: 11},
		100: host,
	}))
	require.NoError(t, err)
	second, err := walkToAgentHost(201, fakeTree(map[int]processInfo{
		201: {PID: 200, Name: "zsh", Start: 19},
		200: host,
	}))
	require.NoError(t, err)
	assert.Equal(t, processSessionID(first.PID, first.Start), processSessionID(second.PID, second.Start),
		"two commands of one run share a session")

	later, err := walkToAgentHost(301, fakeTree(map[int]processInfo{
		301: {PID: 300, Name: "zsh", Start: 29},
		300: {PID: 60, Name: "claude", Start: 27},
	}))
	require.NoError(t, err)
	assert.NotEqual(t, processSessionID(first.PID, first.Start), processSessionID(later.PID, later.Start),
		"a second run is a second session")
}

// TestASessionIDIsShortAndSurvivesAReusedPID: the id is typed by hand at
// `kapi context revert --session`, and a pid the machine handed out twice must
// not merge two runs.
func TestASessionIDIsShortAndSurvivesAReusedPID(t *testing.T) {
	id := processSessionID(50, 7)
	assert.Equal(t, id, processSessionID(50, 7))
	assert.NotEqual(t, id, processSessionID(50, 8))
	assert.NotEqual(t, id, processSessionID(51, 7))
	assert.True(t, strings.HasPrefix(id, "s"), id)
	assert.Len(t, id, 13, "short enough to type")
}

// TestAgentRunSessionAnswersOnThisMachine covers the platform half: every
// supported platform reads the tree, and the rest fall back to the session
// AgentRunSession documents.
func TestAgentRunSessionAnswersOnThisMachine(t *testing.T) {
	session := AgentRunSession()
	assert.True(t, strings.HasPrefix(session, "s"), session)
	assert.Equal(t, session, AgentRunSession(), "one kapi process answers once")

	_, err := parentProcess(os.Getpid())
	switch runtime.GOOS {
	case "darwin", "linux":
		require.NoError(t, err, "the process tree is readable here")
	default:
		require.ErrorIs(t, err, errNoAgentHostProcess)
	}
}
