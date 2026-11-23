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
// same path, or a path that leads to another folder, asks again. A recipe
// with no `id:` (one written before kapi init minted ids) contributes an
// empty id, so for it the recipe's path and the folder alone identify the
// approval. The id is plain text in the recipe, so it tells projects apart
// rather than proving which one this is.
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
func resolveContextFolder(layout project.Layout, location string) (resolved, existing string, err error) {
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
				return "", "", err
			}
			if info.Mode()&fs.ModeSymlink != 0 {
				return "", "", fmt.Errorf("the context folder %s is inside the project and %s is a link: "+
					"name the folder itself, or a folder outside the project", location, p)
			}
		}
	}
	p, rest := location, ""
	for {
		real, err := filepath.EvalSymlinks(p)
		if err == nil {
			return filepath.Join(real, rest), real, nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return "", "", err
		}
		parent := filepath.Dir(p)
		if parent == p {
			return location, "", nil
		}
		rest = filepath.Join(filepath.Base(p), rest)
		p = parent
	}
}

// contextFolder is a folder the recipe names, as an approval covers it.
type contextFolder struct {
	// Resolved is the folder with every link resolved.
	Resolved string
	// Existing is the nearest part of Resolved that exists: Resolved itself,
	// or the ancestor a first sync creates the folder under.
	Existing string
	// Info identifies the directory at Resolved when it exists, taken as it
	// is resolved, so the remote can refuse another directory swapped in
	// before it opens the folder.
	Info fs.FileInfo
	// Digest is what an approval of the folder carries.
	Digest string
}

// exists reports whether the folder is there yet.
func (f contextFolder) exists() bool { return f.Existing == f.Resolved }

// contextStoreFingerprint resolves the folder the recipe names and the digest
// an approval of it carries.
func contextStoreFingerprint(layout project.Layout, location string) (contextFolder, error) {
	resolved, existing, err := resolveContextFolder(layout, location)
	if err != nil {
		return contextFolder{}, err
	}
	folder := contextFolder{Resolved: resolved, Existing: existing}
	if folder.exists() {
		if folder.Info, err = os.Stat(resolved); err != nil {
			return contextFolder{}, err
		}
	}
	proj, err := project.LoadWithOptions(layout.RecipePath, project.LoadOptions{SkipRequiresCheck: true})
	if err != nil {
		return contextFolder{}, fmt.Errorf("load project: %w", err)
	}
	folder.Digest = contextStoreDigest(proj.ID, resolved)
	return folder, nil
}

// createContextFolder makes an approved folder that does not exist yet,
// beneath the existing ancestor it was resolved under and through a root
// opened there, and resolves it again. The approval covers the folder that
// is there afterwards: one that resolves anywhere else is refused.
func createContextFolder(layout project.Layout, location string, folder contextFolder) (contextFolder, error) {
	if folder.exists() || folder.Existing == "" {
		return folder, nil
	}
	parent, err := os.OpenRoot(folder.Existing)
	if err != nil {
		return contextFolder{}, err
	}
	defer func() { _ = parent.Close() }()
	rel, err := filepath.Rel(folder.Existing, folder.Resolved)
	if err != nil {
		return contextFolder{}, err
	}
	if err := parent.MkdirAll(rel, 0o755); err != nil {
		return contextFolder{}, fmt.Errorf("create the context folder %s: %w", folder.Resolved, err)
	}
	created, err := contextStoreFingerprint(layout, location)
	if err != nil {
		return contextFolder{}, err
	}
	if created.Digest != folder.Digest || !created.exists() {
		return contextFolder{}, fmt.Errorf("the context folder was created under %q but now resolves to %q, not the folder that was allowed (%q); run the sync again to be asked about it",
			folder.Existing, created.Resolved, folder.Resolved)
	}
	return created, nil
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
func (a *App) requireContextStore(layout project.Layout, info ContextBackendInfo, ask bool) (contextFolder, error) {
	if info.Kind != project.ContextBackendFile {
		return contextFolder{}, nil
	}
	key := contextStoreKey(layout.RecipePath)
	refuse := func(location, reason string) error {
		return &ContextStoreApprovalError{Recipe: layout.RecipePath, Location: location, Reason: reason}
	}
	// A sync (ask set) creates an approved folder that is not there yet; a
	// status read leaves it as it is.
	approved := func(folder contextFolder) (contextFolder, error) {
		if !ask {
			return folder, nil
		}
		return createContextFolder(layout, info.Location, folder)
	}
	if rec, ok := loadExecTrust().Projects[key]; ok && rec.Decision == execTrustAllow {
		folder, err := contextStoreFingerprint(layout, info.Location)
		if err != nil {
			return contextFolder{}, err
		}
		if folder.Digest == rec.Digest {
			return approved(folder)
		}
	}
	if a.agentRunning() {
		return contextFolder{}, refuse(info.Location, "An agent is running this command, and only a person can allow a folder")
	}
	// A surface that said who acts (ActAs: the desktop) asks through its own
	// dialog before it syncs, never at a terminal it may have been started
	// from.
	if !ask || a.actingActor != nil {
		return contextFolder{}, refuse(info.Location, "Nothing here asks for the approval")
	}
	isTTY := a.isTTY
	if isTTY == nil {
		isTTY = defaultIsStdinTTY
	}
	if !isTTY() {
		return contextFolder{}, refuse(info.Location, "No terminal is attached, so there is nobody to ask")
	}
	folder, err := contextStoreFingerprint(layout, info.Location)
	if err != nil {
		return contextFolder{}, err
	}
	printContextStorePrompt(os.Stderr, layout.RecipePath, folder)
	ok, err := ConfirmDefaultNo(os.Stdin, os.Stderr, "Allow this project to keep its context in that folder? [y/N] ")
	if err != nil {
		return contextFolder{}, err
	}
	if !ok {
		return contextFolder{}, refuse(folder.Resolved, "It was not allowed")
	}
	if err := recordExecTrust(key, folder.Digest, execTrustAllow); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not record the answer in %s: %v\n", ExecTrustPath(), err)
	}
	return approved(folder)
}

// contextStoreTrustNotice is what the prompt and the Sharing guide say about
// what an approval allows.
const contextStoreTrustNotice = "kapi writes the project's decisions, terms and voice there, and merges in what other\n" +
	"machines put there. Anyone who can write to that folder can add to this project's context.\n"

// printContextStorePrompt states what the answer allows, and where a folder
// that is not there yet will be created.
func printContextStorePrompt(w interface{ Write([]byte) (int, error) }, recipePath string, folder contextFolder) {
	// The resolved path follows links someone else may have written, so it is
	// quoted: a control character in it cannot reach the terminal.
	fmt.Fprintf(w, "\n%s keeps the project's context in a folder:\n\n  %q\n\n", recipePath, folder.Resolved)
	if !folder.exists() && folder.Existing != "" {
		fmt.Fprintf(w, "The folder does not exist yet. The first sync will create it under %q.\n\n", folder.Existing)
	}
	fmt.Fprintf(w, "%s\n", contextStoreTrustNotice)
}

// contextStoreFor resolves the folder the recipe at recipePath keeps its
// context in and the digest an approval of it carries. location is "" for
// any other backend.
func contextStoreFor(recipePath string) (contextFolder, error) {
	layout, err := project.LayoutFor(recipePath)
	if err != nil {
		return contextFolder{}, err
	}
	info, err := resolveBackend(layout)
	if err != nil {
		return contextFolder{}, err
	}
	if info.Kind != project.ContextBackendFile {
		return contextFolder{}, nil
	}
	return contextStoreFingerprint(layout, info.Location)
}

// ContextStoreLocation is the folder the recipe at recipePath keeps its
// context in, with every link resolved, and "" for any other backend. It
// reads the path, never the folder's contents; a person asked to allow the
// folder sees this.
func ContextStoreLocation(recipePath string) (string, error) {
	folder, err := contextStoreFor(recipePath)
	return folder.Resolved, err
}

// ContextStoreAllowed reports whether the person allowed the folder the
// recipe at recipePath keeps its context in now. A recipe with no folder
// backend has nothing to allow and reports false.
func ContextStoreAllowed(recipePath string) bool {
	rec, ok := loadExecTrust().Projects[contextStoreKey(recipePath)]
	if !ok || rec.Decision != execTrustAllow {
		return false
	}
	folder, err := contextStoreFor(recipePath)
	return err == nil && folder.Resolved != "" && folder.Digest == rec.Digest
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
	folder, err := contextStoreFor(recipePath)
	if err != nil {
		return "", err
	}
	if folder.Resolved == "" {
		return "", fmt.Errorf("%s keeps no context in a folder, so there is nothing to allow", recipePath)
	}
	if shown != "" && filepath.Clean(shown) != folder.Resolved {
		return "", fmt.Errorf("%s now keeps its context in %s, not the folder shown (%s): look at the folder again before allowing it",
			recipePath, folder.Resolved, shown)
	}
	if err := recordExecTrust(contextStoreKey(recipePath), folder.Digest, execTrustAllow); err != nil {
		return "", err
	}
	return folder.Resolved, nil
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
	folder, err := contextStoreFor(key)
	if err != nil {
		return out, err
	}
	if folder.Resolved == "" {
		out.Reason = "the recipe keeps no context in a folder, so there is nothing to allow"
		return out, nil
	}
	location := folder.Resolved
	out.ContextStore = location
	printContextStorePrompt(cmd.ErrOrStderr(), key, folder)
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
	if err := recordExecTrust(contextStoreKey(key), folder.Digest, execTrustAllow); err != nil {
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
