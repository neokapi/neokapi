package host

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/host/output"
)

// The `kapi trust` family reads and edits the execution-trust record that
// ensureExecTrust writes. Exec is the only trust decision kapi keeps, so the
// family is flat: every verb is about the exec surface, and the record keeps
// the layout exectrust.go gives it.

// ExecTrustList reports every recorded decision, in path order, with how each
// stands against what is at its path now.
func (a *App) ExecTrustList() output.TrustListOutput {
	rec := loadExecTrust()
	keys := make([]string, 0, len(rec.Projects))
	for k := range rec.Projects {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := output.TrustListOutput{Record: ExecTrustPath(), Entries: make([]output.TrustListEntry, 0, len(keys))}
	for _, key := range keys {
		r := rec.Projects[key]
		e := output.TrustListEntry{
			Path:       key,
			Kind:       execTrustEntryKind(key),
			Decision:   r.Decision,
			RecordedAt: r.RecordedAt,
			Digest:     r.Digest,
		}
		e.Status, e.Detail = execTrustStanding(key, e.Kind, r.Digest)
		out.Entries = append(out.Entries, e)
	}
	return out
}

// execTrustEntryKind tells a recipe entry from a formatter one by the key: the
// recipe arm keys by the recipe file, the formatter arm by the configuration
// file that selected the formatter.
func execTrustEntryKind(key string) string {
	if filepath.Base(key) == project.RecipeFileName {
		return "recipe"
	}
	return "formatter"
}

// execTrustStanding compares a recorded digest with what the file at key
// would run now.
func execTrustStanding(key, kind, digest string) (status, detail string) {
	if _, err := os.Stat(key); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return output.TrustStatusMissing, ""
		}
		return output.TrustStatusUnreadable, err.Error()
	}
	if kind != "recipe" {
		return output.TrustStatusUnverified, "a formatter decision; its digest covers the formatter's executable and configuration, which kapi apply checks at the edit"
	}
	proj, err := project.LoadWithOptions(key, project.LoadOptions{SkipRequiresCheck: true})
	if err != nil {
		return output.TrustStatusUnreadable, err.Error()
	}
	sites := project.ExecSurface(proj)
	if len(sites) == 0 {
		return output.TrustStatusChanged, "the recipe no longer runs commands"
	}
	if project.ExecSurfaceDigest(sites) == digest {
		return output.TrustStatusCurrent, ""
	}
	return output.TrustStatusChanged, "the recipe changed what it runs; the next run asks again"
}

// ExecTrustShow reports what the recipe at recipePath would run and the
// decision that applies to it.
func (a *App) ExecTrustShow(recipePath string) (output.TrustShowOutput, error) {
	key := execTrustKey(recipePath)
	proj, err := project.LoadWithOptions(key, project.LoadOptions{SkipRequiresCheck: true})
	if err != nil {
		return output.TrustShowOutput{}, err
	}
	sites := project.ExecSurface(proj)
	out := output.TrustShowOutput{
		Path:       key,
		Sites:      trustSites(sites),
		Digest:     project.ExecSurfaceDigest(sites),
		EnvGranted: execTrustEnvGranted(),
		Record:     ExecTrustPath(),
	}
	if r, ok := loadExecTrust().Projects[key]; ok {
		out.Recorded = &output.TrustRecorded{Decision: r.Decision, RecordedAt: r.RecordedAt, Digest: r.Digest}
	}
	switch {
	case len(sites) == 0:
		out.Status = output.TrustShowNothingToDecide
	case out.Recorded == nil:
		out.Status = output.TrustShowUndecided
	case out.Recorded.Digest != out.Digest:
		out.Status = output.TrustShowChanged
	case out.Recorded.Decision == execTrustAllow:
		out.Status = output.TrustShowAllowed
	default:
		out.Status = output.TrustShowDeclined
	}
	return out, nil
}

// ExecTrustRevoke withdraws the decision recorded for path, which names a
// recipe, its directory, or the configuration file a formatter decision is
// keyed by. Withdrawing a decision nobody recorded is not an error.
func (a *App) ExecTrustRevoke(path string) (output.TrustRevokeOutput, error) {
	key := execTrustRevokeKey(path)
	out := output.TrustRevokeOutput{Path: key, Record: ExecTrustPath()}
	rec := loadExecTrust()
	r, ok := rec.Projects[key]
	if !ok {
		return out, nil
	}
	delete(rec.Projects, key)
	if err := saveExecTrust(rec); err != nil {
		return out, err
	}
	out.Removed, out.Decision = true, r.Decision
	return out, nil
}

// execTrustRevokeKey turns what a person named into the record's key: a
// directory stands for the recipe in it.
func execTrustRevokeKey(path string) string {
	if info, err := os.Stat(path); err == nil && info.IsDir() {
		path = filepath.Join(path, project.RecipeFileName)
	}
	return execTrustKey(path)
}

// ExecTrustAllow records an allow for what the recipe at recipePath runs now,
// without starting a run. It shows the surface first and confirms at a
// terminal; with nobody to confirm it records nothing unless --yes was given.
// The command is the explicit act the gate's own prompt stands for, which is
// why --yes confirms it here and never grants the gate itself.
func (a *App) ExecTrustAllow(cmd Command, recipePath string) (output.TrustAllowOutput, error) {
	key := execTrustKey(recipePath)
	proj, err := project.LoadWithOptions(key, project.LoadOptions{SkipRequiresCheck: true})
	if err != nil {
		return output.TrustAllowOutput{}, err
	}
	sites := project.ExecSurface(proj)
	out := output.TrustAllowOutput{Path: key, Sites: trustSites(sites), Record: ExecTrustPath()}
	if len(sites) == 0 {
		out.Reason = "the recipe runs no commands, so there is nothing to approve"
		return out, nil
	}
	out.Digest = project.ExecSurfaceDigest(sites)

	errW := cmd.ErrOrStderr()
	printExecTrustPrompt(errW, key, sites)
	if !a.AssumeYes {
		isTTY := a.isTTY
		if isTTY == nil {
			isTTY = defaultIsStdinTTY
		}
		if !isTTY() {
			return out, fmt.Errorf("no terminal is attached, so there is nobody to confirm; pass --yes to approve what kapi trust show -p %s lists", key)
		}
		ok, err := ConfirmDefaultNo(cmd.InOrStdin(), errW, "Allow this project to run these commands? [y/N] ")
		if err != nil {
			return out, err
		}
		if !ok {
			out.Reason = "not confirmed"
			return out, nil
		}
	}
	if err := recordExecTrust(key, out.Digest, execTrustAllow); err != nil {
		return out, err
	}
	out.Recorded = true
	return out, nil
}

// trustSites projects the surface into the output shape.
func trustSites(sites []project.ExecSite) []output.TrustSite {
	out := make([]output.TrustSite, 0, len(sites))
	for _, s := range sites {
		out = append(out, output.TrustSite{Where: s.Where, Kind: s.Kind, Name: s.Name, Detail: s.Detail})
	}
	return out
}
