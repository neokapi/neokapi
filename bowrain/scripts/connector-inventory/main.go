package main

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/neokapi/neokapi/bowrain/connector"
	platform "github.com/neokapi/neokapi/bowrain/core/connector"
	"github.com/neokapi/neokapi/core/registry"
)

type entry struct {
	ID       string   `json:"id"`
	Category string   `json:"category"`
	Surfaces []string `json:"surfaces"`
}

func main() {
	formats := registry.NewFormatRegistry()
	local, server, remote := platform.NewRegistry(), platform.NewRegistry(), platform.NewRegistry()
	connector.RegisterLocal(local, formats)
	connector.RegisterServer(server, formats)
	connector.RegisterRemote(remote)

	entries := map[string]*entry{}
	for _, surface := range []struct {
		name     string
		registry *platform.Registry
	}{{"local", local}, {"server", server}, {"remote", remote}} {
		for _, info := range surface.registry.List() {
			if entries[info.Name] == nil {
				entries[info.Name] = &entry{ID: info.Name, Category: string(info.Category)}
			}
			entries[info.Name].Surfaces = append(entries[info.Name].Surfaces, surface.name)
		}
	}

	rows := make([]entry, 0, len(entries))
	for _, item := range entries {
		rows = append(rows, *item)
	}
	slices.SortFunc(rows, func(a, b entry) int {
		return strings.Compare(a.ID, b.ID)
	})
	if err := json.NewEncoder(os.Stdout).Encode(rows); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
