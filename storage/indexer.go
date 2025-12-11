package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Indexer управляет индексом фото по номерам счетчиков
type Indexer struct {
	indexDir string
	index    map[string][]*PhotoInfo
	mu       sync.RWMutex
}

// PhotoInfo содержит информацию о фото
type PhotoInfo struct {
	Path        string    `json:"path"`
	FullPath    string    `json:"fullPath"`
	Date        time.Time `json:"date"`
	Size        int64     `json:"size"`
	Hash        string    `json:"hash"`
	UserComment string    `json:"userComment,omitempty"` // USER_COMMENT из EXIF метаданных
}

// NewIndexer создает новый индексер
func NewIndexer(indexDir string) *Indexer {
	indexer := &Indexer{
		indexDir: indexDir,
		index:    make(map[string][]*PhotoInfo),
	}

	// Загружаем существующий индекс
	indexer.loadIndex()

	return indexer
}

// AddPhoto добавляет фото в индекс
func (idx *Indexer) AddPhoto(counterNumber string, relPath string, fullPath string, date time.Time, size int64, hash string, userComment string) error {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	normalizedCounter := NormalizeCounterNumber(counterNumber)
	
	if idx.index[normalizedCounter] == nil {
		idx.index[normalizedCounter] = []*PhotoInfo{}
	}

	// Проверяем, нет ли уже этого файла в индексе
	for _, photo := range idx.index[normalizedCounter] {
		if photo.Path == relPath {
			return nil // Уже есть
		}
	}

	// Добавляем информацию о фото
	photo := &PhotoInfo{
		Path:        relPath,
		FullPath:    fullPath,
		Date:        date,
		Size:        size,
		Hash:        hash,
		UserComment: userComment,
	}

	idx.index[normalizedCounter] = append(idx.index[normalizedCounter], photo)

	// Сортируем по дате (новые первыми)
	photos := idx.index[normalizedCounter]
	for i := 0; i < len(photos)-1; i++ {
		for j := i + 1; j < len(photos); j++ {
			if photos[i].Date.Before(photos[j].Date) {
				photos[i], photos[j] = photos[j], photos[i]
			}
		}
	}

	// Сохраняем индекс
	return idx.saveIndex()
}

// GetPhotosByCounter возвращает все фото для указанного номера счетчика
func (idx *Indexer) GetPhotosByCounter(counterNumber string) []*PhotoInfo {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	normalizedCounter := NormalizeCounterNumber(counterNumber)
	return idx.index[normalizedCounter]
}

// GetAllCounters возвращает все номера счетчиков в индексе
func (idx *Indexer) GetAllCounters() []string {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	counters := make([]string, 0, len(idx.index))
	for counter := range idx.index {
		counters = append(counters, counter)
	}
	return counters
}

// loadIndex загружает индекс из файла
func (idx *Indexer) loadIndex() {
	indexFile := filepath.Join(idx.indexDir, "photo_index.json")
	
	data, err := os.ReadFile(indexFile)
	if err != nil {
		if !os.IsNotExist(err) {
			fmt.Printf("Warning: Failed to load index: %v\n", err)
		}
		return
	}

	var indexData map[string][]map[string]interface{}
	if err := json.Unmarshal(data, &indexData); err != nil {
		fmt.Printf("Warning: Failed to parse index: %v\n", err)
		return
	}

	// Конвертируем даты из строк в time.Time
	for counter, photosData := range indexData {
		convertedPhotos := make([]*PhotoInfo, 0, len(photosData))
		for _, photoData := range photosData {
			photo := &PhotoInfo{
				Path:        getString(photoData, "path"),
				FullPath:    getString(photoData, "fullPath"),
				Size:        getInt64(photoData, "size"),
				Hash:        getString(photoData, "hash"),
				UserComment: getString(photoData, "userComment"),
			}

			// Парсим дату
			if dateStr, ok := photoData["date"].(string); ok {
				if parsed, err := time.Parse(time.RFC3339, dateStr); err == nil {
					photo.Date = parsed
				} else {
					photo.Date = time.Now()
				}
			} else {
				photo.Date = time.Now()
			}

			convertedPhotos = append(convertedPhotos, photo)
		}
		idx.index[counter] = convertedPhotos
	}
}

// saveIndex сохраняет индекс в файл
func (idx *Indexer) saveIndex() error {
	indexFile := filepath.Join(idx.indexDir, "photo_index.json")

	// Конвертируем индекс в JSON-совместимый формат
	indexData := make(map[string][]map[string]interface{})
	for counter, photos := range idx.index {
		photoList := make([]map[string]interface{}, len(photos))
		for i, photo := range photos {
			photoMap := map[string]interface{}{
				"path":     photo.Path,
				"fullPath": photo.FullPath,
				"date":     photo.Date.Format(time.RFC3339),
				"size":     photo.Size,
				"hash":     photo.Hash,
			}
			if photo.UserComment != "" {
				photoMap["userComment"] = photo.UserComment
			}
			photoList[i] = photoMap
		}
		indexData[counter] = photoList
	}

	data, err := json.MarshalIndent(indexData, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal index: %w", err)
	}

	if err := os.WriteFile(indexFile, data, 0644); err != nil {
		return fmt.Errorf("failed to save index: %w", err)
	}

	return nil
}

// NormalizeCounterNumber нормализует номер счетчика для сравнения
func NormalizeCounterNumber(counterNumber string) string {
	// Приводим к нижнему регистру и удаляем спецсимволы
	result := strings.ToLower(counterNumber)
	result = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || 
		   (r >= 'а' && r <= 'я') || r == 'ё' {
			return r
		}
		return -1 // Удаляем символ
	}, result)
	return result
}

// getString извлекает строку из map
func getString(m map[string]interface{}, key string) string {
	if val, ok := m[key].(string); ok {
		return val
	}
	return ""
}

// getInt64 извлекает int64 из map
func getInt64(m map[string]interface{}, key string) int64 {
	switch v := m[key].(type) {
	case int64:
		return v
	case int:
		return int64(v)
	case float64:
		return int64(v)
	default:
		return 0
	}
}

