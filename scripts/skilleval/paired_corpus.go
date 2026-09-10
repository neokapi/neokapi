package main

import (
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

const pairedCorpusVersion = "pilot-v1"

// PairedTask describes an identical editing task across agent integrations.
// Its private specification and acceptance criteria never enter the workspace.
type PairedTask struct {
	ID     string `json:"id"`
	Family string `json:"family"`
	Prompt string `json:"prompt"`
	spec   pairedTaskSpec
}

type pairedTaskSpec struct {
	ID                string            `json:"id"`
	Family            string            `json:"family"`
	Prompt            string            `json:"prompt"`
	Editable          []string          `json:"editable"`
	Criteria          []pairedCriterion `json:"criteria"`
	HumanReviewRubric []string          `json:"humanReviewRubric"`
}

type pairedCriterion struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	Path        string `json:"path"`
	Key         string `json:"key"`
	Value       string `json:"value"`
	Description string `json:"description"`
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
		if err := json.Unmarshal(data, &spec); err != nil {
			panic(fmt.Errorf("decode embedded paired task: %w", err))
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
