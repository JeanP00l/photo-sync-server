package storage

import (
	"sync"
	"time"

	"photo-sync-server/models"
)

// SessionStore хранит активные сессии синхронизации
type SessionStore struct {
	sessions map[string]*models.Session
	mu       sync.RWMutex
}

// NewSessionStore создает новое хранилище сессий
func NewSessionStore() *SessionStore {
	store := &SessionStore{
		sessions: make(map[string]*models.Session),
	}

	// Запускаем очистку старых сессий каждую минуту
	go store.cleanup()

	return store
}

// Create создает новую сессию
func (s *SessionStore) Create(token string) *models.Session {
	s.mu.Lock()
	defer s.mu.Unlock()

	session := models.NewSession(token)
	s.sessions[token] = session
	return session
}

// Get получает сессию по токену
func (s *SessionStore) Get(token string) (*models.Session, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	session, exists := s.sessions[token]
	return session, exists
}

// Update обновляет сессию
func (s *SessionStore) Update(token string, updater func(*models.Session)) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	session, exists := s.sessions[token]
	if !exists {
		return false
	}

	updater(session)
	session.Update()
	return true
}

// Delete удаляет сессию
func (s *SessionStore) Delete(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.sessions, token)
}

// cleanup удаляет сессии старше 1 часа
func (s *SessionStore) cleanup() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		s.mu.Lock()
		now := time.Now()
		for token, session := range s.sessions {
			if now.Sub(session.LastUpdate) > 1*time.Hour {
				delete(s.sessions, token)
			}
		}
		s.mu.Unlock()
	}
}


