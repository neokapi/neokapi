package output

import (
	"fmt"
	"io"
	"strings"
	"time"
)

// StatusOutput represents sync status.
type StatusOutput struct {
	Project     ProjectInfo `json:"project"`
	Stream      string      `json:"stream,omitempty"`
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
	ConfigDir string `json:"config_dir"` // path to the project's .kapi recipe file
	Server    string `json:"server,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
}

func (s StatusOutput) FormatText(w io.Writer) error {
	fmt.Fprintf(w, "Project root: %s\n", s.Project.Root)
	fmt.Fprintf(w, "Recipe:       %s\n", s.Project.ConfigDir)
	if s.Stream != "" && s.Stream != "main" {
		fmt.Fprintf(w, "Stream:       %s\n", s.Stream)
	}

	if s.Project.Server == "" {
		fmt.Fprintln(w, "\nNot connected to a server.")
		fmt.Fprintln(w, "  Run 'kapi init' with a server, or add a `server:` block to the recipe.")
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

// InitOutput represents the result of kapi init.
type InitOutput struct {
	Root       string `json:"root"`
	ConfigDir  string `json:"config_dir"` // path to the new .kapi recipe file
	ProjectID  string `json:"project_id,omitempty"`
	Server     string `json:"server,omitempty"`
	Workspace  string `json:"workspace,omitempty"`
	ClaimToken string `json:"claim_token,omitempty"`
	ClaimURL   string `json:"claim_url,omitempty"`
	ClaimEmail string `json:"claim_email,omitempty"`
}

func (o InitOutput) FormatText(w io.Writer) error {
	fmt.Fprintf(w, "Initialized kapi project in: %s\n", o.Root)
	fmt.Fprintf(w, "Recipe: %s\n", o.ConfigDir)

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
		fmt.Fprintln(w, "  1. Run: kapi push    — upload content to the server")
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
		fmt.Fprintln(w, "  1. Edit the .kapi recipe to configure your project")
		fmt.Fprintln(w, "  2. Add file mappings to sync with Bowrain Server")
		fmt.Fprintln(w, "  3. Run: kapi auth login")
		fmt.Fprintln(w, "  4. Run: kapi pull to sync translations")
	}

	return nil
}

// PullOutput represents the result of kapi pull.
type PullOutput struct {
	BlocksPulled int    `json:"blocks_pulled"`
	LocalesCount int    `json:"locales_count"`
	FilesWritten int    `json:"files_written,omitempty"`
	Stream       string `json:"stream,omitempty"`
	DryRun       bool   `json:"dry_run,omitempty"`
	UpToDate     bool   `json:"up_to_date,omitempty"`
}

func (o PullOutput) FormatText(w io.Writer) error {
	if o.Stream != "" && o.Stream != "main" {
		fmt.Fprintf(w, "Stream: %s\n", o.Stream)
	}
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

// PushOutput represents the result of kapi push.
type PushOutput struct {
	BlocksPushed int    `json:"blocks_pushed"`
	WordCount    int    `json:"word_count"`
	FilesScanned int    `json:"files_scanned"`
	Stream       string `json:"stream,omitempty"`
	DryRun       bool   `json:"dry_run,omitempty"`
	UpToDate     bool   `json:"up_to_date,omitempty"`
}

func (o PushOutput) FormatText(w io.Writer) error {
	if o.Stream != "" && o.Stream != "main" {
		fmt.Fprintf(w, "Stream: %s\n", o.Stream)
	}
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

// ConfigOutput represents the result of kapi config.
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

// AuthLoginOutput represents the result of kapi auth login.
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

// AuthLogoutOutput represents the result of kapi auth logout.
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

// AuthClaimOutput represents the result of kapi auth claim.
type AuthClaimOutput struct {
	ProjectID     string `json:"project_id"`
	WorkspaceSlug string `json:"workspace_slug"`
}

func (o AuthClaimOutput) FormatText(w io.Writer) error {
	fmt.Fprintf(w, "Project claimed into workspace %s\n", o.WorkspaceSlug)
	fmt.Fprintf(w, "Project ID: %s\n", o.ProjectID)
	return nil
}

// TokenCreateOutput represents the result of kapi auth token create.
type TokenCreateOutput struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Token       string     `json:"token"`
	TokenPrefix string     `json:"token_prefix"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
}

func (o TokenCreateOutput) FormatText(w io.Writer) error {
	fmt.Fprintf(w, "Token created: %s\n", o.Name)
	fmt.Fprintf(w, "ID:     %s\n", o.ID)
	fmt.Fprintf(w, "Prefix: %s\n", o.TokenPrefix)
	if o.ExpiresAt != nil {
		fmt.Fprintf(w, "Expires: %s\n", o.ExpiresAt.Format("2006-01-02 15:04:05"))
	}
	fmt.Fprintf(w, "\n  %s\n\n", o.Token)
	fmt.Fprintln(w, "Save this token — it will not be shown again.")
	return nil
}

// TokenListOutput represents the result of kapi auth token list.
type TokenListOutput struct {
	Tokens []TokenEntry `json:"tokens"`
}

// TokenEntry represents a single API token in list output.
type TokenEntry struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	TokenPrefix string     `json:"token_prefix"`
	LastUsedAt  *time.Time `json:"last_used_at,omitempty"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

func (o TokenListOutput) FormatText(w io.Writer) error {
	if len(o.Tokens) == 0 {
		fmt.Fprintln(w, "No API tokens.")
		return nil
	}

	nameW := 4 // "NAME"
	for _, t := range o.Tokens {
		if len(t.Name) > nameW {
			nameW = len(t.Name)
		}
	}
	nameW += 2

	fmt.Fprintf(w, "  %-*s %-10s %-20s %-20s %s\n", nameW, "NAME", "PREFIX", "LAST USED", "EXPIRES", "ID")
	fmt.Fprintf(w, "  %-*s %-10s %-20s %-20s %s\n", nameW, "----", "------", "---------", "-------", "--")

	for _, t := range o.Tokens {
		lastUsed := "never"
		if t.LastUsedAt != nil {
			lastUsed = t.LastUsedAt.Format("2006-01-02 15:04")
		}
		expires := "never"
		if t.ExpiresAt != nil {
			expires = t.ExpiresAt.Format("2006-01-02 15:04")
		}
		fmt.Fprintf(w, "  %-*s %-10s %-20s %-20s %s\n", nameW, t.Name, t.TokenPrefix, lastUsed, expires, t.ID)
	}

	fmt.Fprintf(w, "\n%d token(s)\n", len(o.Tokens))
	return nil
}

// TokenDeleteOutput represents the result of kapi auth token delete.
type TokenDeleteOutput struct {
	ID string `json:"id"`
}

func (o TokenDeleteOutput) FormatText(w io.Writer) error {
	fmt.Fprintf(w, "Token %s deleted.\n", o.ID)
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

// ---------------------------------------------------------------------------
// Stream types
// ---------------------------------------------------------------------------

// StreamEntry represents a single stream in list output.
type StreamEntry struct {
	Name        string `json:"name"`
	Parent      string `json:"parent,omitempty"`
	Visibility  string `json:"visibility"`
	Description string `json:"description,omitempty"`
	Archived    bool   `json:"archived,omitempty"`
	Active      bool   `json:"active,omitempty"`
}

// StreamListOutput represents the result of kapi stream list.
type StreamListOutput struct {
	Streams []StreamEntry `json:"streams"`
}

func (o StreamListOutput) FormatText(w io.Writer) error {
	if len(o.Streams) == 0 {
		fmt.Fprintln(w, "No streams.")
		return nil
	}

	nameW := 4
	for _, s := range o.Streams {
		if len(s.Name) > nameW {
			nameW = len(s.Name)
		}
	}
	nameW += 2

	fmt.Fprintf(w, "  %-*s %-10s %-10s %s\n", nameW, "NAME", "PARENT", "VISIBILITY", "DESCRIPTION")
	fmt.Fprintf(w, "  %-*s %-10s %-10s %s\n", nameW, "----", "------", "----------", "-----------")

	for _, s := range o.Streams {
		marker := "  "
		if s.Active {
			marker = "* "
		}
		archived := ""
		if s.Archived {
			archived = " (archived)"
		}
		parent := s.Parent
		if parent == "" {
			parent = "-"
		}
		fmt.Fprintf(w, "%s%-*s %-10s %-10s %s%s\n", marker, nameW, s.Name, parent, s.Visibility, s.Description, archived)
	}

	fmt.Fprintf(w, "\n%d stream(s)\n", len(o.Streams))
	return nil
}

// StreamCreateOutput represents the result of kapi stream create.
type StreamCreateOutput struct {
	Name        string `json:"name"`
	Parent      string `json:"parent"`
	Visibility  string `json:"visibility"`
	Description string `json:"description,omitempty"`
}

func (o StreamCreateOutput) FormatText(w io.Writer) error {
	fmt.Fprintf(w, "Created stream %q (parent: %s, visibility: %s)\n", o.Name, o.Parent, o.Visibility)
	if o.Description != "" {
		fmt.Fprintf(w, "Description: %s\n", o.Description)
	}
	fmt.Fprintf(w, "\nSwitch to it with: kapi push --stream %s\n", o.Name)
	return nil
}

// DiffChangeEntry represents a single block change in a diff.
type DiffChangeEntry struct {
	BlockID    string `json:"block_id"`
	ChangeType string `json:"change_type"`
}

// StreamDiffOutput represents the result of kapi stream diff.
type StreamDiffOutput struct {
	Stream  string            `json:"stream"`
	Parent  string            `json:"parent"`
	Changes []DiffChangeEntry `json:"changes"`
}

func (o StreamDiffOutput) FormatText(w io.Writer) error {
	fmt.Fprintf(w, "Stream: %s (parent: %s)\n\n", o.Stream, o.Parent)

	if len(o.Changes) == 0 {
		fmt.Fprintln(w, "No changes.")
		return nil
	}

	added, modified, removed := 0, 0, 0
	for _, c := range o.Changes {
		switch c.ChangeType {
		case "added":
			added++
		case "modified":
			modified++
		case "removed":
			removed++
		}
	}

	fmt.Fprintf(w, "%d change(s):", len(o.Changes))
	if added > 0 {
		fmt.Fprintf(w, " %d added", added)
	}
	if modified > 0 {
		fmt.Fprintf(w, " %d modified", modified)
	}
	if removed > 0 {
		fmt.Fprintf(w, " %d removed", removed)
	}
	fmt.Fprintln(w)

	for _, c := range o.Changes {
		fmt.Fprintf(w, "  %s  %s\n", c.ChangeType, c.BlockID)
	}
	return nil
}

// StreamMergeOutput represents the result of kapi stream merge.
type StreamMergeOutput struct {
	Stream         string `json:"stream"`
	MergedBlocks   int    `json:"merged_blocks"`
	AddedBlocks    int    `json:"added_blocks"`
	ModifiedBlocks int    `json:"modified_blocks"`
	RemovedBlocks  int    `json:"removed_blocks"`
	DryRun         bool   `json:"dry_run,omitempty"`
}

func (o StreamMergeOutput) FormatText(w io.Writer) error {
	if o.DryRun {
		fmt.Fprintf(w, "Dry run — would merge %d blocks from %q:\n", o.MergedBlocks, o.Stream)
	} else {
		fmt.Fprintf(w, "Merged %d blocks from %q:\n", o.MergedBlocks, o.Stream)
	}
	fmt.Fprintf(w, "  Added:    %d\n", o.AddedBlocks)
	fmt.Fprintf(w, "  Modified: %d\n", o.ModifiedBlocks)
	fmt.Fprintf(w, "  Removed:  %d\n", o.RemovedBlocks)
	return nil
}

// StreamArchiveOutput represents the result of kapi stream archive.
type StreamArchiveOutput struct {
	Stream string `json:"stream"`
}

func (o StreamArchiveOutput) FormatText(w io.Writer) error {
	fmt.Fprintf(w, "Archived stream %q\n", o.Stream)
	return nil
}

// StreamStatusOutput represents the result of kapi stream status.
type StreamStatusOutput struct {
	Stream      string `json:"stream"`
	Parent      string `json:"parent,omitempty"`
	Visibility  string `json:"visibility,omitempty"`
	Description string `json:"description,omitempty"`
	Archived    bool   `json:"archived,omitempty"`
	Active      bool   `json:"active"`
	Exists      bool   `json:"exists"`
	// Ahead counts blocks changed on this stream relative to its parent
	// (i.e. the diff against parent). -1 when unknown.
	Ahead int `json:"ahead"`
	// Behind counts remote changes available to pull into the local checkout.
	// -1 when unknown but more are available.
	Behind int `json:"behind"`
	// PendingPush counts blocks changed locally but not yet pushed.
	PendingPush int `json:"pending_push"`
	// AddedVsParent / ModifiedVsParent / RemovedVsParent break down Ahead.
	AddedVsParent    int `json:"added_vs_parent"`
	ModifiedVsParent int `json:"modified_vs_parent"`
	RemovedVsParent  int `json:"removed_vs_parent"`
}

func (o StreamStatusOutput) FormatText(w io.Writer) error {
	fmt.Fprintf(w, "Stream: %s", o.Stream)
	if o.Active {
		fmt.Fprint(w, " (active)")
	}
	fmt.Fprintln(w)

	if !o.Exists {
		if o.Stream == "main" {
			fmt.Fprintln(w, "State:  main (base stream)")
		} else {
			fmt.Fprintln(w, "State:  not created on the server yet")
			fmt.Fprintf(w, "  Create it with: kapi stream create %s\n", o.Stream)
			return nil
		}
	} else {
		if o.Parent != "" {
			fmt.Fprintf(w, "Parent: %s\n", o.Parent)
		}
		state := "active"
		if o.Archived {
			state = "archived"
		}
		fmt.Fprintf(w, "State:  %s\n", state)
		if o.Visibility != "" {
			fmt.Fprintf(w, "Visibility: %s\n", o.Visibility)
		}
		if o.Description != "" {
			fmt.Fprintf(w, "Description: %s\n", o.Description)
		}
	}

	if o.Parent != "" {
		switch {
		case o.Ahead < 0:
			fmt.Fprintf(w, "Ahead of %s: changes present\n", o.Parent)
		case o.Ahead > 0:
			fmt.Fprintf(w, "Ahead of %s: %d block(s)", o.Parent, o.Ahead)
			parts := []string{}
			if o.AddedVsParent > 0 {
				parts = append(parts, fmt.Sprintf("%d added", o.AddedVsParent))
			}
			if o.ModifiedVsParent > 0 {
				parts = append(parts, fmt.Sprintf("%d modified", o.ModifiedVsParent))
			}
			if o.RemovedVsParent > 0 {
				parts = append(parts, fmt.Sprintf("%d removed", o.RemovedVsParent))
			}
			if len(parts) > 0 {
				fmt.Fprintf(w, " (%s)", joinComma(parts))
			}
			fmt.Fprintln(w)
		default:
			fmt.Fprintf(w, "Ahead of %s: up to date\n", o.Parent)
		}
	}

	if o.Behind < 0 {
		fmt.Fprintln(w, "Behind: remote changes available")
	} else if o.Behind > 0 {
		fmt.Fprintf(w, "Behind: %d remote change(s) to pull\n", o.Behind)
	}

	if o.PendingPush > 0 {
		fmt.Fprintf(w, "Pending push: %d block(s) changed locally\n", o.PendingPush)
	}

	return nil
}

// joinComma joins parts with ", ".
func joinComma(parts []string) string {
	return strings.Join(parts, ", ")
}

// ---------------------------------------------------------------------------
// Diff (local vs remote) types
// ---------------------------------------------------------------------------

// DiffBlockEntry is a single changed block in a diff.
type DiffBlockEntry struct {
	BlockID string `json:"block_id"`
	Name    string `json:"name,omitempty"`
	Preview string `json:"preview,omitempty"`
	Change  string `json:"change"` // "added", "changed", "removed"
}

// DiffFileEntry groups changed blocks for a single file.
type DiffFileEntry struct {
	Path    string           `json:"path"`
	Format  string           `json:"format,omitempty"`
	Added   int              `json:"added"`
	Changed int              `json:"changed"`
	Removed int              `json:"removed"`
	Blocks  []DiffBlockEntry `json:"blocks,omitempty"`
}

// DiffOutput represents the result of kapi diff.
type DiffOutput struct {
	Project     ProjectInfo     `json:"project"`
	Stream      string          `json:"stream,omitempty"`
	Files       []DiffFileEntry `json:"files"`
	Added       int             `json:"added"`
	Changed     int             `json:"changed"`
	Removed     int             `json:"removed"`
	PendingPull int             `json:"pending_pull"`
	// Verbose controls whether per-block detail is printed in text output.
	Verbose bool `json:"-"`
	// Connected reports whether a server is configured. Diff still computes
	// local-vs-cache deltas without a server, but reports that explicitly.
	Connected bool `json:"connected"`
}

func (o DiffOutput) FormatText(w io.Writer) error {
	if o.Stream != "" && o.Stream != "main" {
		fmt.Fprintf(w, "Stream: %s\n", o.Stream)
	}

	if len(o.Files) == 0 {
		if !o.Connected {
			fmt.Fprintln(w, "No local changes since the last sync.")
			fmt.Fprintln(w, "  (no server configured — comparing against the local sync cache)")
		} else if o.PendingPull != 0 {
			printPendingPull(w, o.PendingPull)
		} else {
			fmt.Fprintln(w, "No changes. Local and remote are in sync.")
		}
		return nil
	}

	// Per-file summary (git diff --stat style).
	pathW := 4
	for _, f := range o.Files {
		if len(f.Path) > pathW {
			pathW = len(f.Path)
		}
	}
	pathW += 2

	for _, f := range o.Files {
		stat := diffStat(f.Added, f.Changed, f.Removed)
		fmt.Fprintf(w, "  %-*s %s\n", pathW, f.Path, stat)
		if o.Verbose {
			for _, b := range f.Blocks {
				sigil := changeSigil(b.Change)
				label := b.Name
				if label == "" {
					label = b.BlockID
				}
				if b.Preview != "" {
					fmt.Fprintf(w, "      %s %s — %s\n", sigil, label, b.Preview)
				} else {
					fmt.Fprintf(w, "      %s %s\n", sigil, label)
				}
			}
		}
	}

	fmt.Fprintf(w, "\n%d file(s) changed: ", len(o.Files))
	fmt.Fprintln(w, diffStat(o.Added, o.Changed, o.Removed))

	if o.PendingPull != 0 {
		printPendingPull(w, o.PendingPull)
	}

	if !o.Verbose {
		fmt.Fprintln(w, "\nUse --verbose to see changed block ids/keys.")
	}
	return nil
}

func printPendingPull(w io.Writer, pending int) {
	if pending < 0 {
		fmt.Fprintln(w, "Remote: changes available to pull")
	} else if pending > 0 {
		fmt.Fprintf(w, "Remote: %d change(s) available to pull\n", pending)
	}
}

// diffStat renders a compact "+a ~c -r" style change summary.
func diffStat(added, changed, removed int) string {
	parts := []string{}
	if added > 0 {
		parts = append(parts, fmt.Sprintf("+%d", added))
	}
	if changed > 0 {
		parts = append(parts, fmt.Sprintf("~%d", changed))
	}
	if removed > 0 {
		parts = append(parts, fmt.Sprintf("-%d", removed))
	}
	if len(parts) == 0 {
		return "no changes"
	}
	return strings.Join(parts, " ")
}

// changeSigil maps a change type to a one-character sigil.
func changeSigil(change string) string {
	switch change {
	case "added":
		return "+"
	case "changed":
		return "~"
	case "removed":
		return "-"
	default:
		return "?"
	}
}

// WorkspaceItem is a single workspace in WorkspaceListOutput.
type WorkspaceItem struct {
	ID   string `json:"id,omitempty"`
	Name string `json:"name"`
	Slug string `json:"slug"`
	Type string `json:"type,omitempty"`
}

// WorkspaceListOutput lists the workspaces a user can access.
type WorkspaceListOutput struct {
	Server     string          `json:"server,omitempty"`
	Workspaces []WorkspaceItem `json:"workspaces"`
}

func (o WorkspaceListOutput) FormatText(w io.Writer) error {
	if len(o.Workspaces) == 0 {
		fmt.Fprintln(w, "No workspaces found.")
		return nil
	}
	for _, ws := range o.Workspaces {
		line := ws.Slug
		if ws.Name != "" && ws.Name != ws.Slug {
			line = fmt.Sprintf("%s (%s)", ws.Slug, ws.Name)
		}
		if ws.Type == "personal" {
			line += " [personal]"
		}
		fmt.Fprintln(w, line)
	}
	return nil
}

// WorkspaceCreateOutput represents the result of kapi workspace create.
type WorkspaceCreateOutput struct {
	ID   string `json:"id,omitempty"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}

func (o WorkspaceCreateOutput) FormatText(w io.Writer) error {
	fmt.Fprintf(w, "Workspace created: %s", o.Slug)
	if o.Name != "" && o.Name != o.Slug {
		fmt.Fprintf(w, " (%s)", o.Name)
	}
	fmt.Fprintln(w)
	return nil
}
