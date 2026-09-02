package backend

import (
	"bytes"
	"context"
	"fmt"
	"time"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/host"
)

// writtenBackTimeout bounds one write-back: it reads a source file, applies the
// stored targets and serializes the result, all of which is local work on one
// document.
const writtenBackTimeout = 60 * time.Second

// WrittenBackFile returns one of the open project's content files as its format
// writer emits it for a locale, with the targets the project store holds applied
// to it. An empty locale returns the source file through the same round trip,
// which is what a reader sees when no locale is in view.
//
// It is the text behind the preview's File view: a reader looking at a catalog's
// keys can ask what the file itself will look like, marked at the unit in focus.
// The bytes come from the same reader, writer and recipe configuration `kapi
// merge` materializes with, so what the preview shows is what the next merge
// writes.
func (a *App) WrittenBackFile(tabID, filePath, locale string) (string, error) {
	op := a.getOpenProject(tabID)
	if op == nil {
		return "", fmt.Errorf("tab %q not found", tabID)
	}
	if op.Project == nil {
		return "", fmt.Errorf("tab %q has no project", tabID)
	}

	ctx, cancel := context.WithTimeout(context.Background(), writtenBackTimeout)
	defer cancel()

	var buf bytes.Buffer
	engine := a.borrowEngine(&host.App{FormatReg: a.formatReg, ToolReg: a.toolReg})
	err := engine.WriteBack(ctx, host.WriteBackOptions{
		Project:     op.Project,
		ProjectPath: op.Path,
		SourcePath:  inspectAbsPath(op, filePath),
		Locale:      model.LocaleID(locale),
	}, &buf)
	if err != nil {
		return "", err
	}
	return buf.String(), nil
}
