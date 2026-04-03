package auth

// DefaultRoleTemplates defines the built-in role templates seeded when a workspace is created.
// Workspace admins can rename, customize permissions, or add custom templates.
// Built-in templates cannot be deleted but their permissions can be modified.
var DefaultRoleTemplates = []RoleTemplate{
	{
		Name:        "project-admin",
		DisplayName: "Project Admin",
		Description: "Full control over the project",
		Permissions: PermAll,
		IsBuiltin:   true,
		Position:    0,
	},
	{
		Name:        "developer",
		DisplayName: "Developer",
		Description: "Manage files, run flows, and contribute translations",
		Permissions: PermViewContent | PermEditSource | PermTranslate |
			PermManageFiles | PermRunFlows | PermManageStreams |
			PermManageConnectors | PermManageAutomation,
		IsBuiltin: true,
		Position:  1,
	},
	{
		Name:        "translator",
		DisplayName: "Translator",
		Description: "Translate content (language-scoped)",
		Permissions: PermViewContent | PermTranslate,
		IsBuiltin:   true,
		Position:    2,
	},
	{
		Name:        "reviewer",
		DisplayName: "Reviewer",
		Description: "Review and approve translations (language-scoped)",
		Permissions: PermViewContent | PermTranslate | PermReview,
		IsBuiltin:   true,
		Position:    3,
	},
	{
		Name:        "observer",
		DisplayName: "Observer",
		Description: "Read-only access to project content",
		Permissions: PermViewContent,
		IsBuiltin:   true,
		Position:    4,
	},
}
