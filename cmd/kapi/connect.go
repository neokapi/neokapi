package main

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/gokapi/gokapi/core/connector"
	"github.com/spf13/cobra"
)

var connectCmd = &cobra.Command{
	Use:   "connect",
	Short: "Manage content connectors",
	Long:  "Add, remove, and list bidirectional connectors to external content sources.",
}

var connectAddCmd = &cobra.Command{
	Use:   "add <type> [flags]",
	Short: "Add a new connector",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		connType := args[0]

		// Build config from flags.
		config := make(map[string]string)
		if v, _ := cmd.Flags().GetString("path"); v != "" {
			config["path"] = v
		}
		if v, _ := cmd.Flags().GetString("url"); v != "" {
			config["url"] = v
		}
		if v, _ := cmd.Flags().GetString("repo"); v != "" {
			config["repo"] = v
		}
		if v, _ := cmd.Flags().GetString("token"); v != "" {
			config["token"] = v
		}
		if v, _ := cmd.Flags().GetString("api-key"); v != "" {
			config["api_key"] = v
		}
		if v, _ := cmd.Flags().GetString("name"); v != "" {
			config["name"] = v
		}
		if v, _ := cmd.Flags().GetString("id"); v != "" {
			config["id"] = v
		}

		reg := connector.NewRegistry()
		connector.RegisterAll(reg, formatReg)

		c, err := reg.NewConnector(connType, config)
		if err != nil {
			return fmt.Errorf("create connector: %w", err)
		}

		fmt.Printf("Added connector: %s (%s) [%s]\n", c.Name(), c.ID(), c.Category())
		return nil
	},
}

var connectListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available connector types",
	RunE: func(cmd *cobra.Command, args []string) error {
		reg := connector.NewRegistry()
		connector.RegisterAll(reg, formatReg)

		infos := reg.List()
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "NAME\tCATEGORY")
		for _, info := range infos {
			fmt.Fprintf(w, "%s\t%s\n", info.Name, info.Category)
		}
		w.Flush()
		return nil
	},
}

func init() {
	connectAddCmd.Flags().String("path", "", "File path for file/git connectors")
	connectAddCmd.Flags().String("url", "", "URL for CMS connectors")
	connectAddCmd.Flags().String("repo", "", "Git repository URL")
	connectAddCmd.Flags().String("token", "", "API token")
	connectAddCmd.Flags().String("api-key", "", "API key")
	connectAddCmd.Flags().String("name", "", "Connector display name")
	connectAddCmd.Flags().String("id", "", "Connector ID")

	connectCmd.AddCommand(connectAddCmd)
	connectCmd.AddCommand(connectListCmd)
	rootCmd.AddCommand(connectCmd)
}
