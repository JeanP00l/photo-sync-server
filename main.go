package main

import (
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/gin-gonic/gin"
	"photo-sync-server/handlers"
	"photo-sync-server/storage"
)

const (
	DefaultPort = 8080
	PhotosDir   = "meter"
)

var (
	baseDir string
)

func main() {
	// Определяем базовую директорию для сохранения фото
	// Пробуем несколько вариантов для гарантированных прав доступа
	var err error
	exePath, err := os.Executable()
	if err != nil {
		exePath = "."
	}
	exeDir := filepath.Dir(exePath)
	
	// Вариант 1: Папка рядом с exe файлом (предпочтительно)
	baseDir = filepath.Join(exeDir, PhotosDir)
	canWrite := tryCreateAndWrite(baseDir)
	
	// Вариант 2: Если не получилось, пробуем временную директорию
	if !canWrite {
		tempDir := os.TempDir()
		baseDir = filepath.Join(tempDir, "photo-sync", PhotosDir)
		log.Printf("Trying alternative location: %s", baseDir)
		canWrite = tryCreateAndWrite(baseDir)
	}
	
	// Вариант 3: Если и это не получилось, пробуем папку пользователя
	if !canWrite {
		userHome, err := os.UserHomeDir()
		if err == nil {
			baseDir = filepath.Join(userHome, "Documents", "photo-sync", PhotosDir)
			log.Printf("Trying user documents location: %s", baseDir)
			canWrite = tryCreateAndWrite(baseDir)
		}
	}
	
	// Если ничего не помогло - выходим с ошибкой
	if !canWrite {
		logErrorAndExit("Failed to create writable directory for photos. Tried: %s and alternatives", baseDir)
	}
	
	log.Printf("Using photo directory: %s", baseDir)

	// Определяем папку для индексов
	var indexDir string
	if canWrite {
		// Пытаемся создать .index внутри baseDir
		indexDir = filepath.Join(baseDir, ".index")
		if err := os.MkdirAll(indexDir, 0755); err != nil {
			log.Printf("Warning: Cannot create .index in %s: %v", baseDir, err)
			log.Printf("Using base directory for index instead")
			indexDir = baseDir // Используем саму папку meter для индекса
		}
	} else {
		// Используем папку рядом с exe файлом
		exeDir, err := os.Executable()
		if err != nil {
			exeDir = "."
		} else {
			exeDir = filepath.Dir(exeDir)
		}
		indexDir = filepath.Join(exeDir, "photo_index")
		if err := os.MkdirAll(indexDir, 0755); err != nil {
			logErrorAndExit("Failed to create index directory %s: %v", indexDir, err)
		}
		log.Printf("Using alternative index location: %s", indexDir)
	}
	log.Printf("Index directory: %s", indexDir)

	// Получаем локальный IP адрес
	localIP, err := getLocalIP()
	if err != nil {
		log.Printf("Warning: Failed to get local IP: %v", err)
		localIP = "localhost"
	}

	// Настраиваем Gin
	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()

	// Настраиваем CORS
	router.Use(corsMiddleware())

	// Инициализируем хранилище сессий
	sessionStore := storage.NewSessionStore()

	// Инициализируем хранилище файлов
	fileManager := storage.NewFileManager(baseDir)

	// Инициализируем индексер
	indexer := storage.NewIndexer(indexDir)

	// Регистрируем обработчики
	handlers.SetupRoutes(router, sessionStore, fileManager, indexer, localIP, DefaultPort)

	// Запускаем сервер
	addr := fmt.Sprintf(":%d", DefaultPort)
	log.Printf("Photo sync server starting on http://%s:%d", localIP, DefaultPort)
	log.Printf("Photos will be saved to: %s", baseDir)
	log.Printf("To start sync, visit: http://localhost:%d/start", DefaultPort)

	// Обработка сигналов для graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("\nShutting down server...")
		os.Exit(0)
	}()

	if err := router.Run(addr); err != nil {
		logErrorAndExit("Failed to start server: %v", err)
	}
}

// tryCreateAndWrite пытается создать директорию и проверить права на запись
func tryCreateAndWrite(dir string) bool {
	// Создаем директорию если её нет
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return false
		}
		log.Printf("Created directory: %s", dir)
	}
	
	// Проверяем права на запись (пробуем создать тестовый файл)
	testFile := filepath.Join(dir, ".write_test")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		return false
	}
	os.Remove(testFile) // Удаляем тестовый файл
	log.Printf("Write permission verified for: %s", dir)
	return true
}

// logErrorAndExit логирует ошибку и ждет перед выходом (чтобы окно не закрылось сразу)
func logErrorAndExit(format string, args ...interface{}) {
	log.Printf("ERROR: "+format, args...)
	log.Println("\nНажмите Enter для выхода...")
	fmt.Scanln() // Ждем нажатия Enter
	os.Exit(1)
}

// getLocalIP получает локальный IP адрес для подключения из локальной сети
func getLocalIP() (string, error) {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return "", err
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP.String(), nil
}

// corsMiddleware настраивает CORS для работы с браузером
func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}

