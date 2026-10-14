package session

import (
	"sort"
	"sync"
)

// Manager is the set of live sessions on this machine.
type Manager struct {
	mu       sync.Mutex
	sessions map[string]*Session
}

func NewManager() *Manager {
	return &Manager{sessions: map[string]*Session{}}
}

// Create starts a session under id, or hands back the live one already using
// that id so a reconnecting client does not spawn a second shell.
func (m *Manager) Create(id string, cols, rows int, cwd, shell string) (s *Session, reused bool, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if existing, ok := m.sessions[id]; ok && existing.Alive() {
		return existing, true, nil
	}
	started, err := Start(id, cols, rows, cwd, shell)
	if err != nil {
		return nil, false, err
	}
	m.sessions[id] = started
	return started, false, nil
}

func (m *Manager) Get(id string) (*Session, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[id]
	return s, ok
}

// List returns live sessions in a stable order: oldest first, ties broken by
// id. Clients render exactly this order, which is what keeps the phone list
// and the PC list one-to-one across reconnects.
func (m *Manager) List() []*Session {
	m.mu.Lock()
	defer m.mu.Unlock()

	out := make([]*Session, 0, len(m.sessions))
	for _, s := range m.sessions {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].CreatedAt.Before(out[j].CreatedAt)
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// Forget drops a session after its process has exited.
func (m *Manager) Forget(id string) {
	m.mu.Lock()
	delete(m.sessions, id)
	m.mu.Unlock()
}

func (m *Manager) Close(id string) error {
	s, ok := m.Get(id)
	if !ok {
		return ErrClosed
	}
	return s.Close()
}

// Shutdown ends every session, children included.
func (m *Manager) Shutdown() {
	for _, s := range m.List() {
		_ = s.Close()
	}
}
