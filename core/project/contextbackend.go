package project

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Context backends a recipe can declare under `context.backend`.
const (
	// ContextBackendLocal keeps the project's context on this machine only.
	// It is the default.
	ContextBackendLocal = "local"
	// ContextBackendFile shares it through a directory: a mounted team share
	// or a network volume.
	ContextBackendFile = "file"
	// ContextBackendGit shares it on a ref of the project's own repository,
	// outside every branch.
	ContextBackendGit = "git"
	// ContextBackendS3 shares it in an S3-compatible bucket.
	ContextBackendS3 = "s3"
	// ContextBackendBowrain shares it with the Bowrain project that the
	// recipe's top-level block of the same name connects to. The `context:`
	// block names no location of its own.
	ContextBackendBowrain = "bowrain"
)

// ContextBackendKinds lists every backend a recipe can declare.
var ContextBackendKinds = []string{ContextBackendLocal, ContextBackendFile, ContextBackendGit, ContextBackendS3, ContextBackendBowrain}

// ContextBackend is where a project's context is shared: the `context:`
// block of a recipe.
//
//	context:
//	  backend: git                  # local (default), file, git, s3, bowrain
//	  ref: refs/kapi/context        # git: a ref in this repository
//	# file:    path: /Volumes/team/kapi/fernwell
//	# s3:      bucket: acme-kapi, prefix: fernwell/
//	# bowrain: no further keys; the project is the one `bowrain: {url: ...}` names
//
// Credentials never appear here: a git backend uses the repository's own
// remote access, an S3 backend the standard AWS environment and profile, and
// a bowrain backend the sign-in that `kapi auth login` stores.
// `kapi context sync` shares the context through it.
type ContextBackend struct {
	// Backend names the kind: local, file, git, s3 or bowrain. Empty means
	// local.
	Backend string `yaml:"backend,omitempty" json:"backend,omitempty"`

	// Ref is the git ref the context is kept on. Default refs/kapi/context.
	Ref string `yaml:"ref,omitempty" json:"ref,omitempty"`
	// Remote is the git remote pushed to and fetched from. Default origin.
	Remote string `yaml:"remote,omitempty" json:"remote,omitempty"`

	// Path is the directory a file backend keeps the context in. A relative
	// path is read against the recipe's directory.
	Path string `yaml:"path,omitempty" json:"path,omitempty"`

	// Bucket, Prefix, Region and Endpoint place an S3 backend. Endpoint is
	// for an S3-compatible service other than AWS.
	Bucket   string `yaml:"bucket,omitempty" json:"bucket,omitempty"`
	Prefix   string `yaml:"prefix,omitempty" json:"prefix,omitempty"`
	Region   string `yaml:"region,omitempty" json:"region,omitempty"`
	Endpoint string `yaml:"endpoint,omitempty" json:"endpoint,omitempty"`

	// Venue is the project a bowrain backend shares the context with. The
	// `context:` block never carries it: the host fills it from the recipe's
	// `bowrain:` block (ReadContextVenue) when it resolves the backend, and
	// the backend's opener reads it from here.
	Venue *ContextVenue `yaml:"-" json:"-"`
}

// Kind returns the backend kind, local when none is named.
func (c *ContextBackend) Kind() string {
	if c == nil || c.Backend == "" {
		return ContextBackendLocal
	}
	return c.Backend
}

// Shared reports whether the backend shares the context beyond this machine.
func (c *ContextBackend) Shared() bool { return c.Kind() != ContextBackendLocal }

// Validate checks that the block names a known backend and carries what that
// backend needs, and nothing another backend would read.
func (c *ContextBackend) Validate() error {
	if c == nil {
		return nil
	}
	field := func(name, value string, allowed ...string) error {
		if value == "" {
			return nil
		}
		for _, kind := range allowed {
			if c.Kind() == kind {
				return nil
			}
		}
		return fmt.Errorf("context.%s: applies to the %s backend, and this recipe names %s", name, strings.Join(allowed, " and "), c.Kind())
	}
	if err := field("ref", c.Ref, ContextBackendGit); err != nil {
		return err
	}
	if err := field("remote", c.Remote, ContextBackendGit); err != nil {
		return err
	}
	if err := field("path", c.Path, ContextBackendFile); err != nil {
		return err
	}
	for _, f := range [][2]string{{"bucket", c.Bucket}, {"prefix", c.Prefix}, {"region", c.Region}, {"endpoint", c.Endpoint}} {
		if err := field(f[0], f[1], ContextBackendS3); err != nil {
			return err
		}
	}
	switch c.Kind() {
	case ContextBackendLocal:
	case ContextBackendFile:
		if c.Path == "" {
			return errors.New("context.path: a file backend names the directory it keeps the context in")
		}
	case ContextBackendGit:
		if c.Ref != "" && !strings.HasPrefix(c.Ref, "refs/") {
			return fmt.Errorf("context.ref: %q is not a full ref name; use one under refs/, such as refs/kapi/context", c.Ref)
		}
		if strings.HasPrefix(c.Ref, "refs/heads/") || strings.HasPrefix(c.Ref, "refs/tags/") {
			return fmt.Errorf("context.ref: %q is a branch or a tag; keep the context on a ref outside them, such as refs/kapi/context", c.Ref)
		}
	case ContextBackendS3:
		if c.Bucket == "" {
			return errors.New("context.bucket: an s3 backend names its bucket")
		}
	case ContextBackendBowrain:
	default:
		return fmt.Errorf("context.backend: %q is not a backend; use local, file, git, s3 or bowrain", c.Backend)
	}
	return nil
}

// ContextVenue is the Bowrain project a bowrain backend shares a project's
// context with, as the recipe's `bowrain:` block names it.
type ContextVenue struct {
	// URL is the block's compound project URL: <server>/<workspace>/<project>
	// or <server>/projects/<project>.
	URL string `yaml:"url"`
	// Stream is the block's content stream. A project has one context, which
	// every stream of it reads, so the stream does not place the backend.
	Stream string `yaml:"stream"`
}

// ReadContextVenue reads the project a bowrain backend shares the context
// with from the recipe's top-level `bowrain:` block. A recipe with no such
// block, or a block with no url, is refused with the key to add.
func ReadContextVenue(recipePath string) (ContextVenue, error) {
	data, err := os.ReadFile(recipePath)
	if err != nil {
		return ContextVenue{}, err
	}
	var head map[string]yaml.Node
	if err := yaml.Unmarshal(data, &head); err != nil {
		return ContextVenue{}, fmt.Errorf("%s: %w", recipePath, err)
	}
	node, ok := head[ContextBackendBowrain]
	if !ok {
		return ContextVenue{}, fmt.Errorf(
			"context.backend: a bowrain backend shares the context with the project the recipe's `%s:` block connects to, and %s has no `%s:` block; connect the project with `kapi share --via bowrain <server-url>`",
			ContextBackendBowrain, recipePath, ContextBackendBowrain)
	}
	var v ContextVenue
	if err := node.Decode(&v); err != nil {
		return ContextVenue{}, fmt.Errorf("%s: %s: %w", recipePath, ContextBackendBowrain, err)
	}
	if strings.TrimSpace(v.URL) == "" {
		return ContextVenue{}, fmt.Errorf(
			"context.backend: a bowrain backend shares the context with the project `%s.url` names, and the `%s:` block in %s has no url",
			ContextBackendBowrain, ContextBackendBowrain, recipePath)
	}
	return v, nil
}
