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

// mcpFormatterTrust is the trust MCP apply_edits holds formatters to: a
// recorded allow and nothing else. Nobody is present to ask, and the
// environment grant does not apply, because a project's own MCP client
// configuration can set the server's environment.
func mcpFormatterTrust() *formatterTrust {
	trust := newFormatterTrust(false)
	trust.unanswered = "apply_edits never asks, so run kapi apply with a comment edit in this project from a terminal to answer once"
	return trust
}

// allow reports whether command, the formatter configFile selects under name,
// may run. It returns nil when it may, and otherwise an error that says why not
// and how to answer.
func (t *formatterTrust) allow(configFile, name string, command []string) error {
	site, err := t.site(configFile, name, command)
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
// SHA-256 of the executable command runs and of configFile, so a change to
// either asks again. Modules the executable or the configuration loads are not
// read, and a change to them keeps a decision.
func (t *formatterTrust) site(configFile, name string, command []string) (project.ExecSite, error) {
	executable, err := t.hash(command[0])
	if err != nil {
		return project.ExecSite{}, fmt.Errorf("read the %s executable before asking whether it may run: %w", name, err)
	}
	config, err := t.hash(configFile)
	if err != nil {
		return project.ExecSite{}, fmt.Errorf("read %s before asking whether %s may run: %w", DisplayName(configFile), name, err)
	}
	return project.FormatterExecSite(configFile, name, command, map[string]any{
		"executable_sha256": executable,
		"config_sha256":     config,
	}), nil
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
	fmt.Fprintf(w, "The answer is remembered in %s and is asked again if the formatter or that file changes.\n\n", ExecTrustPath())
}
