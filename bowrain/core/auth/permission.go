package auth

import (
	"maps"
	"strings"
)

// Permission represents a single capability that can be granted via a role template.
// Permissions are stored as a bitmask for O(1) checks and compact storage.
type Permission uint64

const (
	PermViewContent      Permission = 1 << iota // View source and target content
	PermEditSource                              // Edit source text
	PermTranslate                               // Add/edit translations (language-scoped)
	PermReview                                  // Approve/reject translations (language-scoped)
	PermManageTerms                             // Edit terminology
	PermManageMemory                            // Edit content memory
	PermRunFlows                                // Execute processing flows
	PermManageFiles                             // Upload/delete files (items)
	PermManageStreams                           // Create/merge/delete streams
	PermManageConnectors                        // Configure connectors
	PermManageAutomation                        // Create/edit automation rules
	PermManageMembers                           // Add/remove project members
	PermManageProject                           // Edit project settings, archive
	PermManageVoice                             // Edit voice profiles
	PermManageAssets                            // Upload/delete media assets
	PermAuditRead                               // Read the audit log
	PermRollbackChanges                         // Roll back / restore content to a prior state

	permCount = iota
)

// PermAll is the union of all defined permissions.
const PermAll Permission = (1 << permCount) - 1

// Has reports whether p includes all bits in required.
func (p Permission) Has(required Permission) bool {
	return p&required == required
}

// LanguageScoped reports whether the permission is language-scoped.
// Language-scoped permissions are restricted by the user's assigned languages.
func (p Permission) LanguageScoped() bool {
	return p == PermTranslate || p == PermReview
}

// CoordinateScoped reports whether the permission narrows by region: the powers
// that decide what governs content at a point, rather than what the content is.
//
// The distinction from LanguageScoped is not cosmetic. Language scopes a
// *permission* — translating into German and into Japanese are different acts.
// Brand, product and channel scope *which content* the same act may be performed
// on. A grant can carry both, and they are checked separately.
func (p Permission) CoordinateScoped() bool {
	return p == PermReview || p == PermManageVoice || p == PermManageTerms
}

// CustodialPermissions are the powers that author what governs content, rather
// than exercising it: voice and terms. Holding either over a bounded region is
// what makes someone a custodian of that region — see IsCustodian.
//
// PermReview is coordinate-scoped but deliberately absent. Reviewing content is
// volume work whose count grows with what was pushed; authoring a rule is not.
// A reviewer bounded to one brand is a contributor with a narrow beat, and
// folding them in here would both bill them as a custodian and — via
// TaskType.IsVolume routing — stop sending them the very work they are there to
// do.
const CustodialPermissions = PermManageVoice | PermManageTerms

// IsCustodian reports whether a grant is custody of a region rather than blanket
// authority: at least one coordinate-scoped permission, held over something
// narrower than the whole space.
//
// Custodian is derived, never declared, so the billable role and the
// authorization model cannot drift apart. Whoever can decide what governs
// content at a point IS the custodian of that point, by construction.
func IsCustodian(perms Permission, reach CoordinateReach) bool {
	return perms&CustodialPermissions != 0 && !reach.Unconstrained()
}

// permNames maps each single-bit permission to its string name.
var permNames = [permCount]string{
	"view_content",
	"edit_source",
	"translate",
	"review",
	"manage_terms",
	"manage_memory",
	"run_flows",
	"manage_files",
	"manage_streams",
	"manage_connectors",
	"manage_automation",
	"manage_members",
	"manage_project",
	"manage_voice",
	"manage_assets",
	"audit_read",
	"rollback_changes",
}

// permAliases are retired spellings still accepted on input. The bitmask is what
// role templates and deny rules persist, so a rename here is free in the
// database and costly on the wire: an API client or a stored request body naming
// the old spelling must keep resolving to the same bit. Nothing emits these —
// Strings() answers with permNames — so the alias fades as callers update.
var permAliases = map[string]Permission{
	"manage_tm": PermManageMemory, // pre-#1522 spelling of manage_memory
}

// permLookup maps permission string names to their bitmask values.
var permLookup = func() map[string]Permission {
	m := make(map[string]Permission, permCount+len(permAliases))
	for i := range permCount {
		m[permNames[i]] = 1 << i
	}
	maps.Copy(m, permAliases)
	return m
}()

// Strings returns the human-readable names of all set permission bits.
func (p Permission) Strings() []string {
	var out []string
	for i := range permCount {
		if p&(1<<i) != 0 {
			out = append(out, permNames[i])
		}
	}
	return out
}

// String returns a comma-separated list of permission names.
func (p Permission) String() string {
	return strings.Join(p.Strings(), ",")
}

// ParsePermission converts a permission name string to a Permission value.
// Returns 0 if the name is not recognized.
func ParsePermission(name string) Permission {
	return permLookup[name]
}

// ParsePermissions converts a slice of permission name strings to a combined Permission bitmask.
func ParsePermissions(names []string) Permission {
	var p Permission
	for _, name := range names {
		p |= permLookup[name]
	}
	return p
}
