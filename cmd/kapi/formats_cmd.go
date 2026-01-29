package main

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"
)

var formatsCmd = &cobra.Command{
	Use:   "formats",
	Short: "List available data formats",
	Run: func(cmd *cobra.Command, args []string) {
		readers := formatReg.ReaderNames()
		writers := formatReg.WriterNames()

		writerSet := make(map[string]bool)
		for _, w := range writers {
			writerSet[w] = true
		}

		all := make(map[string]bool)
		for _, r := range readers {
			all[r] = true
		}
		for _, w := range writers {
			all[w] = true
		}

		names := make([]string, 0, len(all))
		for name := range all {
			names = append(names, name)
		}
		sort.Strings(names)

		fmt.Println("Available formats:")
		fmt.Println()
		fmt.Printf("  %-20s %-10s %-10s\n", "FORMAT", "READ", "WRITE")
		fmt.Printf("  %-20s %-10s %-10s\n", "------", "----", "-----")
		for _, name := range names {
			read := "-"
			write := "-"
			if formatReg.HasReader(name) {
				read = "yes"
			}
			if writerSet[name] {
				write = "yes"
			}
			fmt.Printf("  %-20s %-10s %-10s\n", name, read, write)
		}
		fmt.Printf("\nTotal: %d format(s)\n", len(names))
	},
}

func init() {
	rootCmd.AddCommand(formatsCmd)
}
