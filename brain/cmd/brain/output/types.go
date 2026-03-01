package output

import (
	"fmt"
	"io"
	"time"
)

// AddEntry represents a single pattern added by brain add.
type AddEntry struct {
	Pattern string `json:"pattern"`
	Format  string `json:"format,omitempty"`
	Files   int    `json:"files"`
	Skipped bool   `json:"skipped,omitempty"`
}

// AddOutput represents the result of brain add.
type AddOutput struct {
	Added []AddEntry `json:"added"`
}

func (o AddOutput) FormatText(w io.Writer) error {
	for _, e := range o.Added {
		if e.Skipped {
			fmt.Fprintf(w, "Already tracked: %s\n", e.Pattern)
			continue
		}
		if e.Format != "" {
			fmt.Fprintf(w, "Added %s (%s) — %d file(s)\n", e.Pattern, e.Format, e.Files)
		} else {
			fmt.Fprintf(w, "Added %s — %d file(s)\n", e.Pattern, e.Files)
		}
	}
	return nil
}

// RmEntry represents a single pattern processed by brain rm.
type RmEntry struct {
	Pattern string `json:"pattern"`
	Action  string `json:"action"`           // "removed", "excluded", "already_excluded"
	Format  string `json:"format,omitempty"` // only for "removed"
	Files   int    `json:"files,omitempty"`  // only for "excluded"
}

// RmOutput represents the result of brain rm.
type RmOutput struct {
	Entries []RmEntry `json:"entries"`
}

func (o RmOutput) FormatText(w io.Writer) error {
	for _, e := range o.Entries {
		switch e.Action {
		case "removed":
			if e.Format != "" {
				fmt.Fprintf(w, "Removed %s (was: %s)\n", e.Pattern, e.Format)
			} else {
				fmt.Fprintf(w, "Removed %s\n", e.Pattern)
			}
		case "excluded":
			fmt.Fprintf(w, "Excluded %s — %d file(s) now excluded\n", e.Pattern, e.Files)
		case "already_excluded":
			fmt.Fprintf(w, "Already excluded: %s\n", e.Pattern)
		}
	}
	return nil
}

// StatusOutput represents sync status.
type StatusOutput struct {
	Project     ProjectInfo `json:"project"`
	ItemCount   int         `json:"item_count"`
	FileCount   int         `json:"file_count"`
	WordCount   int         `json:"word_count"`
	PendingPush int         `json:"pending_push"`
	PendingPull int         `json:"pending_pull"`
	LastSync    *time.Time  `json:"last_sync,omitempty"`
	Errors      []string    `json:"errors,omitempty"`
	UpToDate    bool        `json:"up_to_date"`
}

type ProjectInfo struct {
	Root      string `json:"root"`
	ConfigDir string `json:"config_dir"`
	Server    string `json:"server,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
}

func (s StatusOutput) FormatText(w io.Writer) error {
	fmt.Fprintf(w, "Project root: %s\n", s.Project.Root)
	fmt.Fprintf(w, "Config:       %s\n", s.Project.ConfigDir)

	if s.Project.Server == "" {
		fmt.Fprintln(w, "\nNot connected to a server.")
		fmt.Fprintln(w, "  Run 'brain init' with a server or set one in .bowrain/config.yaml")
		return nil
	}

	fmt.Fprintf(w, "\nLocal: %d files, %d blocks (%d words)\n", s.FileCount, s.ItemCount, s.WordCount)

	if s.PendingPush > 0 {
		fmt.Fprintf(w, "Pending push: %d blocks changed locally\n", s.PendingPush)
	}
	if s.PendingPull > 0 {
		fmt.Fprintf(w, "Pending pull: %d remote changes available\n", s.PendingPull)
	} else if s.PendingPull < 0 {
		fmt.Fprintln(w, "Pending pull: remote changes available")
	}
	if s.UpToDate {
		fmt.Fprintln(w, "Up to date.")
	}

	if s.LastSync != nil && !s.LastSync.IsZero() {
		fmt.Fprintf(w, "Last sync:    %s\n", s.LastSync.Format("2006-01-02 15:04:05 UTC"))
	}

	if len(s.Errors) > 0 {
		fmt.Fprintln(w, "\nErrors:")
		for _, e := range s.Errors {
			fmt.Fprintf(w, "  - %s\n", e)
		}
	}

	return nil
}

// AuthStatusOutput represents auth status.
type AuthStatusOutput struct {
	LoggedIn  bool       `json:"logged_in"`
	Server    string     `json:"server,omitempty"`
	User      string     `json:"user,omitempty"`
	UserID    string     `json:"user_id,omitempty"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

func (a AuthStatusOutput) FormatText(w io.Writer) error {
	if !a.LoggedIn {
		fmt.Fprintln(w, "Not logged in.")
		return nil
	}

	fmt.Fprintf(w, "Server: %s\n", a.Server)
	fmt.Fprintf(w, "User:   %s", a.User)
	if a.UserID != "" {
		fmt.Fprintf(w, " (ID: %s)", a.UserID)
	}
	fmt.Fprintln(w)
	if a.ExpiresAt != nil {
		fmt.Fprintf(w, "Token expires: %s\n", a.ExpiresAt.Format("2006-01-02 15:04:05"))
	}
	return nil
}

// InitOutput represents the result of brain init.
type InitOutput struct {
	Root       string `json:"root"`
	ConfigDir  string `json:"config_dir"`
	ProjectID  string `json:"project_id,omitempty"`
	Server     string `json:"server,omitempty"`
	Workspace  string `json:"workspace,omitempty"`
	ClaimToken string `json:"claim_token,omitempty"`
	ClaimURL   string `json:"claim_url,omitempty"`
	ClaimEmail string `json:"claim_email,omitempty"`
}

func (o InitOutput) FormatText(w io.Writer) error {
	fmt.Fprintf(w, "Initialized .bowrain/ project in: %s\n", o.Root)
	fmt.Fprintf(w, "Configuration: %s\n", o.ConfigDir)

	if o.ProjectID != "" {
		fmt.Fprintf(w, "\nProject created: %s\n", o.ProjectID)
	}
	if o.Workspace != "" {
		fmt.Fprintf(w, "Workspace: %s\n", o.Workspace)
	}
	if o.ClaimEmail != "" {
		fmt.Fprintf(w, "A claim link has been sent to %s\n", o.ClaimEmail)
	} else if o.ClaimURL != "" {
		fmt.Fprintf(w, "Claim URL: %s\n", o.ClaimURL)
	}

	fmt.Fprintln(w)
	if o.Server != "" {
		fmt.Fprintln(w, "Next steps:")
		fmt.Fprintln(w, "  1. Run: brain push    — upload content to the server")
		if o.ClaimEmail != "" {
			fmt.Fprintln(w, "  2. Check your email for the claim link to take ownership")
			fmt.Fprintln(w, "  3. Invite translators from the web dashboard")
		} else if o.ClaimURL != "" {
			fmt.Fprintln(w, "  2. Open the claim URL to take ownership of the project")
			fmt.Fprintln(w, "  3. Invite translators from the web dashboard")
		} else {
			fmt.Fprintln(w, "  2. Invite translators from the web dashboard")
		}
	} else {
		fmt.Fprintln(w, "Next steps:")
		fmt.Fprintln(w, "  1. Edit .bowrain/config.yaml to configure your project")
		fmt.Fprintln(w, "  2. Add file mappings to sync with Bowrain Server")
		fmt.Fprintln(w, "  3. Run: brain auth login")
		fmt.Fprintln(w, "  4. Run: brain pull to sync translations")
	}

	return nil
}

// LsEntry represents a single file in the ls output.
type LsEntry struct {
	Path   string `json:"path"`
	Format string `json:"format"`
	Blocks int    `json:"blocks,omitempty"`
	Words  int    `json:"words,omitempty"`
	Dirty  int    `json:"dirty,omitempty"`
}

// LsOutput represents the result of brain ls.
type LsOutput struct {
	Files    []LsEntry `json:"files"`
	Total    int       `json:"total"`
	Blocks   int       `json:"blocks,omitempty"`
	Words    int       `json:"words,omitempty"`
	Changed  int       `json:"changed,omitempty"`
	HasStats bool      `json:"-"`
	HasDirty bool      `json:"-"`
}

func (o LsOutput) FormatText(w io.Writer) error {
	if len(o.Files) == 0 {
		fmt.Fprintln(w, "No tracked files.")
		return nil
	}

	if o.HasStats || o.HasDirty {
		pathW := 4 // "PATH"
		fmtW := 6  // "FORMAT"
		for _, f := range o.Files {
			if len(f.Path) > pathW {
				pathW = len(f.Path)
			}
			if len(f.Format) > fmtW {
				fmtW = len(f.Format)
			}
		}
		pathW += 2
		fmtW += 2

		header := fmt.Sprintf("  %-*s %-*s %8s %8s", pathW, "PATH", fmtW, "FORMAT", "BLOCKS", "WORDS")
		separator := fmt.Sprintf("  %-*s %-*s %8s %8s", pathW, "----", fmtW, "------", "------", "-----")
		if o.HasDirty {
			header += fmt.Sprintf(" %8s", "DIRTY")
			separator += fmt.Sprintf(" %8s", "-----")
		}
		fmt.Fprintln(w, header)
		fmt.Fprintln(w, separator)

		for _, f := range o.Files {
			line := fmt.Sprintf("  %-*s %-*s %8d %8d", pathW, f.Path, fmtW, f.Format, f.Blocks, f.Words)
			if o.HasDirty {
				line += fmt.Sprintf(" %8d", f.Dirty)
			}
			fmt.Fprintln(w, line)
		}

		summary := fmt.Sprintf("\n%d file(s), %d blocks, %d words", o.Total, o.Blocks, o.Words)
		if o.HasDirty {
			summary += fmt.Sprintf(", %d changed", o.Changed)
		}
		fmt.Fprintln(w, summary)
	} else {
		pathW := 4
		for _, f := range o.Files {
			if len(f.Path) > pathW {
				pathW = len(f.Path)
			}
		}
		pathW += 2

		for _, f := range o.Files {
			fmt.Fprintf(w, "%-*s %s\n", pathW, f.Path, f.Format)
		}
		fmt.Fprintf(w, "\n%d file(s)\n", o.Total)
	}
	return nil
}

// PullOutput represents the result of brain pull.
type PullOutput struct {
	BlocksPulled int  `json:"blocks_pulled"`
	LocalesCount int  `json:"locales_count"`
	FilesWritten int  `json:"files_written,omitempty"`
	DryRun       bool `json:"dry_run,omitempty"`
	UpToDate     bool `json:"up_to_date,omitempty"`
}

func (o PullOutput) FormatText(w io.Writer) error {
	if o.DryRun {
		fmt.Fprintf(w, "Would pull %d blocks for %d locales\n", o.BlocksPulled, o.LocalesCount)
		return nil
	}
	if o.UpToDate {
		fmt.Fprintln(w, "Already up to date.")
		return nil
	}
	fmt.Fprintf(w, "Pulled %d blocks for %d locales\n", o.BlocksPulled, o.LocalesCount)
	if o.FilesWritten > 0 {
		fmt.Fprintf(w, "Updated %d file(s)\n", o.FilesWritten)
	}
	return nil
}

// PushOutput represents the result of brain push.
type PushOutput struct {
	BlocksPushed int  `json:"blocks_pushed"`
	WordCount    int  `json:"word_count"`
	FilesScanned int  `json:"files_scanned"`
	DryRun       bool `json:"dry_run,omitempty"`
	UpToDate     bool `json:"up_to_date,omitempty"`
}

func (o PushOutput) FormatText(w io.Writer) error {
	if o.DryRun {
		fmt.Fprintf(w, "Would push %d blocks, %d words (scanned %d files)\n", o.BlocksPushed, o.WordCount, o.FilesScanned)
		return nil
	}
	if o.UpToDate {
		fmt.Fprintln(w, "Already up to date.")
		return nil
	}
	fmt.Fprintf(w, "Pushed %d blocks, %d words (scanned %d files)\n", o.BlocksPushed, o.WordCount, o.FilesScanned)
	return nil
}

// ConfigOutput represents the result of brain config.
type ConfigOutput struct {
	Path   string `json:"path,omitempty"`
	Key    string `json:"key,omitempty"`
	Value  string `json:"value,omitempty"`
	Action string `json:"action,omitempty"` // "path", "get", "set"
}

func (o ConfigOutput) FormatText(w io.Writer) error {
	switch o.Action {
	case "set":
		fmt.Fprintf(w, "Set %s = %s in %s\n", o.Key, o.Value, o.Path)
	case "get":
		fmt.Fprintln(w, o.Value)
	case "path":
		fmt.Fprintln(w, o.Path)
	}
	return nil
}

// AuthLoginOutput represents the result of brain auth login.
type AuthLoginOutput struct {
	Server string `json:"server"`
	User   string `json:"user,omitempty"`
}

func (o AuthLoginOutput) FormatText(w io.Writer) error {
	if o.User != "" {
		fmt.Fprintf(w, "Logged in as %s\n", o.User)
	} else {
		fmt.Fprintln(w, "Login successful! Token saved.")
	}
	return nil
}

// AuthLogoutOutput represents the result of brain auth logout.
type AuthLogoutOutput struct {
	WasLoggedIn bool `json:"was_logged_in"`
}

func (o AuthLogoutOutput) FormatText(w io.Writer) error {
	if o.WasLoggedIn {
		fmt.Fprintln(w, "Logged out. Token removed.")
	} else {
		fmt.Fprintln(w, "No stored token found.")
	}
	return nil
}

// AuthClaimOutput represents the result of brain auth claim.
type AuthClaimOutput struct {
	ProjectID     string `json:"project_id"`
	WorkspaceSlug string `json:"workspace_slug"`
}

func (o AuthClaimOutput) FormatText(w io.Writer) error {
	fmt.Fprintf(w, "Project claimed into workspace %s\n", o.WorkspaceSlug)
	fmt.Fprintf(w, "Project ID: %s\n", o.ProjectID)
	return nil
}

// PresetsValidateOutput represents the result of preset validation.
type PresetsValidateOutput struct {
	Valid  bool     `json:"valid"`
	Errors []string `json:"errors,omitempty"`
}

func (p PresetsValidateOutput) FormatText(w io.Writer) error {
	if p.Valid {
		fmt.Fprintln(w, "All presets and overrides are valid.")
		return nil
	}
	fmt.Fprintf(w, "Found %d validation error(s):\n", len(p.Errors))
	for _, e := range p.Errors {
		fmt.Fprintf(w, "  - %s\n", e)
	}
	return nil
}
