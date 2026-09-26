package host

import (
	"fmt"
	"slices"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Tool sets: what `kapi mcp --tools <set>[,<set>...]` serves.
//
// An assistant reads every tool description it is offered into its context,
// and chooses among them. A writing session needs a handful, so the server
// serves the writing set unless it is asked for more, and each further set is
// a named decision about what the assistant is there to do.

// The tool sets `kapi mcp --tools` takes.
const (
	// MCPSetWriting is what an assistant writing in the project needs: the
	// context:// resources, the context tools that ask and record, and
	// check_file. It is the set served when none is named.
	MCPSetWriting = "writing"
	// MCPSetContent works on content directly: checks on text, voice rewrites,
	// extraction and the write verb.
	MCPSetContent = "content"
	// MCPSetTranslation runs the loop that fills target languages.
	MCPSetTranslation = "translation"
	// MCPSetReview is the review queue and an agent's pre-review of a unit.
	MCPSetReview = "review"
	// MCPSetAll is every set.
	MCPSetAll = "all"
)

// mcpToolSets names the tools in each set. A tool on no list is served
// whatever sets are named: the widened registry and flow surfaces and a
// plugin's own tools, which their own flags and installation decide.
var mcpToolSets = map[string][]string{
	MCPSetWriting: {
		"context_read", "context_search", "context_observe", "context_correct",
		"context_withdraw", "context_session_summary", "check_file",
	},
	MCPSetContent: {
		"check_text", "voice_check", "voice_rewrite", "term-check",
		"extract_content", "detect_format", "apply_edits", "redact",
	},
	MCPSetTranslation: {"translate", "up", "up_plan", "stats"},
	MCPSetReview:      {"review_queue", "review_unit", "pre_review_unit"},
}

// mcpResourceSet is the set the context:// resources belong to.
const mcpResourceSet = MCPSetWriting

// MCPToolSetNames lists the set names `--tools` takes, in the order help
// prints them.
func MCPToolSetNames() []string {
	return []string{MCPSetWriting, MCPSetContent, MCPSetTranslation, MCPSetReview, MCPSetAll}
}

// MCPToolSetTools returns the tools one set serves, nil for an unknown name.
func MCPToolSetTools(set string) []string {
	return slices.Clone(mcpToolSets[set])
}

// ParseMCPToolSets reads the `--tools` values into the sets they select. An
// empty list selects the writing set; `all` selects every set. A name that is
// not a set fails with the list of sets.
func ParseMCPToolSets(values []string) (map[string]bool, error) {
	sets := map[string]bool{}
	for _, v := range values {
		for name := range strings.SplitSeq(v, ",") {
			name = strings.ToLower(strings.TrimSpace(name))
			switch {
			case name == "":
				continue
			case name == MCPSetAll:
				for set := range mcpToolSets {
					sets[set] = true
				}
			case mcpToolSets[name] != nil:
				sets[name] = true
			default:
				return nil, fmt.Errorf("unknown tool set %q: the sets are %s",
					name, strings.Join(MCPToolSetNames(), ", "))
			}
		}
	}
	if len(sets) == 0 {
		sets[MCPSetWriting] = true
	}
	return sets, nil
}

// AllMCPToolSets selects every set.
func AllMCPToolSets() map[string]bool {
	sets := map[string]bool{}
	for set := range mcpToolSets {
		sets[set] = true
	}
	return sets
}

// pruneMCPToolSets removes from the server every tool and resource whose set
// is not selected. A nil selection serves every set, for a caller that builds
// a server without going through `kapi mcp`.
func pruneMCPToolSets(server *mcp.Server, selected map[string]bool) {
	if selected == nil {
		return
	}
	for set, tools := range mcpToolSets {
		if !selected[set] {
			server.RemoveTools(tools...)
		}
	}
	if !selected[mcpResourceSet] {
		server.RemoveResourceTemplates(contextLocationTemplate, contextProfileTemplate)
	}
}
