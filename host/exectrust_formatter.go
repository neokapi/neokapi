package host

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/neokapi/neokapi/core/project"
)

// Formatter trust: a comment edit runs the formatter the edited file's project
// configures (host/comment_formatter.go), and that formatter runs code the
// project controls, through its executable and its configuration. Execution
// trust decides it as it decides a recipe's exec steps, with the same record,
// prompt and environment grant. It is decided at the edit, since no other
// command runs a formatter, and keyed by the configuration file that selected
// the formatter, since that formatter's project often has no recipe.

// formatterTrust decides whether a run of comment edits may start a project's
// formatter, and remembers each outcome for the run.
type formatterTrust struct {
	// env is set when KAPI_TRUST_EXEC grants execution trust to this process.
	env bool
	// ask puts the question to a person and returns the answer. It is nil
	// where nobody can answer.
	ask func(site project.ExecSite) (bool, error)
	// unanswered says how to answer when nothing is recorded and nobody can
	// be asked.
	unanswered string
	// refuse, when set, says why this surface never runs a project's
	// formatter, whatever is recorded or granted.
	refuse string
	// warn receives a note when an answer cannot be recorded.
	warn io.Writer
	// decided holds this run's outcomes by record key and digest, and hashed
	// the SHA-256 of each file read for a digest, by path.
	decided map[string]error
	hashed  map[string]string
}

func newFormatterTrust(env bool) *formatterTrust {
	return &formatterTrust{env: env, decided: map[string]error{}, hashed: map[string]string{}}
}

// applyFormatterTrust is the trust kapi apply holds formatters to: the
// environment grant, then a recorded decision, then a question put to a person
// at a terminal. A change-set read from standard input leaves nobody to answer,
// since the input an answer would come from is the change-set.
func (a *App) applyFormatterTrust(cmd Command, changeSetOnStdin bool) *formatterTrust {
	trust := newFormatterTrust(execTrustEnvGranted())
	trust.unanswered = fmt.Sprintf("run kapi apply from a terminal, with the change-set in a file, to answer once, or set %s=1 if whoever configured this environment trusts the project", execTrustEnvVar)
	trust.warn = cmd.ErrOrStderr()
	isTTY := a.isTTY
	if isTTY == nil {
		isTTY = defaultIsStdinTTY
	}
	if changeSetOnStdin || !isTTY() {
		return trust
	}
	trust.ask = func(site project.ExecSite) (bool, error) {
		printFormatterTrustPrompt(cmd.ErrOrStderr(), site)
		return ConfirmDefaultNo(cmd.InOrStdin(), cmd.ErrOrStderr(), "Allow this formatter to run? [y/N] ")
	}
	return trust
}

// mcpFormatterTrust is the trust MCP apply_edits holds formatters to, which
// allows none. A project's formatter runs code the project controls, through
// its executable and the configuration files it loads, and an agent that may
// write files can write those, so apply_edits never runs one, whatever is
// recorded. The environment grant does not apply either, because a project's
// own MCP client configuration can set the server's environment.
func mcpFormatterTrust() *formatterTrust {
	trust := newFormatterTrust(false)
	trust.refuse = "apply_edits never runs a project's formatter, because the formatter runs code the project controls: run kapi apply with this edit from a terminal"
	return trust
}

// refusal says why the formatter configFile selects under name may not run on
// this surface whatever is recorded, and is empty where a decision can allow it.
func (t *formatterTrust) refusal(configFile, name string) string {
	if t.refuse == "" {
		return ""
	}
	return fmt.Sprintf("%s selects %s, and %s", DisplayName(configFile), name, t.refuse)
}

// allow reports whether command, the formatter configFile selects under name,
// may run for a file whose formatter can load configs. It returns nil when it
// may, and otherwise an error that says why not and how to answer.
func (t *formatterTrust) allow(configFile, name string, command, configs []string) error {
	if reason := t.refusal(configFile, name); reason != "" {
		return fmt.Errorf("%s", reason)
	}
	site, err := t.site(configFile, name, command, configs)
	if err != nil {
		return err
	}
	key, digest := execTrustKey(configFile), project.ExecSurfaceDigest([]project.ExecSite{site})
	memo := key + "\x00" + digest
	if outcome, done := t.decided[memo]; done {
		return outcome
	}
	outcome := t.decide(key, digest, site)
	t.decided[memo] = outcome
	return outcome
}

// decide resolves the question in execution trust's order: the environment
// grant, a decision recorded at this digest, a person asked, and otherwise no.
func (t *formatterTrust) decide(key, digest string, site project.ExecSite) error {
	if t.env {
		return nil
	}
	switch decision, found := lookupExecTrust(key, digest); {
	case found && decision == execTrustAllow:
		return nil
	case found && decision == execTrustDeny:
		return fmt.Errorf("running %s, which %s selects, was declined (edit or delete its entry in %s to answer again)",
			site.Name, DisplayName(key), ExecTrustPath())
	}
	if t.ask == nil {
		return fmt.Errorf("%s selects %s, which runs code the project controls, and no one has allowed it to run: %s",
			DisplayName(key), site.Name, t.unanswered)
	}
	ok, err := t.ask(site)
	if err != nil {
		return fmt.Errorf("ask whether %s may run: %w", site.Name, err)
	}
	decision := execTrustDeny
	if ok {
		decision = execTrustAllow
	}
	if rerr := recordExecTrust(key, digest, decision); rerr != nil && t.warn != nil {
		fmt.Fprintf(t.warn, "Warning: could not record the answer in %s: %v\n", ExecTrustPath(), rerr)
	}
	if !ok {
		return fmt.Errorf("running %s, which %s selects, was declined", site.Name, DisplayName(key))
	}
	return nil
}

// site is command's exec site as configFile selects it. Its digest holds the
// SHA-256 of what the formatter runs and loads, as far as kapi can read it: the
// executable, the interpreter that runs it (formatterRuntimeOf), the script a
// node_modules/.bin shim hands that interpreter, configFile, and every other
// configuration file the formatter can load for the edited file (configs). A
// change to any of them asks again. The modules the script, the interpreter or
// a configuration file imports, and programs a shell script runs, are not
// read, and a change to them keeps a decision.
func (t *formatterTrust) site(configFile, name string, command, configs []string) (project.ExecSite, error) {
	facts := map[string]any{}
	executable, err := t.hash(command[0])
	if err != nil {
		return project.ExecSite{}, fmt.Errorf("read the %s executable before asking whether it may run: %w", name, err)
	}
	facts["executable_sha256"] = executable
	rt := formatterRuntimeOf(command[0])
	if rt.interpreter != "" {
		sum, err := t.hash(rt.interpreter)
		if err != nil {
			return project.ExecSite{}, fmt.Errorf("read %s, which runs the %s executable, before asking whether it may run: %w", rt.interpreter, name, err)
		}
		facts["interpreter"], facts["interpreter_sha256"] = rt.interpreter, sum
	}
	if rt.script != "" {
		sum, err := t.hash(rt.script)
		if err != nil {
			return project.ExecSite{}, fmt.Errorf("read %s, which the %s executable runs, before asking whether it may run: %w", rt.script, name, err)
		}
		facts["script"], facts["script_sha256"] = rt.script, sum
	}
	config, err := t.hash(configFile)
	if err != nil {
		return project.ExecSite{}, fmt.Errorf("read %s before asking whether %s may run: %w", DisplayName(configFile), name, err)
	}
	facts["config_sha256"] = config
	loaded := make(map[string]string, len(configs))
	for _, path := range configs {
		sum, err := t.hash(path)
		if err != nil {
			return project.ExecSite{}, fmt.Errorf("read %s before asking whether %s may run: %w", DisplayName(path), name, err)
		}
		loaded[path] = sum
	}
	facts["configs_sha256"] = loaded
	return project.FormatterExecSite(configFile, name, command, facts), nil
}

// hash returns the SHA-256 of the file at path, with symbolic links resolved.
func (t *formatterTrust) hash(path string) (string, error) {
	if sum, ok := t.hashed[path]; ok {
		return sum, nil
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	f, err := os.Open(resolved)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	sum := hex.EncodeToString(h.Sum(nil))
	t.hashed[path] = sum
	return sum, nil
}

// printFormatterTrustPrompt states the facts the answer depends on: the file
// that selects the formatter, what would run, and what it can reach.
func printFormatterTrustPrompt(w io.Writer, site project.ExecSite) {
	fmt.Fprintf(w, "\n%s selects the formatter a comment edit runs on this project's files:\n\n", site.Where)
	fmt.Fprintln(w, formatExecSites([]project.ExecSite{site}))
	fmt.Fprintln(w, "\nIt runs with your privileges and your environment, including any provider API")
	fmt.Fprintln(w, "keys kapi can read, and it runs code the project controls: its executable and")
	fmt.Fprintln(w, "the configuration it loads. Approve only if you trust the source of this project.")
	fmt.Fprintf(w, "The answer is remembered in %s and is asked again if the formatter, what runs it or its configuration changes.\n\n", ExecTrustPath())
}
