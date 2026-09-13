package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const pkgX = "example.com/x"

// ev renders one `go test -json` event line.
func ev(action, pkg, test string) string {
	b, _ := json.Marshal(map[string]string{"Action": action, "Package": pkg, "Test": test})
	return string(b) + "\n"
}

// out renders one output event line.
func out(pkg, test, output string) string {
	b, _ := json.Marshal(map[string]string{"Action": "output", "Package": pkg, "Test": test, "Output": output})
	return string(b) + "\n"
}

func buildFailed(pkg string, compiler ...string) string {
	var s strings.Builder
	for _, line := range compiler {
		b, _ := json.Marshal(map[string]string{"ImportPath": pkg + " [" + pkg + ".test]", "Action": "build-output", "Output": line + "\n"})
		s.Write(b)
		s.WriteString("\n")
	}
	return s.String() + `{"ImportPath":"` + pkg + ` [` + pkg + `.test]","Action":"build-fail"}` + "\n" +
		`{"Action":"start","Package":"` + pkg + `"}` + "\n" +
		`{"Action":"fail","Package":"` + pkg + `","Elapsed":0,"FailedBuild":"` + pkg + ` [` + pkg + `.test]"}` + "\n"
}

func TestClassify(t *testing.T) {
	cases := []struct {
		name   string
		stream string
		test   string
		want   string
	}{
		{"pass", ev("run", pkgX, "TestA") + ev("pass", pkgX, "TestA"), "TestA", resultPassed},
		{"fail", ev("run", pkgX, "TestA") + ev("fail", pkgX, "TestA"), "TestA", resultFailed},
		{"skip", ev("run", pkgX, "TestA") + ev("skip", pkgX, "TestA"), "TestA", resultSkipped},
		{
			"a parent whose every subtest skipped",
			ev("skip", pkgX, "TestA/one") + ev("skip", pkgX, "TestA/two") + ev("pass", pkgX, "TestA"),
			"TestA", resultSubtestsSkipped,
		},
		{
			"a parent with one subtest that checked something",
			ev("skip", pkgX, "TestA/one") + ev("pass", pkgX, "TestA/two") + ev("pass", pkgX, "TestA"),
			"TestA", resultPassed,
		},
		{
			"a skipped leaf under a passing group",
			ev("skip", pkgX, "TestA/group/leaf") + ev("pass", pkgX, "TestA/group") + ev("pass", pkgX, "TestA"),
			"TestA", resultSubtestsSkipped,
		},
		{"a package that failed to build", buildFailed(pkgX), "TestA", resultBuildFailed},
		{"no tests to run", ev("start", pkgX, "") + ev("pass", pkgX, ""), "TestA", resultNoResult},
		{"a package the stream never mentions", ev("pass", "example.com/other", "TestA"), "TestA", resultNoResult},
		{
			"a test cut off by an earlier panic",
			ev("run", pkgX, "TestB") + ev("fail", pkgX, "TestB") + ev("fail", pkgX, ""),
			"TestA", resultNoResult,
		},
		{"noise that is not an event", "go: downloading example.com/y v1.0.0\n" + ev("pass", pkgX, "TestA"), "TestA", resultPassed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			results, _ := FoldEvents(strings.NewReader(tc.stream))
			assert.Equal(t, tc.want, Classify(results[pkgX], tc.test))
		})
	}
}

func TestTestOutput(t *testing.T) {
	t.Run("a failure keeps its assertion and drops the runner's framing", func(t *testing.T) {
		stream := ev("run", pkgX, "TestA") +
			out(pkgX, "TestA", "=== RUN   TestA\n") +
			out(pkgX, "TestA", "    a_test.go:9: \n") +
			out(pkgX, "TestA", "        \tError:      \tShould be true\n") +
			out(pkgX, "TestA/sub", "    a_test.go:12: from a subtest\n") +
			out(pkgX, "TestA", "--- FAIL: TestA (0.00s)\n") +
			ev("fail", pkgX, "TestA")
		results, _ := FoldEvents(strings.NewReader(stream))
		got := TestOutput(results[pkgX], "TestA", resultFailed)
		assert.Contains(t, got, "Should be true")
		assert.Contains(t, got, "from a subtest", "a subtest's output belongs to its parent")
		assert.NotContains(t, got, "=== RUN")
		assert.NotContains(t, got, "--- FAIL")
	})
	t.Run("a build failure keeps the compiler's output", func(t *testing.T) {
		results, _ := FoldEvents(strings.NewReader(buildFailed(pkgX, "# example.com/x", "a_test.go:5:2: undefined: nope")))
		assert.Contains(t, TestOutput(results[pkgX], "TestA", resultBuildFailed), "undefined: nope")
	})
	t.Run("only the tail is kept", func(t *testing.T) {
		var stream strings.Builder
		for i := range 20 {
			stream.WriteString(out(pkgX, "TestA", fmt.Sprintf("line %02d\n", i)))
		}
		stream.WriteString(ev("fail", pkgX, "TestA"))
		results, _ := FoldEvents(strings.NewReader(stream.String()))
		got := TestOutput(results[pkgX], "TestA", resultFailed)
		assert.Contains(t, got, "line 19")
		assert.NotContains(t, got, "line 07")
		assert.Len(t, strings.Split(got, "\n"), outputLines)
	})
}

func TestOnlyAPassIsMet(t *testing.T) {
	assert.Equal(t, Met, OutcomeOf(resultPassed))
	assert.Equal(t, NotMet, OutcomeOf(resultFailed))
	for _, r := range []string{resultSkipped, resultSubtestsSkipped, resultBuildFailed, resultNoResult} {
		assert.Equal(t, DidNotRun, OutcomeOf(r), r)
	}
}

func tests(results ...string) []RungTest {
	out := make([]RungTest, len(results))
	for i, r := range results {
		out[i] = RungTest{Name: "TestProseP1_go", Result: r}
	}
	return out
}

func TestFoldRung(t *testing.T) {
	cases := []struct {
		name    string
		tests   []RungTest
		outcome Outcome
		reason  string
	}{
		{"no test claims the rung", nil, NotMet, reasonNoTest},
		{"every test passed", tests(resultPassed, resultPassed), Met, resultPassed},
		{"one failure", tests(resultPassed, resultFailed, resultSkipped), NotMet, resultFailed},
		{"one test did not run", tests(resultPassed, resultBuildFailed), DidNotRun, resultBuildFailed},
		{"a lone skip", tests(resultSkipped), DidNotRun, resultSkipped},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := FoldRung(tc.tests)
			assert.Equal(t, tc.outcome, got.Outcome)
			assert.Equal(t, tc.reason, got.Reason)
		})
	}
}

func rungs(outcomes ...Outcome) map[string]RungResult {
	m := map[string]RungResult{}
	for i, o := range outcomes {
		m[Rungs[i]] = RungResult{Outcome: o}
	}
	for _, r := range Rungs[len(outcomes):] {
		m[r] = RungResult{Outcome: NotMet}
	}
	return m
}

// TestLevelIsCapped proves a missing lower rung caps the level: a met rung
// above one that is not met, or that did not run, awards nothing.
func TestLevelIsCapped(t *testing.T) {
	cases := []struct {
		name  string
		rungs map[string]RungResult
		want  string
	}{
		{"nothing met", rungs(), "P0"},
		{"P1", rungs(Met), "P1"},
		{"P1 and P2", rungs(Met, Met), "P2"},
		{"every rung", rungs(Met, Met, Met, Met), "P4"},
		{"P2 met above an unmet P1", rungs(NotMet, Met), "P0"},
		{"P2 met above a P1 that did not run", rungs(DidNotRun, Met), "P0"},
		{"P3 met above an unmet P2", rungs(Met, NotMet, Met, Met), "P1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, Level(tc.rungs))
		})
	}
}

func rungTest(subject, rung, result string) RungTest {
	return RungTest{Name: "TestProse" + rung + "_" + subject, Result: result, subject: subject, rung: rung}
}

func rungTestOutput(subject, rung, result, output string) RungTest {
	t := rungTest(subject, rung, result)
	t.Output = output
	return t
}

func TestScorePresence(t *testing.T) {
	t.Run("a format is present with no tests at all", func(t *testing.T) {
		s := Score("yaml", KindFormat, nil)
		assert.Equal(t, Present, s.Presence)
		assert.Equal(t, "P0", s.Level)
		assert.Equal(t, NotMet, s.Rungs["P1"].Outcome)
		assert.Equal(t, reasonNoTest, s.Rungs["P1"].Reason)
	})
	t.Run("a language with no presence test is absent and has no level", func(t *testing.T) {
		s := Score("python", KindLanguage, nil)
		assert.Equal(t, Absent, s.Presence)
		assert.Equal(t, reasonNoPresenceTest, s.PresenceReason)
		assert.Empty(t, s.Level)
	})
	t.Run("a passing presence test makes a language present", func(t *testing.T) {
		s := Score("ruby", KindLanguage, []RungTest{rungTest("ruby", "P0", resultPassed)})
		assert.Equal(t, Present, s.Presence)
		assert.Equal(t, "TestProseP0_ruby passed", s.PresenceReason)
		assert.Equal(t, "P0", s.Level)
		assert.Len(t, s.PresenceTests, 1)
	})
	t.Run("must fail: a passing rung test without a presence test is absent", func(t *testing.T) {
		s := Score("go", KindLanguage, []RungTest{rungTest("go", "P1", resultPassed), rungTest("go", "P2", resultPassed)})
		assert.Equal(t, Absent, s.Presence, "a package can pass its rung tests while no binary links it")
		assert.Empty(t, s.Level)
	})
	t.Run("must fail: a failing presence test did not run, and is never absent", func(t *testing.T) {
		s := Score("go", KindLanguage, []RungTest{
			rungTest("go", "P0", resultPassed),
			rungTestOutput("go", "P0", resultFailed, "Error: Should be true\nMessages: no provider for .go files"),
			rungTest("go", "P1", resultPassed),
		})
		assert.Equal(t, PresenceUnproven, s.Presence, "a failure cannot tell a removed reader from a broken test")
		assert.Contains(t, s.PresenceReason, "TestProseP0_go failed")
		assert.Contains(t, s.PresenceReason, "no provider for .go files", "the failing test's output is kept in the reason")
		assert.Empty(t, s.Level)
	})
	t.Run("a presence test that did not build did not run", func(t *testing.T) {
		s := Score("go", KindLanguage, []RungTest{rungTestOutput("go", "P0", resultBuildFailed, "undefined: commentProviders"), rungTest("go", "P1", resultPassed)})
		assert.Equal(t, PresenceUnproven, s.Presence)
		assert.Contains(t, s.PresenceReason, "build failed: undefined: commentProviders")
		assert.Empty(t, s.Level)
	})
	t.Run("a present language that meets P1 and P2", func(t *testing.T) {
		s := Score("go", KindLanguage, []RungTest{rungTest("go", "P0", resultPassed), rungTest("go", "P1", resultPassed), rungTest("go", "P2", resultPassed)})
		assert.Equal(t, Present, s.Presence)
		assert.Equal(t, "P2", s.Level)
	})
	t.Run("must fail: a present language with no passing P1 scores exactly P0", func(t *testing.T) {
		s := Score("go", KindLanguage, []RungTest{rungTest("go", "P0", resultPassed), rungTest("go", "P1", resultFailed)})
		assert.Equal(t, Present, s.Presence)
		assert.Equal(t, "P0", s.Level)
	})
}

// canaryResults is what every canary test comes to under a probe run.
func canaryResults() []RungTest {
	return []RungTest{
		rungTest("canary", "P0", resultPassed),
		rungTest("canary", "P1", resultSkipped),
		rungTest("canary", "P2", resultPassed),
		rungTest("canary", "P3", resultFailed),
		rungTest("canary", "P4", resultSubtestsSkipped),
		rungTest("canaryunlinked", "P1", resultPassed),
		rungTest("canarypresent", "P0", resultPassed),
		rungTest("canarypresent", "P1", resultFailed),
		rungTestOutput("canarybroken", "P0", resultFailed, "canary_test.go:84: "+canaryBrokenMessage),
		rungTest("canarybroken", "P1", resultPassed),
	}
}

func expectedCanaries() map[string]Subject {
	by := map[string][]RungTest{}
	for _, t := range canaryResults() {
		by[t.subject] = append(by[t.subject], t)
	}
	subjects := map[string]Subject{}
	for id, ts := range by {
		subjects[id] = Score(id, KindLanguage, ts)
	}
	return subjects
}

func TestCheckCanary(t *testing.T) {
	expected := expectedCanaries()
	require.Len(t, expected, len(canaries), "every canary has results")
	for id, s := range expected {
		require.Empty(t, CheckCanary(s), id)
	}

	mutations := map[string]struct {
		id     string
		mutate func(*Subject)
	}{
		"canary awarded P2":                 {"canary", func(s *Subject) { s.Level = "P2" }},
		"canary skip counted as met":        {"canary", func(s *Subject) { s.Rungs["P1"] = RungResult{Outcome: Met, Reason: resultPassed} }},
		"canary failure did not run":        {"canary", func(s *Subject) { s.Rungs["P3"] = RungResult{Outcome: DidNotRun, Reason: resultNoResult} }},
		"canary empty parent as pass":       {"canary", func(s *Subject) { s.Rungs["P4"] = RungResult{Outcome: Met, Reason: resultPassed} }},
		"canary never ran":                  {"canary", func(s *Subject) { *s = Score("canary", KindLanguage, nil) }},
		"unlinked scored present":           {"canaryunlinked", func(s *Subject) { s.Presence, s.Level = Present, "P1" }},
		"present with no P1 given no level": {"canarypresent", func(s *Subject) { s.Level = "" }},
		"present with no P1 given P1":       {"canarypresent", func(s *Subject) { s.Level = "P1" }},
		"broken presence read as absent":    {"canarybroken", func(s *Subject) { s.Presence = Absent }},
		"broken presence without output":    {"canarybroken", func(s *Subject) { s.PresenceReason = "TestProseP0_canarybroken failed" }},
		"a canary the probe does not know":  {"canarybroken", func(s *Subject) { s.ID = "canaryunknown" }},
	}
	for name, m := range mutations {
		t.Run(name, func(t *testing.T) {
			s := expected[m.id]
			s.Rungs = maps.Clone(s.Rungs)
			m.mutate(&s)
			assert.NotEmpty(t, CheckCanary(s))
		})
	}
}

// TestCanaryUnderRealGoTest runs the canary package through the real go
// command, the way a probe run does, and requires every canary to score as
// built. Every classification above is proven here against events `go test`
// emitted rather than events written by hand.
func TestCanaryUnderRealGoTest(t *testing.T) {
	var names []string
	for _, rt := range canaryResults() {
		names = append(names, rt.Name)
	}
	stdout, stderr, err := ExecGo(context.Background(), ".", append(os.Environ(), canaryEnv+"=1"),
		[]string{"test", "-json", "-count=1", "-run", "^(" + strings.Join(names, "|") + ")$", "./canary"})
	require.NoError(t, err, string(stderr))

	results, events := FoldEvents(bytes.NewReader(stdout))
	require.Positive(t, events, "go test emitted no events: %s", stderr)
	by := map[string][]RungTest{}
	for _, n := range names {
		sm := strictTestRe.FindStringSubmatch(n)
		require.NotNil(t, sm)
		pr := results[canaryPackage]
		rt := RungTest{Name: n, Result: Classify(pr, n), subject: sm[2], rung: "P" + sm[1]}
		if rt.Result != resultPassed {
			rt.Output = TestOutput(pr, n, rt.Result)
		}
		by[rt.subject] = append(by[rt.subject], rt)
	}
	require.Len(t, by, len(canaries))
	for id, found := range by {
		c := Score(id, KindLanguage, found)
		assert.Empty(t, CheckCanary(c), "%s results: %+v", id, found)
	}
}

// fixtureRoot lays out a repository in miniature: a workspace module with a
// format's rung test, the probe module with the canaries, and a plugin module
// with a language presence test.
func fixtureRoot(t *testing.T, extra map[string]string) string {
	t.Helper()
	root := t.TempDir()
	var canaryFile strings.Builder
	for _, rt := range canaryResults() {
		canaryFile.WriteString("func " + rt.Name + "(t *testing.T) {}\n")
	}
	files := map[string]string{
		"go.work":                                "go 1.27.0\n\nuse (\n\t./frame // the framework\n\t./scripts/proseprobe\n)\n",
		"frame/go.mod":                           "module example.com/frame\n",
		"frame/formats/yaml/r_test.go":           "package yaml\n\nfunc TestProseP1_yaml(t *testing.T) {}\n",
		"frame/node_modules/x/x_test.go":         "func TestProseP1_notseen(t *testing.T) {}\n",
		"frame/testdata/y_test.go":               "func TestProseP1_notseen(t *testing.T) {}\n",
		"frame/nested/go.mod":                    "module example.com/nested\n",
		"frame/nested/n_test.go":                 "func TestProseP1_notseen(t *testing.T) {}\n",
		"scripts/proseprobe/go.mod":              "module github.com/neokapi/neokapi/scripts/proseprobe\n",
		"scripts/proseprobe/canary/c_test.go":    canaryFile.String(),
		"plugins/src/go.mod":                     "module example.com/src\n",
		"plugins/src/read/p_test.go":             "func TestProseP0_ruby(t *testing.T) {}\n",
		"core/formats/yaml/reader.go":            "package yaml\n",
		"core/formats/pdf/structure.yaml":        "version: 1\n",
		"core/formats/sourcecode/structure.yaml": "version: 1\n",
		"core/formats/exec/reader.go":            "package exec\n",
		"core/formats/helper/notes.txt":          "not a format\n",
		"core/formats/prose.yaml":                "languages:\n  ruby:\n    name: Ruby\n    provider: sourcecode\n  python:\n    name: Python\n",
	}
	maps.Copy(files, extra)
	for name, body := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(body), 0o644))
	}
	return root
}

// canaryStream is the event stream the canary package produces under a probe
// run.
func canaryStream() string {
	return ev("pass", canaryPackage, "TestProseP0_canary") +
		ev("skip", canaryPackage, "TestProseP1_canary") +
		ev("pass", canaryPackage, "TestProseP2_canary") +
		ev("fail", canaryPackage, "TestProseP3_canary") +
		ev("skip", canaryPackage, "TestProseP4_canary/only-case") +
		ev("pass", canaryPackage, "TestProseP4_canary") +
		ev("pass", canaryPackage, "TestProseP1_canaryunlinked") +
		ev("pass", canaryPackage, "TestProseP0_canarypresent") +
		ev("fail", canaryPackage, "TestProseP1_canarypresent") +
		out(canaryPackage, "TestProseP0_canarybroken", "    canary_test.go:84: "+canaryBrokenMessage+"\n") +
		ev("fail", canaryPackage, "TestProseP0_canarybroken") +
		ev("pass", canaryPackage, "TestProseP1_canarybroken") +
		ev("fail", canaryPackage, "")
}

// fakeGo answers each module directory with a canned event stream and records
// the arguments and environment it was started with.
type fakeGo struct {
	streams map[string]string
	calls   map[string][]string
	envs    map[string][]string
}

func (f *fakeGo) run(_ context.Context, dir string, env, args []string) ([]byte, []byte, error) {
	base := filepath.ToSlash(dir)
	for suffix, stream := range f.streams {
		if strings.HasSuffix(base, suffix) {
			f.calls[suffix] = args
			f.envs[suffix] = env
			return []byte(stream), nil, nil
		}
	}
	return nil, []byte("no stream for " + dir), nil
}

func newFakeGo(canary string) *fakeGo {
	return &fakeGo{
		streams: map[string]string{
			"/frame":              ev("pass", "example.com/frame/formats/yaml", "TestProseP1_yaml"),
			"/scripts/proseprobe": canary,
			"/plugins/src":        ev("pass", "example.com/src/read", "TestProseP0_ruby"),
		},
		calls: map[string][]string{},
		envs:  map[string][]string{},
	}
}

func subjectByID(t *testing.T, rep *Report, id string) Subject {
	t.Helper()
	for _, s := range rep.Subjects {
		if s.ID == id {
			return s
		}
	}
	t.Fatalf("no subject %q in the report", id)
	return Subject{}
}

func TestProbeOverAFixtureRepository(t *testing.T) {
	root := fixtureRoot(t, nil)
	fake := newFakeGo(canaryStream())
	rep, problems, err := Probe(context.Background(), Options{Root: root, Tags: "fts5", Run: fake.run})
	require.NoError(t, err)
	require.Empty(t, problems)

	var ids []string
	for _, s := range rep.Subjects {
		ids = append(ids, s.ID)
	}
	assert.Equal(t, []string{"pdf", "sourcecode", "yaml", "python", "ruby"}, ids, "formats first, then registered languages; no canary is a subject")
	assert.Equal(t, []string{"ruby"}, subjectByID(t, rep, "sourcecode").Languages, "a format that reads a language points at the language's row")
	assert.Equal(t, "P1", subjectByID(t, rep, "yaml").Level)
	assert.Equal(t, "P0", subjectByID(t, rep, "pdf").Level)
	assert.Equal(t, Absent, subjectByID(t, rep, "python").Presence)
	ruby := subjectByID(t, rep, "ruby")
	assert.Equal(t, Present, ruby.Presence)
	assert.Equal(t, "P0", ruby.Level)
	assert.Equal(t, "sourcecode", ruby.Provider)

	var canaryIDsSeen []string
	for _, c := range rep.Canaries {
		canaryIDsSeen = append(canaryIDsSeen, c.ID)
	}
	assert.Equal(t, []string{"canary", "canarybroken", "canarypresent", "canaryunlinked"}, canaryIDsSeen)

	assert.Contains(t, fake.calls["/frame"], "fts5", "workspace modules build with the tags")
	assert.NotContains(t, fake.calls["/plugins/src"], "-tags", "plugin modules build as their make targets do")
	assert.Contains(t, fake.envs["/plugins/src"], "GOWORK=off")
	assert.Contains(t, fake.envs["/scripts/proseprobe"], canaryEnv+"=1")
	assert.Contains(t, fake.calls["/frame"], "^(TestProseP1_yaml)$")
	assert.Contains(t, fake.calls["/frame"], "./formats/yaml")
}

func TestProbeRefusesTestsItCannotPlace(t *testing.T) {
	cases := map[string]map[string]string{
		"an unregistered subject":              {"frame/formats/yaml/bad_test.go": "func TestProseP1_golang(t *testing.T) {}\n"},
		"a rung above P4":                      {"frame/formats/yaml/bad_test.go": "func TestProseP5_yaml(t *testing.T) {}\n"},
		"an uppercase subject":                 {"frame/formats/yaml/bad_test.go": "func TestProseP1_Yaml(t *testing.T) {}\n"},
		"a suffix after the subject":           {"frame/formats/yaml/bad_test.go": "func TestProseP1_yaml_edges(t *testing.T) {}\n"},
		"the canary outside its package":       {"frame/formats/yaml/bad_test.go": "func TestProseP1_canary(t *testing.T) {}\n"},
		"a canary subject outside its package": {"frame/formats/yaml/bad_test.go": "func TestProseP1_canaryunlinked(t *testing.T) {}\n"},
		"a canary the probe does not know":     {"scripts/proseprobe/canary/extra_test.go": "func TestProseP1_canaryextra(t *testing.T) {}\n"},
		"canary tests the run would require":   {"scripts/proseprobe/canary/c_test.go": "func TestProseP1_canary(t *testing.T) {}\n"},
	}
	for name, extra := range cases {
		t.Run(name, func(t *testing.T) {
			root := fixtureRoot(t, extra)
			canary := canaryStream()
			if name == "canary tests the run would require" {
				canary = ev("skip", canaryPackage, "TestProseP1_canary")
			}
			_, problems, err := Probe(context.Background(), Options{Root: root, Run: newFakeGo(canary).run})
			require.NoError(t, err)
			assert.NotEmpty(t, problems)
		})
	}
}

func TestInvalidRunWritesNoReport(t *testing.T) {
	root := fixtureRoot(t, nil)
	out := filepath.Join(t.TempDir(), "report.json")
	awarded := strings.Replace(canaryStream(), `"Action":"skip","Package":"`+canaryPackage+`","Test":"TestProseP1_canary"`,
		`"Action":"pass","Package":"`+canaryPackage+`","Test":"TestProseP1_canary"`, 1)
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{"-root", root, "-o", out}, &stdout, &stderr, newFakeGo(awarded).run)

	assert.Equal(t, 1, code)
	assert.Contains(t, stderr.String(), "canary scored \"P2\"")
	assert.NoFileExists(t, out)
	assert.Empty(t, stdout.String())
}

func TestValidRunWritesReport(t *testing.T) {
	root := fixtureRoot(t, nil)
	out := filepath.Join(t.TempDir(), "report.json")
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{"-root", root, "-o", out}, &stdout, &stderr, newFakeGo(canaryStream()).run)
	require.Equal(t, 0, code, stderr.String())

	data, err := os.ReadFile(out)
	require.NoError(t, err)
	var rep Report
	require.NoError(t, json.Unmarshal(data, &rep))
	assert.Equal(t, ProbeVersion, rep.ProbeVersion)
	assert.Len(t, rep.Canaries, len(canaries))
	assert.Len(t, rep.Subjects, 5)
	assert.Contains(t, stderr.String(), "canaries as built")
}

func TestParseWorkUse(t *testing.T) {
	work := "go 1.27.0\n\nuse ./single\n\nuse (\n\t.\n\t./a/b // comment\n\t\"./quoted\"\n)\n"
	assert.Equal(t, []string{"single", ".", "a/b", "quoted"}, parseWorkUse(work))
}
