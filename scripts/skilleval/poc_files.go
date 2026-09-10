package main

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

func loadPOCStage(path string) (pocStage, string, error) {
	var stage pocStage
	data, err := os.ReadFile(path)
	if err != nil {
		return stage, "", err
	}
	if err := decodeMeaningJSON(data, &stage); err != nil {
		return stage, "", fmt.Errorf("POC manifest: %w", err)
	}
	if !pairedIDPattern.MatchString(stage.ID) {
		return stage, "", errors.New("POC stage id must be path-safe lowercase")
	}
	if stage.Condition != "baseline" && stage.Condition != "skill" {
		return stage, "", errors.New("POC condition must be baseline or skill")
	}
	if stage.TimeoutSeconds < 1 || stage.TimeoutSeconds > 1800 || stage.MaxTurns < 1 || stage.MaxTurns > 100 {
		return stage, "", errors.New("POC timeout must be 1..1800 seconds and max_turns 1..100")
	}
	for _, files := range [][]string{stage.InputFiles, stage.ExpectedOutputs} {
		if err := validatePOCPaths(files); err != nil {
			return stage, "", err
		}
	}
	for _, value := range []*string{&stage.Workspace, &stage.PromptFile, &stage.KapiBin} {
		if *value == "" {
			continue
		}
		if !filepath.IsAbs(*value) {
			*value = filepath.Join(filepath.Dir(path), *value)
		}
		*value, err = filepath.Abs(*value)
		if err != nil {
			return stage, "", err
		}
	}
	if stage.Workspace == "" || stage.PromptFile == "" {
		return stage, "", errors.New("POC workspace and prompt_file are required")
	}
	if stage.Condition == "skill" && stage.KapiBin == "" {
		return stage, "", errors.New("POC skill condition requires kapi_bin")
	}
	if stage.KapiBin != "" {
		stage.KapiBin, err = filepath.EvalSymlinks(stage.KapiBin)
		if err != nil {
			return stage, "", err
		}
	}
	prompt, err := os.ReadFile(stage.PromptFile)
	if err != nil {
		return stage, "", err
	}
	if strings.TrimSpace(string(prompt)) == "" {
		return stage, "", errors.New("POC prompt must not be empty")
	}
	return stage, string(prompt), nil
}

func validatePOCPaths(paths []string) error {
	if len(paths) == 0 {
		return errors.New("POC input_files and expected_outputs must each list files")
	}
	seen := make(map[string]bool, len(paths))
	for _, path := range paths {
		if !fs.ValidPath(path) || path == "." || strings.Contains(path, "\\") || seen[path] {
			return fmt.Errorf("invalid or duplicate relative POC file %q", path)
		}
		seen[path] = true
	}
	return nil
}

func pocSeparateLedger(workspace, ledger string) error {
	// Resolve an existing ancestor too: a not-yet-created ledger beneath a
	// symlink into the workspace must not expose transcripts or account state.
	actualWorkspace, err := filepath.EvalSymlinks(workspace)
	if err != nil {
		return err
	}
	ancestor := ledger
	suffix := []string{}
	for {
		actual, err := filepath.EvalSymlinks(ancestor)
		if err == nil {
			for _, part := range slices.Backward(suffix) {
				actual = filepath.Join(actual, part)
			}
			rel, err := filepath.Rel(actualWorkspace, actual)
			if err != nil {
				return err
			}
			outside := rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator))
			if !outside {
				return errors.New("POC ledger must be outside the authoring workspace")
			}
			return nil
		}
		if !errors.Is(err, os.ErrNotExist) || ancestor == filepath.Dir(ancestor) {
			return err
		}
		suffix = append(suffix, filepath.Base(ancestor))
		ancestor = filepath.Dir(ancestor)
	}
}

func freezePOCInputs(opts pocOptions, stage pocStage, prompt, dir string) (pocFrozen, error) {
	frozen := pocFrozen{Schema: 1, Stage: stage, PromptHash: meaningTextHash(prompt), InputHashes: map[string]string{}}
	root, err := openPairedRoot(stage.Workspace)
	if err != nil {
		return frozen, err
	}
	defer root.Close()
	for _, path := range stage.InputFiles {
		body, err := readPairedFile(root, path)
		if err != nil {
			return frozen, err
		}
		frozen.InputHashes[path] = meaningTextHash(string(body))
		if err := ensurePOCFile(filepath.Join(dir, "inputs", filepath.FromSlash(path)), body); err != nil {
			return frozen, err
		}
	}
	if err := ensurePOCFile(filepath.Join(dir, "prompt.txt"), []byte(prompt)); err != nil {
		return frozen, err
	}
	frozen.CodeHash, err = pairedTreeHash(filepath.Join(opts.RepoRoot, "scripts", "skilleval"))
	if err != nil {
		return frozen, err
	}
	if stage.KapiBin != "" {
		frozen.BinaryHash, err = pairedFileHash(stage.KapiBin)
		if err != nil {
			return frozen, err
		}
	}
	if stage.Condition == "skill" {
		skill := filepath.Join(opts.RepoRoot, "cli", "skills", "data", "kapi")
		frozen.SkillHash, err = pairedTreeHash(skill)
		if err != nil {
			return frozen, err
		}
		copyPath := filepath.Join(dir, "skill-input")
		if _, err := os.Stat(copyPath); errors.Is(err, os.ErrNotExist) {
			if err := copyTree(skill, copyPath); err != nil {
				return frozen, err
			}
		}
		retainedHash, err := pairedTreeHash(copyPath)
		if err != nil {
			return frozen, err
		}
		if retainedHash != frozen.SkillHash {
			return frozen, errors.New("shipped skill differs from retained POC input")
		}
	}
	frozen.Fingerprint, err = pairedHash(frozen)
	if err != nil {
		return frozen, err
	}
	if err := ensurePairedJSON(filepath.Join(dir, "frozen.json"), frozen); err != nil {
		return frozen, err
	}
	return frozen, nil
}

func ensurePOCFile(path string, body []byte) error {
	retained, err := os.ReadFile(path)
	if err == nil {
		if !bytes.Equal(body, retained) {
			return fmt.Errorf("immutable POC input differs: %s", path)
		}
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return writePairedExclusive(path, body)
}

func verifyPOCInputs(frozen pocFrozen) error {
	root, err := openPairedRoot(frozen.Stage.Workspace)
	if err != nil {
		return err
	}
	defer root.Close()
	for path, hash := range frozen.InputHashes {
		body, err := readPairedFile(root, path)
		if err != nil {
			return err
		}
		if meaningTextHash(string(body)) != hash {
			return fmt.Errorf("POC input changed during preparation: %s", path)
		}
	}
	return nil
}

func capturePOCOutputs(stage pocStage, dir string) []pocArtifact {
	outputs := make([]pocArtifact, 0, len(stage.ExpectedOutputs))
	root, rootErr := openPairedRoot(stage.Workspace)
	if rootErr == nil {
		defer root.Close()
	}
	for _, path := range stage.ExpectedOutputs {
		output := pocArtifact{Path: path}
		var body []byte
		err := rootErr
		if err == nil {
			body, err = readPairedFile(root, path)
		}
		if err == nil {
			output.SHA256, output.Bytes = meaningTextHash(string(body)), len(body)
			err = ensurePOCFile(filepath.Join(dir, "outputs", filepath.FromSlash(path)), body)
		}
		if err != nil {
			output.Error = err.Error()
		}
		outputs = append(outputs, output)
	}
	return outputs
}
