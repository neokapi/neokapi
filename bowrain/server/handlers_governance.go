package server

import (
	"net/http"

	"github.com/labstack/echo/v4"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platev "github.com/neokapi/neokapi/bowrain/core/event"
)

// All governance endpoints are workspace-scoped and restricted to admin/owner.

func (s *Server) govRequireAdmin(c echo.Context) error {
	return s.requireRole(c, platauth.RoleAdmin, platauth.RoleOwner)
}

// groupInWorkspace verifies the :gid group belongs to the request's workspace,
// preventing cross-workspace access via a known group ID (IDOR). It writes a
// 404 and returns a non-nil error on mismatch so the caller aborts.
func (s *Server) groupInWorkspace(c echo.Context) error {
	wsID, _ := c.Get("workspace_id").(string)
	if _, err := s.AuthStore.GetGroup(c.Request().Context(), wsID, c.Param("gid")); err != nil {
		_ = c.JSON(http.StatusNotFound, ErrorResponse{Error: "group not found"})
		return errAccessDenied
	}
	return nil
}

// enforceSoD applies the workspace separation-of-duties policy when an actor
// would act on (e.g. approve) work they themselves authored. In "block" mode it
// writes a 403 and returns a non-nil error; in "warn" mode it records a
// violation event but allows the action; "off" is a no-op. It is a reusable
// primitive for review/approval handlers (e.g. translation review): pass the
// acting user and the work's author.
func (s *Server) enforceSoD(c echo.Context, actorID, authorID, resource string) error {
	if actorID == "" || authorID == "" || actorID != authorID {
		return nil // different people (or unknown) — no conflict of interest
	}
	if s.AuthStore == nil {
		return nil
	}
	wsID, _ := c.Get("workspace_id").(string)
	mode, err := s.AuthStore.GetSoDMode(c.Request().Context(), wsID)
	if err != nil || mode == platauth.SoDOff {
		return nil
	}
	s.emitAudit(c, auditEvent{
		Type:   platev.EventType("sod.violation"),
		Effect: "deny",
		Data:   map[string]string{"actor": actorID, "resource": resource, "mode": string(mode)},
	})
	if mode == platauth.SoDBlock {
		// Use deny() so the caller's `if err != nil { return err }` actually
		// aborts (c.JSON alone returns nil — a fail-open).
		return deny(c, "separation of duties: you cannot review or approve your own work")
	}
	return nil // warn: recorded, but allowed
}

// ── Groups ────────────────────────────────────────────────────────────────

type groupRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func (s *Server) HandleListGroups(c echo.Context) error {
	if s.AuthStore == nil {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "auth not configured"})
	}
	wsID, _ := c.Get("workspace_id").(string)
	groups, err := s.AuthStore.ListGroups(c.Request().Context(), wsID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}
	if groups == nil {
		groups = []*platauth.Group{}
	}
	return c.JSON(http.StatusOK, groups)
}

func (s *Server) HandleCreateGroup(c echo.Context) error {
	if err := s.govRequireAdmin(c); err != nil {
		return err
	}
	var req groupRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}
	if req.Name == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "name is required"})
	}
	wsID, _ := c.Get("workspace_id").(string)
	g := &platauth.Group{WorkspaceID: wsID, Name: req.Name, Description: req.Description}
	if err := s.AuthStore.CreateGroup(c.Request().Context(), g); err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}
	s.emitAudit(c, auditEvent{Type: platev.EventType("group.created"), ResourceType: "group", ResourceID: g.ID, Data: map[string]string{"name": g.Name}})
	return c.JSON(http.StatusCreated, g)
}

func (s *Server) HandleDeleteGroup(c echo.Context) error {
	if err := s.govRequireAdmin(c); err != nil {
		return err
	}
	wsID, _ := c.Get("workspace_id").(string)
	gid := c.Param("gid")
	if err := s.AuthStore.DeleteGroup(c.Request().Context(), wsID, gid); err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}
	s.emitAudit(c, auditEvent{Type: platev.EventType("group.deleted"), ResourceType: "group", ResourceID: gid})
	return c.NoContent(http.StatusNoContent)
}

func (s *Server) HandleListGroupMembers(c echo.Context) error {
	if err := s.groupInWorkspace(c); err != nil {
		return err
	}
	members, err := s.AuthStore.ListGroupMembers(c.Request().Context(), c.Param("gid"))
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}
	if members == nil {
		members = []string{}
	}
	return c.JSON(http.StatusOK, members)
}

func (s *Server) HandleAddGroupMember(c echo.Context) error {
	if err := s.govRequireAdmin(c); err != nil {
		return err
	}
	if err := s.groupInWorkspace(c); err != nil {
		return err
	}
	var req struct {
		UserID string `json:"user_id"`
	}
	if err := c.Bind(&req); err != nil || req.UserID == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "user_id is required"})
	}
	gid := c.Param("gid")
	if err := s.AuthStore.AddGroupMember(c.Request().Context(), gid, req.UserID); err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}
	s.emitAudit(c, auditEvent{Type: platev.EventType("group.member_added"), ResourceType: "group", ResourceID: gid, Data: map[string]string{"user_id": req.UserID}})
	return c.NoContent(http.StatusNoContent)
}

func (s *Server) HandleRemoveGroupMember(c echo.Context) error {
	if err := s.govRequireAdmin(c); err != nil {
		return err
	}
	if err := s.groupInWorkspace(c); err != nil {
		return err
	}
	gid := c.Param("gid")
	uid := c.Param("uid")
	if err := s.AuthStore.RemoveGroupMember(c.Request().Context(), gid, uid); err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}
	s.emitAudit(c, auditEvent{Type: platev.EventType("group.member_removed"), ResourceType: "group", ResourceID: gid, Data: map[string]string{"user_id": uid}})
	return c.NoContent(http.StatusNoContent)
}

type groupBindingRequest struct {
	ProjectID string   `json:"project_id"`
	RoleID    string   `json:"role_id"`
	Languages []string `json:"languages,omitempty"`
}

func (s *Server) HandleListGroupBindings(c echo.Context) error {
	if err := s.groupInWorkspace(c); err != nil {
		return err
	}
	bindings, err := s.AuthStore.ListGroupRoleBindings(c.Request().Context(), c.Param("gid"))
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}
	if bindings == nil {
		bindings = []*platauth.GroupRoleBinding{}
	}
	return c.JSON(http.StatusOK, bindings)
}

func (s *Server) HandleAddGroupBinding(c echo.Context) error {
	if err := s.govRequireAdmin(c); err != nil {
		return err
	}
	if err := s.groupInWorkspace(c); err != nil {
		return err
	}
	var req groupBindingRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}
	if req.ProjectID == "" || req.RoleID == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "project_id and role_id are required"})
	}
	wsID, _ := c.Get("workspace_id").(string)
	// Verify the target project belongs to this workspace (prevents binding a
	// group to a project in another workspace).
	if s.ContentStore != nil {
		proj, err := s.ContentStore.GetProject(c.Request().Context(), req.ProjectID)
		if err != nil || proj == nil || proj.WorkspaceID != wsID {
			return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "project not in this workspace"})
		}
	}
	if _, err := s.AuthStore.GetRoleTemplate(c.Request().Context(), wsID, req.RoleID); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid role_id"})
	}
	b := &platauth.GroupRoleBinding{
		GroupID:     c.Param("gid"),
		WorkspaceID: wsID,
		ProjectID:   req.ProjectID,
		RoleID:      req.RoleID,
		Languages:   req.Languages,
	}
	if err := s.AuthStore.AddGroupRoleBinding(c.Request().Context(), b); err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}
	s.emitAudit(c, auditEvent{Type: platev.EventType("group.binding_added"), ProjectID: req.ProjectID, ResourceType: "group_binding", ResourceID: b.ID, Data: map[string]string{"group_id": b.GroupID, "role_id": req.RoleID}})
	return c.JSON(http.StatusCreated, b)
}

func (s *Server) HandleRemoveGroupBinding(c echo.Context) error {
	if err := s.govRequireAdmin(c); err != nil {
		return err
	}
	if err := s.groupInWorkspace(c); err != nil {
		return err
	}
	wsID, _ := c.Get("workspace_id").(string)
	bid := c.Param("bid")
	if err := s.AuthStore.RemoveGroupRoleBinding(c.Request().Context(), wsID, bid); err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}
	s.emitAudit(c, auditEvent{Type: platev.EventType("group.binding_removed"), ResourceType: "group_binding", ResourceID: bid})
	return c.NoContent(http.StatusNoContent)
}

// ── Deny rules ──────────────────────────────────────────────────────────────

type denyRuleRequest struct {
	SubjectType string   `json:"subject_type"` // user | role | group
	SubjectID   string   `json:"subject_id"`
	ProjectID   string   `json:"project_id,omitempty"` // empty = workspace-wide
	Permissions []string `json:"permissions"`
	Reason      string   `json:"reason,omitempty"`
}

func (s *Server) HandleListDenyRules(c echo.Context) error {
	if err := s.govRequireAdmin(c); err != nil {
		return err
	}
	wsID, _ := c.Get("workspace_id").(string)
	rules, err := s.AuthStore.ListDenyRules(c.Request().Context(), wsID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}
	if rules == nil {
		rules = []*platauth.DenyRule{}
	}
	return c.JSON(http.StatusOK, rules)
}

func (s *Server) HandleCreateDenyRule(c echo.Context) error {
	if err := s.govRequireAdmin(c); err != nil {
		return err
	}
	var req denyRuleRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}
	st := platauth.DenySubjectType(req.SubjectType)
	if st != platauth.DenySubjectUser && st != platauth.DenySubjectRole && st != platauth.DenySubjectGroup {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "subject_type must be user, role, or group"})
	}
	if req.SubjectID == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "subject_id is required"})
	}
	wsID, _ := c.Get("workspace_id").(string)
	r := &platauth.DenyRule{
		WorkspaceID: wsID,
		SubjectType: st,
		SubjectID:   req.SubjectID,
		ProjectID:   req.ProjectID,
		DeniedPerms: platauth.ParsePermissions(req.Permissions),
		Reason:      req.Reason,
	}
	if err := s.AuthStore.CreateDenyRule(c.Request().Context(), r); err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}
	s.emitAudit(c, auditEvent{
		Type: platev.EventType("deny_rule.created"), ProjectID: req.ProjectID,
		ResourceType: "deny_rule", ResourceID: r.ID,
		Data: map[string]string{"subject_type": req.SubjectType, "subject_id": req.SubjectID, "denied": r.DeniedPerms.String()},
	})
	return c.JSON(http.StatusCreated, r)
}

func (s *Server) HandleDeleteDenyRule(c echo.Context) error {
	if err := s.govRequireAdmin(c); err != nil {
		return err
	}
	wsID, _ := c.Get("workspace_id").(string)
	rid := c.Param("rid")
	if err := s.AuthStore.DeleteDenyRule(c.Request().Context(), wsID, rid); err != nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: err.Error()})
	}
	s.emitAudit(c, auditEvent{Type: platev.EventType("deny_rule.deleted"), ResourceType: "deny_rule", ResourceID: rid})
	return c.NoContent(http.StatusNoContent)
}

// ── Workspace role overrides ────────────────────────────────────────────────

func (s *Server) HandleListRoleOverrides(c echo.Context) error {
	if err := s.govRequireAdmin(c); err != nil {
		return err
	}
	wsID, _ := c.Get("workspace_id").(string)
	overrides, err := s.AuthStore.ListWorkspaceRoleOverrides(c.Request().Context(), wsID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}
	out := map[string][]string{}
	for role, perms := range overrides {
		out[string(role)] = perms.Strings()
	}
	return c.JSON(http.StatusOK, out)
}

type roleOverrideRequest struct {
	Permissions []string `json:"permissions"`
}

func (s *Server) HandleSetRoleOverride(c echo.Context) error {
	if err := s.govRequireAdmin(c); err != nil {
		return err
	}
	role := platauth.Role(c.Param("role"))
	if !platauth.ValidRoles[role] {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid role"})
	}
	var req roleOverrideRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}
	wsID, _ := c.Get("workspace_id").(string)
	perms := platauth.ParsePermissions(req.Permissions)
	if err := s.AuthStore.SetWorkspaceRoleOverride(c.Request().Context(), wsID, role, perms); err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}
	s.emitAudit(c, auditEvent{
		Type: platev.EventType("role_override.set"), ResourceType: "workspace_role", ResourceID: string(role),
		After: map[string]string{"permissions": perms.String()},
	})
	return c.JSON(http.StatusOK, map[string]any{"role": role, "permissions": perms.Strings()})
}

// ── Separation of duties ────────────────────────────────────────────────────

func (s *Server) HandleGetSoD(c echo.Context) error {
	wsID, _ := c.Get("workspace_id").(string)
	mode, err := s.AuthStore.GetSoDMode(c.Request().Context(), wsID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]string{"mode": string(mode)})
}

func (s *Server) HandleSetSoD(c echo.Context) error {
	if err := s.govRequireAdmin(c); err != nil {
		return err
	}
	var req struct {
		Mode string `json:"mode"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}
	mode := platauth.SoDMode(req.Mode)
	if !platauth.ValidSoDModes[mode] {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "mode must be off, warn, or block"})
	}
	wsID, _ := c.Get("workspace_id").(string)
	if err := s.AuthStore.SetSoDMode(c.Request().Context(), wsID, mode); err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}
	s.emitAudit(c, auditEvent{Type: platev.EventType("sod.mode_changed"), ResourceType: "sod_policy", After: map[string]string{"mode": string(mode)}})
	return c.JSON(http.StatusOK, map[string]string{"mode": string(mode)})
}
