package main

import (
	"bytes"
	"context"
	"encoding/json"
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

func buildFailed(pkg string) string {
	return `{"ImportPath":"` + pkg + ` [` + pkg + `.test]","Action":"build-fail"}` + "\n" +
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

func TestScorePresence(t *testing.T) {
	t.Run("a format is present with no tests at all", func(t *testing.T) {
		s := Score("yaml", KindFormat, nil)
		assert.Equal(t, Present, s.Presence)
		assert.Equal(t, "P0", s.Level)
		assert.Equal(t, NotMet, s.Rungs["P1"].Outcome)
		assert.Equal(t, reasonNoTest, s.Rungs["P1"].Reason)
	})
	t.Run("a language no test names is absent and has no level", func(t *testing.T) {
		s := Score("python", KindLanguage, nil)
		assert.Equal(t, Absent, s.Presence)
		assert.Empty(t, s.Level)
	})
	t.Run("a language whose tests did not run has no level", func(t *testing.T) {
		s := Score("go", KindLanguage, []RungTest{rungTest("go", "P1", resultBuildFailed), rungTest("go", "P2", resultSkipped)})
		assert.Equal(t, PresenceUnproven, s.Presence)
		assert.Empty(t, s.Level)
	})
	t.Run("a presence test alone makes a language present at P0", func(t *testing.T) {
		s := Score("ruby", KindLanguage, []RungTest{rungTest("ruby", "P0", resultPassed)})
		assert.Equal(t, Present, s.Presence)
		assert.Equal(t, "P0", s.Level)
		assert.Len(t, s.PresenceTests, 1)
	})
	t.Run("a failing rung test is still evidence the language is read", func(t *testing.T) {
		s := Score("go", KindLanguage, []RungTest{rungTest("go", "P1", resultFailed)})
		assert.Equal(t, Present, s.Presence)
		assert.Equal(t, "P0", s.Level)
	})
	t.Run("a language that meets P1 and P2", func(t *testing.T) {
		s := Score("go", KindLanguage, []RungTest{rungTest("go", "P1", resultPassed), rungTest("go", "P2", resultPassed)})
		assert.Equal(t, "P2", s.Level)
	})
}

func expectedCanary() Subject {
	return Score(CanaryID, KindLanguage, []RungTest{
		rungTest(CanaryID, "P1", resultSkipped),
		rungTest(CanaryID, "P2", resultPassed),
		rungTest(CanaryID, "P3", resultFailed),
		rungTest(CanaryID, "P4", resultSubtestsSkipped),
	})
}

func TestCheckCanary(t *testing.T) {
	require.Empty(t, CheckCanary(expectedCanary()))

	mutations := map[string]func(*Subject){
		"awarded P2":           func(s *Subject) { s.Level = "P2" },
		"skip counted as met":  func(s *Subject) { s.Rungs["P1"] = RungResult{Outcome: Met, Reason: resultPassed} },
		"failure did not run":  func(s *Subject) { s.Rungs["P3"] = RungResult{Outcome: DidNotRun, Reason: resultNoResult} },
		"empty parent as pass": func(s *Subject) { s.Rungs["P4"] = RungResult{Outcome: Met, Reason: resultPassed} },
		"canary never ran":     func(s *Subject) { *s = Score(CanaryID, KindLanguage, nil) },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			s := expectedCanary()
			s.Rungs = maps.Clone(s.Rungs)
			mutate(&s)
			assert.NotEmpty(t, CheckCanary(s))
		})
	}
}

// TestCanaryUnderRealGoTest runs the canary package through the real go
// command, the way a probe run does, and requires the one acceptable score.
// Every classification above is proven here against events `go test` emitted
// rather than events written by hand.
func TestCanaryUnderRealGoTest(t *testing.T) {
	names := []string{"TestProseP1_canary", "TestProseP2_canary", "TestProseP3_canary", "TestProseP4_canary"}
	stdout, stderr, err := ExecGo(context.Background(), ".", append(os.Environ(), canaryEnv+"=1"),
		[]string{"test", "-json", "-count=1", "-run", "^(" + strings.Join(names, "|") + ")$", "./canary"})
	require.NoError(t, err, string(stderr))

	results, events := FoldEvents(bytes.NewReader(stdout))
	require.Positive(t, events, "go test emitted no events: %s", stderr)
	var found []RungTest
	for _, n := range names {
		sm := strictTestRe.FindStringSubmatch(n)
		require.NotNil(t, sm)
		found = append(found, RungTest{Name: n, Result: Classify(results[canaryPackage], n), subject: sm[2], rung: "P" + sm[1]})
	}
	canary := Score(CanaryID, KindLanguage, found)
	assert.Empty(t, CheckCanary(canary), "results: %+v", found)
	assert.Equal(t, "P0", canary.Level)
}

// fixtureRoot lays out a repository in miniature: a workspace module with a
// format's rung test, the probe module with the canary, and a plugin module
// with a language presence test.
func fixtureRoot(t *testing.T, extra map[string]string) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"go.work":                        "go 1.27.0\n\nuse (\n\t./frame // the framework\n\t./scripts/proseprobe\n)\n",
		"frame/go.mod":                   "module example.com/frame\n",
		"frame/formats/yaml/r_test.go":   "package yaml\n\nfunc TestProseP1_yaml(t *testing.T) {}\n",
		"frame/node_modules/x/x_test.go": "func TestProseP1_notseen(t *testing.T) {}\n",
		"frame/testdata/y_test.go":       "func TestProseP1_notseen(t *testing.T) {}\n",
		"frame/nested/go.mod":            "module example.com/nested\n",
		"frame/nested/n_test.go":         "func TestProseP1_notseen(t *testing.T) {}\n",
		"scripts/proseprobe/go.mod":      "module github.com/neokapi/neokapi/scripts/proseprobe\n",
		"scripts/proseprobe/canary/c_test.go": "func TestProseP1_canary(t *testing.T) {}\nfunc TestProseP2_canary(t *testing.T) {}\n" +
			"func TestProseP3_canary(t *testing.T) {}\nfunc TestProseP4_canary(t *testing.T) {}\n",
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

// canaryStream is the event stream the canary produces under a probe run.
func canaryStream() string {
	return ev("skip", canaryPackage, "TestProseP1_canary") +
		ev("pass", canaryPackage, "TestProseP2_canary") +
		ev("fail", canaryPackage, "TestProseP3_canary") +
		ev("skip", canaryPackage, "TestProseP4_canary/only-case") +
		ev("pass", canaryPackage, "TestProseP4_canary") +
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
	assert.Equal(t, []string{"pdf", "sourcecode", "yaml", "python", "ruby"}, ids, "formats first, then registered languages; the canary is never a subject")
	assert.Equal(t, []string{"ruby"}, subjectByID(t, rep, "sourcecode").Languages, "a format that reads a language points at the language's row")
	assert.Equal(t, "P1", subjectByID(t, rep, "yaml").Level)
	assert.Equal(t, "P0", subjectByID(t, rep, "pdf").Level)
	assert.Equal(t, Absent, subjectByID(t, rep, "python").Presence)
	ruby := subjectByID(t, rep, "ruby")
	assert.Equal(t, Present, ruby.Presence)
	assert.Equal(t, "P0", ruby.Level)
	assert.Equal(t, "sourcecode", ruby.Provider)
	assert.Equal(t, "P0", rep.Canary.Level)

	assert.Contains(t, fake.calls["/frame"], "fts5", "workspace modules build with the tags")
	assert.NotContains(t, fake.calls["/plugins/src"], "-tags", "plugin modules build as their make targets do")
	assert.Contains(t, fake.envs["/plugins/src"], "GOWORK=off")
	assert.Contains(t, fake.envs["/scripts/proseprobe"], canaryEnv+"=1")
	assert.Contains(t, fake.calls["/frame"], "^(TestProseP1_yaml)$")
	assert.Contains(t, fake.calls["/frame"], "./formats/yaml")
}

func TestProbeRefusesTestsItCannotPlace(t *testing.T) {
	cases := map[string]map[string]string{
		"an unregistered subject":         {"frame/formats/yaml/bad_test.go": "func TestProseP1_golang(t *testing.T) {}\n"},
		"a rung above P4":                 {"frame/formats/yaml/bad_test.go": "func TestProseP5_yaml(t *testing.T) {}\n"},
		"an uppercase subject":            {"frame/formats/yaml/bad_test.go": "func TestProseP1_Yaml(t *testing.T) {}\n"},
		"a suffix after the subject":      {"frame/formats/yaml/bad_test.go": "func TestProseP1_yaml_edges(t *testing.T) {}\n"},
		"the canary outside its package":  {"frame/formats/yaml/bad_test.go": "func TestProseP1_canary(t *testing.T) {}\n"},
		"a test the canary would require": {"scripts/proseprobe/canary/c_test.go": "func TestProseP1_canary(t *testing.T) {}\n"},
	}
	for name, extra := range cases {
		t.Run(name, func(t *testing.T) {
			root := fixtureRoot(t, extra)
			canary := canaryStream()
			if name == "a test the canary would require" {
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
	assert.Equal(t, "P0", rep.Canary.Level)
	assert.Len(t, rep.Subjects, 5)
	assert.Contains(t, stderr.String(), "canary P0")
}

func TestParseWorkUse(t *testing.T) {
	work := "go 1.27.0\n\nuse ./single\n\nuse (\n\t.\n\t./a/b // comment\n\t\"./quoted\"\n)\n"
	assert.Equal(t, []string{"single", ".", "a/b", "quoted"}, parseWorkUse(work))
}
