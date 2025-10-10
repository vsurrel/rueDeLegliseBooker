package server

import (
	"crypto/rand"
	"encoding/base64"
	"sync"
	"time"
)

type sessionManager struct {
	mu       sync.Mutex
	sessions map[string]time.Time
	lifetime time.Duration
}

func newSessionManager(lifetime time.Duration) *sessionManager {
	return &sessionManager{
		sessions: make(map[string]time.Time),
		lifetime: lifetime,
	}
}

func (m *sessionManager) Create() (string, error) {
	token, err := generateToken(32)
	if err != nil {
		return "", err
	}

	expiry := time.Now().Add(m.lifetime)

	m.mu.Lock()
	m.sessions[token] = expiry
	m.mu.Unlock()

	return token, nil
}

func (m *sessionManager) Validate(token string) bool {
	if token == "" {
		return false
	}

	now := time.Now()

	m.mu.Lock()
	defer m.mu.Unlock()

	expiry, ok := m.sessions[token]
	if !ok {
		return false
	}

	if now.After(expiry) {
		delete(m.sessions, token)
		return false
	}

	return true
}

func generateToken(size int) (string, error) {
	buffer := make([]byte, size)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}
