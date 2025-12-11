package models

import "time"

// SyncStatus представляет статус синхронизации
type SyncStatus string

const (
	StatusWaiting  SyncStatus = "waiting"
	StatusReady    SyncStatus = "ready"
	StatusSyncing  SyncStatus = "syncing"
	StatusCompleted SyncStatus = "completed"
	StatusError    SyncStatus = "error"
)

// Session представляет сессию синхронизации
type Session struct {
	Token       string     `json:"token"`
	Status      SyncStatus `json:"status"`
	Total       int        `json:"total"`
	Uploaded    int        `json:"uploaded"`
	Skipped     int        `json:"skipped"`
	StartTime   time.Time  `json:"startTime"`
	LastUpdate  time.Time  `json:"lastUpdate"`
	CurrentFile string     `json:"currentFile,omitempty"`
	Errors      []string   `json:"errors,omitempty"`
}

// NewSession создает новую сессию
func NewSession(token string) *Session {
	now := time.Now()
	return &Session{
		Token:      token,
		Status:     StatusWaiting,
		Total:      0,
		Uploaded:   0,
		Skipped:    0,
		StartTime:  now,
		LastUpdate: now,
		Errors:     []string{},
	}
}

// Update обновляет время последнего обновления
func (s *Session) Update() {
	s.LastUpdate = time.Now()
}

// GetProgress возвращает процент выполнения
func (s *Session) GetProgress() float64 {
	if s.Total == 0 {
		return 0
	}
	return float64(s.Uploaded+s.Skipped) / float64(s.Total) * 100
}

// GetEstimatedTimeRemaining возвращает оценку оставшегося времени в секундах
func (s *Session) GetEstimatedTimeRemaining() int {
	if s.Uploaded == 0 {
		return 0
	}
	elapsed := time.Since(s.StartTime).Seconds()
	avgTimePerFile := elapsed / float64(s.Uploaded+s.Skipped)
	remaining := float64(s.Total-s.Uploaded-s.Skipped) * avgTimePerFile
	return int(remaining)
}


