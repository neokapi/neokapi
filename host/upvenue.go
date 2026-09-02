package host

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/neokapi/neokapi/core/convergence"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/host/pluginhost"
)

// Where the convergence loop runs.
//
// `kapi up` is one verb with two venues: the loop on this machine, or the loop
// on the server the recipe binds — pushed, converged on org keys against the
// shared content memory and terminology, streamed back, pulled. Which one a run
// gets is decided here, once, so every surface that offers the verb decides it
// the same way. A surface that resolved it for itself would hand its callers a
// different `up` than the terminal gives: an agent that skipped the push would
// converge a stale server copy and pull nothing back.

// UpVenue names where a convergence run executes.
type UpVenue string

const (
	// UpVenueLocal runs the loop in this process.
	UpVenueLocal UpVenue = "local"
	// UpVenueServer dispatches the run to the plugin that owns the venue's
	// plumbing (the hidden `server-up` command).
	UpVenueServer UpVenue = "server"
)

// UpVenueOptions are the venue-relevant knobs of an up run.
type UpVenueOptions struct {
	// Local keeps the loop on this machine in a connected project. The run
	// still dispatches to the plumbing, which converges locally and then
	// pushes so the server copy never goes stale.
	Local bool
	// ForceServer refuses to fall back to a local run: without a bound venue
	// or without the plumbing, resolution fails instead.
	ForceServer bool
	// Plan is a dry run, which never dispatches: the server runs the same
	// loop, so the local plan against the working tree is the honest preview.
	Plan bool
}

// UpVenueDecision is the resolved answer to "where does this run happen".
type UpVenueDecision struct {
	// Venue is where the loop runs.
	Venue UpVenue
	// HasServer reports whether the recipe binds a convergence venue at all,
	// which is what separates "local because there is no server" from "local
	// because the plumbing is missing" — the second warrants a warning.
	HasServer bool
	// ServerURL is the bound venue's URL, for the line a surface prints
	// before the run.
	ServerURL string
	// Route is the plugin command the server venue dispatches to; nil for a
	// local run.
	Route *pluginhost.CommandRoute
}

// ResolveUpVenue decides where an `up` run executes for the recipe at
// projectPath. The server venue engages when the recipe binds a convergence
// venue AND an installed plugin provides the `server-up` plumbing.
//
// The errors are the --server contract: a surface that demanded the server
// venue and cannot have it is told which half is missing.
func (a *App) ResolveUpVenue(projectPath string, opts UpVenueOptions) (UpVenueDecision, error) {
	hasServer, serverURL := a.ServerRecipeURL(projectPath)
	dec := UpVenueDecision{Venue: UpVenueLocal, HasServer: hasServer, ServerURL: serverURL}

	var route *pluginhost.CommandRoute
	if a.PluginHost != nil {
		route = a.PluginHost.CommandRoute(serverUpCommand)
	}

	if opts.ForceServer {
		switch {
		case !hasServer:
			return dec, fmt.Errorf("--server needs a recipe connected to a bowrain server%s. Run 'kapi init --server <url>' to connect this project", venueBlockHint())
		case route == nil:
			return dec, errors.New("--server needs the server venue plumbing, which no installed plugin provides. Install the bowrain plugin (kapi plugin install bowrain)")
		}
	}

	if hasServer && route != nil && !opts.Plan {
		dec.Venue, dec.Route = UpVenueServer, route
	}
	return dec, nil
}

// serverUpCommand is the hidden plumbing command a venue plugin contributes to
// own the server half of `kapi up`.
const serverUpCommand = "server-up"

// venueBlockHint names the recipe block a user would add to connect a project,
// as the registered venue extension spells it. A binary linking no platform has
// no key to name and gets no hint — the caller's next case tells it to install
// the plugin, which is the honest first step there.
func venueBlockHint() string {
	if key, ok := project.VenueKey(); ok {
		return fmt.Sprintf(" (a `%s:` block)", key)
	}
	return ""
}

// RunUpDispatch is the embedded `kapi up` with its venue resolved: a connected
// project runs on the server through the plugin plumbing, everything else runs
// the loop here. It is what an embedded surface (the MCP `up` tool, a UI)
// calls instead of RunUp, so an agent gets the run a human gets — push,
// converge at the venue, pull — rather than a local loop that leaves the server
// stale and the results unpulled.
//
// Both venues return the same ConvergeOutput: the plumbing speaks the same
// NDJSON document `kapi up --json` writes, and its closing result record is
// this function's return value.
func (a *App) RunUpDispatch(ctx context.Context, projectPath, sourceLang string, opts UpOptions) (*ConvergeOutput, error) {
	dec, err := a.ResolveUpVenue(projectPath, opts.VenueOptions())
	if err != nil {
		return nil, err
	}
	if dec.Venue != UpVenueServer {
		return a.RunUp(ctx, projectPath, sourceLang, opts)
	}
	return runUpAtVenue(ctx, dec.Route, projectPath, opts)
}

// VenueOptions projects an embedded run's options onto the venue decision.
func (o UpOptions) VenueOptions() UpVenueOptions {
	return UpVenueOptions{Local: o.Local, ForceServer: o.ForceServer}
}

// runUpAtVenue dispatches the run to the venue plumbing and reads the result
// out of its NDJSON document. --json is not optional here: it is what makes the
// subprocess's stdout a structured document rather than a terminal rendering,
// and it is also what tells the plumbing not to prompt (an embedded run has no
// one to answer).
func runUpAtVenue(ctx context.Context, route *pluginhost.CommandRoute, projectPath string, opts UpOptions) (*ConvergeOutput, error) {
	args := []string{"--project=" + projectPath, "--json"}
	if opts.MaxPasses > 0 && !opts.UntilGate {
		args = append(args, "--passes=1")
	} else if opts.MaxPasses > 0 {
		args = append(args, "--passes="+strconv.Itoa(opts.MaxPasses))
	}
	if opts.Jobs > 0 {
		args = append(args, "--jobs="+strconv.Itoa(opts.Jobs))
	}
	if opts.NoChecks {
		args = append(args, "--no-checks=true")
	}
	if opts.Materialize {
		args = append(args, "--materialize=true")
	}
	if opts.Local {
		args = append(args, "--local=true")
	}

	// Streamed, not captured: the plumbing writes one event per line as the run
	// makes progress, and a caller with an OnEvent renders those lines live. A
	// buffered read would hand it the same events after the run finished, which
	// is a transcript rather than progress.
	fold := newUpResultFold(opts.OnEvent)
	if err := route.StreamStdout(ctx, fold.line, args...); err != nil {
		return nil, err
	}
	out, err := fold.result()
	if err != nil {
		return nil, fmt.Errorf("server venue: %w", err)
	}
	return out, nil
}

// parseUpResultStream reads the closing result record out of an NDJSON `up`
// document. Every other line is a progress event; the record is discriminated
// by its type and carries the ConvergeOutput flat, so the same line decodes
// into the result a local run returns.
//
// The `changeset` record is the one other line this reader keeps. A venue that
// governs terminology emits it separately from the result — the proposal is
// made in the run's push phase, long before the result is known — and it is
// folded onto the result here so an embedded caller receives one object
// describing the whole run. Dropped, it would leave every consumer downstream
// of the venue (the host's own summary, the `up` MCP tool, a CI job summary
// built from either) reporting a converged run and never mentioning that a
// governed edit is sitting on a reviewer.
func parseUpResultStream(raw []byte) (*ConvergeOutput, error) {
	fold := newUpResultFold(nil)
	for line := range strings.SplitSeq(string(raw), "\n") {
		fold.line([]byte(line))
	}
	return fold.result()
}

// upResultFold reads an NDJSON `up` document one line at a time, forwarding the
// run's progress events and keeping the records that make up its result. The
// same fold serves a buffered document and a streamed one, so a venue run is
// read identically whether or not the caller wanted live progress.
type upResultFold struct {
	onEvent   func(convergence.Event)
	found     *ConvergeOutput
	decodeErr error
	proposed  struct {
		ConceptsProposed int    `json:"concepts_proposed"`
		ChangesetID      string `json:"changeset_id"`
		ChangesetURL     string `json:"changeset_url"`
	}
}

func newUpResultFold(onEvent func(convergence.Event)) *upResultFold {
	return &upResultFold{onEvent: onEvent}
}

// line folds one NDJSON line. Records are discriminated by their type, which
// the run's progress events share with the framing records: an event's own
// EventType occupies the same `type` key.
func (f *upResultFold) line(raw []byte) {
	line := bytes.TrimSpace(raw)
	if len(line) == 0 {
		return
	}
	var kind struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(line, &kind); err != nil {
		return // a plugin line that is not part of the document
	}
	switch kind.Type {
	case "changeset":
		// Best-effort: a malformed proposal record must not fail a run whose
		// content half succeeded.
		_ = json.Unmarshal(line, &f.proposed)
	case "result":
		var out ConvergeOutput
		if err := json.Unmarshal(line, &out); err != nil {
			f.decodeErr = fmt.Errorf("decode the run's result record: %w", err)
			return
		}
		f.found = &out
	default:
		if f.onEvent == nil || !convergence.KnownEventType(convergence.EventType(kind.Type)) {
			return
		}
		var ev convergence.Event
		if err := json.Unmarshal(line, &ev); err != nil {
			// A malformed event costs a progress line, never the run.
			return
		}
		f.onEvent(ev)
	}
}

// result reports what the document said the run produced.
func (f *upResultFold) result() (*ConvergeOutput, error) {
	if f.decodeErr != nil {
		return nil, f.decodeErr
	}
	if f.found == nil {
		return nil, errors.New("the run produced no result record")
	}
	if f.proposed.ConceptsProposed > 0 {
		f.found.ConceptsProposed = f.proposed.ConceptsProposed
		f.found.ChangesetID = f.proposed.ChangesetID
		f.found.ChangesetURL = f.proposed.ChangesetURL
	}
	return f.found, nil
}
