package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/neokapi/neokapi/bowrain/review"
)

// ReviewAuthority answers the review gate's permission question for a caller
// with no request to read it from.
//
// The web routes read a caller's effective permissions off the request, where
// the project access middleware has already resolved them. The sync worker runs
// long after the request that carried the push has been answered, and it is
// where a pushed approval is actually written, so it resolves the same answer
// the same way: an explicit project membership (or group binding) decides, the
// workspace role stands in when the user has neither, deny rules are subtracted
// last, and a membership that names languages narrows the answer to them.
//
// The request path resolves two further things this one does not need. Claim
// tokens grant access before a membership exists, and a claim-token push is
// attributed to no user at all, so it holds no review permission here and its
// verdicts are refused. Custodial lapse suspends the permissions that author
// what governs content; review is deliberately not one of them
// (platauth.CustodialPermissions).
//
// Every store failure is reported rather than swallowed. A permission that
// could not be resolved is not a permission that was denied: the worker fails
// the push and the producer sends it again, where reading the failure as a
// denial would silently discard an approval somebody was entitled to make.
type ReviewAuthority struct {
	Store AuthStore
}

// NewReviewAuthority binds the gate's permission questions to an auth store.
func NewReviewAuthority(store AuthStore) *ReviewAuthority {
	return &ReviewAuthority{Store: store}
}

// GetSoDMode reads the workspace's separation-of-duties policy.
func (a *ReviewAuthority) GetSoDMode(ctx context.Context, workspaceID string) (platauth.SoDMode, error) {
	if a == nil || a.Store == nil {
		return platauth.SoDOff, errors.New("no auth store to read the separation-of-duties policy from")
	}
	return a.Store.GetSoDMode(ctx, workspaceID)
}

// AllowsLanguage reports whether the user holds the permission for the language
// in the project.
func (a *ReviewAuthority) AllowsLanguage(ctx context.Context, q review.Query) (bool, error) {
	if a == nil || a.Store == nil {
		return false, errors.New("no auth store to resolve review permissions from")
	}
	if q.UserID == "" {
		// Nobody in particular pushed this. There is no one to hold the
		// permission and no one to attribute the decision to.
		return false, nil
	}
	perms, languages, err := a.resolve(ctx, q)
	if err != nil {
		return false, err
	}
	if !perms.Has(q.Permission) {
		return false, nil
	}
	return len(languages) == 0 || slices.Contains(languages, q.Locale), nil
}

// resolve is the permission ladder, in the order ProjectAccessMiddleware walks
// it.
func (a *ReviewAuthority) resolve(ctx context.Context, q review.Query) (platauth.Permission, []string, error) {
	wsRole, err := a.workspaceRole(ctx, q)
	if err != nil {
		return 0, nil, err
	}

	var perms platauth.Permission
	var languages []string
	resolved, err := a.Store.ResolveProjectPermissions(ctx, q.ProjectID, q.UserID)
	switch {
	case err == nil && resolved != nil:
		perms, languages = resolved.Permissions, resolved.Languages
	case err != nil && !errors.Is(err, sql.ErrNoRows):
		return 0, nil, fmt.Errorf("resolve project permissions: %w", err)
	case wsRole == "":
		return 0, nil, nil // no project membership and no workspace role: deny
	default:
		perms, err = a.workspaceRolePermissions(ctx, q.WorkspaceID, wsRole)
		if err != nil {
			return 0, nil, err
		}
	}

	if q.WorkspaceID != "" {
		denied, derr := a.Store.ResolveDenies(ctx, q.WorkspaceID, q.ProjectID, q.UserID, wsRole)
		if derr != nil {
			return 0, nil, fmt.Errorf("resolve deny rules: %w", derr)
		}
		perms &^= denied
	}
	return perms, languages, nil
}

// workspaceRole is the user's role in the workspace that owns the project, or
// empty when they are not a member of it.
func (a *ReviewAuthority) workspaceRole(ctx context.Context, q review.Query) (platauth.Role, error) {
	if q.WorkspaceID == "" {
		return "", nil
	}
	m, err := a.Store.GetMembership(ctx, q.WorkspaceID, q.UserID)
	switch {
	case err == nil && m != nil:
		return m.Role, nil
	case err != nil && !errors.Is(err, sql.ErrNoRows):
		return "", fmt.Errorf("read workspace membership: %w", err)
	}
	return "", nil
}

// workspaceRolePermissions honours a per-workspace override of the role's
// default permissions, exactly as the request path does.
func (a *ReviewAuthority) workspaceRolePermissions(ctx context.Context, workspaceID string, role platauth.Role) (platauth.Permission, error) {
	if workspaceID != "" {
		perms, ok, err := a.Store.GetWorkspaceRoleOverride(ctx, workspaceID, role)
		if err != nil {
			return 0, fmt.Errorf("read workspace role override: %w", err)
		}
		if ok {
			return perms, nil
		}
	}
	return platauth.DefaultPermissionsForRole(role).Permissions, nil
}
