package host

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/neokapi/neokapi/core/sectionedit"
	"github.com/pmezard/go-difflib/difflib"
	yamlv3 "gopkg.in/yaml.v3"
)

// SectionEditResult exposes the writer's immutable offset plan and its content
// range. Status distinguishes a preview from a committed file replacement.
type SectionEditResult struct {
	File   string `json:"file"`
	Status string `json:"status"`
	sectionedit.Prepared
}

// InspectSections reads the POC's structural section surface. Context retrieval
// remains the existing file-scoped context/voice surface, and check findings use
// the same native block IDs present in each range.
func (a *App) InspectSections(ctx context.Context, file string) (sectionedit.Document, error) {
	if err := ctx.Err(); err != nil {
		return sectionedit.Document{}, err
	}
	_, data, name, err := readSectionSource(file)
	if err != nil {
		return sectionedit.Document{}, err
	}
	doc, err := sectionedit.Inspect(ctx, name, data)
	doc.File = file
	return doc, err
}

func (a *App) RunInspectSections(cmd Command, file, outFormat string) error {
	doc, err := a.InspectSections(cmd.Context(), file)
	if err != nil {
		return err
	}
	if outFormat == "yaml" {
		// JSON field names are the public surface for both serializers.
		data, err := json.Marshal(doc)
		if err != nil {
			return err
		}
		var value any
		if err := json.Unmarshal(data, &value); err != nil {
			return err
		}
		return yamlv3.NewEncoder(cmd.OutOrStdout()).Encode(value)
	}
	encoder := json.NewEncoder(cmd.OutOrStdout())
	if outFormat != "jsonl" {
		encoder.SetIndent("", "  ")
	}
	return encoder.Encode(doc)
}

func readSectionSource(file string) (os.FileInfo, []byte, string, error) {
	name, err := sectionedit.FormatForFile(file)
	if err != nil {
		return nil, nil, "", err
	}
	info, err := os.Lstat(file)
	if err != nil {
		return nil, nil, "", err
	}
	if !info.Mode().IsRegular() {
		return nil, nil, "", errors.New("section editing requires a regular local file, without a symlink")
	}
	data, err := os.ReadFile(file)
	return info, data, name, err
}

// ApplySectionEdit prepares a block-range replacement as offsets, then either
// returns a preview or atomically replaces the file with the writer's output.
func (a *App) ApplySectionEdit(ctx context.Context, file string, edit sectionedit.Edit, preview bool, backup string) (SectionEditResult, error) {
	if err := ctx.Err(); err != nil {
		return SectionEditResult{}, err
	}
	info, original, name, err := readSectionSource(file)
	if err != nil {
		return SectionEditResult{}, err
	}
	prepared, err := sectionedit.Prepare(ctx, name, original, edit)
	if err != nil {
		return SectionEditResult{}, err
	}
	result := SectionEditResult{File: file, Status: "preview", Prepared: prepared}
	if preview {
		return result, nil
	}
	if err := commitSectionFile(ctx, sectionCommit{
		file: file, info: info, original: original, prepared: prepared, backup: backup,
	}); err != nil {
		return SectionEditResult{}, err
	}
	result.Status = "applied"
	return result, nil
}

type sectionCommit struct {
	file     string
	info     os.FileInfo
	original []byte
	prepared sectionedit.Prepared
	backup   string
}

func commitSectionFile(ctx context.Context, change sectionCommit) error {
	tmp, err := os.CreateTemp(filepath.Dir(change.file), ".kapi-section-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()
	if err := tmp.Chmod(change.info.Mode().Perm()); err != nil {
		return err
	}
	if _, err := tmp.Write(change.prepared.Data); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	currentInfo, current, _, err := readSectionSource(change.file)
	if err != nil {
		return err
	}
	if !os.SameFile(change.info, currentInfo) || sectionedit.Snapshot(current) != change.prepared.Plan.Snapshot {
		return sectionedit.ErrStale
	}
	if change.backup != "" {
		backup, err := os.OpenFile(change.file+change.backup, os.O_WRONLY|os.O_CREATE|os.O_EXCL, change.info.Mode().Perm())
		if err != nil {
			return fmt.Errorf("create section backup: %w", err)
		}
		_, writeErr := backup.Write(change.original)
		closeErr := backup.Close()
		if writeErr != nil {
			return writeErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return os.Rename(tmp.Name(), change.file)
}

func validateSectionChangeSet(entries []changeEntry) error {
	for _, entry := range entries {
		if entry.Kind != kindSection {
			continue
		}
		if len(entries) != 1 {
			return errors.New("the section-edit POC accepts one section entry per change-set; re-inspect after applying it")
		}
		if entry.File == "" || entry.ID == "" || entry.Snapshot == "" {
			return errors.New("section edits require file, id and snapshot from inspect --sections")
		}
		if entry.Replacement != "" || entry.ContentHash != "" || entry.Op != "" {
			return errors.New("section edits use text for the Markdown body and snapshot for drift protection")
		}
	}
	return nil
}

func (a *App) runSectionApply(cmd Command, entry changeEntry, preview bool, backup string, asJSON bool) error {
	if a.FormatFlag != "" {
		return errors.New("section edits detect the supported format from the file extension; omit --format")
	}
	result, err := a.ApplySectionEdit(cmd.Context(), entry.File, sectionedit.Edit{
		ID: entry.ID, Snapshot: entry.Snapshot, Text: entry.Text,
	}, preview, backup)
	if err != nil {
		return err
	}
	if asJSON {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	}
	if !preview {
		_, err := fmt.Fprintf(cmd.ErrOrStderr(), "section %s: applied; run kapi check on %s\n", entry.ID, entry.File)
		return err
	}
	diff, err := difflib.GetUnifiedDiffString(difflib.UnifiedDiff{
		A: difflib.SplitLines(result.Before.Content), B: difflib.SplitLines(result.After.Content),
		FromFile: entry.File + ":" + entry.ID + " (before)", ToFile: entry.File + ":" + entry.ID + " (after)", Context: 3,
	})
	if err != nil {
		return err
	}
	_, err = fmt.Fprint(cmd.OutOrStdout(), diff)
	return err
}
