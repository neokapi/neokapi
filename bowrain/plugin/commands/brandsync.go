package commands

import (
	"context"
	"errors"
	"path/filepath"

	apiclient "github.com/neokapi/neokapi/bowrain/core/client"
	bproject "github.com/neokapi/neokapi/bowrain/core/project"
	bconn "github.com/neokapi/neokapi/bowrain/plugin/connector"
	corebrand "github.com/neokapi/neokapi/core/brand"
	"github.com/neokapi/neokapi/host"
)

// brandsync.go folds the recipe-bound brand voice profile into the ordinary
// project push, so the workspace brand hub is populated from day one. A push
// (kapi push, unless --no-brand) resolves the profile the recipe binds —
// defaults.brand_voice (profile_file, pack, or local-store profile) or the
// brand.yaml convention file — and upserts it into the workspace by name:
// created when the hub has no profile of that name, a no-op when the content
// is unchanged, and otherwise a new version through the server's profile
// versioning (the previous state is archived, never clobbered; rules the
// server promoted from corrections are preserved).

// PushBrandResult holds what a brand-profile push reported.
type PushBrandResult struct {
	Name    string // profile name
	Action  string // created | updated | unchanged | skipped | would-push (dry run)
	Version int    // stored profile version after the action (0 when unknown)
	Reason  string // set when Action == "skipped"
}

// PushBrandProfile upserts the resolved local profile into the workspace brand
// hub through the idempotent upsert endpoint. A 403 (the caller lacks the
// manage-brand permission) degrades to a "skipped" result rather than failing
// the push — the content transport already succeeded.
func PushBrandProfile(ctx context.Context, client *apiclient.BowrainClient, profile *corebrand.VoiceProfile) (*PushBrandResult, error) {
	res, err := client.UpsertBrandProfile(ctx, apiclient.BrandProfileUpsertFromProfile(profile))
	if err != nil {
		if errors.Is(err, apiclient.ErrForbidden) {
			return &PushBrandResult{
				Name:   profile.Name,
				Action: "skipped",
				Reason: "requires the manage-brand permission",
			}, nil
		}
		return nil, err
	}
	out := &PushBrandResult{Name: profile.Name, Action: res.Action}
	if res.Profile != nil {
		out.Version = res.Profile.Version
	}
	return out, nil
}

// brandPush runs the project-level brand-profile push: it builds the workspace
// client (skipping silently — returning nil — when the project is not claimed
// into a workspace or has no auth, exactly like conceptPush), resolves the
// profile the recipe binds via the host resolution ladder (skipping when the
// project binds none), and upserts it. A dry run resolves and reports what
// would be pushed without contacting the server.
func brandPush(ctx context.Context, proj *bproject.Project, dryRun bool) (*PushBrandResult, error) {
	if app == nil {
		return nil, nil
	}
	client, err := bconn.NewKnowledgeClient(proj)
	if err != nil {
		return nil, nil
	}
	profile, _, found, err := app.ResolveBrandProfile(ctx, &proj.Recipe.KapiProject, proj.Root, host.BrandResolveOptions{
		StorePath: filepath.Join(proj.Root, "brand.db"),
	})
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, nil
	}
	if dryRun {
		return &PushBrandResult{Name: profile.Name, Action: "would-push"}, nil
	}
	return PushBrandProfile(ctx, client, profile)
}
