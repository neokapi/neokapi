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
		resp, err := sc.Status(callCtx, &pb.StatusRequest{Project: ref})
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
		resp, err := sc.ListFiles(callCtx, &pb.ListFilesRequest{Project: ref, Paths: rest})
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
		resp, err := sc.Push(callCtx, &pb.PushRequest{
			Project: ref,
			Paths:   positional,
			Force:   force,
			DryRun:  dryRun,
		})
		if err != nil {
			return fmt.Errorf("daemon Push: %w", err)
		}
		fmt.Printf("pushed %d blocks (%d words) across %d files; assets: %d; push_id: %s\n",
			resp.GetBlocksPushed(), resp.GetWordCount(), resp.GetFilesScanned(), resp.GetAssetsPushed(), resp.GetPushId())
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
		resp, err := sc.Pull(callCtx, &pb.PullRequest{
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
		return nil
	}
	return fmt.Errorf("unknown source-connector op %q", op)
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
	flags.ParseErrorsAllowlist.UnknownFlags = true
	_ = flags.Parse(args)
	return flags.Args()
}
