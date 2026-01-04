package pty

import (
	"fmt"
	"sync"
)

// Manager handles multiple PTY sessions
type Manager struct {
	sessions map[string]*Session
	mu       sync.RWMutex
}

// NewManager creates a new PTY manager
func NewManager() *Manager {
	return &Manager{
		sessions: make(map[string]*Session),
	}
}

// Spawn creates and starts a new PTY session
func (m *Manager) Spawn(id, workdir, task string) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.sessions[id]; exists {
		return nil, fmt.Errorf("session %s already exists", id)
	}

	session := NewSession(id, workdir, task)
	if err := session.Start(); err != nil {
		return nil, fmt.Errorf("failed to start session: %w", err)
	}

	m.sessions[id] = session
	return session, nil
}

// Get returns a session by ID
func (m *Manager) Get(id string) (*Session, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	session, ok := m.sessions[id]
	return session, ok
}

// Kill stops and removes a session
func (m *Manager) Kill(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, ok := m.sessions[id]
	if !ok {
		return fmt.Errorf("session %s not found", id)
	}

	if err := session.Stop(); err != nil {
		return err
	}

	delete(m.sessions, id)
	return nil
}

// List returns all session IDs
func (m *Manager) List() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	ids := make([]string, 0, len(m.sessions))
	for id := range m.sessions {
		ids = append(ids, id)
	}
	return ids
}

// Write sends input to a specific session
func (m *Manager) Write(id string, data []byte) (int, error) {
	session, ok := m.Get(id)
	if !ok {
		return 0, fmt.Errorf("session %s not found", id)
	}
	return session.Write(data)
}

// ResizeAll resizes all sessions
func (m *Manager) ResizeAll(rows, cols int) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, session := range m.sessions {
		session.Resize(rows, cols)
	}
}

// StopAll stops all sessions
func (m *Manager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for id, session := range m.sessions {
		session.Stop()
		delete(m.sessions, id)
	}
}

// Count returns the number of active sessions
func (m *Manager) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.sessions)
}
