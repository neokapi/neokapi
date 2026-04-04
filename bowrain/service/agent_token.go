package service

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"
)

// AgentToken is a short-lived scoped token used by @bravo to authenticate
// MCP tool calls on behalf of a user.
type AgentToken struct {
	Token          string // full token: "bwt_bravo_<random>"
	UserID         string // user the agent acts on behalf of
	WorkspaceID    string // workspace scope
	ConversationID string // conversation scope
	WorkspaceRole  string // inherited workspace role
	CreatedAt      time.Time
	ExpiresAt      time.Time
}

// AgentTokenStore manages the lifecycle of scoped agent tokens.
// Tokens are stored in-memory and automatically expire.
type AgentTokenStore struct {
	mu     sync.RWMutex
	tokens map[string]*AgentToken // token string → AgentToken
}

// NewAgentTokenStore creates a new in-memory agent token store.
func NewAgentTokenStore() *AgentTokenStore {
	return &AgentTokenStore{
		tokens: make(map[string]*AgentToken),
	}
}

// Create generates a new scoped agent token for a conversation.
// The token is valid for the specified duration.
func (s *AgentTokenStore) Create(userID, workspaceID, conversationID, workspaceRole string, ttl time.Duration) (*AgentToken, error) {
	random, err := generateRandom(24)
	if err != nil {
		return nil, fmt.Errorf("generate token: %w", err)
	}

	now := time.Now()
	token := &AgentToken{
		Token:          "bwt_bravo_" + random,
		UserID:         userID,
		WorkspaceID:    workspaceID,
		ConversationID: conversationID,
		WorkspaceRole:  workspaceRole,
		CreatedAt:      now,
		ExpiresAt:      now.Add(ttl),
	}

	s.mu.Lock()
	s.tokens[token.Token] = token
	s.mu.Unlock()

	return token, nil
}

// Validate looks up a token and returns it if valid and not expired.
func (s *AgentTokenStore) Validate(tokenStr string) (*AgentToken, error) {
	s.mu.RLock()
	token, ok := s.tokens[tokenStr]
	s.mu.RUnlock()

	if !ok {
		return nil, errors.New("invalid agent token")
	}
	if time.Now().After(token.ExpiresAt) {
		// Expired — clean up.
		s.mu.Lock()
		delete(s.tokens, tokenStr)
		s.mu.Unlock()
		return nil, errors.New("agent token expired")
	}
	return token, nil
}

// Revoke removes a specific token.
func (s *AgentTokenStore) Revoke(tokenStr string) {
	s.mu.Lock()
	delete(s.tokens, tokenStr)
	s.mu.Unlock()
}

// RevokeForConversation removes all tokens associated with a conversation.
func (s *AgentTokenStore) RevokeForConversation(conversationID string) {
	s.mu.Lock()
	for k, t := range s.tokens {
		if t.ConversationID == conversationID {
			delete(s.tokens, k)
		}
	}
	s.mu.Unlock()
}

// PurgeExpired removes all expired tokens.
func (s *AgentTokenStore) PurgeExpired() int {
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	count := 0
	for k, t := range s.tokens {
		if now.After(t.ExpiresAt) {
			delete(s.tokens, k)
			count++
		}
	}
	return count
}

func generateRandom(bytes int) (string, error) {
	b := make([]byte, bytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
