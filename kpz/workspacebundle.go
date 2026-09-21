package kpz

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/neokapi/neokapi/core/safeio"
	"github.com/neokapi/neokapi/core/schemaversion"
)

// The workspace profile of the container (KindWorkspace).
//
// A workspace holds the authored context of every project a machine account
// works on, one database each, outside every checkout. A package of it is one
// member per project, each a complete KindContext package, plus a registry
// member saying which project each one is.
//
// Nesting a package inside a package is what keeps the whole of it streamable:
// a member is a reference, so packing reads one project's archive at a time
// from disk and unpacking hands back a reader over one entry. It also means a
// project inside a workspace package is a file `kapi context restore` reads on
// its own once the archive is unzipped.
//
// What a workspace records and a package does not: the checkout paths a project
// has been seen at. They name directories on one machine and mean nothing on
// the one restoring.

// registryKind is the workspace registry member's envelope discriminator.
const registryKind = "kapi-workspace-registry"

// ProjectDoc is one project's context package inside a workspace package.
type ProjectDoc struct {
	// Path is the archive path under projects/, e.g. "projects/prj_9f2k.kpz".
	Path string
	// Key identifies the project in a workspace: the recipe's stable `id:`, or
	// its `name:` where the recipe carries none. A restore registers the
	// project under it, so two workspaces hold one project under one identity.
	Key string
	// Name is the display name the recipe carries. It may be empty, and it may
	// change, which is why it is not the key.
	Name string
	// Content is the project's KindContext package, carried as a reference so
	// neither packing nor unpacking holds a whole workspace.
	Content Content
}

// projectRegistry is the workspace registry member's serialization.
type projectRegistry struct {
	SchemaVersion string            `json:"schemaVersion"`
	Kind          string            `json:"kind"`
	Projects      []registryProject `json:"projects"`
}

// registryProject is one project's identity in the registry member.
type registryProject struct {
	Key    string `json:"key"`
	Name   string `json:"name,omitempty"`
	Bundle string `json:"bundle"`
}

// marshalProjectRegistry writes the registry member: one entry per project,
// ordered by key, so a workspace holding the same projects packs to the same
// bytes however the registry listed them.
func marshalProjectRegistry(docs []ProjectDoc) ([]byte, error) {
	out := projectRegistry{
		SchemaVersion: SchemaVersion,
		Kind:          registryKind,
		Projects:      make([]registryProject, 0, len(docs)),
	}
	seen := make(map[string]bool, len(docs))
	for _, d := range docs {
		switch {
		case d.Key == "":
			return nil, fmt.Errorf("kpz: project member %q names no project", d.Path)
		case d.Path == "":
			return nil, fmt.Errorf("kpz: project %q needs Path", d.Key)
		case d.Content == nil:
			return nil, fmt.Errorf("kpz: project %q needs Content", d.Key)
		case seen[d.Key]:
			return nil, fmt.Errorf("kpz: project %q appears twice", d.Key)
		}
		seen[d.Key] = true
		out.Projects = append(out.Projects, registryProject{Key: d.Key, Name: d.Name, Bundle: d.Path})
	}
	sort.Slice(out.Projects, func(i, j int) bool { return out.Projects[i].Key < out.Projects[j].Key })

	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("kpz: encode the workspace registry: %w", err)
	}
	return append(data, '\n'), nil
}

// applyProjectRegistry ties each project member to the project it holds.
//
// Every name in the registry is content that came off the wire, so a bundle
// path is validated the way the manifest's paths are, and an entry naming a
// member the package does not carry is refused rather than dropped: a
// workspace package that lost a project is a backup that would restore
// silently short.
func applyProjectRegistry(pkg *Package, data []byte) error {
	if len(data) == 0 {
		if len(pkg.Projects) > 0 {
			return fmt.Errorf("kpz: %d project members and no %s saying which projects they are",
				len(pkg.Projects), RegistryPath)
		}
		return nil
	}
	var reg projectRegistry
	if err := json.Unmarshal(data, &reg); err != nil {
		return fmt.Errorf("kpz: parse %s: %w", RegistryPath, err)
	}
	if reg.Kind != registryKind {
		return fmt.Errorf("kpz: %s has kind %q (want %q)", RegistryPath, reg.Kind, registryKind)
	}
	major, ok := schemaversion.Major(reg.SchemaVersion)
	if !ok {
		return fmt.Errorf("kpz: %s has an invalid schemaVersion %q", RegistryPath, reg.SchemaVersion)
	}
	if want, _ := schemaversion.Major(SchemaVersion); major != want {
		return fmt.Errorf("kpz: %s speaks major schemaVersion %d (this build speaks %s)",
			RegistryPath, major, SchemaVersion)
	}

	byPath := make(map[string]int, len(pkg.Projects))
	for i, d := range pkg.Projects {
		byPath[d.Path] = i
	}
	claimed := make(map[string]bool, len(reg.Projects))
	for _, entry := range reg.Projects {
		if entry.Key == "" {
			return fmt.Errorf("kpz: %s holds an entry with no project key", RegistryPath)
		}
		if !safeio.IsLocalPath(entry.Bundle) {
			return fmt.Errorf("kpz: project bundle %q is not a path inside the package", entry.Bundle)
		}
		i, ok := byPath[entry.Bundle]
		if !ok {
			return fmt.Errorf("kpz: %s names %q, which the package does not carry", RegistryPath, entry.Bundle)
		}
		if claimed[entry.Bundle] {
			return fmt.Errorf("kpz: %s claims %q twice", RegistryPath, entry.Bundle)
		}
		claimed[entry.Bundle] = true
		pkg.Projects[i].Key, pkg.Projects[i].Name = entry.Key, entry.Name
	}
	for _, d := range pkg.Projects {
		if d.Key == "" {
			return fmt.Errorf("kpz: %s says nothing about %q", RegistryPath, d.Path)
		}
	}
	sort.Slice(pkg.Projects, func(i, j int) bool { return pkg.Projects[i].Key < pkg.Projects[j].Key })
	return nil
}
