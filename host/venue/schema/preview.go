package schema

import (
	"fmt"
	"net/url"
	"slices"
	"strings"
)

// PreviewSpec is where a collection's strings can be read in place: the
// component explorer or running site that shows them as a reader sees them.
//
// It is declared per collection rather than per project because a repository
// publishes as many of them as it ships surfaces — this one has two, one for
// the desktop app's components and one for the web app's — and per collection
// rather than in the format config because it is not something a reader
// consumes. The reader turns files into blocks; this says where those blocks
// are rendered for a person.
//
// # Why a kind
//
// The hosts differ in how a view is FOUND, not in how it renders. A Storybook
// publishes an index that maps components to stories, so an item resolves to a
// story by the components its blocks name. A running site is addressed by
// route, which is a different resolution and needs more than a URL to express.
// What they share is the runtime bridge that pushes the reviewer's translations
// into the page, which belongs to the i18n runtime rather than to any one host.
//
// So the kind travels, and a host a client does not recognise offers no
// in-context reading rather than a guess at a URL shape it cannot resolve.
type PreviewSpec struct {
	// Kind names how to resolve a view within this host.
	Kind PreviewKind `yaml:"kind,omitempty" json:"kind,omitempty"`

	// URL is the root of the preview host, without a view path.
	URL string `yaml:"url,omitempty" json:"url,omitempty"`
}

// PreviewKind names a way of resolving a view for an item.
type PreviewKind string

// PreviewKindStorybook is a published Storybook: views are stories, resolved
// through the index it publishes at `index.json`.
const PreviewKindStorybook PreviewKind = "storybook"

// PreviewKinds is every kind this version can resolve. A recipe naming one
// outside it is refused at load rather than at review time, when the person
// waiting for a preview cannot tell a typo from an empty component.
var PreviewKinds = []PreviewKind{PreviewKindStorybook}

// Validate refuses a spec that could not produce a preview.
//
// Both fields are required together: a kind with no URL names a host that is
// nowhere, and a URL with no kind names a host nobody knows how to read. An
// absent block is the way to declare no preview.
func (p *PreviewSpec) Validate() error {
	if p == nil {
		return nil
	}
	if p.Kind == "" && p.URL == "" {
		return nil
	}
	if p.Kind == "" {
		return fmt.Errorf("preview.kind is required (one of %s)", previewKindList())
	}
	if p.URL == "" {
		return fmt.Errorf("preview.url is required for kind %q", p.Kind)
	}

	if !slices.Contains(PreviewKinds, p.Kind) {
		return fmt.Errorf("preview.kind %q is not one this version can read (known: %s)",
			p.Kind, previewKindList())
	}

	parsed, err := url.Parse(p.URL)
	// The scheme is judged before the host, so a URL that named a scheme and
	// got it wrong is told which rule it broke rather than the vaguer one.
	if err == nil && parsed.Scheme != "" && parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("preview.url %q must be http or https", p.URL)
	}
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("preview.url %q is not an absolute URL: a preview is fetched from a "+
			"browser that is not in this repository", p.URL)
	}
	return nil
}

func previewKindList() string {
	names := make([]string, 0, len(PreviewKinds))
	for _, k := range PreviewKinds {
		names = append(names, string(k))
	}
	return strings.Join(names, ", ")
}
