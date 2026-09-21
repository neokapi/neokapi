package host

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/neokapi/neokapi/core/contextop"
	"github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/workspace"
)

// Growing a project's context out of ordinary work (C-11), agent half.
//
// Most projects record nothing. What they know is in the heads of the people
// who write them, and an agent reading such a project learns the same facts
// every session and forgets them at the end of it. These tools are where that
// goes instead: one small call per habit, cheap enough to make mid-task.
//
// Each tool wraps exactly one call in host/contextops.go and adds no rule of
// its own. What an agent may record is decided by core/contextop's policy, not
// here: observe, propose and record a correction, all of which advise and none
// of which can fail a check. Confirming, discarding another actor's work,
// reverting and widening belong to a person, so this surface carries no tool
// for them at all.
//
// The actor is never the caller's to state. Kind is agent, the name is what the
// client called itself at initialize, and the session is minted once per server
// process, so everything one run recorded reads back and reverts together.

func init() { RegisterMCPToolFactory(registerContextGrowthMCPTools) }

func registerContextGrowthMCPTools(server *mcp.Server, a *App) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "context_observe",
		Description: "Record one fact you noticed about how this project writes: a product name as it spells it, " +
			"a spelling it is consistent about, who its text addresses, the register it keeps. " +
			"Call it while you read, as soon as you notice something, one fact per call. " +
			"It states no rule, changes no check and takes effect immediately. " +
			"Say where you saw it: `path` and `quote` are what let a person judge the fact later, " +
			"and an observation with neither is worth much less than one with both.",
	}, a.handleContextObserve)

	mcp.AddTool(server, &mcp.Tool{
		Name: "context_propose",
		Description: "Propose a rule about a word: write this instead of that. " +
			"Use it when the project is consistent about a word and nothing records it yet. " +
			"Evidence is required: `path` names the file you saw it in and `quote` the wording as it stands there. " +
			"The result is a CANDIDATE. It advises from the moment you record it, every check reports it, " +
			"and no check can fail on it until a person confirms it. Do not tell anyone a rule is now in force.",
	}, a.handleContextPropose)

	mcp.AddTool(server, &mcp.Tool{
		Name: "context_correct",
		Description: "Record that the person changed your wording: what you wrote, what they replaced it with, and where. " +
			"Call it as soon as you see the edit, before you carry on. " +
			"A correction is evidence about this project's wording, and it is the cheapest context there is, " +
			"because someone has already made the judgement. " +
			"Set `propose` to also record the rule it implies, so the next use of the old wording is reported.",
	}, a.handleContextCorrect)

	mcp.AddTool(server, &mcp.Tool{
		Name: "context_session_summary",
		Description: "Report what this session recorded, proposed and had confirmed. " +
			"Call it before you say the work is done, and end your report with what it says, " +
			"including the command it gives for reviewing the session.",
	}, a.handleContextSessionSummary)
}

// ─── The session this server process records under ──────────────────────────

// mcpRunSession is the identity every operation this process records carries. It
// is minted once: a client reconnecting mid-task is the same run to a person
// reading the log, and a second id would split one session's work in two.
var mcpRunSession = sync.OnceValue(func() mcpSessionIdentity {
	return mcpSessionIdentity{ID: newSessionID(), Started: time.Now().UTC()}
})

// mcpSessionIdentity is the session id and when this process opened it.
type mcpSessionIdentity struct {
	ID      string
	Started time.Time
}

// MCPSessionID is the session `kapi mcp` records under, as `kapi context log
// --session` and `kapi context revert --session` take it.
func MCPSessionID() string { return mcpRunSession().ID }

// newSessionID mints a session id. It is short enough to type at a command
// line, because reverting a session is something a person does by hand.
func newSessionID() string {
	var raw [6]byte
	if _, err := rand.Read(raw[:]); err != nil {
		// A clock reading distinguishes this process from the others on the
		// machine, which is the whole job of a session id.
		return "s" + strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return "s" + hex.EncodeToString(raw[:])
}

// mcpAgentActor is who a call is recorded as. The caller never says: an MCP
// client that could claim to be a person would be claiming the rights the
// policy reserves for one.
func mcpAgentActor(req *mcp.CallToolRequest) contextop.Actor {
	return contextop.Actor{
		Kind:    contextop.ActorAgent,
		Name:    mcpClientName(req),
		Session: MCPSessionID(),
	}
}

// mcpClientName is the name the client gave at initialize, empty when it gave
// none or when the request carries no session (a direct call in a test).
func mcpClientName(req *mcp.CallToolRequest) string {
	if req == nil {
		return ""
	}
	if info := req.ClientInfo(); info != nil {
		return info.Name
	}
	return ""
}

// NoteMCPSession records that this server process is at work in a project, so
// another process can show it. It is a heartbeat over one row: the project, the
// client's name, when the session opened and when it was last seen.
//
// Every failure is swallowed. A session note is something the workspace offers
// other surfaces, and an agent mid-task has nothing to do about a workspace
// that will not open; failing its call over a note would be the tool getting in
// the way of the work it exists to support.
func (a *App) NoteMCPSession(ctx context.Context, recipe, agent string) {
	ws, err := a.Workspace(ctx)
	if err != nil || ws == nil {
		return
	}
	var key workspace.ProjectKey
	if recipe != "" {
		if identity, _ := recipeIdentity(recipe); identity != "" {
			key = workspace.ProjectKey(identity)
		}
	}
	session := mcpRunSession()
	_ = ws.NoteAgentSession(ctx, workspace.AgentSession{
		ID:       session.ID,
		Project:  key,
		Agent:    agent,
		Started:  session.Started,
		LastSeen: time.Now().UTC(),
	})
}

// MCPSessionMiddleware keeps this server's session note fresh. It runs on every
// request the server answers, so a session that is reading rather than writing
// still says it is here.
func (a *App) MCPSessionMiddleware() mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			a.NoteMCPSession(ctx, a.mcpRecipePath, sessionClientName(req))
			return next(ctx, method, req)
		}
	}
}

// sessionClientName reads the client's name off any request. It is empty until
// the client has introduced itself, which is one request in.
func sessionClientName(req mcp.Request) string {
	if req == nil {
		return ""
	}
	ss, ok := req.GetSession().(*mcp.ServerSession)
	if !ok || ss == nil {
		return ""
	}
	params := ss.InitializeParams()
	if params == nil || params.ClientInfo == nil {
		return ""
	}
	return params.ClientInfo.Name
}

// ─── What the write tools take ──────────────────────────────────────────────

// contextObserveInput is one fact an agent noticed while reading a project.
type contextObserveInput struct {
	Text    string `json:"text" jsonschema:"the fact, in one sentence, in your own words"`
	Path    string `json:"path,omitempty" jsonschema:"the project-relative file you saw it in"`
	Unit    string `json:"unit,omitempty" jsonschema:"the block or unit id inside that file, when you have one"`
	Quote   string `json:"quote,omitempty" jsonschema:"the wording you saw, quoted from the file"`
	Project string `json:"project,omitempty" jsonschema:"the project this call acts on: its kapi.yaml recipe, its root directory, or any path inside it (default: the project the MCP server started in)"`
}

// contextProposeInput is a candidate rule about one word.
type contextProposeInput struct {
	Term     string `json:"term" jsonschema:"the word or phrase the rule is about"`
	Use      string `json:"use,omitempty" jsonschema:"what to write instead; a rule without one records the word and matches nothing"`
	List     string `json:"list,omitempty" jsonschema:"propose a voice-profile rule in this list instead of a project term: forbidden, competitor or preferred"`
	Severity string `json:"severity,omitempty" jsonschema:"how hard the rule bites once a person confirms it: minor and neutral report, anything else fails a check"`
	Note     string `json:"note,omitempty" jsonschema:"why you are proposing it"`
	Path     string `json:"path" jsonschema:"the project-relative file you saw the wording in"`
	Unit     string `json:"unit,omitempty" jsonschema:"the block or unit id inside that file, when you have one"`
	Quote    string `json:"quote,omitempty" jsonschema:"the wording as it stands there"`
	Project  string `json:"project,omitempty" jsonschema:"the project this call acts on: its kapi.yaml recipe, its root directory, or any path inside it (default: the project the MCP server started in)"`
}

// contextCorrectInput is wording the person changed, at the place they changed
// it.
type contextCorrectInput struct {
	From     string `json:"from" jsonschema:"the wording that was there"`
	To       string `json:"to" jsonschema:"the wording that replaced it"`
	Path     string `json:"path" jsonschema:"the project-relative file they changed it in"`
	Unit     string `json:"unit,omitempty" jsonschema:"the block or unit id inside that file, when you have one"`
	Quote    string `json:"quote,omitempty" jsonschema:"the sentence the change was made in"`
	Propose  bool   `json:"propose,omitempty" jsonschema:"also record the rule the change implies, so the next use of the old wording is reported"`
	Severity string `json:"severity,omitempty" jsonschema:"how hard that rule bites once a person confirms it: minor and neutral report, anything else fails a check"`
	Note     string `json:"note,omitempty" jsonschema:"what they said about the change"`
	Project  string `json:"project,omitempty" jsonschema:"the project this call acts on: its kapi.yaml recipe, its root directory, or any path inside it (default: the project the MCP server started in)"`
}

// contextSessionInput asks what one session did.
type contextSessionInput struct {
	Session string `json:"session,omitempty" jsonschema:"the session to summarize (default: this MCP server's own)"`
	Project string `json:"project,omitempty" jsonschema:"the project this call acts on: its kapi.yaml recipe, its root directory, or any path inside it (default: the project the MCP server started in)"`
}

// ─── What they answer with ──────────────────────────────────────────────────

// contextRecordOutput is what a write tool answers: the operation it recorded,
// what that operation counts as, and how a person reviews it.
type contextRecordOutput struct {
	// Operation is the id, which is what `kapi context confirm` takes.
	Operation string `json:"operation"`
	// Kind is what was done: observe, propose or correct.
	Kind string `json:"kind"`
	// Status is what the operation counts as. `candidate` advises and fails
	// nothing until a person confirms it.
	Status string `json:"status"`
	// Session groups everything this run recorded.
	Session string `json:"session"`
	// Project is the project it was recorded in.
	Project string `json:"project,omitempty"`
	// Recorded is the one line a log prints for it.
	Recorded string `json:"recorded"`
	// Review is the command a person reads this session's work with.
	Review string `json:"review"`
	// Next says what happens to it, so an answer is never read as a rule now in
	// force.
	Next string `json:"next"`
}

// contextSessionOutput is what one session did, as an agent reports it.
type contextSessionOutput struct {
	Session string `json:"session"`
	Project string `json:"project,omitempty"`
	Agent   string `json:"agent,omitempty"`
	// Operations is how many operations the session recorded in all.
	Operations int `json:"operations"`
	// Observed, Proposed and Corrected count what it recorded.
	Observed  int `json:"observed"`
	Proposed  int `json:"proposed"`
	Corrected int `json:"corrected"`
	// Candidates is how many of them are still waiting for a decision,
	// Confirmed how many a person made binding, and Discarded and Reverted what
	// became of the rest.
	Candidates int `json:"candidates"`
	Confirmed  int `json:"confirmed"`
	Discarded  int `json:"discarded"`
	Reverted   int `json:"reverted"`
	// First and Last bound the session in time, RFC 3339.
	First string `json:"first,omitempty"`
	Last  string `json:"last,omitempty"`
	// Review is the command a person reads the session with.
	Review string `json:"review"`
	// Report is the sentence to end a task report with.
	Report string `json:"report"`
}

// ─── Handlers ───────────────────────────────────────────────────────────────

func (a *App) handleContextObserve(ctx context.Context, req *mcp.CallToolRequest, in contextObserveInput) (*mcp.CallToolResult, contextRecordOutput, error) {
	recipe, err := a.RequireMCPCallProject(in.Project)
	if err != nil {
		return nil, contextRecordOutput{}, err
	}
	if strings.TrimSpace(in.Text) == "" {
		return nil, contextRecordOutput{}, errors.New("context_observe: say what you noticed in `text`")
	}
	op, err := a.RecordContextObservation(ctx, ContextObserveRequest{
		Actor:    mcpAgentActor(req),
		Project:  recipe,
		Text:     in.Text,
		Evidence: mcpEvidence(in.Path, in.Unit, in.Quote),
	})
	if err != nil {
		return nil, contextRecordOutput{}, err
	}
	a.NoteMCPSession(ctx, recipe, mcpClientName(req))
	out := recordedOutput(op)
	out.Next = "nothing has to happen to it. It is material for a rule somebody proposes later."
	return nil, out, nil
}

func (a *App) handleContextPropose(ctx context.Context, req *mcp.CallToolRequest, in contextProposeInput) (*mcp.CallToolResult, contextRecordOutput, error) {
	recipe, err := a.RequireMCPCallProject(in.Project)
	if err != nil {
		return nil, contextRecordOutput{}, err
	}
	if strings.TrimSpace(in.Term) == "" {
		return nil, contextRecordOutput{}, errors.New("context_propose: name the word the rule is about in `term`")
	}
	evidence := mcpEvidence(in.Path, in.Unit, in.Quote)
	if err := requireEvidence("context_propose", evidence); err != nil {
		return nil, contextRecordOutput{}, err
	}
	rule := profile.TermRule{Term: in.Term, Replacement: in.Use, Severity: in.Severity, Note: in.Note}
	proposal := ContextProposeRequest{
		Actor:    mcpAgentActor(req),
		Project:  recipe,
		Evidence: evidence,
		Note:     in.Note,
	}
	if in.List != "" {
		proposal.Voice = &contextop.VoiceRule{List: in.List, Rule: rule}
	} else {
		proposal.Term = &rule
	}
	op, err := a.ProposeContextRule(ctx, proposal)
	if err != nil {
		return nil, contextRecordOutput{}, err
	}
	a.NoteMCPSession(ctx, recipe, mcpClientName(req))
	out := recordedOutput(op)
	out.Next = fmt.Sprintf(
		"a candidate: every check reports it and none can fail on it. It binds when a person runs `kapi context confirm %s`.",
		op.ID)
	return nil, out, nil
}

func (a *App) handleContextCorrect(ctx context.Context, req *mcp.CallToolRequest, in contextCorrectInput) (*mcp.CallToolResult, contextRecordOutput, error) {
	recipe, err := a.RequireMCPCallProject(in.Project)
	if err != nil {
		return nil, contextRecordOutput{}, err
	}
	if strings.TrimSpace(in.From) == "" || strings.TrimSpace(in.To) == "" {
		return nil, contextRecordOutput{}, errors.New(
			"context_correct: give both wordings, `from` (what was there) and `to` (what replaced it)")
	}
	evidence := mcpEvidence(in.Path, in.Unit, in.Quote)
	if err := requireEvidence("context_correct", evidence); err != nil {
		return nil, contextRecordOutput{}, err
	}
	op, err := a.RecordContextCorrection(ctx, ContextCorrectRequest{
		Actor:    mcpAgentActor(req),
		Project:  recipe,
		From:     in.From,
		To:       in.To,
		Evidence: evidence,
		Propose:  in.Propose,
		Severity: in.Severity,
		Note:     in.Note,
	})
	if err != nil {
		return nil, contextRecordOutput{}, err
	}
	a.NoteMCPSession(ctx, recipe, mcpClientName(req))
	out := recordedOutput(op)
	out.Recorded = fmt.Sprintf("correction %q to %q", in.From, in.To)
	if in.Propose {
		out.Next = fmt.Sprintf(
			"a candidate rule came with it: every check reports it and none can fail on it. "+
				"It binds when a person runs `kapi context confirm %s`.", op.ID)
	} else {
		out.Next = "recorded as evidence. Pass `propose` next time to record the rule it implies with it."
	}
	return nil, out, nil
}

func (a *App) handleContextSessionSummary(ctx context.Context, req *mcp.CallToolRequest, in contextSessionInput) (*mcp.CallToolResult, contextSessionOutput, error) {
	recipe, err := a.RequireMCPCallProject(in.Project)
	if err != nil {
		return nil, contextSessionOutput{}, err
	}
	session := strings.TrimSpace(in.Session)
	if session == "" {
		session = MCPSessionID()
	}
	summary, err := a.ContextSessionSummary(ctx, ContextSessionRequest{Project: recipe, Session: session})
	if err != nil {
		return nil, contextSessionOutput{}, err
	}
	a.NoteMCPSession(ctx, recipe, mcpClientName(req))
	return nil, sessionOutput(summary), nil
}

// ─── Rendering ──────────────────────────────────────────────────────────────

// recordedOutput renders one recorded operation. The caller fills in Next,
// which is the part that differs by kind.
func recordedOutput(op ContextOperation) contextRecordOutput {
	return contextRecordOutput{
		Operation: op.ID,
		Kind:      string(op.Kind),
		Status:    string(op.Status),
		Session:   op.Actor.Session,
		Project:   string(op.Project),
		Recorded:  op.Subject.Describe(),
		Review:    "kapi context log --session " + op.Actor.Session,
	}
}

// sessionOutput renders a session summary flat, counted by what it recorded and
// by what became of it, with the sentence an agent ends its report on.
func sessionOutput(s contextop.SessionSummary) contextSessionOutput {
	out := contextSessionOutput{
		Session:    s.Session,
		Project:    string(s.Project),
		Agent:      s.Actor.Name,
		Operations: s.Operations,
		Observed:   s.ByKind[contextop.KindObserve],
		Proposed:   s.ByKind[contextop.KindPropose],
		Corrected:  s.ByKind[contextop.KindCorrect],
		Candidates: s.ByStatus[contextop.StatusCandidate],
		Confirmed:  s.ByStatus[contextop.StatusConfirmed],
		Discarded:  s.ByStatus[contextop.StatusDiscarded],
		Reverted:   s.ByStatus[contextop.StatusReverted],
		Review:     "kapi context log --session " + s.Session,
	}
	if !s.First.IsZero() {
		out.First = s.First.UTC().Format(time.RFC3339)
	}
	if !s.Last.IsZero() {
		out.Last = s.Last.UTC().Format(time.RFC3339)
	}
	out.Report = sessionReport(out)
	return out
}

// sessionReport is the sentence an agent ends its task report with. A session
// that recorded nothing says so, because "I recorded nothing about this
// project" is a fact a person can act on.
func sessionReport(s contextSessionOutput) string {
	if s.Operations == 0 {
		return fmt.Sprintf("Recorded nothing about this project's context (session %s).", s.Session)
	}
	var parts []string
	if s.Observed > 0 {
		parts = append(parts, fmt.Sprintf("%s observed", plural(s.Observed, "fact", "facts")))
	}
	if s.Proposed > 0 {
		parts = append(parts, fmt.Sprintf("%s proposed", plural(s.Proposed, "rule", "rules")))
	}
	if s.Corrected > 0 {
		parts = append(parts, fmt.Sprintf("%s recorded", plural(s.Corrected, "correction", "corrections")))
	}
	report := "Context, session " + s.Session + ": " + strings.Join(parts, ", ") + "."
	if s.Candidates > 0 {
		report += fmt.Sprintf(" %s %s waiting for a decision.",
			plural(s.Candidates, "candidate", "candidates"), verb(s.Candidates, "is", "are"))
	}
	return report + " Review with `" + s.Review + "`."
}

// mcpEvidence renders the location a call named as the evidence behind it.
func mcpEvidence(path, unit, quote string) []contextop.Evidence {
	path, unit, quote = strings.TrimSpace(path), strings.TrimSpace(unit), strings.TrimSpace(quote)
	if path == "" && unit == "" && quote == "" {
		return nil
	}
	return []contextop.Evidence{{Path: path, Unit: unit, Quote: quote}}
}

// requireEvidence refuses a rule nobody can check. A rule with evidence behind
// it can be argued with; a rule without any is a preference somebody typed, and
// a person reviewing a list of them has no way to tell the two apart.
func requireEvidence(tool string, evidence []contextop.Evidence) error {
	if len(evidence) > 0 {
		return nil
	}
	return fmt.Errorf(
		"%s needs evidence: pass `path`, the project-relative file you saw this in, and `quote`, the wording as it stands there",
		tool)
}
