package host

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/neokapi/neokapi/core/contextop"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/host/output"
)

// A cloned recipe can name any folder as its context store, so the first sync
// through one shows the resolved folder and asks. The answer is kept in the
// execution-trust record, bound to the project's id and the resolved folder,
// and each sync resolves the folder again.
//
// A recipe and an MCP tool call cannot answer, and an assistant that
// announces itself is refused. An assistant with a shell acts with the
// person's permissions, and this gate does not contain it.

// contextStoreKeySuffix marks a trust record entry as a context-store
// approval: the recipe's own key, which an exec decision uses, with this
// suffix.
const contextStoreKeySuffix = "#context-store"

// contextStoreKey is the trust record key for the recipe's folder approval.
func contextStoreKey(recipePath string) string {
	return execTrustKey(recipePath) + contextStoreKeySuffix
}

// contextStoreDigest fingerprints what an approval covers: the project's id
// and the folder with every link resolved. Another repository cloned to the
// same path, or a path that leads to another folder, asks again.
func contextStoreDigest(projectID, resolved string) string {
	sum := sha256.Sum256([]byte(projectID + "\x00" + filepath.Clean(resolved)))
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

// checkContextPath refuses a folder path kapi would read other than the way
// its author meant: a leading `~` or an environment variable, which kapi
// expands neither of, so `~/team` would name a directory called `~` inside
// the project.
func checkContextPath(path string) error {
	if strings.HasPrefix(path, "~") || strings.Contains(path, "$") {
		return fmt.Errorf("the context folder %q starts with ~ or names an environment variable, and kapi expands neither: "+
			"write the folder's full path in kapi.yaml", path)
	}
	return nil
}

// resolveContextFolder resolves the folder at location, the absolute path the
// recipe names. Every link on the way is followed, and the part of the path
// that does not exist yet is kept as written beneath what does. A folder
// inside the project that is, or passes through, a link is refused: a pull
// from the repository's remote can replace a directory with a link.
func resolveContextFolder(layout project.Layout, location string) (string, error) {
	if rel, err := filepath.Rel(layout.Root, location); err == nil && rel != "." &&
		rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		p := layout.Root
		for part := range strings.SplitSeq(rel, string(filepath.Separator)) {
			p = filepath.Join(p, part)
			info, err := os.Lstat(p)
			if errors.Is(err, fs.ErrNotExist) {
				break
			}
			if err != nil {
				return "", err
			}
			if info.Mode()&fs.ModeSymlink != 0 {
				return "", fmt.Errorf("the context folder %s is inside the project and %s is a link: "+
					"name the folder itself, or a folder outside the project", location, p)
			}
		}
	}
	p, rest := location, ""
	for {
		resolved, err := filepath.EvalSymlinks(p)
		if err == nil {
			return filepath.Join(resolved, rest), nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(p)
		if parent == p {
			return location, nil
		}
		rest = filepath.Join(filepath.Base(p), rest)
		p = parent
	}
}

// contextStoreFingerprint resolves the folder the recipe names and the digest
// an approval of it carries.
func contextStoreFingerprint(layout project.Layout, location string) (resolved, digest string, err error) {
	resolved, err = resolveContextFolder(layout, location)
	if err != nil {
		return "", "", err
	}
	proj, err := project.LoadWithOptions(layout.RecipePath, project.LoadOptions{SkipRequiresCheck: true})
	if err != nil {
		return "", "", fmt.Errorf("load project: %w", err)
	}
	return resolved, contextStoreDigest(proj.ID, resolved), nil
}

// agentRunning reports an agent driving this process: it serves an MCP
// client, the actor resolves to an agent, or an agent host's marker is set
// beside KAPI_ACTOR=person, which does not make an agent's answer a person's.
// An assistant that runs kapi with neither marker is not detected.
func (a *App) agentRunning() bool {
	if a.servingMCP {
		return true
	}
	actor, err := a.commandActor()
	return err != nil || actor.Actor.Kind == contextop.ActorAgent || actor.PersonOverride
}

// requireContextStore lets a sync through a folder go ahead only once the
// person allowed that folder, and returns the resolved folder to open. For
// every other backend it returns "". With no allow recorded for the project
// the folder is not touched. With one recorded, the folder is resolved again
// and must still match. With ask set and a person at a terminal, it asks and
// records the answer; otherwise it refuses with the command that approves.
func (a *App) requireContextStore(layout project.Layout, info ContextBackendInfo, ask bool) (string, error) {
	if info.Kind != project.ContextBackendFile {
		return "", nil
	}
	key := contextStoreKey(layout.RecipePath)
	refuse := func(location, reason string) error {
		return &ContextStoreApprovalError{Recipe: layout.RecipePath, Location: location, Reason: reason}
	}
	if rec, ok := loadExecTrust().Projects[key]; ok && rec.Decision == execTrustAllow {
		resolved, digest, err := contextStoreFingerprint(layout, info.Location)
		if err != nil {
			return "", err
		}
		if digest == rec.Digest {
			return resolved, nil
		}
	}
	if a.agentRunning() {
		return "", refuse(info.Location, "An agent is running this command, and only a person can allow a folder")
	}
	// A surface that said who acts (ActAs: the desktop) asks through its own
	// dialog before it syncs, never at a terminal it may have been started
	// from.
	if !ask || a.actingActor != nil {
		return "", refuse(info.Location, "Nothing here asks for the approval")
	}
	isTTY := a.isTTY
	if isTTY == nil {
		isTTY = defaultIsStdinTTY
	}
	if !isTTY() {
		return "", refuse(info.Location, "No terminal is attached, so there is nobody to ask")
	}
	resolved, digest, err := contextStoreFingerprint(layout, info.Location)
	if err != nil {
		return "", err
	}
	printContextStorePrompt(os.Stderr, layout.RecipePath, resolved)
	ok, err := ConfirmDefaultNo(os.Stdin, os.Stderr, "Allow this project to keep its context in that folder? [y/N] ")
	if err != nil {
		return "", err
	}
	if !ok {
		return "", refuse(resolved, "It was not allowed")
	}
	if err := recordExecTrust(key, digest, execTrustAllow); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not record the answer in %s: %v\n", ExecTrustPath(), err)
	}
	return resolved, nil
}

// contextStoreTrustNotice is what the prompt and the Sharing guide say about
// what an approval allows.
const contextStoreTrustNotice = "kapi writes the project's decisions, terms and voice there, and merges in what other\n" +
	"machines put there. Anyone who can write to that folder can add to this project's context.\n"

// printContextStorePrompt states what the answer allows.
func printContextStorePrompt(w interface{ Write([]byte) (int, error) }, recipePath, location string) {
	fmt.Fprintf(w, "\n%s keeps the project's context in a folder:\n\n  %s\n\n%s\n", recipePath, location, contextStoreTrustNotice)
}

// contextStoreFor resolves the folder the recipe at recipePath keeps its
// context in and the digest an approval of it carries. location is "" for
// any other backend.
func contextStoreFor(recipePath string) (location, digest string, err error) {
	layout, err := project.LayoutFor(recipePath)
	if err != nil {
		return "", "", err
	}
	info, err := resolveBackend(layout)
	if err != nil {
		return "", "", err
	}
	if info.Kind != project.ContextBackendFile {
		return "", "", nil
	}
	return contextStoreFingerprint(layout, info.Location)
}

// ContextStoreLocation is the folder the recipe at recipePath keeps its
// context in, with every link resolved, and "" for any other backend. It
// reads the path, never the folder's contents; a person asked to allow the
// folder sees this.
func ContextStoreLocation(recipePath string) (string, error) {
	location, _, err := contextStoreFor(recipePath)
	return location, err
}

// ContextStoreAllowed reports whether the person allowed the folder the
// recipe at recipePath keeps its context in now. A recipe with no folder
// backend has nothing to allow and reports false.
func ContextStoreAllowed(recipePath string) bool {
	rec, ok := loadExecTrust().Projects[contextStoreKey(recipePath)]
	if !ok || rec.Decision != execTrustAllow {
		return false
	}
	location, digest, err := contextStoreFor(recipePath)
	return err == nil && location != "" && digest == rec.Digest
}

// ApproveContextStore records that the person allows the recipe's folder,
// after the person saw it (`kapi share --via folder`, the desktop's setup
// dialog). It refuses when an agent is running.
func (a *App) ApproveContextStore(recipePath string) (string, error) {
	return a.approveContextStore(recipePath, "")
}

// ApproveContextStoreAt records the allow only when the folder the recipe
// names resolves to shown, the location the person was shown, so an edit
// between the dialog and the click asks again.
func (a *App) ApproveContextStoreAt(recipePath, shown string) (string, error) {
	if shown == "" {
		return "", errors.New("name the folder that was shown")
	}
	return a.approveContextStore(recipePath, shown)
}

func (a *App) approveContextStore(recipePath, shown string) (string, error) {
	if a.agentRunning() {
		return "", errors.New("an agent is running this command, and only a person can allow a folder for a project's context")
	}
	location, digest, err := contextStoreFor(recipePath)
	if err != nil {
		return "", err
	}
	if location == "" {
		return "", fmt.Errorf("%s keeps no context in a folder, so there is nothing to allow", recipePath)
	}
	if shown != "" && filepath.Clean(shown) != location {
		return "", fmt.Errorf("%s now keeps its context in %s, not the folder shown (%s): look at the folder again before allowing it",
			recipePath, location, shown)
	}
	if err := recordExecTrust(contextStoreKey(recipePath), digest, execTrustAllow); err != nil {
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
	location, digest, err := contextStoreFor(key)
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
	if err := recordExecTrust(contextStoreKey(key), digest, execTrustAllow); err != nil {
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
