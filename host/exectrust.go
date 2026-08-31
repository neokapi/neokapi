package host

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/neokapi/neokapi/core/project"
)

// Execution trust: a recipe that names an exec-class step runs code chosen by
// whoever wrote the recipe, with the privileges and the environment of whoever
// runs kapi. Because a recipe is discovered by an upward walk from the working
// directory (core/project.ResolveLayout), entering a directory is enough to
// bind one — so the decision to let a particular project run commands is taken
// once, by a person, and remembered.
//
// This is the recipe arm of a classification the codebase already applies
// elsewhere: MCP withholds the same tools from the agent surface even under
// --all-tools (AD-037), and a recipe arriving inside a .kpz has them stripped
// on ingest (kpz.SanitizeRecipe). Those two surfaces can refuse outright
// because no person is present to ask. A recipe in the working directory is
// the one case where the answer legitimately depends on the user, so it is the
// one case that asks.

// Trust decisions.
const (
	execTrustAllow = "allow"
	execTrustDeny  = "deny"
)

// execTrustEnvVar grants execution trust for the current process without
// consulting or writing the record. It exists for CI, where no terminal is
// attached and the pipeline author — not the repository being processed — is
// the one deciding.
//
// It is deliberately NOT the global --yes flag. --yes means "do not stop for
// prompts I would obviously accept", and it is already passed by scripts that
// want unattended plugin installation; reusing it would mean every existing
// automation silently acquired the right to execute whatever a checked-out
// recipe names, on the day this gate shipped. Same reasoning AD-037 applied to
// `kapi mcp --all-tools`: "show me everything" and "run arbitrary commands" are
// different classes of decision, and bundling them grants the second by
// accident.
const execTrustEnvVar = "KAPI_TRUST_EXEC"

// execTrustFileName is the record's name under ConfigDir().
//
// It lives in the kapi config root, not in the project's own `.kapi/` state
// directory: `.kapi/` is documented as safe to delete and regenerate from the
// block store (core/project.StateManifest), so a trust record kept there would
// evaporate on a routine clean and re-arm the prompt — or, worse, ship inside
// the project for the next person to inherit. ConfigDir() honours
// KAPI_CONFIG_DIR, so a run isolated per the in-repo contract reads and writes
// a throwaway record rather than the developer's real decisions.
const execTrustFileName = "trust.json"

// execTrustRecord is one project's remembered decision.
type execTrustRecord struct {
	// Digest fingerprints the exec sites that were shown when the decision was
	// taken. A recipe edit that changes what would run changes the digest, and
	// the decision no longer applies.
	Digest string `json:"digest"`
	// Decision is execTrustAllow or execTrustDeny.
	Decision string `json:"decision"`
	// RecordedAt is when the answer was given, for a person auditing the file.
	RecordedAt string `json:"recordedAt"`
}

// execTrustFile is the on-disk shape of the record, keyed by absolute recipe
// path. Path is part of the key on purpose: copying a project elsewhere, or
// cloning a second checkout of it, is a new decision rather than an inherited
// one.
type execTrustFile struct {
	Version  int                        `json:"version"`
	Projects map[string]execTrustRecord `json:"projects,omitempty"`
}

// ExecTrustPath returns the absolute path of the execution-trust record.
// Exported so the prompt and the refusal messages can name it — the file is
// plain JSON, and deleting an entry is how a decision is withdrawn.
func ExecTrustPath() string {
	return filepath.Join(ConfigDir(), execTrustFileName)
}

// execTrustEnvGranted reports whether the environment grants execution trust.
//
// Unlike KAPI_NO_PROJECT and KAPI_PLUGINS_DIR_ONLY, which treat any non-empty
// value as set, this requires an affirmative one. The convention is convenient
// for a switch whose worst outcome is a skipped lookup; for a switch that
// grants code execution, reading KAPI_TRUST_EXEC=0 as "yes" is the wrong way
// to be wrong.
func execTrustEnvGranted() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(execTrustEnvVar))) {
	case "1", "true", "yes":
		return true
	default:
		return false
	}
}

// execTrustKey normalises a recipe path into the record's key.
func execTrustKey(recipePath string) string {
	if abs, err := filepath.Abs(recipePath); err == nil {
		return filepath.Clean(abs)
	}
	return filepath.Clean(recipePath)
}

// loadExecTrust reads the record. A missing or unreadable file is not an
// error: it means nothing has been decided yet, which is the same starting
// state as an empty one.
func loadExecTrust() execTrustFile {
	f := execTrustFile{Version: 1, Projects: map[string]execTrustRecord{}}
	data, err := os.ReadFile(ExecTrustPath())
	if err != nil {
		return f
	}
	var parsed execTrustFile
	if err := json.Unmarshal(data, &parsed); err != nil {
		return f
	}
	if parsed.Projects == nil {
		parsed.Projects = map[string]execTrustRecord{}
	}
	parsed.Version = 1
	return parsed
}

// saveExecTrust writes the record, replacing it atomically. The file records
// security decisions, so it is owner-only and never group- or world-readable.
func saveExecTrust(f execTrustFile) error {
	path := ExecTrustPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return fmt.Errorf("encode trust record: %w", err)
	}
	data = append(data, '\n')
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write trust record: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("replace trust record: %w", err)
	}
	return nil
}

// lookupExecTrust returns the decision recorded for this recipe at this
// digest. A record whose digest no longer matches is reported as absent: what
// was approved is not what would run now.
func lookupExecTrust(recipePath, digest string) (string, bool) {
	rec, ok := loadExecTrust().Projects[execTrustKey(recipePath)]
	if !ok || rec.Digest != digest {
		return "", false
	}
	return rec.Decision, true
}

// recordExecTrust remembers a decision for this recipe at this digest.
func recordExecTrust(recipePath, digest, decision string) error {
	f := loadExecTrust()
	f.Projects[execTrustKey(recipePath)] = execTrustRecord{
		Digest:     digest,
		Decision:   decision,
		RecordedAt: time.Now().UTC().Format(time.RFC3339),
	}
	return saveExecTrust(f)
}

// ErrExecNotTrusted is returned when a recipe would run commands that the user
// has not approved. Callers surface it as-is: the message names the project,
// what it would run, and how to answer.
var ErrExecNotTrusted = errors.New("project not approved to run commands")

// ensureExecTrust is the gate. It is a no-op for the overwhelming majority of
// recipes, which name no exec-class step at all.
//
// Order of resolution:
//
//  1. No exec sites — nothing to decide.
//  2. KAPI_TRUST_EXEC set affirmatively — granted for this process, not
//     recorded (CI is stateless, and a container's config dir is not a place
//     to persist a decision).
//  3. A recorded decision at the current digest — honoured, silently.
//  4. A terminal — show what would run, ask, record the answer.
//  5. Otherwise — refuse. A non-interactive run has nobody to ask, and
//     assuming yes would make the gate a formality.
func (a *App) ensureExecTrust(recipePath string, proj *project.KapiProject, opts LoadProjectInteractiveOptions) error {
	sites := project.ExecSurface(proj)
	if len(sites) == 0 {
		return nil
	}
	digest := project.ExecSurfaceDigest(sites)

	if execTrustEnvGranted() {
		a.execTrustGranted = true
		return nil
	}

	switch decision, found := lookupExecTrust(recipePath, digest); {
	case found && decision == execTrustAllow:
		a.execTrustGranted = true
		return nil
	case found && decision == execTrustDeny:
		return fmt.Errorf("%w: %s was declined (edit or delete its entry in %s to answer again)",
			ErrExecNotTrusted, recipePath, ExecTrustPath())
	}

	out := opts.Out
	if out == nil {
		out = os.Stderr
	}
	isTTY := opts.IsTTYFn
	if isTTY == nil {
		isTTY = defaultIsStdinTTY
	}

	if !isTTY() {
		return fmt.Errorf("%w: %s\n%s\nno terminal is attached, so there is nobody to ask — run kapi once interactively to answer, or set %s=1 if this project is trusted by whoever configured this environment",
			ErrExecNotTrusted, recipePath, formatExecSites(sites), execTrustEnvVar)
	}

	in := opts.In
	if in == nil {
		in = os.Stdin
	}
	printExecTrustPrompt(out, recipePath, sites)

	ok, err := ConfirmDefaultNo(in, out, "Allow this project to run these commands? [y/N] ")
	if err != nil {
		return err
	}
	decision := execTrustDeny
	if ok {
		decision = execTrustAllow
	}
	if rerr := recordExecTrust(recipePath, digest, decision); rerr != nil {
		// A decision that cannot be written is still a decision for this run;
		// say so rather than failing an approved run on a config-dir problem.
		fmt.Fprintf(out, "Warning: could not record the answer in %s: %v\n", ExecTrustPath(), rerr)
	}
	if !ok {
		return fmt.Errorf("%w: %s", ErrExecNotTrusted, recipePath)
	}
	a.execTrustGranted = true
	return nil
}

// printExecTrustPrompt states the facts the answer depends on: which recipe,
// what it would run, and what those commands would have access to.
func printExecTrustPrompt(w io.Writer, recipePath string, sites []project.ExecSite) {
	fmt.Fprintf(w, "\n%s runs commands chosen by the recipe:\n\n", recipePath)
	fmt.Fprintln(w, formatExecSites(sites))
	fmt.Fprintln(w, "They run with your privileges and your environment, including any provider")
	fmt.Fprintln(w, "API keys kapi can read. Approve only if you trust the source of this project.")
	fmt.Fprintf(w, "The answer is remembered in %s and is asked again if the recipe changes what it runs.\n\n", ExecTrustPath())
}

// formatExecSites renders the surface as one indented line per site.
func formatExecSites(sites []project.ExecSite) string {
	var b strings.Builder
	for _, s := range sites {
		fmt.Fprintf(&b, "  %s  %s", s.Where, s.Name)
		if s.Detail != "" {
			fmt.Fprintf(&b, "  %s", s.Detail)
		}
		b.WriteByte('\n')
	}
	return strings.TrimRight(b.String(), "\n")
}

// checkExecToolAllowed is the backstop under tool construction. The prompt runs
// at project load, which every project-aware command reaches through
// LoadProjectInteractive — but a handful of internal paths load a recipe
// directly, and a tool that runs commands should not depend on which of them
// got there first. This refuses to build an exec-class tool from recipe-driven
// configuration unless trust is established.
//
// It never prompts: by the time a flow is assembling its steps there is no
// sensible place to stop and ask. It re-reads the record instead, so a project
// already approved keeps working on the paths that skip the prompt.
//
// `kapi exec <tool>` is unaffected — it constructs tools straight from the
// registry with an argv the user typed, which is the user's own intent rather
// than a file's.
func (a *App) checkExecToolAllowed(toolName string) error {
	if !project.IsExecClassTool(toolName) {
		return nil
	}
	if a.execTrustGranted || execTrustEnvGranted() {
		return nil
	}
	if a.execTrustRecordedAllow() {
		a.execTrustGranted = true
		return nil
	}
	return fmt.Errorf("%w: the %q step has not been approved for this project (run kapi interactively once to answer, or set %s=1)",
		ErrExecNotTrusted, toolName, execTrustEnvVar)
}

// execTrustRecordedAllow reports whether the active project has a recorded
// allow at its current digest. Returns false when no project is bound, which
// is the safe answer: recipe-driven configuration with no identifiable recipe
// has no decision attached to it.
func (a *App) execTrustRecordedAllow() bool {
	if a.ProjectContext == nil || a.ProjectContext.Project == nil {
		return false
	}
	recipePath := filepath.Join(a.ProjectContext.ProjectDir, project.RecipeFileName)
	sites := project.ExecSurface(a.ProjectContext.Project)
	if len(sites) == 0 {
		return false
	}
	decision, found := lookupExecTrust(recipePath, project.ExecSurfaceDigest(sites))
	return found && decision == execTrustAllow
}
