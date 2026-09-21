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
	"path/filepath"
	"slices"
	"strings"
)

// The fixture is a product team's repository, written so that an attentive
// reader picks up conventions nobody has written down: the product name with
// one casing, one verb for entering the product, a feature name that keeps its
// spelling everywhere, and a second-person address. Nothing in it mentions
// kapi, and it carries no recorded context at all. What `kapi init` writes over
// it is the whole of the wiring under test.
//
// Every attempt gets its own copy, generated outside this repository, because
// an agent host walks up from its working directory looking for CLAUDE.md and
// kapi walks up looking for kapi.yaml. A fixture inside the tree would find
// neokapi's own and the drill would measure the wrong project.

//go:embed all:testdata/coldstart
var coldStartFixtures embed.FS

const coldStartFixtureVersion = "coldstart-v1"

// coldStartRepoName is the fixture repository's directory name, which becomes
// the project name `kapi init` writes into the recipe.
const coldStartRepoName = "fernwell-ledger"

// ColdStartTask is one writing task. Its prompt names no kapi surface, no
// context, and nothing about recording or checking: an agent that reaches for
// kapi does so because the skill and the server are there.
type ColdStartTask struct {
	ID    string `json:"id"`
	Stage string `json:"stage"`
	// Correction marks the one session-one task whose prompt carries a person's
	// wording correction.
	Correction bool   `json:"correction"`
	Prompt     string `json:"prompt"`
	// Note is the evaluator's, and never enters a workspace or a prompt.
	Note string `json:"note"`
}

func coldStartTasks() []ColdStartTask {
	entries, err := coldStartFixtures.ReadDir("testdata/coldstart/tasks")
	if err != nil {
		panic(fmt.Errorf("read embedded cold-start tasks: %w", err))
	}
	tasks := make([]ColdStartTask, 0, len(entries))
	for _, entry := range entries {
		data, err := coldStartFixtures.ReadFile("testdata/coldstart/tasks/" + entry.Name())
		if err != nil {
			panic(fmt.Errorf("read embedded cold-start task: %w", err))
		}
		var task ColdStartTask
		if err := json.Unmarshal(data, &task); err != nil {
			panic(fmt.Errorf("decode embedded cold-start task %s: %w", entry.Name(), err))
		}
		tasks = append(tasks, task)
	}
	return tasks
}

func findColdStartTask(id string) (ColdStartTask, error) {
	for _, task := range coldStartTasks() {
		if task.ID == id {
			return task, nil
		}
	}
	return ColdStartTask{}, fmt.Errorf("unknown cold-start task %q", id)
}

// coldStartFixtureHash binds resumed runs to the repository and every prompt.
func coldStartFixtureHash() (string, error) {
	hash := sha256.New()
	encoder := json.NewEncoder(hash)
	if err := encoder.Encode(coldStartFixtureVersion); err != nil {
		return "", err
	}
	err := fs.WalkDir(coldStartFixtures, "testdata/coldstart", func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		data, err := coldStartFixtures.ReadFile(name)
		if err != nil {
			return err
		}
		return encoder.Encode(struct {
			Name string
			Data []byte
		}{Name: name, Data: data})
	})
	if err != nil {
		return "", fmt.Errorf("hash cold-start fixture: %w", err)
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// coldStartRepoFiles reads the fixture repository out of the embedded tree.
func coldStartRepoFiles() (map[string][]byte, error) {
	const prefix = "testdata/coldstart/repo"
	files := map[string][]byte{}
	err := fs.WalkDir(coldStartFixtures, prefix, func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		data, err := coldStartFixtures.ReadFile(name)
		if err != nil {
			return err
		}
		files[name[len(prefix)+1:]] = data
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("read cold-start fixture: %w", err)
	}
	return files, nil
}

// materializeColdStartRepo writes the fixture into an empty directory.
func materializeColdStartRepo(dir string) error {
	files, err := coldStartRepoFiles()
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

// coldStartDiscoveryNames are the files a kapi or an agent host finds by
// walking up from a working directory. One of them above the fixture would
// bind the session to a project the drill is not measuring.
var coldStartDiscoveryNames = []string{"kapi.yaml", "CLAUDE.md", "AGENTS.md", ".mcp.json", ".kapi"}

// coldStartAncestorFindings reports every discoverable file above dir, which is
// what makes "outside the neokapi tree" a measured property rather than an
// assumption. An empty result is the passing one.
func coldStartAncestorFindings(dir string) ([]string, error) {
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return nil, err
	}
	findings := []string{}
	for current := filepath.Dir(resolved); ; current = filepath.Dir(current) {
		for _, name := range coldStartDiscoveryNames {
			if _, err := os.Lstat(filepath.Join(current, name)); err == nil {
				findings = append(findings, filepath.Join(current, name))
			}
		}
		parent := filepath.Dir(current)
		if parent == current {
			return findings, nil
		}
	}
}

// coldStartRecipeMapping completes the content mapping `kapi init` leaves for
// the person: the scaffolded recipe holds an empty collection list and a
// starter voice pack, and its own comments say to point collections at the
// files to keep in voice.
//
// The pack goes because the drill starts from an empty context. A pack states
// tone, style and a vocabulary of its own, so a session run over one would be
// measured against wording the fixture never used.
func coldStartRecipeMapping(recipe []byte) ([]byte, error) {
	const (
		emptyCollections = "collections: []"
		starterPack      = "  voice:\n    pack: professional-b2b\n"
	)
	body := string(recipe)
	if !strings.Contains(body, emptyCollections) {
		return nil, fmt.Errorf("scaffolded recipe no longer holds %q, so the fixture's content mapping was not written", emptyCollections)
	}
	if !strings.Contains(body, starterPack) {
		return nil, fmt.Errorf("scaffolded recipe no longer binds a starter voice pack, so the drill cannot establish an empty context")
	}
	mapping := "collections:\n" +
		"  - path: \"README.md\"\n    format: markdown\n" +
		"  - path: \"docs/**/*.md\"\n    format: markdown\n" +
		"  - path: \"emails/**/*.md\"\n    format: markdown\n" +
		"  - path: \"ui/strings.json\"\n    format: json"
	body = strings.Replace(body, starterPack, "", 1)
	body = strings.Replace(body, emptyCollections, mapping, 1)
	return []byte(body), nil
}
