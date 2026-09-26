package project

import (
	"errors"
	"fmt"
	"strings"
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
)

// ContextBackend is where a project's context is shared: the `context:`
// block of a recipe.
//
//	context:
//	  backend: git                  # local (default), file, git, s3
//	  ref: refs/kapi/context        # git: a ref in this repository
//	# file: path: /Volumes/team/kapi/fernwell
//	# s3:   bucket: acme-kapi, prefix: fernwell/
//
// Credentials never appear here: a git backend uses the repository's own
// remote access, and an S3 backend the standard AWS environment and profile.
// A person can use another backend on one machine (`kapi context backend`),
// which is kept in that machine's configuration.
type ContextBackend struct {
	// Backend names the kind: local, file, git or s3. Empty means local.
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
	default:
		return fmt.Errorf("context.backend: %q is not a backend; use local, file, git or s3", c.Backend)
	}
	return nil
}
