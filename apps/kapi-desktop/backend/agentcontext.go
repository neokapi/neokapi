package backend

import (
	"strings"

	"github.com/neokapi/neokapi/host"
)

// What an agent is told about a file.
//
// An agent working in this project reads the `context://<path>` resource, and a
// person at a terminal runs `kapi context <path>`. Both are served by one host
// resolution, and so is this: ContextAt assembles the answer and FormatText
// renders the body the resource returns. The app shows the same words, not a
// second description of them, so a writer can see what the agent editing beside
// them has been given.

// AgentContextView is the resolved context for one file, in both the shapes a
// reader needs: the answer's own fields, for a view that lays them out, and the
// text an agent receives verbatim.
type AgentContextView struct {
	// Path is the file the answer is about, project-relative.
	Path string `json:"path"`
	// Answer is the resolution: the point, the voice in force, the terms that
	// apply, the governance windows around them, and the notes that say what
	// could not be consulted.
	Answer *host.ContextAnswer `json:"answer"`
	// Text is the resource body, exactly as the `context://` resource serves it
	// and `kapi context <path>` prints it.
	Text string `json:"text"`
}

// AgentContextAt returns the resolved context for one file in the open project.
//
// limit caps the terms rendered, and zero takes the same default the CLI and
// the resource take, so the app's list is the agent's list.
func (a *App) AgentContextAt(tabID, relPath string, limit int) (*AgentContextView, error) {
	answer, err := a.ContextAt(tabID, relPath, limit)
	if err != nil {
		return nil, err
	}
	var body strings.Builder
	if err := answer.FormatText(&body); err != nil {
		return nil, err
	}
	return &AgentContextView{Path: relPath, Answer: answer, Text: body.String()}, nil
}
