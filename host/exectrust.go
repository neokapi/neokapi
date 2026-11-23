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

// Execution trust: a recipe that names an exec-class step runs code its author
// chose, with the privileges of whoever runs kapi. Entering a directory binds
// its recipe (core/project.ResolveLayout), so a person decides once per
// project whether it may run commands, and the answer is remembered.
//
// MCP withholds the same tools even under --all-tools (AD-037), and a recipe
// inside a .kpz has them stripped (kpz.SanitizeRecipe): nobody is there to
// ask. A recipe in the working directory is the case that asks.

// Trust decisions.
const (
	execTrustAllow = "allow"
	execTrustDeny  = "deny"
)

// execTrustEnvVar grants execution trust for the current process without
// consulting or writing the record. It exists for CI, where no terminal is
// attached and the pipeline's author decides, not the repository it checks out.
//
// It is not the global --yes flag. Scripts pass --yes for unattended plugin
// installs, and reading it as a grant would let each of them run whatever a
// checked-out recipe names. As with `kapi mcp --all-tools` (AD-037), "do not
// stop for prompts" and "run arbitrary commands" are different decisions.
const execTrustEnvVar = "KAPI_TRUST_EXEC"

// execTrustFileName is the record's name under ConfigDir().
//
// It lives in the kapi config root, not in the project's own `.kapi/` state
// directory: `.kapi/` is documented as safe to delete and regenerate from the
// block store (core/project.StateManifest). A trust record kept there would be
// lost on a routine clean, or ship inside the project for the next person to
// inherit. ConfigDir() honours
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
	// Files is the SHA-256 of each project file the commands name, as they
	// were when the answer was given, so the next question can say which
	// one changed (execSurfaceOf).
	Files map[string]string `json:"files,omitempty"`
	// Sites locates each command the answer was given about, so the next
	// question can say which one was added since.
	Sites []string `json:"sites,omitempty"`
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
// Exported so the prompt and the refusal messages can name it. The file is
// plain JSON; `kapi trust list` reads it and `kapi trust revoke` withdraws a
// decision (host/exectrustadmin.go).
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

// recordExecTrustSurface remembers a decision about the project's commands
// with the files it checked and the commands it was shown.
func recordExecTrustSurface(recipePath, decision string, surface execSurface) error {
	f := loadExecTrust()
	f.Projects[execTrustKey(recipePath)] = execTrustRecord{
		Digest:     surface.Digest,
		Decision:   decision,
		RecordedAt: time.Now().UTC().Format(time.RFC3339),
		Files:      surface.Files,
		Sites:      siteNames(surface.Sites),
	}
	return saveExecTrust(f)
}

// ErrExecNotTrusted is returned when a recipe would run commands that the user
// has not approved. Callers surface it as-is: the message names the project,
// what it would run, and how to answer.
var ErrExecNotTrusted = errors.New("project not approved to run commands")

// ensureExecTrust checks authorization for recipe-supplied commands.
// It applies the first matching rule:
//
//  1. Recipes without exec sites need no authorization, and a browser build
//     grants it to every recipe (execTrustImplied).
//  2. An affirmative KAPI_TRUST_EXEC grants access for this process without
//     persisting a decision.
//  3. A recorded decision for the current exec-surface digest applies.
//  4. An interactive terminal prompts and records the decision.
//  5. Other runs return ErrExecNotTrusted.
func (a *App) ensureExecTrust(recipePath string, proj *project.KapiProject, opts LoadProjectInteractiveOptions) error {
	if execTrustImplied || execTrustEnvGranted() {
		return nil
	}
	surface, err := execSurfaceOf(recipePath, proj)
	if err != nil {
		return fmt.Errorf("%w: %s: %w", ErrExecNotTrusted, recipePath, err)
	}
	if len(surface.Sites) == 0 {
		return nil
	}
	digest := surface.Digest

	switch decision, found := lookupExecTrust(recipePath, digest); {
	case found && decision == execTrustAllow:
		a.execTrust = execTrustAnswer{recipe: recipePath, proj: proj, digest: digest}
		return nil
	case found && decision == execTrustDeny:
		return fmt.Errorf("%w: %s was declined (run kapi trust revoke %s to answer again)",
			ErrExecNotTrusted, recipePath, recipePath)
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
		return execNotTrustedError(recipePath, surface, "no terminal is attached, so there is nobody to ask")
	}

	in := opts.In
	if in == nil {
		in = os.Stdin
	}
	printExecTrustPrompt(out, recipePath, surface)

	ok, err := ConfirmDefaultNo(in, out, "Allow this project to run these commands? [y/N] ")
	if err != nil {
		return err
	}
	decision := execTrustDeny
	if ok {
		decision = execTrustAllow
	}
	if rerr := recordExecTrustSurface(recipePath, decision, surface); rerr != nil {
		// A decision that cannot be written is still a decision for this run;
		// say so rather than failing an approved run on a config-dir problem.
		fmt.Fprintf(out, "Warning: could not record the answer in %s: %v\n", ExecTrustPath(), rerr)
	}
	if !ok {
		return fmt.Errorf("%w: %s", ErrExecNotTrusted, recipePath)
	}
	a.execTrust = execTrustAnswer{recipe: recipePath, proj: proj, digest: digest}
	return nil
}

// execNotTrustedError refuses the project's commands where nobody can answer,
// with what they run, what an answer would check, and the approving command.
func execNotTrustedError(recipePath string, surface execSurface, why string) error {
	return fmt.Errorf("%w: %s\n%s\n%s%s. Approve it from a terminal with kapi trust allow -p %s, or set %s=1 if this project is trusted by whoever configured this environment",
		ErrExecNotTrusted, recipePath, formatExecSites(surface.Sites), formatExecCoverage(surface, execTrustKey(recipePath)), why, recipePath, execTrustEnvVar)
}

// printExecTrustPrompt states the facts the answer depends on: which recipe,
// what it would run, and what those commands would have access to.
func printExecTrustPrompt(w io.Writer, recipePath string, surface execSurface) {
	fmt.Fprintf(w, "\n%s runs commands chosen by the project:\n\n", recipePath)
	fmt.Fprintln(w, formatExecSites(surface.Sites))
	fmt.Fprint(w, "\n"+formatExecCoverage(surface, execTrustKey(recipePath)))
	fmt.Fprintln(w, "They run with your privileges and your environment, including any provider")
	fmt.Fprintln(w, "API keys kapi can read. Approve only if you trust the source of this project.")
	fmt.Fprintf(w, "The answer is remembered in %s and is asked again if the project changes what it runs\n", ExecTrustPath())
	fmt.Fprintln(w, "or a checked file changes.")
	fmt.Fprintln(w, "kapi trust list shows the recorded answers and kapi trust revoke withdraws one.")
	fmt.Fprintln(w)
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
// LoadProjectInteractive, but a handful of internal paths load a recipe
// directly, and a tool that runs commands should not depend on which of them
// got there first. This refuses to build an exec-class tool from recipe-driven
// configuration unless trust is established.
//
// It never prompts: by the time a flow is assembling its steps there is no
// sensible place to stop and ask. It checks again at every build, reading the
// project's commands and named files (rehashing only a file whose size or
// modification time moved), so a long-running process (the desktop, a
// daemon) refuses a script changed after the answer, naming it.
//
// `kapi exec <tool>` is unaffected: it constructs tools straight from the
// registry with an argv the user typed, which is the user's own intent rather
// than a file's.
func (a *App) checkExecToolAllowed(toolName string) error {
	if !project.IsExecClassTool(toolName) {
		return nil
	}
	if execTrustImplied || execTrustEnvGranted() {
		return nil
	}
	recipePath, proj := a.execTrust.recipe, a.execTrust.proj
	if a.ProjectContext != nil && a.ProjectContext.Project != nil {
		recipePath, proj = filepath.Join(a.ProjectContext.ProjectDir, project.RecipeFileName), a.ProjectContext.Project
	}
	if proj == nil {
		return fmt.Errorf("%w: the %q step has not been approved for this project (approve it with kapi trust allow, or set %s=1)",
			ErrExecNotTrusted, toolName, execTrustEnvVar)
	}
	surface, err := execSurfaceOf(recipePath, proj)
	if err != nil {
		return fmt.Errorf("%w: %s: %w", ErrExecNotTrusted, recipePath, err)
	}
	if surface.Digest != "" && surface.Digest == a.execTrust.digest && execTrustKey(recipePath) == execTrustKey(a.execTrust.recipe) {
		return nil
	}
	if decision, found := lookupExecTrust(recipePath, surface.Digest); found && decision == execTrustAllow {
		return nil
	}
	return execNotTrustedError(recipePath, surface, fmt.Sprintf("the %q step has not been approved as the project stands now", toolName))
}

// execTrustAnswer is the project a decision in this process covered: the
// recipe, the project as loaded, and the digest of its commands then.
type execTrustAnswer struct {
	recipe string
	proj   *project.KapiProject
	digest string
}
