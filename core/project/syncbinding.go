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
	Server  string `json:"server"`
	Project string `json:"project"`
}

// String names the binding for a person.
func (b SyncBinding) String() string {
	return b.Server + " project " + b.Project
}

// same reports whether two bindings name one server project. Servers compare
// by scheme and host, case-insensitively, with no trailing slash.
func (b SyncBinding) same(o SyncBinding) bool {
	return normServer(b.Server) == normServer(o.Server) && b.Project == o.Project && b.Project != ""
}

func normServer(s string) string {
	return strings.ToLower(strings.TrimRight(strings.TrimSpace(s), "/"))
}

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
	return SyncBinding{Server: u.Scheme + "://" + u.Host, Project: segments[len(segments)-1]}, true
}

// syncWithheld reports whether a relink leaves the recorded sync state where
// it is, and why. Sync state bound to another server project than the recipe
// names stays, whatever the person states. Sync state of a checkout whose
// recipe names no server project stays unless opts.SameProject, since nothing
// can be compared; so does sync state whose cache names no project while the
// recipe names one.
func syncWithheld(l Layout, root *os.Root, opts RelinkOptions) (bool, string) {
	recorded, haveRecorded := recordedSyncBinding(root)
	recipe, haveRecipe := recipeSyncBinding(l)
	switch {
	case haveRecipe && haveRecorded && recipe.same(recorded):
		return false, ""
	case haveRecipe && haveRecorded:
		return true, "the sync state is bound to " + recorded.String() + ", and this recipe connects to " + recipe.String() + ", so the sync state stays"
	case opts.SameProject:
		return false, ""
	case haveRecipe:
		return true, "the sync state names no server project to compare with " + recipe.String() + ", so it stays; pass --same-project to take it"
	default:
		return true, "this recipe connects to no server project to compare the sync state with, so the sync state stays; pass --same-project to take it"
	}
}

// recordedSyncBinding reads the server project the sync cache in the state
// directory dir is bound to. ok is false when there is no sync cache or it
// names no project.
func recordedSyncBinding(root *os.Root) (SyncBinding, bool) {
	data, err := root.ReadFile(filepath.Join(SyncDirName, "sync-cache.json"))
	if err != nil {
		return SyncBinding{}, false
	}
	var doc struct {
		ServerURL string `json:"server_url"`
		ProjectID string `json:"project_id"`
	}
	if json.Unmarshal(data, &doc) != nil || doc.ServerURL == "" || doc.ProjectID == "" {
		return SyncBinding{}, false
	}
	return SyncBinding{Server: displaySafe(doc.ServerURL), Project: displaySafe(doc.ProjectID)}, true
}
