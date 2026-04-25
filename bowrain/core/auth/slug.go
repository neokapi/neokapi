package auth

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// slugPattern matches valid slugs: lowercase alphanumeric + hyphens, 2-64 chars,
// no leading/trailing hyphens, no consecutive hyphens.
var slugPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,62}[a-z0-9])?$`)

// ReservedWorkspaceSlugs are top-level API route segments that cannot be used
// as workspace slugs. Includes historical routes consolidated into /info
// (AD-040) so that stale clients get a clearer "no such endpoint" response
// instead of "workspace not found".
var ReservedWorkspaceSlugs = map[string]bool{
	"auth":       true,
	"admin":      true,
	"health":     true,
	"ready":      true,
	"info":       true,
	"pulse":      true,
	"badges":     true,
	"join":       true,
	"webhooks":   true,
	"workspaces": true,
	"projects":   true,
	"connectors": true,
	"config":     true,
	"formats":    true,
	"tools":      true,
	"locales":    true,
	"fetch":      true,
	"publish":    true,
	"_":          true,
}

// ReservedProjectSlugs are workspace-level route segments that cannot be used
// as project slugs.
var ReservedProjectSlugs = map[string]bool{
	"members":                  true,
	"invites":                  true,
	"roles":                    true,
	"tokens":                   true,
	"billing":                  true,
	"audit-log":                true,
	"providers":                true,
	"terms":                    true,
	"translation-memory":       true,
	"connectors":               true,
	"tasks":                    true,
	"jobs":                     true,
	"ai-usage":                 true,
	"bravo":                    true,
	"graph":                    true,
	"activities":               true,
	"notifications":            true,
	"notification-preferences": true,
	"digest-settings":          true,
	"brand-profiles":           true,
	"archived-projects":        true,
	"projects":                 true,
	"streams":                  true,
	"tags":                     true,
	"refs":                     true,
	"blocks":                   true,
	"items":                    true,
	"sync":                     true,
	"actions":                  true,
	"assets":                   true,
	"collections":              true,
	"preview":                  true,
	"word-count":               true,
	"review-queue":             true,
	"brand-voice":              true,
	"collab":                   true,
	"dashboard":                true,
	"automations":              true,
	"settings":                 true,
}

// ValidateSlug checks that a slug meets format requirements.
func ValidateSlug(slug string) error {
	if len(slug) < 2 {
		return errors.New("slug must be at least 2 characters")
	}
	if len(slug) > 64 {
		return errors.New("slug must be at most 64 characters")
	}
	if strings.Contains(slug, "--") {
		return errors.New("slug must not contain consecutive hyphens")
	}
	if !slugPattern.MatchString(slug) {
		return errors.New("slug must contain only lowercase alphanumeric characters and hyphens, and must not start or end with a hyphen")
	}
	return nil
}

// ValidateWorkspaceSlug validates a workspace slug (format + reserved name check).
func ValidateWorkspaceSlug(slug string) error {
	if err := ValidateSlug(slug); err != nil {
		return err
	}
	if ReservedWorkspaceSlugs[slug] {
		return fmt.Errorf("slug %q is reserved and cannot be used as a workspace name", slug)
	}
	return nil
}

// ValidateProjectSlug validates a project slug (format + reserved name check).
func ValidateProjectSlug(slug string) error {
	if err := ValidateSlug(slug); err != nil {
		return err
	}
	if ReservedProjectSlugs[slug] {
		return fmt.Errorf("slug %q is reserved and cannot be used as a project name", slug)
	}
	return nil
}

// SuggestSlug derives a candidate workspace slug from an email address by
// taking the local part, lowercasing it, and replacing common non-slug
// characters with hyphens. The result is not guaranteed to pass
// ValidateWorkspaceSlug (e.g. emails starting with a digit-only prefix may
// fail other rules); callers should validate and fall back as needed.
func SuggestSlug(email string) string {
	local, _, _ := strings.Cut(email, "@")
	slug := strings.ToLower(local)
	slug = strings.NewReplacer(".", "-", "_", "-", "+", "-").Replace(slug)
	// Trim leading/trailing hyphens.
	slug = strings.Trim(slug, "-")
	// Collapse repeated hyphens to a single hyphen.
	for strings.Contains(slug, "--") {
		slug = strings.ReplaceAll(slug, "--", "-")
	}
	return slug
}
