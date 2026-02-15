package server

import "sync"

// presenceStore tracks per-project user presence in memory.
type presenceStore struct {
	mu sync.RWMutex
	// projectID → userID → presenceEntry
	users map[string]map[string]*presenceEntry
}

type presenceEntry struct {
	UserID    string
	UserName  string
	AvatarURL string
	ItemName  string
	BlockID   string
}

func newPresenceStore() *presenceStore {
	return &presenceStore{
		users: make(map[string]map[string]*presenceEntry),
	}
}

// Update sets or updates a user's presence in a project.
func (ps *presenceStore) Update(projectID string, entry *presenceEntry) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	if ps.users[projectID] == nil {
		ps.users[projectID] = make(map[string]*presenceEntry)
	}
	ps.users[projectID][entry.UserID] = entry
}

// Remove removes a user's presence from a project.
func (ps *presenceStore) Remove(projectID, userID string) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	if m := ps.users[projectID]; m != nil {
		delete(m, userID)
		if len(m) == 0 {
			delete(ps.users, projectID)
		}
	}
}

// List returns all present users in a project.
func (ps *presenceStore) List(projectID string) []*presenceEntry {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	m := ps.users[projectID]
	entries := make([]*presenceEntry, 0, len(m))
	for _, e := range m {
		entries = append(entries, e)
	}
	return entries
}
