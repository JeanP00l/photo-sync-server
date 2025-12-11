package handlers

import (
	"github.com/gin-gonic/gin"
	"photo-sync-server/storage"
)

// SetupRoutes настраивает маршруты API
func SetupRoutes(router *gin.Engine, sessionStore *storage.SessionStore, fileManager *storage.FileManager, indexer *storage.Indexer, localIP string, port int) {
	duplicateCheck := storage.NewDuplicateCheck()

	handlers := NewHandlers(sessionStore, fileManager, indexer, duplicateCheck, localIP, port)

	// API endpoints
	api := router.Group("/")
	{
		api.GET("/start", handlers.StartHandler)
		api.POST("/init", handlers.InitHandler)
		api.POST("/sync", handlers.SyncHandler)
		api.GET("/status", handlers.StatusHandler)
		api.GET("/index", handlers.IndexHandler)
		api.DELETE("/session", handlers.DeleteSessionHandler)
	}
}

