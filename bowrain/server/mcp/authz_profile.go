package mcp

import (
	"context"
	"errors"
	"log/slog"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	coreprofile "github.com/neokapi/neokapi/core/profile"
)

// ErrProfileNotFound is the one answer a voice tool gives for a profile the
// caller cannot reach, whether it is absent or owned by a workspace the caller
// does not belong to. The two cases read alike for the reason ErrProjectNotFound
// gives: a separate "forbidden" would confirm that a guessed profile id names
// something real.
var ErrProfileNotFound = errors.New("voice profile not found")

// authorizeProfile resolves a client-supplied profile_id and proves the
// authenticated principal belongs to the workspace that owns the profile.
// Every tool taking a profile_id goes through it before it reads or writes the
// profile, so the membership rule and its failure answer have one
// implementation. It is the MCP sibling of profileInRequestWorkspace, the REST
// guard on the same store: coreprofile.Store.GetProfile takes a global id and
// ignores VoiceProfile.Scope, so nothing below this point knows about tenants.
//
// The profile itself is returned, so a caller that goes on to use it reads the
// store once.
func (s *MCPServer) authorizeProfile(ctx context.Context, req *mcp.CallToolRequest, profileID string) (*coreprofile.VoiceProfile, error) {
	return s.authorizeProfileForUser(ctx, callerID(req), profileID)
}

// authorizeOptionalProfile is authorizeProfile for a tool whose profile_id is
// optional. An empty id passes through, leaving the profile to be resolved from
// the voice-scope ladder instead.
func (s *MCPServer) authorizeOptionalProfile(ctx context.Context, req *mcp.CallToolRequest, profileID string) error {
	if profileID == "" {
		return nil
	}
	_, err := s.authorizeProfile(ctx, req, profileID)
	return err
}

func (s *MCPServer) authorizeProfileForUser(ctx context.Context, userID, profileID string) (*coreprofile.VoiceProfile, error) {
	if profileID == "" || s.voiceStore == nil {
		return nil, ErrProfileNotFound
	}
	p, err := s.voiceStore.GetProfile(ctx, profileID)
	if err != nil {
		// The store reports an absent profile and a failed read as the same
		// plain error, so both deny. A transient failure must not open the
		// workspace boundary either way; the cause is logged rather than
		// returned, which would tell the caller a guessed id exists.
		slog.DebugContext(ctx, "mcp: voice profile lookup failed; denying",
			"profile_id", profileID, "error", err)
		return nil, ErrProfileNotFound
	}
	if p == nil || !s.mayReachProfile(ctx, userID, p) {
		return nil, ErrProfileNotFound
	}
	return p, nil
}

// mayReachProfile reports whether the principal belongs to the workspace that
// owns p. VoiceProfile.Scope is that workspace: the platform writes its tenant
// key there, and a profile carrying none belongs to nobody a membership check
// can name. Without a membership checker every profile is reachable, matching
// mayReachProject: a deployment with no auth has no tenant to cross.
func (s *MCPServer) mayReachProfile(ctx context.Context, userID string, p *coreprofile.VoiceProfile) bool {
	if s.membership == nil {
		return true
	}
	return userID != "" && p != nil && p.Scope != "" && s.membership.IsMember(ctx, p.Scope, userID)
}
