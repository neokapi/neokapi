package project

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// A checkout's sync state is bound to one server project: its sync cache
// records the server and the project it syncs with, beside the claim token
// that project issued. Two checkouts of one project share the recipe's id, and
// so does a fork or a copied recipe, so the id alone does not say the sync
// state belongs to the checkout taking it. The recipe's venue block names the
// server project the checkout syncs with, and a relink brings the sync state
// only when that is the project the sync state is bound to.

// SyncBinding is the server project a sync state is bound to, or a recipe
// connects to.
type SyncBinding struct {
	Server    string `json:"server"`
	Workspace string `json:"workspace,omitempty"`
	Project   string `json:"project"`
}

// String names the binding for a person.
func (b SyncBinding) String() string {
	if b.Workspace != "" {
		return b.Server + " project " + b.Workspace + "/" + b.Project
	}
	return b.Server + " project " + b.Project
}

// ServerURLNormalizer canonicalises a server URL before two are compared. The
// host sets it to the normalisation its sync claim check uses
// (host/venue/config.NormalizeServerURL), so the two agree on what one server
// is. The default, for a binary that links no host, trims the URL and folds
// its case.
var ServerURLNormalizer = func(s string) string {
	return strings.ToLower(strings.TrimRight(strings.TrimSpace(s), "/"))
}

// fieldVerdict is the answer one field of a binding gives.
type fieldVerdict int

const (
	fieldSame fieldVerdict = iota
	fieldDiffers
	fieldUnknown
)

// compareField decides one field (the server, the workspace or the project)
// of the binding a sync state records against the recipe's, by one rule:
//
//	sync state has it | recipe has it | values equal | answer
//	yes               | yes           | yes          | same
//	yes               | yes           | no           | differs
//	yes               | no            | -            | cannot say
//	no                | yes           | -            | cannot say
//	no                | no            | -            | cannot say
//
// Only a value both sides name, equal after norm, is the same. A side that
// names nothing proves nothing, so it cannot say.
func compareField(recorded, recipe string, norm func(string) string) fieldVerdict {
	switch {
	case recorded == "" || recipe == "":
		return fieldUnknown
	case norm(recorded) == norm(recipe):
		return fieldSame
	default:
		return fieldDiffers
	}
}

// bindingVerdict compares the server project a sync state is bound to with
// the one a recipe connects to, field by field (compareField). A field that
// differs makes them different server projects (same false, known true).
// Every field the same makes them one (same and known true). Otherwise the
// record cannot say (known false), as for a cache written before kapi
// recorded the workspace.
func bindingVerdict(recorded, recipe SyncBinding) (same, known bool) {
	verdicts := []fieldVerdict{
		compareField(recorded.Server, recipe.Server, ServerURLNormalizer),
		compareField(recorded.Workspace, recipe.Workspace, identity),
		compareField(recorded.Project, recipe.Project, identity),
	}
	unknown := false
	for _, v := range verdicts {
		switch v {
		case fieldDiffers:
			return false, true
		case fieldUnknown:
			unknown = true
		}
	}
	return !unknown, !unknown
}

func identity(s string) string { return s }

// recipeSyncBinding reads the server project the layout's recipe connects to,
// from its venue block (RecipeVenue). The compound URL is
// <server>/<workspace>/<project>, <server>/projects/<project> or
// <server>/<project>, as the venue's own parser reads it. ok is false for a
// recipe that names no server project.
func recipeSyncBinding(l Layout) (SyncBinding, bool) {
	v, ok, err := RecipeVenue(l.RecipePath)
	if err != nil || !ok {
		return SyncBinding{}, false
	}
	u, err := url.Parse(strings.TrimSpace(v.URL))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return SyncBinding{}, false
	}
	segments := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(segments) == 0 || len(segments) > 2 || segments[len(segments)-1] == "" {
		return SyncBinding{}, false
	}
	b := SyncBinding{Server: u.Scheme + "://" + u.Host, Project: segments[len(segments)-1]}
	if len(segments) == 2 && segments[0] != "projects" {
		b.Workspace = segments[0]
	}
	return b, true
}

// syncWithheld reports whether a relink leaves the recorded sync state where
// it is, and why. Sync state bound to another server project than the recipe
// names stays, whatever the person states. Sync state of a checkout whose
// recipe names no server project stays unless opts.SameProject, since nothing
// can be compared; so does sync state whose cache names no project, or no
// workspace, while the recipe names one.
func syncWithheld(l Layout, root *os.Root, opts RelinkOptions) (bool, string) {
	recorded, haveRecorded := recordedSyncBinding(root)
	recipe, haveRecipe := recipeSyncBinding(l)
	same, known := false, true
	if haveRecipe && haveRecorded {
		same, known = bindingVerdict(recorded, recipe)
	}
	switch {
	case same:
		return false, ""
	case haveRecipe && haveRecorded && known:
		return true, "the sync state is bound to " + recorded.String() + ", and this recipe connects to " + recipe.String() + ", so the sync state stays"
	case opts.SameProject:
		return false, ""
	case haveRecipe && haveRecorded:
		return true, "the sync state is bound to " + recorded.String() + " and this recipe connects to " + recipe.String() + ", and one of them names no workspace, so kapi cannot tell whether they are one server project and the sync state stays; pass --same-project to take it"
	case haveRecipe:
		return true, "the sync state names no server project to compare with " + recipe.String() + ", so it stays; pass --same-project to take it"
	default:
		return true, "this recipe connects to no server project to compare the sync state with, so the sync state stays; pass --same-project to take it"
	}
}

// recordedSyncBinding reads the server project the sync cache in a state
// directory (root) is bound to. ok is false when there is no sync cache or it
// names no project.
func recordedSyncBinding(root *os.Root) (SyncBinding, bool) {
	return syncBindingAt(root, filepath.Join(SyncDirName, "sync-cache.json"))
}

// syncBindingAt reads the server project the sync cache at rel in root is
// bound to.
func syncBindingAt(root *os.Root, rel string) (SyncBinding, bool) {
	data, err := root.ReadFile(rel)
	if err != nil {
		return SyncBinding{}, false
	}
	var doc struct {
		ServerURL string `json:"server_url"`
		Workspace string `json:"workspace"`
		ProjectID string `json:"project_id"`
	}
	if json.Unmarshal(data, &doc) != nil || doc.ServerURL == "" || doc.ProjectID == "" {
		return SyncBinding{}, false
	}
	return SyncBinding{Server: displaySafe(doc.ServerURL), Workspace: displaySafe(doc.Workspace), Project: displaySafe(doc.ProjectID)}, true
}
