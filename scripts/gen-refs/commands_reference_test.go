package main

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"

	"github.com/neokapi/neokapi/cli"
)

// A command marked with cli.ListInReference stays off the root help and gets
// its reference entry, subcommands included. A hidden command without the
// mark gets none.
func TestCollectCommands_ListsAHiddenCommandMarkedForTheReference(t *testing.T) {
	root := &cobra.Command{Use: "kapi"}
	run := func(*cobra.Command, []string) {}
	marked := cli.ListInReference(&cobra.Command{Use: "marked", Short: "x", Hidden: true})
	marked.AddCommand(&cobra.Command{Use: "child", Short: "x", Run: run})
	root.AddCommand(marked, &cobra.Command{Use: "plumbing", Short: "x", Hidden: true, Run: run})

	ids := map[string]bool{}
	for _, e := range collectCommands(root, nil) {
		ids[e.ID] = true
	}
	assert.True(t, ids["marked"])
	assert.True(t, ids["marked.child"])
	assert.False(t, ids["plumbing"])
}

// `kapi state` and `kapi hook` are hidden from the root help and listed in the
// reference.
func TestCollectCommands_ListsStateAndHook(t *testing.T) {
	root := buildKapiRoot()
	ids := map[string]bool{}
	for _, e := range collectCommands(root, nil) {
		ids[e.ID] = true
	}
	for _, id := range []string{"state", "state.path", "state.import", "state.relink", "hook"} {
		assert.True(t, ids[id], "%s is in the reference", id)
	}
	for _, c := range root.Commands() {
		if c.Name() == "state" || c.Name() == "hook" {
			assert.True(t, c.Hidden, "%s stays off the root help", c.Name())
		}
	}
}
