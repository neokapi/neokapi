package main

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path"
	"slices"
)

const pairedCorpusVersion = "wp5-v2"

// PairedTask describes an identical editing task across agent integrations.
// Its private specification and acceptance criteria never enter the workspace.
type PairedTask struct {
	ID     string `json:"id"`
	Family string `json:"family"`
	Prompt string `json:"prompt"`
	spec   pairedTaskSpec
}

type pairedTaskSpec struct {
	ID     string `json:"id"`
	Family string `json:"family"`
	Prompt string `json:"prompt"`
	// Editable lists the fixture files the task may change. Every other
	// fixture file must stay byte-identical.
	Editable []string `json:"editable"`
	// Creates lists the files the task may add.
	Creates []string `json:"creates,omitempty"`
	// Scope lists the directories in which no file other than a fixture file
	// or one Creates names may appear.
	Scope []string `json:"scope"`
	// Interference is the change another editor makes to a file once the
	// agent has seen the text it changes, for a task that measures recovery
	// from it. A file it names is graded against the reference when the
	// change landed and against the reference without it when it did not.
	Interference *PairedInterference `json:"interference,omitempty"`
	// LateContext is context a person adds once the agent has read the
	// project's, for a task that measures recovery from a refusal.
	LateContext       *PairedLateContext `json:"late_context,omitempty"`
	Criteria          []pairedCriterion  `json:"criteria"`
	HumanReviewRubric []string           `json:"humanReviewRubric"`
}

// pairedCriterion is one deterministic assertion on the files an attempt
// leaves, or on what its transcript shows; validatePairedCriterion lists the
// kinds. An informational criterion is reported and never decides
// ObjectivePassed.
type pairedCriterion struct {
	ID      string   `json:"id"`
	Kind    string   `json:"kind"`
	Path    string   `json:"path,omitempty"`
	Source  string   `json:"source,omitempty"`
	Value   string   `json:"value,omitempty"`
	Block   int      `json:"block,omitempty"`
	Except  []int    `json:"except,omitempty"`
	Require []string `json:"require,omitempty"`
	Forbid  []string `json:"forbid,omitempty"`
	// Key names a top-level member of a JSON catalog, and Branch a branch of
	// the ICU plural or select its value holds.
	Key           string `json:"key,omitempty"`
	Branch        string `json:"branch,omitempty"`
	Informational bool   `json:"informational,omitempty"`
	Description   string `json:"description"`
}

// PairedInterference replaces Find with Replace in Path once, as soon as a
// tool result has shown the agent Find: the agent has then read the text the
// other editor changes, and any edit it bases on that read is stale.
type PairedInterference struct {
	Path        string `json:"path"`
	Find        string `json:"find"`
	Replace     string `json:"replace"`
	Description string `json:"description,omitempty"`
}

// pairedReference returns the evaluator's expected bytes for a task's file.
func pairedReference(task PairedTask, name string) ([]byte, error) {
	if !fs.ValidPath(name) {
		return nil, fmt.Errorf("invalid reference path %q", name)
	}
	return pairedFixtures.ReadFile("testdata/paired/references/" + task.ID + "/" + name)
}

// all: includes the .kapi inputs. Only common and workspace files are copied;
// task specifications and reference repairs stay with the evaluator.
//
//go:embed all:testdata/paired
var pairedFixtures embed.FS

func pairedTasks() []PairedTask {
	entries, err := pairedFixtures.ReadDir("testdata/paired/tasks")
	if err != nil {
		panic(fmt.Errorf("read embedded paired corpus: %w", err))
	}
	tasks := make([]PairedTask, 0, len(entries))
	for _, entry := range entries {
		data, err := pairedFixtures.ReadFile("testdata/paired/tasks/" + entry.Name() + "/task.json")
		if err != nil {
			panic(fmt.Errorf("read embedded paired task: %w", err))
		}
		var spec pairedTaskSpec
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&spec); err != nil {
			panic(fmt.Errorf("decode embedded paired task %s: %w", entry.Name(), err))
		}
		tasks = append(tasks, PairedTask{ID: spec.ID, Family: spec.Family, Prompt: spec.Prompt, spec: spec})
	}
	return tasks
}

// pairedCorpusHash binds resumed runs to every input, criterion and review rubric.
func pairedCorpusHash() (string, error) {
	hash := sha256.New()
	encoder := json.NewEncoder(hash)
	if err := encoder.Encode(pairedCorpusVersion); err != nil {
		return "", err
	}
	err := fs.WalkDir(pairedFixtures, "testdata/paired", func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		data, err := pairedFixtures.ReadFile(name)
		if err != nil {
			return err
		}
		return encoder.Encode(struct {
			Name string
			Data []byte
		}{Name: name, Data: data})
	})
	if err != nil {
		return "", fmt.Errorf("hash paired corpus: %w", err)
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func pairedTaskFiles(task PairedTask) (map[string][]byte, error) {
	if !fs.ValidPath(task.ID) || path.Base(task.ID) != task.ID {
		return nil, fmt.Errorf("invalid paired task ID %q", task.ID)
	}
	files := map[string][]byte{}
	for _, prefix := range []string{"testdata/paired/common", "testdata/paired/tasks/" + task.ID + "/workspace"} {
		err := fs.WalkDir(pairedFixtures, prefix, func(name string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				return nil
			}
			data, err := pairedFixtures.ReadFile(name)
			if err != nil {
				return err
			}
			files[name[len(prefix)+1:]] = data
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("read paired fixture %q: %w", task.ID, err)
		}
	}
	return files, nil
}

func materializePairedTask(dir string, task PairedTask) error {
	files, err := pairedTaskFiles(task)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	root, err := openPairedRoot(dir)
	if err != nil {
		return err
	}
	defer root.Close()
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		if err := root.MkdirAll(path.Dir(name), 0o700); err != nil {
			return err
		}
		if err := checkPairedPath(root, path.Dir(name)); err != nil {
			return err
		}
		file, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			return fmt.Errorf("create fixture %q: %w", name, err)
		}
		_, writeErr := file.Write(files[name])
		closeErr := file.Close()
		if writeErr != nil {
			return writeErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}
