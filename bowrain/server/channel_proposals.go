package server

import (
	"context"
	"log/slog"
	"net/http"
	"sort"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	coresync "github.com/neokapi/neokapi/bowrain/core/sync"
	fwproject "github.com/neokapi/neokapi/core/project"

	"github.com/labstack/echo/v4"
)

// The workspace's channel-slug equivalence surface.
//
// Slug fragmentation is invisible from inside a project: a recipe declares its
// own channels and resolves them offline, and nothing in it can know that the
// workspace's other project spells the same surface differently. The workspace
// can see it, so it says so — and stops there. Rewriting a pushed slug would
// make one recipe mean two things depending on whether it had been connected,
// which is the property that makes offline resolution worth having.

// raiseChannelAliasProposals compares the channels the pushed project declares
// against the ones its workspace's other projects hold, and records the pairs
// that look like one channel.
//
// It runs after a push, beside the graph rebuild, because that is when the
// workspace's vocabulary last changed. Failure is logged and dropped: a
// proposal is an observation about naming, and losing one costs the next push's
// worth of delay.
func (s *Server) raiseChannelAliasProposals(ctx context.Context, proj *platstore.Project, stream string) {
	aliases, ok := s.ContentStore.(platstore.ChannelAliasStore)
	if !ok || proj == nil || proj.WorkspaceID == "" {
		return
	}
	mine, err := s.channelsByProfile(ctx, proj.ID, stream)
	if err != nil {
		slog.Warn("channel proposals: skipped", "reason", "read own collections",
			"project", proj.ID, "error", err)
		return
	}
	if len(mine) == 0 {
		return
	}
	theirs, err := s.workspaceChannelsByProfile(ctx, proj.WorkspaceID, proj.ID)
	if err != nil {
		slog.Warn("channel proposals: skipped", "reason", "read workspace collections",
			"workspace", proj.WorkspaceID, "error", err)
		return
	}

	var proposals []platstore.ChannelAliasProposal
	for profile, channels := range mine {
		for _, alias := range coresync.ProposeChannelAliases(sortedKeys(channels), sortedKeys(theirs[profile])) {
			proposals = append(proposals, platstore.ChannelAliasProposal{
				WorkspaceID:     proj.WorkspaceID,
				Profile:         profile,
				ProposedChannel: alias.Proposed,
				ExistingChannel: alias.Existing,
				Evidence:        alias.Evidence,
				ProjectID:       proj.ID,
				Collection:      channels[alias.Proposed],
			})
		}
	}
	if len(proposals) == 0 {
		return
	}
	if _, err := aliases.UpsertChannelAliasProposals(ctx, proposals); err != nil {
		slog.Warn("channel proposals: write failed", "workspace", proj.WorkspaceID, "error", err)
		return
	}
	slog.Info("channel proposals: recorded",
		"workspace", proj.WorkspaceID, "project", proj.ID, "count", len(proposals))
}

// channelsByProfile reads one project stream's declared points as
// profile → channel → the collection that declared it.
func (s *Server) channelsByProfile(ctx context.Context, projectID, stream string) (map[string]map[string]string, error) {
	collections, err := s.ContentStore.ListCollections(ctx, projectID, stream)
	if err != nil {
		return nil, err
	}
	out := map[string]map[string]string{}
	for _, c := range collections {
		if c == nil {
			continue
		}
		channel := c.Context[fwproject.ChannelAxis]
		if channel == "" {
			continue
		}
		profile := c.Context[fwproject.ProductAxis]
		if out[profile] == nil {
			out[profile] = map[string]string{}
		}
		if _, held := out[profile][channel]; !held {
			out[profile][channel] = c.Name
		}
	}
	return out, nil
}

// workspaceChannelsByProfile reads the points every OTHER project in the
// workspace holds. The pushing project is excluded so it cannot propose an
// equivalence with itself — a project's own two channels are two channels, and
// only its recipe gets to say otherwise.
//
// Each project is read on its default stream, resolved through
// projectDefaultStream: a project created through the API carries no stream of
// its own, and asking the store for the collections on the empty stream returns
// none — which reads exactly like a workspace that holds no channels, and
// silently proposes nothing.
func (s *Server) workspaceChannelsByProfile(ctx context.Context, workspaceID, exceptProjectID string) (map[string]map[string]string, error) {
	projects, err := s.ContentStore.ListProjects(ctx)
	if err != nil {
		return nil, err
	}
	out := map[string]map[string]string{}
	for _, p := range projects {
		if p == nil || p.ID == exceptProjectID || p.WorkspaceID != workspaceID || p.Archived {
			continue
		}
		byProfile, err := s.channelsByProfile(ctx, p.ID, projectDefaultStream(p))
		if err != nil {
			return nil, err
		}
		for profile, channels := range byProfile {
			if out[profile] == nil {
				out[profile] = map[string]string{}
			}
			for channel, collection := range channels {
				if _, held := out[profile][channel]; !held {
					out[profile][channel] = collection
				}
			}
		}
	}
	return out, nil
}

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// channelProposalsResponse is the listing's body.
type channelProposalsResponse struct {
	Proposals []platstore.ChannelAliasProposal `json:"proposals"`
}

// HandleListChannelAliasProposals reports the workspace's channel-slug
// equivalence proposals: GET /:ws/context/channel-proposals.
//
// Readable by anyone who may read content, because the listing says only what
// the workspace already holds. Settling one takes the governance permission —
// see HandleJudgeChannelAliasProposal.
func (s *Server) HandleListChannelAliasProposals(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermViewContent); err != nil {
		return err
	}
	aliases, ok := s.ContentStore.(platstore.ChannelAliasStore)
	if !ok {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "store not configured"})
	}
	wsID, _ := c.Get("workspace_id").(string)
	if wsID == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "no workspace"})
	}
	proposals, err := aliases.ListChannelAliasProposals(c.Request().Context(), wsID, c.QueryParam("status"))
	if err != nil {
		return serverErr(c, err)
	}
	if proposals == nil {
		proposals = []platstore.ChannelAliasProposal{}
	}
	return c.JSON(http.StatusOK, channelProposalsResponse{Proposals: proposals})
}

// channelProposalJudgement is the judge request's body: the proposal's key and
// the verdict.
type channelProposalJudgement struct {
	Profile         string `json:"profile"`
	ProposedChannel string `json:"proposed_channel"`
	ExistingChannel string `json:"existing_channel"`
	// Status is "accepted" or "dismissed".
	Status string `json:"status"`
}

// HandleJudgeChannelAliasProposal settles one proposal:
// POST /:ws/context/channel-proposals/judge.
//
// Both verdicts are records, not rewrites. Accepting says the two spellings name
// one channel and the workspace will read them as one; it does not touch either
// project's recipe, because resolution belongs to the recipe and happens
// offline. Dismissing says they are two channels, and that is what stops the
// next push from raising the pair again — the upsert leaves a judged row's
// status alone, so a re-sighting refreshes where it was seen and nothing more.
func (s *Server) HandleJudgeChannelAliasProposal(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermManageBrand); err != nil {
		return err
	}
	aliases, ok := s.ContentStore.(platstore.ChannelAliasStore)
	if !ok {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "store not configured"})
	}
	wsID, _ := c.Get("workspace_id").(string)
	if wsID == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "no workspace"})
	}

	var body channelProposalJudgement
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid request body"})
	}
	if body.Status != platstore.ChannelAliasAccepted && body.Status != platstore.ChannelAliasDismissed {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "status must be accepted or dismissed",
		})
	}
	if body.ProposedChannel == "" || body.ExistingChannel == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "proposed_channel and existing_channel are required",
		})
	}

	actor, _ := c.Get("user_id").(string)
	found, err := aliases.JudgeChannelAliasProposal(c.Request().Context(), platstore.ChannelAliasJudgement{
		WorkspaceID:     wsID,
		Profile:         body.Profile,
		ProposedChannel: body.ProposedChannel,
		ExistingChannel: body.ExistingChannel,
		Status:          body.Status,
		JudgedBy:        actor,
	})
	if err != nil {
		return serverErr(c, err)
	}
	if !found {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: "no such proposal"})
	}

	proposals, err := aliases.ListChannelAliasProposals(c.Request().Context(), wsID, "")
	if err != nil {
		return serverErr(c, err)
	}
	for _, p := range proposals {
		if p.Profile == body.Profile &&
			p.ProposedChannel == body.ProposedChannel &&
			p.ExistingChannel == body.ExistingChannel {
			return c.JSON(http.StatusOK, p)
		}
	}
	return c.JSON(http.StatusNotFound, ErrorResponse{Error: "no such proposal"})
}
