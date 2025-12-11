package handlers

import (
	"encoding/binary"
	"fmt"
	"net/http"
	"path/filepath"
	"time"

	"photo-sync-server/models"
	"photo-sync-server/storage"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Handlers содержит все обработчики HTTP запросов
type Handlers struct {
	sessionStore   *storage.SessionStore
	fileManager    *storage.FileManager
	indexer        *storage.Indexer
	duplicateCheck *storage.DuplicateCheck
	localIP        string
	port           int
}

// NewHandlers создает новый набор обработчиков
func NewHandlers(sessionStore *storage.SessionStore, fileManager *storage.FileManager, indexer *storage.Indexer, duplicateCheck *storage.DuplicateCheck, localIP string, port int) *Handlers {
	return &Handlers{
		sessionStore:   sessionStore,
		fileManager:    fileManager,
		indexer:        indexer,
		duplicateCheck: duplicateCheck,
		localIP:        localIP,
		port:           port,
	}
}

// StartHandler обрабатывает запрос на создание сессии
func (h *Handlers) StartHandler(c *gin.Context) {
	token := uuid.New().String()
	_ = h.sessionStore.Create(token) // Создаем сессию

	url := fmt.Sprintf("http://%s:%d/sync?token=%s", h.localIP, h.port, token)

	c.JSON(http.StatusOK, gin.H{
		"token":   token,
		"url":     url,
		"localIP": h.localIP,
		"port":    h.port,
	})
}

// InitHandler обрабатывает инициализацию синхронизации
func (h *Handlers) InitHandler(c *gin.Context) {
	token := c.Query("token")
	if token == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "token is required"})
		return
	}

	var req struct {
		Total      int    `json:"total"`
		FilterDate string `json:"filterDate,omitempty"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	success := h.sessionStore.Update(token, func(session *models.Session) {
		session.Total = req.Total
		session.Status = models.StatusReady
		session.StartTime = time.Now()
	})

	if !success {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":   true,
		"sessionId": token,
	})
}

// SyncHandler обрабатывает загрузку фото
func (h *Handlers) SyncHandler(c *gin.Context) {
	token := c.Query("token")
	if token == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "token is required"})
		return
	}

	session, exists := h.sessionStore.Get(token)
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}

	// Получаем файл из multipart/form-data
	file, err := c.FormFile("photo")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "photo file is required"})
		return
	}

	// Открываем файл
	src, err := file.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to open file"})
		return
	}
	defer src.Close()

	// Читаем данные файла
	data := make([]byte, file.Size)
	if _, err := src.Read(data); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read file"})
		return
	}

	// Получаем дополнительные данные из формы
	counterNumber := c.PostForm("counterNumber")
	originalName := c.PostForm("originalName")
	dateTakenStr := c.PostForm("dateTaken")

	// Парсим дату
	dateTaken := time.Now()
	if dateTakenStr != "" {
		if parsed, err := time.Parse(time.RFC3339, dateTakenStr); err == nil {
			dateTaken = parsed
		}
	}

	// Вычисляем хеш файла
	fileHash := h.fileManager.CalculateHash(data)
	size := int64(len(data))

	// Проверяем дубликаты
	existingFile, reason := h.duplicateCheck.CheckDuplicate(fileHash, size, counterNumber, dateTaken, h.indexer)
	isDuplicate := existingFile != nil

	if isDuplicate {
		// Обновляем сессию
		h.sessionStore.Update(token, func(session *models.Session) {
			session.Skipped++
			session.Status = models.StatusSyncing
			session.CurrentFile = originalName
		})

		c.JSON(http.StatusOK, gin.H{
			"success":      true,
			"uploaded":     session.Uploaded,
			"total":        session.Total,
			"filepath":     existingFile.Path,
			"isDuplicate":  true,
			"reason":       reason,
			"existingFile": existingFile.Path,
		})
		return
	}

	// Извлекаем номер счетчика из EXIF, если не передан
	if counterNumber == "" {
		counterNumber = extractCounterNumberFromEXIF(data)
		if counterNumber == "" {
			counterNumber = "unknown"
		}
	}

	// Извлекаем полный USER_COMMENT из EXIF для сохранения в индекс
	userComment := extractUserCommentFromEXIF(data)

	// Сохраняем файл
	relPath, err := h.fileManager.SaveFile(originalName, data, dateTaken)
	if err != nil {
		h.sessionStore.Update(token, func(session *models.Session) {
			session.Errors = append(session.Errors, err.Error())
		})
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save file"})
		return
	}

	fullPath := filepath.Join(h.fileManager.BaseDir(), relPath)

	// Добавляем в индекс с USER_COMMENT
	if err := h.indexer.AddPhoto(counterNumber, relPath, fullPath, dateTaken, size, fileHash, userComment); err != nil {
		// Логируем ошибку, но не прерываем процесс
		fmt.Printf("Warning: Failed to add photo to index: %v\n", err)
	}

	// Добавляем хеш в базу дубликатов
	h.duplicateCheck.AddHash(fileHash, size, dateTaken, relPath)

	// Обновляем сессию
	h.sessionStore.Update(token, func(session *models.Session) {
		session.Uploaded++
		session.Status = models.StatusSyncing
		session.CurrentFile = originalName

		// Проверяем, завершена ли синхронизация
		if session.Uploaded+session.Skipped >= session.Total {
			session.Status = models.StatusCompleted
		}
	})

	c.JSON(http.StatusOK, gin.H{
		"success":     true,
		"uploaded":    session.Uploaded,
		"total":       session.Total,
		"filepath":    relPath,
		"isDuplicate": false,
	})
}

// StatusHandler возвращает статус синхронизации
func (h *Handlers) StatusHandler(c *gin.Context) {
	token := c.Query("token")
	if token == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "token is required"})
		return
	}

	session, exists := h.sessionStore.Get(token)
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":                 string(session.Status),
		"total":                  session.Total,
		"uploaded":               session.Uploaded,
		"skipped":                session.Skipped,
		"progress":               session.GetProgress(),
		"currentFile":            session.CurrentFile,
		"startTime":              session.StartTime.Format(time.RFC3339),
		"estimatedTimeRemaining": session.GetEstimatedTimeRemaining(),
	})
}

// IndexHandler возвращает индекс фото по номеру счетчика
func (h *Handlers) IndexHandler(c *gin.Context) {
	counterNumber := c.Query("counterNumber")

	if counterNumber == "" {
		// Возвращаем весь индекс
		counters := h.indexer.GetAllCounters()
		result := make(map[string]interface{})
		for _, counter := range counters {
			photos := h.indexer.GetPhotosByCounter(counter)
			result[counter] = photos
		}
		c.JSON(http.StatusOK, result)
		return
	}

	photos := h.indexer.GetPhotosByCounter(counterNumber)
	c.JSON(http.StatusOK, gin.H{
		"counterNumber": counterNumber,
		"photos":        photos,
		"total":         len(photos),
	})
}

// DeleteSessionHandler удаляет сессию
func (h *Handlers) DeleteSessionHandler(c *gin.Context) {
	token := c.Query("token")
	if token == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "token is required"})
		return
	}

	h.sessionStore.Delete(token)
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// extractUserCommentFromEXIF извлекает полный USER_COMMENT из EXIF метаданных
func extractUserCommentFromEXIF(data []byte) string {
	// Проверяем JPEG маркер
	if len(data) < 2 || data[0] != 0xFF || data[1] != 0xD8 {
		return ""
	}

	offset := 2
	exifSegmentsFound := 0
	for offset < len(data)-1 {
		// Ищем маркер сегмента
		if data[offset] != 0xFF {
			break
		}

		marker := data[offset+1]
		offset += 2

		// Пропускаем маркеры без данных
		if marker == 0xFF {
			continue
		}

		// APP1 сегмент содержит EXIF данные
		if marker == 0xE1 {
			exifSegmentsFound++
			if offset+2 > len(data) {
				break
			}
			length := int(binary.BigEndian.Uint16(data[offset : offset+2]))
			if length < 2 || offset+length > len(data) {
				break
			}

			// Проверяем "Exif\0\0" заголовок
			if offset+6 <= len(data) && string(data[offset+2:offset+8]) == "Exif\x00\x00" {
				// Ищем USER_COMMENT в EXIF данных
				comment := findUserComment(data[offset+2 : offset+length])
				if comment != "" {
					return comment
				}
			}

			offset += length
			continue
		}

		// Читаем длину сегмента для других маркеров
		if offset+2 > len(data) {
			break
		}
		length := int(binary.BigEndian.Uint16(data[offset : offset+2]))
		if length < 2 {
			break
		}
		offset += length
	}

	return ""
}

// extractCounterNumberFromEXIF извлекает номер счетчика из EXIF метаданных USER_COMMENT
func extractCounterNumberFromEXIF(data []byte) string {
	// Используем extractUserCommentFromEXIF для получения полного комментария
	// Функция findUserComment уже извлекает номер счетчика из комментария
	return extractUserCommentFromEXIF(data)
}

// findUserComment ищет USER_COMMENT в EXIF данных (упрощенная реализация)
func findUserComment(exifData []byte) string {
	// USER_COMMENT имеет тег 0x9286 в IFD0 или IFD1
	// Это упрощенная реализация, которая ищет строку в EXIF данных
	// Для полной реализации нужен полный парсер EXIF структуры

	// Ищем паттерн, который может быть номером счетчика (цифры и буквы, минимум 10 символов)
	// Ищем в виде строки в EXIF данных
	dataStr := string(exifData)

	// Ищем последовательности букв и цифр длиной >= 10 символов
	// Это может быть номер счетчика
	for i := 0; i < len(dataStr)-10; i++ {
		if isAlphanumeric(dataStr[i]) {
			j := i
			for j < len(dataStr) && (isAlphanumeric(dataStr[j]) || isCyrillic(dataStr[j])) {
				j++
			}
			if j-i >= 10 {
				candidate := dataStr[i:j]
				// Проверяем, что это похоже на номер счетчика (содержит цифры)
				if containsDigit(candidate) {
					return candidate
				}
			}
			i = j
		}
	}

	// Также ищем более короткие последовательности (от 8 символов) для номеров счетчиков
	for i := 0; i < len(dataStr)-8; i++ {
		if isAlphanumeric(dataStr[i]) {
			j := i
			for j < len(dataStr) && (isAlphanumeric(dataStr[j]) || isCyrillic(dataStr[j])) {
				j++
			}
			if j-i >= 8 && j-i < 10 {
				candidate := dataStr[i:j]
				// Проверяем, что это похоже на номер счетчика (содержит цифры и только буквы/цифры)
				if containsDigit(candidate) && isOnlyAlphanumeric(candidate) {
					return candidate
				}
			}
			i = j
		}
	}

	return ""
}

func isAlphanumeric(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
}

func isCyrillic(b byte) bool {
	// Проверяем кириллицу (упрощенно)
	return b >= 0xD0 && b <= 0xDF || b >= 0xE0 && b <= 0xEF
}

func containsDigit(s string) bool {
	for _, r := range s {
		if r >= '0' && r <= '9' {
			return true
		}
	}
	return false
}

func isOnlyAlphanumeric(s string) bool {
	for _, r := range s {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')) {
			return false
		}
	}
	return true
}
