package auth

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"
)

var ErrNotFound = errors.New("auth record not found")

type MemoryStore struct {
	mu        sync.RWMutex
	users     map[string]User
	byEmail   map[string]string
	sessions  map[string]memorySession
	apiTokens map[string]memoryToken
}

type memorySession struct {
	ID        string
	UserID    string
	TokenHash []byte
	ExpiresAt time.Time
}

type memoryToken struct {
	APIToken
	TokenHash []byte
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		users:     map[string]User{},
		byEmail:   map[string]string{},
		sessions:  map[string]memorySession{},
		apiTokens: map[string]memoryToken{},
	}
}

func (s *MemoryStore) BootstrapAdmin(_ context.Context, user User) (User, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	email := strings.ToLower(strings.TrimSpace(user.Email))
	if id, ok := s.byEmail[email]; ok {
		existing := s.users[id]
		if existing.PasswordHash == "" && user.PasswordHash != "" {
			existing.PasswordHash = user.PasswordHash
			s.users[id] = existing
		}
		return existing, false, nil
	}
	user.Email = email
	s.users[user.ID] = user
	s.byEmail[email] = user.ID
	return user, true, nil
}

func (s *MemoryStore) UserByEmail(_ context.Context, email string) (User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	id, ok := s.byEmail[strings.ToLower(strings.TrimSpace(email))]
	if !ok {
		return User{}, ErrNotFound
	}
	return s.users[id], nil
}

func (s *MemoryStore) CreateSession(_ context.Context, sessionID, userID string, tokenHash []byte, expiresAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[sessionID] = memorySession{ID: sessionID, UserID: userID, TokenHash: append([]byte(nil), tokenHash...), ExpiresAt: expiresAt}
	return nil
}

func (s *MemoryStore) PrincipalBySessionHash(_ context.Context, tokenHash []byte, now time.Time) (Principal, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, session := range s.sessions {
		if session.ExpiresAt.Before(now) {
			delete(s.sessions, id)
			continue
		}
		if ConstantTimeTokenEqual(session.TokenHash, tokenHash) {
			user, ok := s.users[session.UserID]
			if !ok || user.DisabledAt != nil {
				return Principal{}, ErrUnauthenticated
			}
			return Principal{ID: user.ID, Name: user.DisplayName, Email: user.Email, Role: user.Role}, nil
		}
	}
	return Principal{}, ErrUnauthenticated
}

func (s *MemoryStore) DeleteSessionByHash(_ context.Context, tokenHash []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, session := range s.sessions {
		if ConstantTimeTokenEqual(session.TokenHash, tokenHash) {
			delete(s.sessions, id)
		}
	}
	return nil
}

func (s *MemoryStore) PrincipalByAPITokenHash(_ context.Context, tokenHash []byte, now time.Time) (Principal, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, token := range s.apiTokens {
		if token.RevokedAt != nil || (token.ExpiresAt != nil && token.ExpiresAt.Before(now)) {
			continue
		}
		if ConstantTimeTokenEqual(token.TokenHash, tokenHash) {
			lastUsed := now
			token.LastUsedAt = &lastUsed
			s.apiTokens[id] = token
			user, ok := s.users[token.UserID]
			if !ok || user.DisabledAt != nil {
				return Principal{}, ErrUnauthenticated
			}
			return Principal{ID: user.ID, Name: user.DisplayName, Email: user.Email, Role: user.Role}, nil
		}
	}
	return Principal{}, ErrUnauthenticated
}

func (s *MemoryStore) CreateAPIToken(_ context.Context, token APIToken, tokenHash []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.apiTokens[token.ID] = memoryToken{APIToken: token, TokenHash: append([]byte(nil), tokenHash...)}
	return nil
}

func (s *MemoryStore) ListAPITokens(_ context.Context, userID string) ([]APIToken, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []APIToken
	for _, token := range s.apiTokens {
		if token.UserID == userID {
			out = append(out, token.APIToken)
		}
	}
	return out, nil
}

func (s *MemoryStore) RevokeAPIToken(_ context.Context, userID, tokenID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	token, ok := s.apiTokens[tokenID]
	if !ok || token.UserID != userID {
		return ErrNotFound
	}
	now := time.Now().UTC()
	token.RevokedAt = &now
	s.apiTokens[tokenID] = token
	return nil
}
