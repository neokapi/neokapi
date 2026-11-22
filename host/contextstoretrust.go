package host

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/neokapi/neokapi/core/contextop"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/host/output"
)

// A recipe with `context: {backend: file}` names a folder that a sync writes
// the project's log to and merges into the person's context. A cloned recipe
// can name any folder, so the first sync through one shows it and asks. The
// answer is kept per user and per project in the execution-trust record
// (exectrust.go), keyed to the folder's path, so a changed path asks again.
// Nothing touches the folder before the answer, and an agent cannot give it.

// contextStoreKeySuffix marks a trust record entry as a context-store
// approval: the recipe's own key, which an exec decision uses, with this
// suffix.
const contextStoreKeySuffix = "#context-store"

// contextStoreKey is the trust record key for the recipe's folder approval.
func contextStoreKey(recipePath string) string {
	return execTrustKey(recipePath) + contextStoreKeySuffix
}

// contextStoreDigest fingerprints the folder an approval covers: its
// absolute, cleaned path. A recipe that names another folder, or the same
// relative path from a moved project, asks again.
func contextStoreDigest(location string) string {
	sum := sha256.Sum256([]byte(filepath.Clean(location)))
	return "sha256:" + hex.EncodeToString(sum[:])
}

// ErrContextStoreNotApproved is the error a sync through an unapproved folder
// returns, wrapped by ContextStoreApprovalError.
var ErrContextStoreNotApproved = errors.New("the project's context folder is not approved")

// ContextStoreApprovalError says which folder needs approving and how.
type ContextStoreApprovalError struct {
	Recipe   string
	Location string
	// Reason is why nobody was asked: no terminal, or an agent running.
	Reason string
}

func (e *ContextStoreApprovalError) Error() string {
	return fmt.Sprintf("%s keeps the project's context in the folder %s, which kapi has not been allowed to use: "+
		"syncing would write the project's decisions, terms and voice there and merge in whatever the folder holds. %s. "+
		"A person approves it with kapi trust allow --context-store -p %s",
		e.Recipe, e.Location, e.Reason, e.Recipe)
}

func (e *ContextStoreApprovalError) Unwrap() error { return ErrContextStoreNotApproved }

// contextStoreApproved reports whether the person allowed the recipe's
// folder at location.
func contextStoreApproved(recipePath, location string) bool {
	decision, found := lookupExecTrust(contextStoreKey(recipePath), contextStoreDigest(location))
	return found && decision == execTrustAllow
}

// recordContextStoreApproval remembers that the person allowed the folder.
func recordContextStoreApproval(recipePath, location string) error {
	return recordExecTrust(contextStoreKey(recipePath), contextStoreDigest(location), execTrustAllow)
}

// agentRunning reports an agent driving this process: it serves an MCP
// client, the actor resolves to an agent, or an agent host's marker is set
// beside KAPI_ACTOR=person, which does not make an agent's answer a person's.
func (a *App) agentRunning() bool {
	if a.servingMCP {
		return true
	}
	actor, err := a.commandActor()
	return err != nil || actor.Actor.Kind == contextop.ActorAgent || actor.PersonOverride
}

// requireContextStore lets a sync through a folder go ahead only once the
// person allowed that folder. For every other backend it returns nil. With
// ask set and a person at a terminal, it asks and records the answer;
// otherwise it refuses with the command that approves.
func (a *App) requireContextStore(layout project.Layout, info ContextBackendInfo, ask bool) error {
	if info.Kind != project.ContextBackendFile {
		return nil
	}
	location := info.Location
	if contextStoreApproved(layout.RecipePath, location) {
		return nil
	}
	refuse := func(reason string) error {
		return &ContextStoreApprovalError{Recipe: layout.RecipePath, Location: location, Reason: reason}
	}
	if a.agentRunning() {
		return refuse("An agent is running this command, and only a person can allow a folder")
	}
	// A surface that said who acts (ActAs: the desktop) asks through its own
	// dialog before it syncs, never at a terminal it may have been started
	// from.
	if !ask || a.actingActor != nil {
		return refuse("Nothing here asks for the approval")
	}
	isTTY := a.isTTY
	if isTTY == nil {
		isTTY = defaultIsStdinTTY
	}
	if !isTTY() {
		return refuse("No terminal is attached, so there is nobody to ask")
	}
	printContextStorePrompt(os.Stderr, layout.RecipePath, location)
	ok, err := ConfirmDefaultNo(os.Stdin, os.Stderr, "Allow this project to keep its context in that folder? [y/N] ")
	if err != nil {
		return err
	}
	if !ok {
		return refuse("It was not allowed")
	}
	if err := recordContextStoreApproval(layout.RecipePath, location); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not record the answer in %s: %v\n", ExecTrustPath(), err)
	}
	return nil
}

// printContextStorePrompt states what the answer allows.
func printContextStorePrompt(w interface{ Write([]byte) (int, error) }, recipePath, location string) {
	fmt.Fprintf(w, "\n%s keeps the project's context in a folder:\n\n  %s\n\n"+
		"kapi writes the project's decisions, terms and voice there, and merges in what other\n"+
		"machines put there. Anyone who can write to that folder can add to this project's context.\n\n",
		recipePath, location)
}

// ContextStoreLocation is the folder the recipe at recipePath keeps its
// context in, resolved to an absolute path, and "" for any other backend.
func ContextStoreLocation(recipePath string) (string, error) {
	layout, err := project.LayoutFor(recipePath)
	if err != nil {
		return "", err
	}
	info, err := resolveBackend(layout)
	if err != nil {
		return "", err
	}
	if info.Kind != project.ContextBackendFile {
		return "", nil
	}
	return info.Location, nil
}

// ApproveContextStore records that the person allows the recipe's folder,
// after the person saw it (the CLI's `kapi trust allow --context-store`, the
// desktop's dialog). It refuses when an agent is running.
func (a *App) ApproveContextStore(recipePath string) (string, error) {
	if a.agentRunning() {
		return "", errors.New("an agent is running this command, and only a person can allow a folder for a project's context")
	}
	location, err := ContextStoreLocation(recipePath)
	if err != nil {
		return "", err
	}
	if location == "" {
		return "", fmt.Errorf("%s keeps no context in a folder, so there is nothing to allow", recipePath)
	}
	if err := recordContextStoreApproval(recipePath, location); err != nil {
		return "", err
	}
	return location, nil
}

// isContextStoreKey reports a trust record key for a folder approval.
func isContextStoreKey(key string) bool { return strings.HasSuffix(key, contextStoreKeySuffix) }

// ContextStoreAllow is `kapi trust allow --context-store`: it shows the
// folder the recipe keeps its context in and records an allow for it, after
// a person confirms at a terminal or passes --yes. An agent cannot allow one.
func (a *App) ContextStoreAllow(cmd Command, recipePath string) (output.TrustAllowOutput, error) {
	key := execTrustKey(recipePath)
	out := output.TrustAllowOutput{Path: key, Sites: []output.TrustSite{}, Record: ExecTrustPath()}
	if a.agentRunning() {
		return out, errors.New("an agent is running this command, and only a person can allow a folder for a project's context")
	}
	location, err := ContextStoreLocation(key)
	if err != nil {
		return out, err
	}
	if location == "" {
		out.Reason = "the recipe keeps no context in a folder, so there is nothing to allow"
		return out, nil
	}
	out.ContextStore = location
	printContextStorePrompt(cmd.ErrOrStderr(), key, location)
	if !a.AssumeYes {
		isTTY := a.isTTY
		if isTTY == nil {
			isTTY = defaultIsStdinTTY
		}
		if !isTTY() {
			return out, fmt.Errorf("no terminal is attached, so there is nobody to confirm; pass --yes to allow %s", location)
		}
		ok, err := ConfirmDefaultNo(cmd.InOrStdin(), cmd.ErrOrStderr(), "Allow this project to keep its context in that folder? [y/N] ")
		if err != nil {
			return out, err
		}
		if !ok {
			out.Reason = "not confirmed"
			return out, nil
		}
	}
	if err := recordContextStoreApproval(key, location); err != nil {
		return out, err
	}
	out.Recorded = true
	return out, nil
}

// ContextStoreRevoke is `kapi trust revoke --context-store`: it withdraws the
// folder approval recorded for the recipe at path.
func (a *App) ContextStoreRevoke(path string) (output.TrustRevokeOutput, error) {
	key := execTrustRevokeKey(path) + contextStoreKeySuffix
	out := output.TrustRevokeOutput{Path: strings.TrimSuffix(key, contextStoreKeySuffix), Record: ExecTrustPath()}
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

// ContextStoreAllowed reports whether the person allowed the folder the
// recipe at recipePath keeps its context in now. A recipe with no folder
// backend has nothing to allow and reports false.
func ContextStoreAllowed(recipePath string) bool {
	location, err := ContextStoreLocation(recipePath)
	return err == nil && location != "" && contextStoreApproved(recipePath, location)
}
