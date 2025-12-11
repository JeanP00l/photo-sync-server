package storage

import (
	"sync"
	"time"
)

// DuplicateCheck проверяет дубликаты файлов
type DuplicateCheck struct {
	hashDB map[string]*FileHashInfo
	mu     sync.RWMutex
}

// FileHashInfo содержит информацию о хеше файла
type FileHashInfo struct {
	Hash string    `json:"hash"`
	Size int64     `json:"size"`
	Date time.Time `json:"date"`
	Path string    `json:"path"`
}

// NewDuplicateCheck создает новый проверщик дубликатов
func NewDuplicateCheck() *DuplicateCheck {
	return &DuplicateCheck{
		hashDB: make(map[string]*FileHashInfo),
	}
}

// CheckDuplicate проверяет, является ли файл дубликатом
func (dc *DuplicateCheck) CheckDuplicate(fileHash string, size int64, counterNumber string, dateTaken time.Time, indexer *Indexer) (*FileHashInfo, string) {
	dc.mu.RLock()
	defer dc.mu.RUnlock()

	// Уровень 1: Проверка по хешу
	if info, exists := dc.hashDB[fileHash]; exists {
		return info, "hash"
	}

	// Уровень 2: Проверка по номеру счетчика + дате (если есть номер)
	if counterNumber != "" && counterNumber != "unknown" {
		normalizedCounter := NormalizeCounterNumber(counterNumber)
		photos := indexer.GetPhotosByCounter(normalizedCounter)
		
		for _, photo := range photos {
			// Проверяем, если разница во времени менее 1 секунды
			if absTimeDiff(photo.Date, dateTaken) < time.Second {
				// Дополнительная проверка: сравниваем хеши
				if photo.Hash == fileHash {
					return &FileHashInfo{
						Hash: photo.Hash,
						Size: photo.Size,
						Date: photo.Date,
						Path: photo.Path,
					}, "counter_and_date"
				}
			}
		}
	}

	return nil, ""
}

// AddHash добавляет хеш файла в базу
func (dc *DuplicateCheck) AddHash(fileHash string, size int64, date time.Time, path string) {
	dc.mu.Lock()
	defer dc.mu.Unlock()

	dc.hashDB[fileHash] = &FileHashInfo{
		Hash: fileHash,
		Size: size,
		Date: date,
		Path: path,
	}
}


// absTimeDiff возвращает абсолютную разницу во времени
func absTimeDiff(t1, t2 time.Time) time.Duration {
	diff := t1.Sub(t2)
	if diff < 0 {
		return -diff
	}
	return diff
}

