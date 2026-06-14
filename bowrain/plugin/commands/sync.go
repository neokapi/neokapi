package commands

import (
	"fmt"
	"time"

	"github.com/neokapi/neokapi/bowrain/core/connector"
	"github.com/neokapi/neokapi/bowrain/core/project"
	bconn "github.com/neokapi/neokapi/bowrain/plugin/connector"
	"github.com/neokapi/neokapi/cli"
	"github.com/spf13/cobra"
)

var (
	syncNoWait  bool
	syncTimeout time.Duration
	syncLocales []string
)

var syncCmd = &cobra.Command{
	Use:   "sync [paths...]",
	Short: "Push content, wait for translations, then pull",
	Long: `Push local content to the server, wait for auto-triggered translations
to complete, then pull translated files back.

Equivalent to running: kapi push && <wait for translations> && kapi pull

Use --no-wait to push without waiting for translations.`,
	RunE: runSync,
}

func runSync(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	// Run pre-push automations.
	if proj := findProjectForAutomations(); proj != nil {
		if err := runLocalAutomations(cmd, proj, "pre-push"); err != nil {
			return fmt.Errorf("pre-push automation: %w", err)
		}
	}

	// Phase 1: Push.
	fmt.Fprintln(cmd.OutOrStdout(), "Pushing content...")
	pushResult, conn, err := doPush(ctx, connector.PushOptions{}, args)
	if err != nil {
		return fmt.Errorf("push: %w", err)
	}
	defer conn.Close()

	if pushResult.UpToDate {
		fmt.Fprintln(cmd.OutOrStdout(), "Already up to date.")
	} else {
		fmt.Fprintf(cmd.OutOrStdout(), "Pushed %d blocks, %d words (scanned %d files)\n",
			pushResult.BlocksPushed, pushResult.WordCount, pushResult.FilesScanned)
	}

	// Reconcile governed terminology edits as part of the push phase.
	if proj, perr := project.FindProject(""); perr == nil {
		if cres, cerr := conceptPush(ctx, proj, false); cerr != nil {
			return fmt.Errorf("concept push: %w", cerr)
		} else if cres != nil {
			printConceptPushSummary(cmd, cres)
		}
	}

	// Phase 2: Wait for auto-translations.
	if pushResult.PushID == "" || pushResult.BlocksPushed == 0 || syncNoWait {
		if !syncNoWait {
			return syncPull(cmd, conn)
		}
		return nil
	}

	client := conn.Client()
	if client == nil {
		return syncPull(cmd, conn)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "\nWaiting for translations (push_id: %s)...\n", shortPushID(pushResult.PushID))

	deadline := time.Now().Add(syncTimeout)

	// Initial delay to let automation create jobs.
	time.Sleep(500 * time.Millisecond)

	lastStatus := ""
	retryCount := 0
	for {
		if time.Now().After(deadline) {
			fmt.Fprintln(cmd.OutOrStdout(), "Timeout waiting for translations. Pulling available results...")
			break
		}

		status, err := client.PushStatus(ctx, pushResult.PushID)
		if err != nil {
			retryCount++
			if retryCount > 3 {
				fmt.Fprintf(cmd.OutOrStdout(), "Could not check translation status: %v\n", err)
				break
			}
			time.Sleep(2 * time.Second)
			continue
		}
		retryCount = 0

		if status.Total == 0 {
			// Jobs not created yet; wait a bit more.
			time.Sleep(1 * time.Second)
			continue
		}

		progress := fmt.Sprintf("  %d/%d completed", status.Completed, status.Total)
		if status.Failed > 0 {
			progress += fmt.Sprintf(", %d failed", status.Failed)
		}

		if progress != lastStatus {
			fmt.Fprintln(cmd.OutOrStdout(), progress)
			lastStatus = progress
		}

		if status.Status == "completed" || status.Status == "failed" {
			if status.Failed > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "\nTranslation completed with %d failure(s).\n", status.Failed)
			} else {
				fmt.Fprintln(cmd.OutOrStdout(), "\nTranslation completed.")
			}
			break
		}

		time.Sleep(2 * time.Second)
	}

	// Phase 3: Pull.
	if err := syncPull(cmd, conn); err != nil {
		return err
	}

	// Run post-pull automations.
	if proj := findProjectForAutomations(); proj != nil {
		if err := runLocalAutomations(cmd, proj, "post-pull"); err != nil {
			return fmt.Errorf("post-pull automation: %w", err)
		}
	}

	return nil
}

// shortPushID returns a display-friendly abbreviation of a push ID, truncated
// to its first 8 characters. It is safe for push IDs shorter than 8 characters
// (e.g. from a non-conforming server), returning the full ID in that case
// rather than panicking on an out-of-range slice.
func shortPushID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

func syncPull(cmd *cobra.Command, conn *bconn.BowrainSourceConnector) error {
	fmt.Fprintln(cmd.OutOrStdout(), "\nPulling translations...")
	pullResult, err := doPull(cmd.Context(), conn, syncLocales, false, false)
	if err != nil {
		return fmt.Errorf("pull: %w", err)
	}

	if pullResult.UpToDate {
		fmt.Fprintln(cmd.OutOrStdout(), "Already up to date.")
	} else {
		fmt.Fprintf(cmd.OutOrStdout(), "Pulled %d blocks for %d locales\n",
			pullResult.BlocksPulled, pullResult.LocalesCount)
		if pullResult.FilesWritten > 0 {
			fmt.Fprintf(cmd.OutOrStdout(), "Updated %d file(s)\n", pullResult.FilesWritten)
		}
	}

	// Fold the workspace's governed terminology into the pull phase. The baseline
	// is recorded on the connector's in-memory cache so the deferred conn.Close()
	// in runSync persists it together with the block-sync state — writing it to
	// disk here would be overwritten by that Close().
	if proj, perr := project.FindProject(""); perr == nil {
		cres, baseline, cerr := conceptPull(cmd.Context(), proj, false)
		if cerr != nil {
			return fmt.Errorf("concept pull: %w", cerr)
		}
		if baseline != nil {
			conn.SetConceptBaseline(baseline)
		}
		if cres != nil && cres.Concepts > 0 {
			fmt.Fprintf(cmd.OutOrStdout(), "Pulled %d governed concept(s), %d relation(s) into the local termbase\n",
				cres.Concepts, cres.Relations)
		}
	}

	return nil
}

func init() {
	syncCmd.Flags().BoolVar(&syncNoWait, "no-wait", false, "Push only, do not wait for translations or pull")
	syncCmd.Flags().DurationVar(&syncTimeout, "timeout", 5*time.Minute, "Maximum time to wait for translations")
	syncCmd.Flags().StringSliceVar(&syncLocales, "locale", nil, "Languages to pull (e.g. fr,de)")
	cli.RegisterCommandFactory(func(parent *cobra.Command, _ *cli.App) { parent.AddCommand(syncCmd) })
}
