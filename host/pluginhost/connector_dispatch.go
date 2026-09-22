package pluginhost

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	pb "github.com/neokapi/neokapi/core/plugin/proto/v1"
	"github.com/neokapi/neokapi/core/project"
	"github.com/spf13/pflag"
)

// connectorRPCTimeout is the per-call deadline for source-connector RPCs.
// Long-running operations (push of large repos) use streaming or chunked
// calls; this guards against a hung daemon on a control-plane call.
const connectorRPCTimeout = 5 * time.Minute

// SourceConnectorOpsClaimed lists the standard op names handled by the
// generic source-connector dispatcher. Plugins that follow the
// SourceConnectorService schema should register a dispatcher with these
// op names.
var SourceConnectorOpsClaimed = []string{"push", "pull", "status", "ls"}

// SourceConnectorCall is one source-connector op with its arguments read: the
// project the daemon acts on and the values the op takes.
type SourceConnectorCall struct {
	Op string
	// ProjectRoot is the absolute path the daemon receives as ProjectRef.Root.
	ProjectRoot string
	// Paths limits push and ls to these paths.
	Paths   []string
	Force   bool
	DryRun  bool
	Locales []string
}

// RouteHelp is the error Prepare returns when a route's arguments ask for
// help. It matches pflag.ErrHelp. Usage is the argument synopsis that follows
// the command name, and Flags holds the flags the route takes, for the help a
// front end prints.
type RouteHelp struct {
	Op    string
	Usage string
	Flags *pflag.FlagSet
}

func (h *RouteHelp) Error() string { return "help requested for " + h.Op }

// Is reports whether target is pflag.ErrHelp.
func (h *RouteHelp) Is(target error) bool { return target == pflag.ErrHelp }

// genericSourceConnectorDispatcher routes "push", "pull", "status" and "ls" to
// the corresponding RPC on the framework's SourceConnectorService. It is the
// default dispatcher kapi installs for any plugin that declares
// source_connectors in its manifest.
//
// It resolves the project the way every kapi command does and sends the
// daemon the project's root; loading the recipe is the daemon's job.
type genericSourceConnectorDispatcher struct {
	pluginName string
}

// NewGenericSourceConnectorDispatcher returns a dispatcher that routes
// "push", "pull", "status", "ls" to the framework's
// SourceConnectorService. Most plugins want this; specialised plugins
// can implement SourceConnectorDispatcher themselves.
func NewGenericSourceConnectorDispatcher(pluginName string) SourceConnectorDispatcher {
	return &genericSourceConnectorDispatcher{pluginName: pluginName}
}

func (g *genericSourceConnectorDispatcher) Plugin() string { return g.pluginName }

// Prepare reads op's arguments. The op's own flags, -p/--project, -h/--help and
// kapi's persistent flags parse, and any other flag refuses the call: the
// daemon carries only what the op defines, so running anyway performs a
// different operation from the one typed, such as a push of every path when an
// unknown flag took the path after it as its value. Push and ls take paths;
// status and pull take none.
//
// With no -p, the project resolves through project.ResolveRecipePath, so
// KAPI_NO_PROJECT leaves the op without a project and Prepare refuses it.
func (g *genericSourceConnectorDispatcher) Prepare(op string, args []string) (SourceConnectorCall, error) {
	call := SourceConnectorCall{Op: op}
	flags, takesPaths, err := sourceConnectorFlags(&call)
	if err != nil {
		return call, err
	}
	var explicit string
	var help bool
	flags.StringVarP(&explicit, "project", "p", "", "path to a kapi.yaml project recipe or its directory (found from the working directory if omitted)")
	flags.BoolVarP(&help, "help", "h", false, "help for "+op)
	routeFlags := pflag.NewFlagSet(op, pflag.ContinueOnError)
	routeFlags.AddFlagSet(flags)
	flags.AddFlagSet(KapiPersistentFlags())

	if err := flags.Parse(args); err != nil {
		return call, fmt.Errorf("kapi %s did not run: %w. kapi %s --help lists the flags it takes", op, err, op)
	}
	if help {
		usage := ""
		if takesPaths {
			usage = "[paths...]"
		}
		return call, &RouteHelp{Op: op, Usage: usage, Flags: routeFlags}
	}
	positional := flags.Args()
	switch {
	case takesPaths:
		call.Paths = positional
	case len(positional) > 0:
		return call, fmt.Errorf("kapi %s did not run: it takes no paths, and was given %s", op, strings.Join(positional, " "))
	}

	root, err := sourceConnectorProjectRoot(op, explicit)
	if err != nil {
		return call, err
	}
	call.ProjectRoot = root
	return call, nil
}

// sourceConnectorFlags returns the flags call.Op takes, bound to call, and
// whether the op takes paths.
func sourceConnectorFlags(call *SourceConnectorCall) (flags *pflag.FlagSet, takesPaths bool, err error) {
	flags = pflag.NewFlagSet(call.Op, pflag.ContinueOnError)
	flags.SetOutput(io.Discard)
	switch call.Op {
	case "status":
	case "ls":
		takesPaths = true
	case "push":
		takesPaths = true
		flags.BoolVar(&call.Force, "force", false, "re-upload everything, even unchanged blocks")
		flags.BoolVar(&call.DryRun, "dry-run", false, "show what would be uploaded without sending it")
	case "pull":
		flags.BoolVar(&call.Force, "force", false, "re-download everything, even unchanged content")
		flags.BoolVar(&call.DryRun, "dry-run", false, "show what would change without writing files")
		flags.StringSliceVar(&call.Locales, "locale", nil, "languages to download (e.g. fr,de)")
	default:
		return nil, false, fmt.Errorf("unknown source-connector op %q", call.Op)
	}
	return flags, takesPaths, nil
}

// sourceConnectorProjectRoot resolves the project op acts on from explicit, the
// -p value, and returns the root the daemon receives: the absolute directory
// holding kapi.yaml, as ProjectRef documents, or the recipe's own path when it
// has another name, which the daemon loads directly.
func sourceConnectorProjectRoot(op, explicit string) (string, error) {
	recipe, err := project.ResolveRecipePath(explicit)
	if err != nil {
		return "", err
	}
	if recipe == "" {
		if os.Getenv(project.NoProjectEnvVar) != "" {
			return "", fmt.Errorf("kapi %s did not run: %s is set and no -p was given, so it has no project. Pass -p <path to kapi.yaml>",
				op, project.NoProjectEnvVar)
		}
		return "", fmt.Errorf("kapi %s did not run: no kapi project found. Pass -p <path to kapi.yaml> or run from inside a kapi project directory", op)
	}
	if _, err := os.Stat(recipe); err != nil {
		return "", fmt.Errorf("kapi %s did not run: no project recipe at %s: %w", op, recipe, err)
	}
	if filepath.Base(recipe) == project.RecipeFileName {
		return filepath.Dir(recipe), nil
	}
	return recipe, nil
}

// Dispatch runs a prepared call against the daemon.
func (g *genericSourceConnectorDispatcher) Dispatch(ctx context.Context, client *DaemonClient, call SourceConnectorCall) error {
	if client == nil || client.Conn == nil {
		return errors.New("daemon client has no gRPC connection")
	}
	ref := &pb.ProjectRef{Root: call.ProjectRoot}

	sc := pb.NewSourceConnectorServiceClient(client.Conn)

	// Each RPC call gets a per-call deadline derived from the caller's context.
	// This guards against a hung daemon without cancelling the outer command ctx.
	rpcCtx := func() (context.Context, context.CancelFunc) {
		return context.WithTimeout(ctx, connectorRPCTimeout)
	}

	switch call.Op {
	case "status":
		callCtx, cancel := rpcCtx()
		defer cancel()
		resp, err := sc.Status(callCtx, &pb.StatusRequest{Project: ref}) //nolint:contextcheck // callCtx derives from the caller ctx via the rpcCtx closure
		if err != nil {
			return fmt.Errorf("daemon Status: %w", err)
		}
		fmt.Printf("connector: %s\n", resp.GetConnectorId())
		fmt.Printf("files: %d  blocks: %d  words: %d\n", resp.GetFileCount(), resp.GetItemCount(), resp.GetWordCount())
		fmt.Printf("pending push: %d  pending pull: %d\n", resp.GetPendingPush(), resp.GetPendingPull())
		if last := resp.GetLastSync(); last != "" {
			fmt.Printf("last sync: %s\n", last)
		}
		for _, e := range resp.GetErrors() {
			fmt.Fprintf(os.Stderr, "warning: %s\n", e)
		}
		return nil

	case "ls":
		callCtx, cancel := rpcCtx()
		defer cancel()
		resp, err := sc.ListFiles(callCtx, &pb.ListFilesRequest{Project: ref, Paths: call.Paths}) //nolint:contextcheck // callCtx derives from the caller ctx via the rpcCtx closure
		if err != nil {
			return fmt.Errorf("daemon ListFiles: %w", err)
		}
		for _, f := range resp.GetFiles() {
			fmt.Printf("%s\t%s\t%d blocks\t%d words\t%d dirty\n",
				f.GetPath(), f.GetFormat(), f.GetBlockCount(), f.GetWordCount(), f.GetDirtyCount())
		}
		return nil

	case "push":
		callCtx, cancel := rpcCtx()
		defer cancel()
		resp, err := sc.Push(callCtx, &pb.PushRequest{ //nolint:contextcheck // callCtx derives from the caller ctx via the rpcCtx closure
			Project: ref,
			Paths:   call.Paths,
			Force:   call.Force,
			DryRun:  call.DryRun,
		})
		if err != nil {
			return fmt.Errorf("daemon Push: %w", err)
		}
		fmt.Printf("pushed %d blocks (%d words) across %d files; uploaded: %d; assets: %d; push_id: %s\n",
			resp.GetBlocksPushed(), resp.GetWordCount(), resp.GetFilesScanned(), resp.GetBlocksUploaded(), resp.GetAssetsPushed(), resp.GetPushId())
		printPushExtras(resp)
		if te := resp.GetTerminologyError(); te != "" {
			return fmt.Errorf("the content above was pushed; its terminology was not: %s", te)
		}
		return nil

	case "pull":
		callCtx, cancel := rpcCtx()
		defer cancel()
		resp, err := sc.Pull(callCtx, &pb.PullRequest{ //nolint:contextcheck // callCtx derives from the caller ctx via the rpcCtx closure
			Project: ref,
			Locales: call.Locales,
			Force:   call.Force,
			DryRun:  call.DryRun,
		})
		if err != nil {
			return fmt.Errorf("daemon Pull: %w", err)
		}
		fmt.Printf("pulled %d blocks across %d locales; wrote %d files\n",
			resp.GetBlocksPulled(), resp.GetLocalesCount(), resp.GetFilesWritten())
		if n := resp.GetDecisionsStaged(); n > 0 {
			fmt.Printf("recorded %d unit-state update(s) from the server ledger\n", n)
		}
		printPullExtras(resp)
		if te := resp.GetTerminologyError(); te != "" {
			return fmt.Errorf("the content above was pulled; the workspace terminology was not: %s", te)
		}
		return nil
	}
	return fmt.Errorf("unknown source-connector op %q", call.Op)
}

// printPushExtras reports the parts of a push that are not blocks: the media
// that failed, what the server did with the upload, and what the terminology
// fold amounted to.
//
// The daemon computes all of them, and a response field nobody prints is
// indistinguishable, from where the user stands, from work that never
// happened: a submitted change-set of governed terminology edits sitting
// awaiting review, under a push that reported only a block count.
func printPushExtras(resp *pb.PushResponse) {
	if n := resp.GetAssetsFailed(); n > 0 {
		fmt.Fprintf(os.Stderr, "warning: %d media asset(s) failed to upload and will be retried by the next push\n", n)
		for _, e := range resp.GetAssetErrors() {
			fmt.Fprintf(os.Stderr, "  %s\n", e)
		}
	}
	switch resp.GetIngest() {
	case "queued":
		fmt.Println("the server accepted this push and is still applying it. `kapi status` shows when it lands")
	case "unknown":
		fmt.Println("the server accepted this push; it could not be asked whether it has been applied yet")
	}
	if a, r := resp.GetConceptsApplied(), resp.GetConceptRelationsApplied(); a > 0 || r > 0 {
		fmt.Printf("applied %d concept edit(s) and %d relation edit(s) directly\n", a, r)
	}
	if n := resp.GetConceptsProposed(); n > 0 {
		fmt.Printf("proposed %d governed terminology edit(s) in change-set %s. They take effect when reviewed\n",
			n, resp.GetChangesetId())
		if u := resp.GetChangesetUrl(); u != "" {
			fmt.Printf("review it at %s\n", u)
		}
	}
}

// printPullExtras reports what a pull carried besides blocks: the terminology
// snapshot, the collections the server governs differently, and any decision it
// could not record.
func printPullExtras(resp *pb.PullResponse) {
	if n := resp.GetDecisionsSkipped(); n > 0 {
		fmt.Fprintf(os.Stderr,
			"warning: %d server decision(s) could not be read and were not recorded; the server does not offer them again\n", n)
	}
	if c, r := resp.GetConceptsPulled(), resp.GetConceptRelationsPulled(); c > 0 || r > 0 {
		fmt.Printf("snapshotted %d concept(s) and %d relation(s) from the workspace terminology\n", c, r)
	}
	// The terminology return leg. Empty when the merge produced the bytes
	// already in git, which is the ordinary pull — a projection that changed
	// nothing must read as silence, not as a line saying zero.
	if p := resp.GetTermsProjection(); p != "" {
		fmt.Println(p)
	}
	if d := resp.GetGovernanceDiverged(); len(d) > 0 {
		fmt.Printf("the server governs these collections differently from this recipe: %s\n", strings.Join(d, ", "))
		fmt.Println("the recipe still decides locally. Reconcile them in kapi.yaml rather than by pulling")
	}
}
