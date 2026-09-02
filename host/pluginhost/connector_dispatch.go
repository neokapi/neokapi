package pluginhost

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	pb "github.com/neokapi/neokapi/core/plugin/proto/v1"
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

// genericSourceConnectorDispatcher routes "push", "pull", "status", "ls"
// commands to the corresponding RPC on the framework's
// SourceConnectorService. It is the default dispatcher kapi installs for
// any plugin that declares source_connectors in its manifest.
//
// The dispatcher discovers the project root by walking up from the
// current working directory looking for any *.kapi recipe — same logic
// as `kapi status`. Recipe parsing is the daemon's job.
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

func (g *genericSourceConnectorDispatcher) Dispatch(ctx context.Context, client *DaemonClient, op string, args []string) error {
	if client == nil || client.Conn == nil {
		return errors.New("daemon client has no gRPC connection")
	}

	root, rest, err := resolveProjectRoot(args)
	if err != nil {
		return err
	}
	ref := &pb.ProjectRef{Root: root}

	sc := pb.NewSourceConnectorServiceClient(client.Conn)

	// Each RPC call gets a per-call deadline derived from the caller's context.
	// This guards against a hung daemon without cancelling the outer command ctx.
	rpcCtx := func() (context.Context, context.CancelFunc) {
		return context.WithTimeout(ctx, connectorRPCTimeout)
	}

	switch op {
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
		resp, err := sc.ListFiles(callCtx, &pb.ListFilesRequest{Project: ref, Paths: rest}) //nolint:contextcheck // callCtx derives from the caller ctx via the rpcCtx closure
		if err != nil {
			return fmt.Errorf("daemon ListFiles: %w", err)
		}
		for _, f := range resp.GetFiles() {
			fmt.Printf("%s\t%s\t%d blocks\t%d words\t%d dirty\n",
				f.GetPath(), f.GetFormat(), f.GetBlockCount(), f.GetWordCount(), f.GetDirtyCount())
		}
		return nil

	case "push":
		flags := pflag.NewFlagSet("push", pflag.ContinueOnError)
		var force, dryRun bool
		flags.BoolVar(&force, "force", false, "")
		flags.BoolVar(&dryRun, "dry-run", false, "")
		positional := parseFlags(flags, rest)
		callCtx, cancel := rpcCtx()
		defer cancel()
		resp, err := sc.Push(callCtx, &pb.PushRequest{ //nolint:contextcheck // callCtx derives from the caller ctx via the rpcCtx closure
			Project: ref,
			Paths:   positional,
			Force:   force,
			DryRun:  dryRun,
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
		flags := pflag.NewFlagSet("pull", pflag.ContinueOnError)
		var force, dryRun bool
		var locales []string
		flags.BoolVar(&force, "force", false, "")
		flags.BoolVar(&dryRun, "dry-run", false, "")
		flags.StringSliceVar(&locales, "locale", nil, "")
		_ = parseFlags(flags, rest)
		callCtx, cancel := rpcCtx()
		defer cancel()
		resp, err := sc.Pull(callCtx, &pb.PullRequest{ //nolint:contextcheck // callCtx derives from the caller ctx via the rpcCtx closure
			Project: ref,
			Locales: locales,
			Force:   force,
			DryRun:  dryRun,
		})
		if err != nil {
			return fmt.Errorf("daemon Pull: %w", err)
		}
		fmt.Printf("pulled %d blocks across %d locales; wrote %d files\n",
			resp.GetBlocksPulled(), resp.GetLocalesCount(), resp.GetFilesWritten())
		if n := resp.GetDecisionsStaged(); n > 0 {
			fmt.Printf("staged %d unit-state update(s) from the server ledger. `kapi commit` publishes them\n", n)
		}
		printPullExtras(resp)
		if te := resp.GetTerminologyError(); te != "" {
			return fmt.Errorf("the content above was pulled; the workspace terminology was not: %s", te)
		}
		return nil
	}
	return fmt.Errorf("unknown source-connector op %q", op)
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
// could not stage.
func printPullExtras(resp *pb.PullResponse) {
	if n := resp.GetDecisionsSkipped(); n > 0 {
		fmt.Fprintf(os.Stderr,
			"warning: %d server decision(s) could not be read and were not staged; the server does not offer them again\n", n)
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

// resolveProjectRoot returns the absolute project root for daemon RPCs.
// It honours --project / -p as the first match; otherwise uses cwd.
// rest is the args slice with -p / --project consumed.
func resolveProjectRoot(args []string) (root string, rest []string, err error) {
	rest = make([]string, 0, len(args))
	skipNext := false
	for i, a := range args {
		if skipNext {
			skipNext = false
			continue
		}
		switch {
		case a == "-p" || a == "--project":
			if i+1 < len(args) {
				root = args[i+1]
				skipNext = true
			}
		case strings.HasPrefix(a, "--project="):
			root = strings.TrimPrefix(a, "--project=")
		case strings.HasPrefix(a, "-p="):
			root = strings.TrimPrefix(a, "-p=")
		default:
			rest = append(rest, a)
		}
	}
	if root == "" {
		// Daemon will resolve cwd-walk-up.
		root, err = os.Getwd()
		if err != nil {
			return "", nil, fmt.Errorf("get cwd: %w", err)
		}
	}
	return root, rest, nil
}

// parseFlags parses what it can from args using flags; unknown flags are
// silently passed through. Returns the positional remainder.
func parseFlags(flags *pflag.FlagSet, args []string) []string {
	// Unknown flags are tolerated because kapi forwards its own global flags
	// through this dispatcher, and refusing them would break every call.
	//
	// But tolerating is not the same as hiding. This route is a THINNER surface
	// than the plugin's cobra command of the same name: `kapi push` reaches the
	// daemon RPC, which carries paths, --force and --dry-run and nothing else,
	// while `kapi-bowrain push` also understands --concepts, --no-brand and
	// --stream. Swallowing those silently meant `kapi push --concepts` ran an
	// ordinary content push and reported success, and `--no-brand` carried the
	// brand anyway — the command did something other than what it was asked,
	// and said nothing.
	//
	// Naming what was dropped is the least this can do until the RPC carries
	// them. A warning, not a refusal: being wrong about which flags kapi may
	// legitimately forward must not break the dispatcher.
	flags.ParseErrorsAllowlist.UnknownFlags = true
	before := unknownFlagsIn(flags, args)
	_ = flags.Parse(args)
	for _, f := range before {
		fmt.Fprintf(os.Stderr,
			"warning: %q is not understood by this route and was ignored. "+
				"The daemon's %s takes paths, --force and --dry-run only\n",
			f, flags.Name())
	}
	return flags.Args()
}

// unknownFlagsIn reports the flag-looking arguments the given set does not
// define, so they can be named rather than dropped in silence.
func unknownFlagsIn(flags *pflag.FlagSet, args []string) []string {
	var unknown []string
	for _, a := range args {
		if !strings.HasPrefix(a, "--") || a == "--" {
			continue
		}
		name, _, _ := strings.Cut(strings.TrimPrefix(a, "--"), "=")
		if name == "" || flags.Lookup(name) != nil {
			continue
		}
		unknown = append(unknown, "--"+name)
	}
	return unknown
}
