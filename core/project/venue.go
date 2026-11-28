package project

import (
	"net/url"
	"sort"
	"strings"
)

// The convergence venue.
//
// A recipe may bind its project to a remote venue: a server that holds the
// content memory, runs the loop on org keys, and carries a review queue. That
// binding is a platform extension, not framework schema. The platform that
// registered it decodes and validates the block.
//
// So the framework asks the registry which extension IS the venue rather than
// looking for a key by name. `kapi` links no platform, and a recipe key it
// cannot name is a key it cannot grow an opinion about.

// VenueBinding is the venue-relevant subset of a venue extension's block.
type VenueBinding struct {
	// Key is the recipe key that carried it, for messages that have to name
	// the block a user would edit.
	Key string
	// URL addresses the venue.
	URL string
	// Converge is the venue's convergence policy, e.g. "manual".
	Converge string
}

// venueFields is the shape decoded out of the extension's YAML node. A decode
// error leaves the fields empty: schema validation is the extension decoder's
// job at load time, and a best-effort read here must not turn a malformed
// block into a failure in a command that only wanted to know the venue.
type venueFields struct {
	URL      string `yaml:"url"`
	Converge string `yaml:"converge"`
}

// Venue returns the convergence venue this recipe binds to, and whether it
// binds one at all. Only an extension registered with Venue: true is
// consulted, so an unregistered key of the same name (a recipe loaded by a
// binary without the platform) reports no venue and no opinion.
func (p *KapiProject) Venue() (VenueBinding, bool) {
	keys := make([]string, 0, len(p.Extras))
	for key := range p.Extras {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		ext, ok := extensionFor(ScopeProject, key)
		if !ok || !ext.Venue {
			continue
		}
		var fields venueFields
		node := p.Extras[key]
		_ = node.Decode(&fields)
		return VenueBinding{Key: key, URL: fields.URL, Converge: fields.Converge}, true
	}
	return VenueBinding{}, false
}

// VenueKey returns the recipe key of the registered venue extension, and
// whether one is registered at all. It answers the question a recipe is not
// needed for ("what does a project connect through in this binary?"), so a
// message about an absent block can still name the block to add.
func VenueKey() (string, bool) {
	for _, name := range RegisteredExtensions(ScopeProject) {
		if ext, ok := extensionFor(ScopeProject, name); ok && ext.Venue {
			return name, true
		}
	}
	return "", false
}

// IsVenueKey reports whether name is the registered venue extension's key.
func IsVenueKey(name string) bool {
	key, ok := VenueKey()
	return ok && key == name
}

// VenueURL is the parts of a venue's compound project URL.
type VenueURL struct {
	// Server is the scheme and host the URL names, with no path: the URL's
	// path is the workspace and the project, so a server it addresses has
	// none of its own.
	Server    string
	Workspace string
	Project   string
}

// ParseVenueURL parses a venue's compound project URL:
//
//	<server>/<workspace>/<project>    a workspace project
//	<server>/projects/<project>       a project in no workspace
//	<server>/<project>                a project in no workspace
//
// Any other path names the server and no project. It is the one parser of
// the URL: the venue reads its connection with it, and the record of a sync
// state is written from its Server, so a relink compares values one function
// produced.
func ParseVenueURL(raw string) VenueURL {
	if raw == "" {
		return VenueURL{}
	}
	u, err := url.Parse(raw)
	if err != nil {
		return VenueURL{}
	}
	segments := strings.Split(strings.Trim(u.Path, "/"), "/")
	server := u.Scheme + "://" + u.Host
	switch {
	case len(segments) == 2 && segments[0] == "projects":
		return VenueURL{Server: server, Project: segments[1]}
	case len(segments) == 2:
		return VenueURL{Server: server, Workspace: segments[0], Project: segments[1]}
	case len(segments) == 1 && segments[0] != "":
		return VenueURL{Server: server, Project: segments[0]}
	default:
		return VenueURL{Server: server}
	}
}
