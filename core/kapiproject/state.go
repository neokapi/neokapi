package kapiproject

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// State tracks sync state between local files and Bowrain Server.
type State struct {
	LastPull time.Time `json:"last_pull,omitempty"`
	LastPush time.Time `json:"last_push,omitempty"`

	// Files maps local file path → file state
	Files map[string]*FileState `json:"files,omitempty"`

	// RemoteItems maps remote item ID → item state
	RemoteItems map[string]*RemoteItemState `json:"remote_items,omitempty"`
}

// FileState tracks the state of a local file.
type FileState struct {
	ContentHash    string    `json:"content_hash"`
	Modified       time.Time `json:"modified"`
	RemoteHash     string    `json:"remote_hash,omitempty"`
	RemoteModified time.Time `json:"remote_modified,omitempty"`
}

// RemoteItemState tracks the state of a remote item.
type RemoteItemState struct {
	ContentHash string    `json:"content_hash"`
	Modified    time.Time `json:"modified"`
	LocalPath   string    `json:"local_path"`
}

// NewState creates a new empty state.
func NewState() *State {
	return &State{
		Files:       make(map[string]*FileState),
		RemoteItems: make(map[string]*RemoteItemState),
	}
}

// LoadState loads the sync state from .kapi/.state.json.
func LoadState(kapiDir string) (*State, error) {
	statePath := filepath.Join(kapiDir, StateFile)

	data, err := os.ReadFile(statePath)
	if err != nil {
		if os.IsNotExist(err) {
			// No state file yet, return empty state
			return NewState(), nil
		}
		return nil, fmt.Errorf("read state: %w", err)
	}

	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("parse state: %w", err)
	}

	// Initialize maps if nil
	if state.Files == nil {
		state.Files = make(map[string]*FileState)
	}
	if state.RemoteItems == nil {
		state.RemoteItems = make(map[string]*RemoteItemState)
	}

	return &state, nil
}

// SaveState saves the sync state to .kapi/.state.json.
func SaveState(kapiDir string, state *State) error {
	statePath := filepath.Join(kapiDir, StateFile)

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}

	if err := os.WriteFile(statePath, data, 0600); err != nil {
		return fmt.Errorf("write state: %w", err)
	}

	return nil
}

// UpdateFileState updates the state for a local file.
func (s *State) UpdateFileState(path string, contentHash string, modified time.Time) {
	if s.Files == nil {
		s.Files = make(map[string]*FileState)
	}

	s.Files[path] = &FileState{
		ContentHash: contentHash,
		Modified:    modified,
	}
}

// UpdateRemoteItemState updates the state for a remote item.
func (s *State) UpdateRemoteItemState(itemID string, contentHash string, modified time.Time, localPath string) {
	if s.RemoteItems == nil {
		s.RemoteItems = make(map[string]*RemoteItemState)
	}

	s.RemoteItems[itemID] = &RemoteItemState{
		ContentHash: contentHash,
		Modified:    modified,
		LocalPath:   localPath,
	}
}
