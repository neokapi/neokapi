package backend

import (
	"context"
	"time"
)

// The first meeting between a checkout carrying context files and a store that
// has never held this project's context.
//
// A project's context lives in the workspace. A checkout that arrived carrying
// `.kapi/terms.json`, `.kapi/voice.yaml`, a content-memory bundle or a decision
// record holds a copy of somebody's, and kapi names those files and the command
// that reads them rather than reading them itself. The app shows the same
// sentence the terminal does, on the home screen and on the project it is about.

// contextNoticeTimeout caps one first-meeting check. It stats a handful of
// paths and asks the project's store one question, and a screen is better drawn
// without the notice than held waiting for it.
const contextNoticeTimeout = 5 * time.Second

// ContextFilesNoticeDTO names the context files a checkout holds and the
// command that reads them into the project's store.
type ContextFilesNoticeDTO struct {
	// Files are project-relative and sorted.
	Files []string `json:"files"`
	// Command is what reads them.
	Command string `json:"command"`
	// Message is the line a surface shows, rendered by host so the app and the
	// terminal say the same thing.
	Message string `json:"message"`
}

// contextFilesNotice reports a project whose checkout carries context files its
// store has never held, or nil when there is nothing to say.
//
// projectPath is the recipe or the project root. Every failure answers nil: the
// notice is something the app offers a reader, and a project that cannot be
// asked has nothing to offer them.
func (a *App) contextFilesNotice(projectPath string) *ContextFilesNoticeDTO {
	if projectPath == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), contextNoticeTimeout)
	defer cancel()

	notice, unread := a.hostEngine().ContextFilesUnread(ctx, projectPath)
	if !unread {
		return nil
	}
	return &ContextFilesNoticeDTO{
		Files:   notice.Files,
		Command: notice.Command,
		Message: notice.Message(),
	}
}
