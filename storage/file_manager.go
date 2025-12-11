package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// FileManager управляет сохранением файлов
type FileManager struct {
	baseDir string
}

// BaseDir возвращает базовую директорию
func (fm *FileManager) BaseDir() string {
	return fm.baseDir
}

// NewFileManager создает новый менеджер файлов
func NewFileManager(baseDir string) *FileManager {
	return &FileManager{
		baseDir: baseDir,
	}
}

// SaveFile сохраняет файл в структурированную папку по дате
func (fm *FileManager) SaveFile(filename string, data []byte, dateTaken time.Time) (string, error) {
	// Если дата равна эпохе Unix (1970-01-01), используем текущую дату
	if dateTaken.Unix() == 0 || dateTaken.Year() < 2000 {
		dateTaken = time.Now()
	}

	// Убеждаемся, что базовая директория существует
	_, err := os.Stat(fm.baseDir)
	if os.IsNotExist(err) {
		if err := os.MkdirAll(fm.baseDir, 0755); err != nil {
			return "", fmt.Errorf("failed to create base directory: %w", err)
		}
	} else if err != nil {
		return "", fmt.Errorf("failed to check base directory: %w", err)
	}

	// Временно сохраняем все файлы напрямую в базовую директорию без подпапок
	dir := fm.baseDir
	
	// Нормализуем путь для Windows
	dir = filepath.Clean(dir)

	// Проверяем права на запись в директорию
	testFile := filepath.Join(dir, ".write_test")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		return "", fmt.Errorf("cannot write to directory %s: %w", dir, err)
	}
	os.Remove(testFile) // Удаляем тестовый файл

	// Формируем имя файла: используем оригинальное имя с уникальным суффиксом
	ext := filepath.Ext(filename)
	if ext == "" {
		ext = ".jpg"
	}
	
	// Используем оригинальное имя файла, но добавляем уникальный суффикс для избежания конфликтов
	baseName := filename[:len(filename)-len(ext)]
	timestamp := time.Now().UnixNano() // Используем наносекунды для уникальности
	newFilename := fmt.Sprintf("%s_%d%s", baseName, timestamp, ext)
	
	fullPath := filepath.Join(dir, newFilename)

	// Сохраняем файл используя os.Create для лучшей совместимости с Windows
	file, err := os.Create(fullPath)
	if err != nil {
		return "", fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()
	
	if _, err := file.Write(data); err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}
	
	if err := file.Sync(); err != nil {
		// Не критично, продолжаем
	}

	// Возвращаем относительный путь
	relPath, err := filepath.Rel(fm.baseDir, fullPath)
	if err != nil {
		return "", fmt.Errorf("failed to get relative path: %w", err)
	}

	return relPath, nil
}

// CalculateHash вычисляет SHA256 хеш файла
func (fm *FileManager) CalculateHash(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

// FileExists проверяет существование файла
func (fm *FileManager) FileExists(relPath string) bool {
	fullPath := filepath.Join(fm.baseDir, relPath)
	_, err := os.Stat(fullPath)
	return err == nil
}

// GetFileInfo возвращает информацию о файле
func (fm *FileManager) GetFileInfo(relPath string) (os.FileInfo, error) {
	fullPath := filepath.Join(fm.baseDir, relPath)
	return os.Stat(fullPath)
}

// ReadFile читает файл
func (fm *FileManager) ReadFile(relPath string) ([]byte, error) {
	fullPath := filepath.Join(fm.baseDir, relPath)
	return os.ReadFile(fullPath)
}

// extractCounterNumber извлекает номер счетчика из имени файла
// Формат: {counterNumber}_{date}_{time}.{ext}
func extractCounterNumber(filename string) string {
	// Убираем расширение
	name := filepath.Base(filename)
	ext := filepath.Ext(name)
	name = name[:len(name)-len(ext)]
	
	// Ищем первую подчеркивание (разделитель между номером счетчика и датой)
	// Формат: counterNumber_date_time
	parts := splitByUnderscore(name)
	if len(parts) >= 1 {
		return parts[0]
	}
	
	return "unknown"
}

// splitByUnderscore разбивает строку по подчеркиваниям
func splitByUnderscore(s string) []string {
	var parts []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '_' {
			if start < i {
				parts = append(parts, s[start:i])
			}
			start = i + 1
		}
	}
	if start < len(s) {
		parts = append(parts, s[start:])
	}
	return parts
}

// CalculateFileHash вычисляет хеш файла по пути
func (fm *FileManager) CalculateFileHash(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}


