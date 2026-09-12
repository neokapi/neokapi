package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"slices"
	"strings"
)

// PairedValidation deliberately has no overall accepted/pass field: objective
// checks cannot establish meaning, audience suitability or reviewer effort.
type PairedValidation struct {
	ObjectivePassed     bool                    `json:"objectivePassed"`
	Criteria            []PairedCriterionResult `json:"criteria"`
	HumanReviewRequired bool                    `json:"humanReviewRequired"`
	HumanReviewStatus   string                  `json:"humanReviewStatus"`
	HumanReviewRubric   []string                `json:"humanReviewRubric"`
}

type PairedCriterionResult struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	Path        string `json:"path"`
	Passed      bool   `json:"passed"`
	Detail      string `json:"detail,omitempty"`
}

func validatePairedTask(dir string, task PairedTask) (PairedValidation, error) {
	result := PairedValidation{
		ObjectivePassed: true, Criteria: []PairedCriterionResult{},
		HumanReviewRequired: true, HumanReviewStatus: "pending",
		HumanReviewRubric: slices.Clone(task.spec.HumanReviewRubric),
	}
	files, err := pairedTaskFiles(task)
	if err != nil {
		return result, err
	}
	root, err := openPairedRoot(dir)
	if err != nil {
		return result, err
	}
	defer root.Close()
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	slices.Sort(names)
	originals := map[string]map[string]string{}
	outputs := map[string]map[string]string{}
	for _, name := range names {
		body, readErr := readPairedFile(root, name)
		if !slices.Contains(task.spec.Editable, name) {
			result.add(PairedCriterionResult{
				ID: "unchanged:" + name, Description: "File outside the edit scope is unchanged", Path: name,
				Passed: readErr == nil && bytes.Equal(body, files[name]), Detail: pairedErrorDetail(readErr),
			})
			continue
		}
		original, err := parsePairedDocument(files[name])
		if err != nil {
			return result, fmt.Errorf("invalid original fixture %q: %w", name, err)
		}
		originals[name] = original
		var output map[string]string
		if readErr == nil {
			output, readErr = parsePairedDocument(body)
		}
		validShape := readErr == nil && samePairedKeys(original, output)
		result.add(PairedCriterionResult{
			ID: "structure:" + name, Description: "Valid JSON with the original keys and string types", Path: name,
			Passed: validShape, Detail: pairedErrorDetail(readErr),
		})
		if validShape {
			outputs[name] = output
		}
	}
	contentErr := fs.WalkDir(root.FS(), "content", func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		_, known := files[name]
		if !known || entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("unexpected content artifact %q", name)
		}
		return nil
	})
	result.add(PairedCriterionResult{
		ID: "content-scope", Description: "No unexpected content files or symlinks", Path: "content",
		Passed: contentErr == nil, Detail: pairedErrorDetail(contentErr),
	})
	for _, criterion := range task.spec.Criteria {
		if !fs.ValidPath(criterion.Path) {
			return result, fmt.Errorf("invalid criterion path %q", criterion.Path)
		}
		output, exists := outputs[criterion.Path]
		value, hasKey := output[criterion.Key]
		passed := false
		switch criterion.Kind {
		case "json_equals":
			passed = exists && hasKey && value == criterion.Value
		case "json_changed":
			passed = exists && hasKey && value != originals[criterion.Path][criterion.Key] && strings.TrimSpace(value) != ""
		default:
			return result, fmt.Errorf("unknown criterion kind %q", criterion.Kind)
		}
		result.add(PairedCriterionResult{
			ID: criterion.ID, Description: criterion.Description, Path: criterion.Path, Passed: passed,
		})
	}
	return result, nil
}

func (result *PairedValidation) add(criterion PairedCriterionResult) {
	result.Criteria = append(result.Criteria, criterion)
	result.ObjectivePassed = result.ObjectivePassed && criterion.Passed
}

func pairedErrorDetail(err error) string {
	if err != nil {
		return err.Error()
	}
	return ""
}

func openPairedRoot(dir string) (*os.Root, error) {
	info, err := os.Lstat(dir)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, errors.New("paired workspace must be a directory, not a symlink or file")
	}
	return os.OpenRoot(dir)
}

// Root confines access even if a component is replaced concurrently. Explicit
// checks additionally reject in-workspace symlinks instead of scoring their targets.
func checkPairedPath(root *os.Root, name string) error {
	if !fs.ValidPath(name) {
		return fmt.Errorf("invalid workspace path %q", name)
	}
	parts := strings.Split(name, "/")
	for index := range parts {
		info, err := root.Lstat(strings.Join(parts[:index+1], "/"))
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink is not an evaluation artifact: %q", name)
		}
	}
	return nil
}

func readPairedFile(root *os.Root, name string) ([]byte, error) {
	if err := checkPairedPath(root, name); err != nil {
		return nil, err
	}
	entry, err := root.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !entry.Mode().IsRegular() {
		return nil, fmt.Errorf("artifact %q is not a regular file", name)
	}
	file, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("artifact %q is not a regular file", name)
	}
	const maxBytes = 4 << 20
	body, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxBytes {
		return nil, fmt.Errorf("artifact %q exceeds 4 MiB", name)
	}
	return body, nil
}

// The authored pilot documents are flat JSON objects with string values. Decode
// token by token so duplicate keys cannot silently overwrite a protected field.
func parsePairedDocument(body []byte) (map[string]string, error) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	start, err := decoder.Token()
	if err != nil || start != json.Delim('{') {
		return nil, errors.New("expected a JSON object")
	}
	values := map[string]string{}
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		key, ok := token.(string)
		if !ok {
			return nil, errors.New("expected a JSON key")
		}
		if _, duplicate := values[key]; duplicate {
			return nil, fmt.Errorf("duplicate JSON key %q", key)
		}
		var value any
		if err := decoder.Decode(&value); err != nil {
			return nil, err
		}
		text, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("field %q must remain a string", key)
		}
		values[key] = text
	}
	if _, err := decoder.Token(); err != nil {
		return nil, err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return nil, errors.New("unexpected trailing JSON content")
	}
	return values, nil
}

func samePairedKeys(original, output map[string]string) bool {
	if len(original) != len(output) {
		return false
	}
	for key := range original {
		if _, found := output[key]; !found {
			return false
		}
	}
	return true
}
