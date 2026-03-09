package main

import (
	"fmt"
	"time"

	"github.com/gokapi/gokapi/platform/connector"
	"github.com/gokapi/gokapi/platform/project"
	"github.com/spf13/cobra"
)

// runLocalAutomations runs all enabled automation rules matching the given trigger.
func runLocalAutomations(cmd *cobra.Command, proj *project.Project, trigger string) error {
	if proj == nil || proj.Config == nil {
		return nil
	}

	for _, rule := range proj.Config.Automations {
		if rule.Trigger != trigger {
			continue
		}
		if !rule.IsEnabled() {
			continue
		}

		fmt.Fprintf(cmd.OutOrStdout(), "Running automation: %s\n", rule.Name)

		for _, action := range rule.Actions {
			if err := executeLocalAction(cmd, action, proj); err != nil {
				return fmt.Errorf("automation %q action %q: %w", rule.Name, action.Type, err)
			}
		}
	}

	return nil
}

// executeLocalAction executes a single automation action.
func executeLocalAction(cmd *cobra.Command, action project.ActionConfig, proj *project.Project) error {
	switch action.Type {
	case "run_flow":
		flowName := action.Config["flow"]
		if flowName == "" {
			return fmt.Errorf("run_flow action missing 'flow' config")
		}
		fmt.Fprintf(cmd.OutOrStdout(), "  Would run flow: %s\n", flowName)
		return nil

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
		// Reuse sync.go wait logic when a push ID is available.
		// For now, this is a placeholder that logs intent.
		return nil

	case "pull":
		fmt.Fprintln(cmd.OutOrStdout(), "  Pulling translations...")
		_, err := doPull(cmd.Context(), nil, nil, false, false)
		return err

	case "push":
		fmt.Fprintln(cmd.OutOrStdout(), "  Pushing content...")
		_, conn, err := doPush(cmd.Context(), connector.PushOptions{}, nil)
		if conn != nil {
			conn.Close()
		}
		return err

	default:
		fmt.Fprintf(cmd.OutOrStdout(), "  Unknown action type: %s (skipping)\n", action.Type)
		return nil
	}
}

// findProjectForAutomations does a lightweight project lookup for automation hooks.
func findProjectForAutomations() *project.Project {
	proj, err := project.FindProject("")
	if err != nil {
		return nil
	}
	return proj
}
