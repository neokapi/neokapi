package commands

import (
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/neokapi/neokapi/host/venue/transfer"

	"github.com/neokapi/neokapi/bowrain/plugin/commands/output"
	"github.com/neokapi/neokapi/cli"
	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/host/venue/project"
	bconn "github.com/neokapi/neokapi/host/venue/source"
	"github.com/spf13/cobra"
)

// runLocalAutomations runs all enabled automation rules matching the given trigger.
func runLocalAutomations(cmd *cobra.Command, proj *project.Project, trigger string) error {
	if proj == nil || proj.Recipe == nil {
		return nil
	}
	out := automationOutput(cmd)

	for _, rule := range proj.Recipe.Automations {
		if rule.Trigger != trigger {
			continue
		}
		if !rule.IsEnabled() {
			continue
		}

		fmt.Fprintf(out, "Running automation: %s\n", rule.Name)

		for _, action := range rule.Actions {
			if err := executeLocalAction(cmd, action, proj); err != nil {
				return fmt.Errorf("automation %q action %q: %w", rule.Name, action.Type, err)
			}
		}
	}

	return nil
}

// automationOutput is where a local automation narrates and where a flow it
// runs prints: the triggering command's stdout, or its stderr when that stdout
// carries JSON, so a consumer of `kapi push --json` or `kapi up --json` never
// reads a findings table as an event.
func automationOutput(cmd *cobra.Command) io.Writer {
	if output.ResolveFormat(cmd) == output.FormatJSON {
		return cmd.ErrOrStderr()
	}
	return cmd.OutOrStdout()
}

// executeLocalAction executes a single automation action.
func executeLocalAction(cmd *cobra.Command, action project.ActionConfig, proj *project.Project) error {
	switch action.Type {
	case project.ActionRunFlow:
		return runFlowAction(cmd, action, proj)

	case "wait_translate":
		timeout := 5 * time.Minute
		if t := action.Config["timeout"]; t != "" {
			d, err := time.ParseDuration(t)
			if err != nil {
				return fmt.Errorf("invalid timeout %q: %w", t, err)
			}
			timeout = d
		}
		fmt.Fprintf(cmd.OutOrStdout(), "  Waiting for translations (timeout: %s)...\n", timeout)

		pushID := action.Config["push_id"]
		if pushID == "" {
			fmt.Fprintln(cmd.OutOrStdout(), "  No push_id available; skipping wait.")
			return nil
		}

		// Build a client from the project to check push status.
		conn, err := bconn.NewSourceConnector(app, proj, app.FormatReg)
		if err != nil {
			return fmt.Errorf("connect to server: %w", err)
		}
		defer conn.Close()
		client := conn.Client()

		// Poll translation status with exponential backoff.
		interval := 2 * time.Second
		deadline := time.Now().Add(timeout)
		for time.Now().Before(deadline) {
			status, err := client.PushStatus(cmd.Context(), pushID)
			if err != nil {
				return fmt.Errorf("check push status: %w", err)
			}
			if status.Completed == status.Total && status.Total > 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "  Translations completed.")
				return nil
			}
			if status.Failed > 0 {
				return fmt.Errorf("translation job failed (%d/%d failed)", status.Failed, status.Total)
			}
			time.Sleep(interval)
			if interval < 30*time.Second {
				interval *= 2
			}
		}
		return fmt.Errorf("timed out waiting for translations after %s", timeout)

	case "pull":
		fmt.Fprintln(automationOutput(cmd), "  Pulling translations...")
		_, err := transfer.Pull(cmd.Context(), app, nil, nil, false, false)
		return err

	case "push":
		fmt.Fprintln(automationOutput(cmd), "  Pushing content...")
		_, conn, err := transfer.Push(cmd.Context(), app, transfer.PushOptions{})
		if conn != nil {
			conn.Close()
		}
		return err

	default:
		fmt.Fprintf(automationOutput(cmd), "  Unknown action type: %s (skipping)\n", action.Type)
		return nil
	}
}

// runFlowAction runs the flow a run_flow action names over the project's
// collections, the way `kapi run <flow>` does with no --input: the recipe's
// content, its locale passes, its standing bindings, the results committed to
// the project store. The flow's own output, the findings its check steps
// report included, prints in the output of the command that triggered it.
//
// A flow that cannot run (an unknown name, a tool that fails to assemble or
// fails part way) fails the action whatever the config says, as a server
// rule's run_flow step fails with its reason. fail_on_error decides what the
// flow's findings do: true aborts the command with the findings summary and
// the gate exit code, unset or false reports them and continues, which is
// what `kapi run` itself does.
func runFlowAction(cmd *cobra.Command, action project.ActionConfig, proj *project.Project) error {
	flowName := strings.TrimSpace(action.Config["flow"])
	if flowName == "" {
		return errors.New("run_flow action names no flow")
	}
	failOnError := false
	if v := strings.TrimSpace(action.Config["fail_on_error"]); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return fmt.Errorf("run_flow: fail_on_error %q is not a boolean", v)
		}
		failOnError = b
	}
	if app == nil {
		return errors.New("run_flow: kapi is not initialised")
	}

	out := automationOutput(cmd)
	fmt.Fprintf(out, "  Running flow: %s\n", flowName)

	// The run reads its flags off a command carrying `kapi run`'s flag set, so
	// every default is the one a bare `kapi run <flow>` gets. Those flags bind
	// the App's language and format fields; they are restored afterwards so
	// the triggering command keeps its own.
	savedSource, savedTarget := app.SourceLang, app.TargetLang
	savedFormat, savedEncoding := app.FormatFlag, app.Encoding
	defer func() {
		app.SourceLang, app.TargetLang = savedSource, savedTarget
		app.FormatFlag, app.Encoding = savedFormat, savedEncoding
	}()
	runCmd := cli.NewRunCmd(app, cli.RunCmdOptions{})
	runCmd.SetContext(cmd.Context())
	runCmd.SetOut(out)
	runCmd.SetErr(cmd.ErrOrStderr())
	recipePath := proj.RecipePath()
	if err := runCmd.Flags().Set("project", recipePath); err != nil {
		return err
	}

	var found check.Summary
	err := app.RunFromProject(runCmd, flowName, recipePath, cli.RunCmdOptions{
		FallbackRunE: app.ResolveFallbackRunE(cli.RunCmdOptions{}),
		OnFindings: func(f cli.FlowFindings) {
			found.Findings += f.Summary.Findings
			found.Critical += f.Summary.Critical
			found.Major += f.Summary.Major
			found.Minor += f.Summary.Minor
			found.Neutral += f.Summary.Neutral
		},
	})
	if err != nil {
		return fmt.Errorf("flow %q: %w", flowName, err)
	}
	if found.Findings > 0 && failOnError {
		return cli.WithExitCode(cli.ExitGate, fmt.Errorf(
			"flow %q found %d finding(s) (%d critical, %d major, %d minor)",
			flowName, found.Findings, found.Critical, found.Major, found.Minor))
	}
	return nil
}

// findProjectForAutomations does a lightweight project lookup for automation hooks.
func findProjectForAutomations() *project.Project {
	proj, err := project.FindProject("")
	if err != nil {
		return nil
	}
	return proj
}
