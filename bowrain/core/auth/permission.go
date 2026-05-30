package auth

import "strings"

// Permission represents a single capability that can be granted via a role template.
// Permissions are stored as a bitmask for O(1) checks and compact storage.
type Permission uint64

const (
	PermViewContent      Permission = 1 << iota // View source and target content
	PermEditSource                              // Edit source text
	PermTranslate                               // Add/edit translations (language-scoped)
	PermReview                                  // Approve/reject translations (language-scoped)
	PermManageTerms                             // Edit terminology
	PermManageTM                                // Edit translation memory
	PermRunFlows                                // Execute processing flows
	PermManageFiles                             // Upload/delete files (items)
	PermManageStreams                           // Create/merge/delete streams
	PermManageConnectors                        // Configure connectors
	PermManageAutomation                        // Create/edit automation rules
	PermManageMembers                           // Add/remove project members
	PermManageProject                           // Edit project settings, archive
	PermManageBrand                             // Edit brand voice profiles
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

// permNames maps each single-bit permission to its string name.
var permNames = [permCount]string{
	"view_content",
	"edit_source",
	"translate",
	"review",
	"manage_terms",
	"manage_tm",
	"run_flows",
	"manage_files",
	"manage_streams",
	"manage_connectors",
	"manage_automation",
	"manage_members",
	"manage_project",
	"manage_brand",
	"manage_assets",
	"audit_read",
	"rollback_changes",
}

// permLookup maps permission string names to their bitmask values.
var permLookup = func() map[string]Permission {
	m := make(map[string]Permission, permCount)
	for i := range permCount {
		m[permNames[i]] = 1 << i
	}
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
