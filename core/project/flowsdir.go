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

// A recipe declares a flow inline under `flows:`. A project with more than a
// couple of them keeps each in its own YAML file instead, named for the flow,
// in the directory the recipe names with `flows_dir:`, where each is reviewed
// and edited on its own. Both are the same flow to everything downstream: one
// steps spec, run through the project runner over the recipe's collections.
//
// The directory has no default. Flow files are authored configuration, so they
// sit wherever the project commits its configuration, and the recipe says
// where. They never sit under `.kapi/`, which is a disposable cache.

// FlowsDirIn returns the absolute path of the file-per-flow directory the
// recipe at root names with `flows_dir:`, or "" when it names none.
func (p *KapiProject) FlowsDirIn(root string) string {
	if p == nil || p.FlowsDir == "" {
		return ""
	}
	return filepath.Join(root, filepath.FromSlash(p.FlowsDir))
}

// validateFlowsDir keeps `flows_dir:` a committed directory every checkout
// resolves the same way: relative to the recipe, and outside the checkout's
// cache, where a flow file is lost to the next clean checkout and never reaches
// a teammate.
func (p *KapiProject) validateFlowsDir() error {
	if p.FlowsDir == "" {
		return nil
	}
	if filepath.IsAbs(p.FlowsDir) {
		return fmt.Errorf("flows_dir: %q is an absolute path. Name the directory relative to the recipe, so every checkout finds it", p.FlowsDir)
	}
	first, _, _ := strings.Cut(filepath.ToSlash(filepath.Clean(p.FlowsDir)), "/")
	if first == StateDirName {
		return fmt.Errorf("flows_dir: %q sits under %s/, a disposable cache for one checkout. Keep flow files in a committed directory, such as flows/", p.FlowsDir, StateDirName)
	}
	return nil
}

// cacheFlowsDirName is the directory under `.kapi/` that once held flow files.
// kapi reads nothing there; FlowFilesInCache finds files left in it so a
// person learns to move them.
const cacheFlowsDirName = "flows"

// FlowFilesInCache reports the flow files sitting in `.kapi/flows/` beside the
// recipe at recipePath, sorted by name. kapi never reads them, so a project
// that keeps flows there runs without them.
func FlowFilesInCache(recipePath string) []string {
	dir := filepath.Join(filepath.Dir(recipePath), StateDirName, cacheFlowsDirName)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".yaml" {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names
}

// flowFilesInCacheWarning is the one line a recipe load reports when
// `.kapi/flows/` holds flow files, or "" when it holds none.
func flowFilesInCacheWarning(recipePath string) string {
	files := FlowFilesInCache(recipePath)
	if len(files) == 0 {
		return ""
	}
	return fmt.Sprintf(
		"kapi does not read flow files in %s/%s/ (%s), because %s/ is a disposable cache. Move them to a committed directory and name it in the recipe with flows_dir: <dir>",
		StateDirName, cacheFlowsDirName, strings.Join(files, ", "), StateDirName)
}

// DirFlow is one flow file in the recipe's `flows_dir:`.
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

// LoadDirFlow reads the flow one file in dir names, dir being the recipe's
// resolved `flows_dir:` (FlowsDirIn). The error wraps os.ErrNotExist when dir
// is empty or holds no such file, which is how a caller tells "no flow by that
// name" from "a flow that will not load".
func LoadDirFlow(dir, name string) (*DirFlow, error) {
	if dir == "" {
		return nil, fmt.Errorf("project: no flow %q: the recipe names no flows_dir: %w", name, os.ErrNotExist)
	}
	if name == "" || strings.ContainsAny(name, `/\`) || name == "." || name == ".." {
		return nil, fmt.Errorf("project: flow name %q is not a file name: %w", name, os.ErrNotExist)
	}
	def := loadDirFlowFile(filepath.Join(dir, name+".yaml"), name)
	if def.Err != nil {
		return nil, def.Err
	}
	return def, nil
}

// ListDirFlows reads every flow file in dir, the recipe's resolved
// `flows_dir:`, ordered by name. An empty dir lists nothing. An entry that
// carries an Err named a file that will not run; a listing shows it with its
// problem, since a flow dropped from the listing is one whose author never
// learns why it does nothing.
func ListDirFlows(dir string) []*DirFlow {
	if dir == "" {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var flows []*DirFlow
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".yaml" {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".yaml")
		flows = append(flows, loadDirFlowFile(filepath.Join(dir, e.Name()), name))
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
