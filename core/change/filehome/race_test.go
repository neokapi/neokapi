package filehome_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/change/filehome"
)

// The race the architecture judge ran against a lock-free check-then-rename:
// two writers edit different blocks of one HTML file, each stages against the
// same content, and a barrier holds both until both have staged. Without the
// lock and the re-apply, the second rename replaced the first edit, and both
// writers reported success. Each writer here is a separate process, because
// that is the case the lock exists for: an agent's MCP server, a CLI run and
// Kapi Desktop on one file.

const raceDoc = `<!DOCTYPE html>
<html><head><meta charset="utf-8"><title>Race</title></head>
<body>
<p>Read the <a href="https://example.com/guide">first guide</a> today.</p>
<p>Then read the <b>second guide</b> tomorrow.</p>
</body></html>
`

// raceChildEnv names the variables a writer process reads.
const (
	raceChildEnv   = "FILEHOME_RACE_CHILD"
	raceDirEnv     = "FILEHOME_RACE_DIR"
	raceLocksEnv   = "FILEHOME_RACE_LOCKS"
	raceBlockEnv   = "FILEHOME_RACE_BLOCK"
	raceRevEnv     = "FILEHOME_RACE_REV"
	raceFindEnv    = "FILEHOME_RACE_FIND"
	raceTextEnv    = "FILEHOME_RACE_TEXT"
	raceNameEnv    = "FILEHOME_RACE_NAME"
	raceWaitForEnv = "FILEHOME_RACE_WAIT_FOR"
)

// TestMain runs a writer when the test binary is started as one.
func TestMain(m *testing.M) {
	if os.Getenv(raceChildEnv) != "" {
		os.Exit(raceChild())
	}
	os.Exit(m.Run())
}

// raceChild applies one replace_text as its own process and prints the
// result. Its BeforeSettle marks it staged and waits for its sibling to have
// staged, and, when told to, for its sibling to have finished.
func raceChild() int {
	dir := os.Getenv(raceDirEnv)
	name := os.Getenv(raceNameEnv)
	barrier := func(string) {
		_ = os.WriteFile(filepath.Join(dir, "staged-"+name), nil, 0o644)
		waitFor(dir, "staged-")
		if other := os.Getenv(raceWaitForEnv); other != "" {
			waitForFile(filepath.Join(dir, "done-"+other))
		}
	}
	home := filehome.New(filehome.DirLayout{Root: dir, Formats: newRegistry(), SourceLocale: "en"},
		filehome.Options{LockDir: os.Getenv(raceLocksEnv), BeforeSettle: barrier})
	svc := change.NewService(filehome.Formats{Registry: newRegistry()}, change.OneHome(home))
	find := os.Getenv(raceFindEnv)
	res, err := svc.Apply(context.Background(), change.Set{Ops: []change.Op{{
		Kind: change.KindReplaceText, At: change.Ref{Doc: "race.html", Block: os.Getenv(raceBlockEnv)}, IfMatch: os.Getenv(raceRevEnv),
		Body: &change.ReplaceText{Edits: []change.TextEdit{{Find: &find, Text: os.Getenv(raceTextEnv)}}},
	}}}, change.Actor{Kind: change.ActorPerson, Name: name})
	_ = os.WriteFile(filepath.Join(dir, "done-"+name), nil, 0o644)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	out, _ := json.Marshal(res)
	fmt.Println(string(out))
	return 0
}

// waitFor waits until two files whose names start with prefix exist in dir.
func waitFor(dir, prefix string) {
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		entries, _ := os.ReadDir(dir)
		n := 0
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), prefix) {
				n++
			}
		}
		if n >= 2 {
			return
		}
		time.Sleep(time.Millisecond)
	}
}

func waitForFile(path string) {
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(time.Millisecond)
	}
}

func TestFileHome_TwoProcessesEditingDifferentBlocksLoseNoEdit(t *testing.T) {
	if testing.Short() {
		t.Skip("starts 100 processes")
	}
	if runtime.GOOS == "js" {
		t.Skip("js/wasm cannot start a process; the writers of one process are TestFileHome_TwoWritersOfOneBlockConflict")
	}
	exe, err := os.Executable()
	require.NoError(t, err)
	locks := t.TempDir()

	const rounds = 50
	for round := range rounds {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "race.html"), []byte(raceDoc), 0o644))
		f := &fixture{dir: dir}
		svc := change.NewService(filehome.Formats{Registry: newRegistry()},
			change.OneHome(filehome.New(filehome.DirLayout{Root: dir, Formats: newRegistry(), SourceLocale: "en"}, filehome.Options{LockDir: locks})))
		page, err := svc.Read(context.Background(), change.ReadRequest{Doc: "race.html"})
		require.NoError(t, err)
		var first, second change.BlockRead
		for _, b := range page.Blocks {
			switch {
			case strings.Contains(b.Text, "first guide"):
				first = b
			case strings.Contains(b.Text, "second guide"):
				second = b
			}
		}
		require.NotEmpty(t, first.Rev)
		require.NotEmpty(t, second.Rev)

		// A round in three: A commits first, B commits first, or neither is
		// held back and the lock decides.
		waitA, waitB := "", ""
		switch round % 3 {
		case 0:
			waitB = "a"
		case 1:
			waitA = "b"
		}
		var wg sync.WaitGroup
		outs := make([]string, 2)
		errs := make([]error, 2)
		run := func(i int, name, block, rev, find, text, waitFor string) {
			defer wg.Done()
			cmd := exec.Command(exe, "-test.run=^$")
			cmd.Env = append(os.Environ(),
				raceChildEnv+"=1", raceDirEnv+"="+dir, raceLocksEnv+"="+locks, raceNameEnv+"="+name,
				raceBlockEnv+"="+block, raceRevEnv+"="+rev, raceFindEnv+"="+find, raceTextEnv+"="+text,
				raceWaitForEnv+"="+waitFor)
			out, err := cmd.CombinedOutput()
			outs[i], errs[i] = string(out), err
		}
		wg.Add(2)
		go run(0, "a", first.Ref.Block, first.Rev, "first guide", "handbook", waitA)
		go run(1, "b", second.Ref.Block, second.Rev, "second guide", "manual", waitB)
		wg.Wait()

		for i := range 2 {
			require.NoError(t, errs[i], "round %d writer %d: %s", round, i, outs[i])
			var res change.Result
			require.NoError(t, json.Unmarshal([]byte(lastLine(outs[i])), &res), "round %d writer %d: %s", round, i, outs[i])
			require.Equal(t, change.SetApplied, res.Status, "round %d writer %d: %s", round, i, outs[i])
		}
		got := f.read(t, "race.html")
		want := strings.Replace(strings.Replace(raceDoc, "first guide", "handbook", 1), "second guide", "manual", 1)
		require.Equal(t, want, got, "round %d lost an edit", round)
	}
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return lines[len(lines)-1]
}

// TestFileHome_TwoWritersOfOneBlockConflict stages two edits of the same
// block against the same content in one process: the first lands, and the
// second is refused stale with the content the first wrote, which is what a
// conflict is.
func TestFileHome_TwoWritersOfOneBlockConflict(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.json"), []byte(`{"title": "Hello"}`+"\n"), 0o644))
	locks := t.TempDir()
	staged := make(chan struct{}, 2)
	release := make(chan struct{})
	var once sync.Once
	barrier := func(string) {
		staged <- struct{}{}
		<-release
	}
	newSvc := func(hook func(string)) *change.Service {
		return change.NewService(filehome.Formats{Registry: newRegistry()},
			change.OneHome(filehome.New(filehome.DirLayout{Root: dir, Formats: newRegistry(), SourceLocale: "en"}, filehome.Options{LockDir: locks, BeforeSettle: hook})))
	}
	reader := newSvc(nil)
	page, err := reader.Read(context.Background(), change.ReadRequest{Doc: "a.json"})
	require.NoError(t, err)
	b := page.Blocks[0]

	results := make([]*change.Result, 2)
	var wg sync.WaitGroup
	for i, text := range []string{"Hi", "Hey"} {
		wg.Go(func() {
			res, err := newSvc(barrier).Apply(context.Background(), change.Set{Ops: []change.Op{{
				Kind: change.KindSetContent, At: b.Ref, IfMatch: b.Rev, Body: &change.SetContent{Text: &text},
			}}}, person)
			assert.NoError(t, err)
			results[i] = res
		})
	}
	<-staged
	<-staged
	once.Do(func() { close(release) })
	wg.Wait()

	var applied, refused *change.Result
	for _, r := range results {
		require.NotNil(t, r)
		switch r.Status {
		case change.SetApplied:
			applied = r
		case change.SetRefused:
			refused = r
		}
	}
	require.NotNil(t, applied, "one writer lands")
	require.NotNil(t, refused, "the other conflicts")
	op := refused.Ops[0]
	require.NotNil(t, op.Error)
	assert.Equal(t, change.CodeStale, op.Error.Code)
	require.NotNil(t, op.Current)
	assert.Equal(t, applied.Ops[0].After, op.Current.Rev, "the refusal carries what the first writer wrote")
	body, err := os.ReadFile(filepath.Join(dir, "a.json"))
	require.NoError(t, err)
	assert.Contains(t, []string{`{"title": "Hi"}` + "\n", `{"title": "Hey"}` + "\n"}, string(body))
	assert.Contains(t, string(body), strconv.Quote(op.Current.Text))
}
