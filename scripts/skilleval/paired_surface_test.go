package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// The surface check fails a cell whose kapi surface differs from its arm, or
// that shows anything of the developer's own.
func TestPairedSurfaceCheck(t *testing.T) {
	state, workspace := t.TempDir(), t.TempDir()
	launch := func(host, condition string) PairedLaunch {
		return PairedLaunch{Agent: PairedAgentSpec{Host: host}, Condition: condition, StateDir: state, Workspace: workspace}
	}
	allMCP := []string{"mcp__kapi__read_blocks", "mcp__kapi__apply_edits", "mcp__kapi__describe_format", "mcp__kapi__check_file"}
	developer := []string{"okapi-expert", "skill-creator"}
	for _, tc := range []struct {
		name    string
		launch  PairedLaunch
		surface PairedSurface
		problem string
	}{
		{name: "baseline as intended", launch: launch("claude", "baseline"),
			surface: PairedSurface{Skills: []string{"dataviz"}, MCPServers: []string{}, Executables: []string{}}},
		{name: "skill arm as intended", launch: launch("claude", "skill-cli"),
			surface: PairedSurface{Skills: []string{"kapi"}, MCPServers: []string{}, Executables: []string{"kapi"}}},
		{name: "MCP arm as intended", launch: launch("claude", "mcp"),
			surface: PairedSurface{MCPServers: []string{"kapi"}, MCPTools: allMCP, Executables: []string{"kapi"}}},
		{name: "project-free arm as intended", launch: launch("codex", "project-free"),
			surface: PairedSurface{Skills: []string{"kapi-files", "skill-creator"}, BundledSkills: []string{"skill-creator"},
				SkillRoots: []string{filepath.Join(state, "codex", "skills", ".system"), filepath.Join(workspace, ".agents", "skills")},
				MCPServers: []string{}, Executables: []string{"kapi-files"}}},
		{name: "kapi skill in the baseline", launch: launch("claude", "baseline"),
			surface: PairedSurface{Skills: []string{"kapi"}, MCPServers: []string{}, Executables: []string{}}, problem: "skill kapi visible=true"},
		{name: "kapi on PATH in the baseline", launch: launch("codex", "baseline"),
			surface: PairedSurface{MCPServers: []string{}, Executables: []string{"kapi"}}, problem: "kapi names on PATH"},
		{name: "MCP server in the skill arm", launch: launch("codex", "skill-cli"),
			surface: PairedSurface{Skills: []string{"kapi"}, MCPServers: []string{"kapi"}, Executables: []string{"kapi"}}, problem: "MCP servers"},
		{name: "a developer's MCP server", launch: launch("claude", "mcp"),
			surface: PairedSurface{MCPServers: []string{"kapi", "playwright"}, MCPTools: allMCP, Executables: []string{"kapi"}}, problem: "MCP servers"},
		{name: "MCP server not connected", launch: launch("claude", "mcp"),
			surface: PairedSurface{MCPServers: []string{"kapi (failed)"}, Executables: []string{"kapi"}}, problem: "MCP servers"},
		{name: "edit tools hidden from the model", launch: launch("claude", "mcp"),
			surface: PairedSurface{MCPServers: []string{"kapi"}, MCPTools: allMCP[:1], Executables: []string{"kapi"}}, problem: "apply_edits not exposed"},
		{name: "the developer's skill", launch: launch("claude", "baseline"),
			surface: PairedSurface{Skills: []string{"okapi-expert"}, MCPServers: []string{}, Executables: []string{}}, problem: "developer's skill okapi-expert"},
		{name: "a skill root outside the cell", launch: launch("codex", "baseline"),
			surface: PairedSurface{Skills: []string{"find-skills"}, SkillRoots: []string{"/home/someone/.agents/skills"},
				MCPServers: []string{}, Executables: []string{}}, problem: "outside the cell"},
		{name: "an installed plugin", launch: launch("claude", "baseline"),
			surface: PairedSurface{Plugins: []string{"codex@/home/someone/.claude/plugins/codex"}, MCPServers: []string{}, Executables: []string{}},
			problem: "plugin codex"},
		{name: "unreadable surface", launch: launch("claude", "baseline"),
			surface: PairedSurface{Error: "claude ended without an init event"}, problem: "surface unreadable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			surface := tc.surface
			checkPairedSurface(&surface, tc.launch, developer)
			if tc.problem == "" {
				assert.Empty(t, surface.Problems)
				return
			}
			assert.True(t, strings.Contains(strings.Join(surface.Problems, "\n"), tc.problem), "%v", surface.Problems)
		})
	}
}

func TestPairedCodexSkillListing(t *testing.T) {
	text := "<skills_instructions>\n### Skill roots\n- `r0` = `/cell/codex/skills/.system`\n- `r1` = `/cell/ws/.agents/skills`\n" +
		"### Available skills\n- imagegen: Generate images. (file: r0/imagegen/SKILL.md)\n" +
		"- kapi-files: Use when reading, editing: any format. (file: r1/kapi-files/SKILL.md)\n</skills_instructions>"
	roots := map[string]string{}
	for _, match := range pairedCodexSkillRoot.FindAllStringSubmatch(text, -1) {
		roots[match[1]] = match[2]
	}
	assert.Equal(t, map[string]string{"r0": "/cell/codex/skills/.system", "r1": "/cell/ws/.agents/skills"}, roots)
	var names []string
	for _, match := range pairedCodexSkill.FindAllStringSubmatch(text, -1) {
		names = append(names, match[1]+"@"+match[2])
	}
	assert.Equal(t, []string{"imagegen@r0", "kapi-files@r1"}, names)
}
