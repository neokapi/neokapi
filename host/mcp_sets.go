package host

import (
	"fmt"
	"slices"
	"strings"
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
	// context:// resources, the context tools that ask and record,
	// check_file, and the edit contract (read_blocks reads a document's
	// blocks, apply_edits sends the change service a change set, and
	// describe_format says what a format supports). It is the set served when
	// none is named.
	MCPSetWriting = "writing"
	// MCPSetContent works on content directly: checks on text, voice
	// rewrites, format detection and redaction.
	MCPSetContent = "content"
	// MCPSetTranslation runs the loop that fills target languages.
	MCPSetTranslation = "translation"
	// MCPSetReview is the review queue, the review picture of one block, and
	// apply_edits, which records an agent's pre-review as a decide operation.
	MCPSetReview = "review"
	// MCPSetAll is every set.
	MCPSetAll = "all"
)

// mcpToolSets names the tools in each set. A tool may sit in more than one
// set, and is served when any set that lists it is named. A tool on no list
// is served whatever sets are named: the widened registry and flow surfaces
// and a plugin's own tools, which their own flags and installation decide.
var mcpToolSets = map[string][]string{
	MCPSetWriting: {
		"context_read", "context_search", "context_observe", "context_correct",
		"context_withdraw", "context_session_summary", "check_file",
		"read_blocks", "apply_edits", "describe_format",
	},
	MCPSetContent: {
		"check_text", "voice_check", "voice_rewrite", "term-check",
		"detect_format", "redact",
	},
	MCPSetTranslation: {"translate", "up", "up_plan", "stats"},
	MCPSetReview:      {"review_queue", "review_block", "apply_edits"},
}

// mcpResourceSet is the set the context:// resources belong to.
const mcpResourceSet = MCPSetWriting

// MCPToolSetNames lists the set names `--tools` takes, in the order help
// prints them.
func MCPToolSetNames() []string {
	return []string{MCPSetWriting, MCPSetContent, MCPSetTranslation, MCPSetReview, MCPSetAll}
}

// MCPToolSetTools returns the tools one set serves, nil for an unknown name.
// A tool another set also lists is among them.
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

// MCPSurface is the tool surface a server exposes. It is set from flags on
// `kapi mcp` (--tools, --all-tools, --all-flows, --all) rather than an
// environment variable: the surface an assistant sees is a property of how the
// server was started, so it belongs on the command that starts it, where
// `--help` lists it.
type MCPSurface struct {
	// Sets are the tool sets served (the MCPSet constants above). nil serves every set,
	// for a caller that builds a server without going through `kapi mcp`.
	Sets map[string]bool
	// AllTools exposes every CLI-visible registry tool instead of the curated
	// set — pipeline steps, format internals, one-off transforms.
	AllTools bool
	// AllFlows exposes the flow-running verbs.
	AllFlows bool
}
