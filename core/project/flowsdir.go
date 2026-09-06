package project

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/neokapi/neokapi/core/flow"
	"gopkg.in/yaml.v3"
)

// FlowsDirName holds the project's file-per-flow definitions, one YAML file
// per flow, named for the flow. A recipe declares a flow inline under `flows:`;
// a project with more than a couple of them keeps each in its own file instead,
// where it is reviewed and edited on its own. Both are the same flow to
// everything downstream: one steps spec, run through the project runner over
// the recipe's collections.
const FlowsDirName = "flows"

// FlowsDir returns the absolute path of the file-per-flow directory.
func (l Layout) FlowsDir() string {
	return filepath.Join(l.StateDir, FlowsDirName)
}

// DirFlow is one flow file in the project's flows directory.
type DirFlow struct {
	// Name is the flow's name, taken from the file name. The `name:` key
	// inside the file is a label; the file name is what `kapi run` resolves.
	Name string
	// Description is the file's `description:` key, empty when it has none.
	Description string
	// Path is the file the flow was read from.
	Path string
	// Spec is the flow's steps, in the shape a recipe's inline `flows:` entry
	// parses to. Nil when Err is set.
	Spec *flow.StepsSpec
	// Err is why the file does not describe a runnable flow. A listing shows
	// the flow with its problem rather than omitting it, so a file that cannot
	// run is visible where the user looks for it.
	Err error
}

// dirFlowFile is the file's own shape: a steps spec plus the two presentation
// keys a file-per-flow definition carries and a recipe entry does not.
type dirFlowFile struct {
	Name           string `yaml:"name"`
	Description    string `yaml:"description"`
	flow.StepsSpec `yaml:",inline"`
}

// dirFlowPathProbe reads the per-step path keys checkDirFlowPaths rejects. It
// names only those, so it stays correct as flow.FlowStep gains fields.
type dirFlowPathProbe struct {
	Steps []struct {
		Tool   string `yaml:"tool"`
		Input  string `yaml:"input"`
		Output string `yaml:"output"`
	} `yaml:"steps"`
}

// LoadDirFlow reads the flow one file in the project's flows directory names.
// The error wraps os.ErrNotExist when the project has no such file, which is
// how a caller tells "no flow by that name" from "a flow that will not load".
func LoadDirFlow(l Layout, name string) (*DirFlow, error) {
	if name == "" || strings.ContainsAny(name, `/\`) || name == "." || name == ".." {
		return nil, fmt.Errorf("project: flow name %q is not a file name: %w", name, os.ErrNotExist)
	}
	def := loadDirFlowFile(filepath.Join(l.FlowsDir(), name+".yaml"), name)
	if def.Err != nil {
		return nil, def.Err
	}
	return def, nil
}

// ListDirFlows reads every flow file in the project's flows directory, ordered
// by name. An entry that carries an Err named a file that will not run; a
// listing shows it with its problem, since a flow dropped from the listing is
// one whose author never learns why it does nothing.
func ListDirFlows(l Layout) []*DirFlow {
	entries, err := os.ReadDir(l.FlowsDir())
	if err != nil {
		return nil
	}

	var flows []*DirFlow
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".yaml" {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".yaml")
		flows = append(flows, loadDirFlowFile(filepath.Join(l.FlowsDir(), e.Name()), name))
	}
	sort.Slice(flows, func(i, j int) bool { return flows[i].Name < flows[j].Name })
	return flows
}

// loadDirFlowFile reads one flow file. The returned DirFlow always carries the
// name and path, so a listing can show a file it could not load.
func loadDirFlowFile(path, name string) *DirFlow {
	def := &DirFlow{Name: name, Path: path}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			def.Err = fmt.Errorf("project: no flow %q under %s: %w", name, filepath.Dir(path), os.ErrNotExist)
			return def
		}
		def.Err = fmt.Errorf("project: read flow %q: %w", name, err)
		return def
	}

	var file dirFlowFile
	if err := yaml.Unmarshal(data, &file); err != nil {
		def.Err = fmt.Errorf("project: parse flow %s: %w", path, err)
		return def
	}
	def.Description = file.Description
	if len(file.Steps) == 0 {
		def.Err = fmt.Errorf("project: flow %s declares no steps", path)
		return def
	}
	if err := checkDirFlowPaths(data, path); err != nil {
		def.Err = err
		return def
	}

	spec := file.StepsSpec
	def.Spec = &spec
	return def
}

// checkDirFlowPaths rejects a step that names a file. A flow declares what to
// do, and the run supplies what to do it to (AD-026): the files come from the
// recipe's collections, or from --input. A step naming its own path asks for a
// run outside the project, with none of the format bindings, locale passes or
// standing bindings a project run resolves.
func checkDirFlowPaths(data []byte, path string) error {
	var probe dirFlowPathProbe
	if err := yaml.Unmarshal(data, &probe); err != nil {
		return nil // the spec decode reports what the file cannot parse as
	}
	for i, step := range probe.Steps {
		var key string
		switch {
		case step.Input != "":
			key = "an input"
		case step.Output != "":
			key = "an output"
		default:
			continue
		}
		where := fmt.Sprintf("step %d", i+1)
		if step.Tool != "" {
			where = fmt.Sprintf("step %d (%s)", i+1, step.Tool)
		}
		return fmt.Errorf(
			"project: flow %s: %s names %s path. A flow's steps take no paths: a run takes its files from the recipe's collections, or from --input",
			path, where, key)
	}
	return nil
}
