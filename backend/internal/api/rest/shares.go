package rest

import (
	"database/sql"

	"backend/internal/auth"
	"backend/internal/cache"
	"backend/internal/share"
	"backend/internal/storage"
	"backend/internal/worker"

	"github.com/gin-gonic/gin"
)

func RegisterShareRoutes(rg *gin.RouterGroup, db *sql.DB, st *storage.MinioStorage, jwtSecret string, c cache.Cache) {
	h := &share.Handler{DB: db, Storage: st, Cache: c, Producer: worker.NewProducer()}

	rg.GET("/s/:token", h.ResolveShare())
	rg.GET("/fs/:token", h.ResolveFolderShare())
	rg.GET("/fs/:token/contents", h.ResolveFolderShareContents())
	rg.GET("/fs/:token/ancestors", h.ResolveFolderShareAncestors())
	rg.GET("/fs/:token/download/:fileId", h.DownloadFromShare())
	rg.GET("/fs/:token/download", h.DownloadFolderArchive())

	protected := rg.Group("/shares")
	protected.Use(auth.RequireAuth(jwtSecret))
	protected.POST("/files/:id/share", h.CreatePublicFileShare())
	protected.GET("/files/:id/download", h.DownloadSharedFile())
	protected.DELETE("/shares/:id", h.RevokeShare())
	protected.POST("/files/:id/share/user", h.ShareFileToUser())
	protected.GET("/files/:id/shares", h.ListFileShares())
	protected.POST("/folders/:id/share", h.CreatePublicFolderShare())
	protected.POST("/folders/:id/share/user", h.ShareFolderToUser())
	protected.GET("/folders/:id/contents", h.ListSharedFolderContents())
	protected.GET("/folders/:id/ancestors", h.ListSharedFolderAncestors())
	protected.GET("/shared-with-me", h.ListSharedWithMe())

	folderProtected := rg.Group("/folder-shares")
	folderProtected.Use(auth.RequireAuth(jwtSecret))
	folderProtected.POST("", h.CreateFolderShare())
	folderProtected.DELETE("/:id", h.RevokeShare())
	folderProtected.GET("/:id", h.GetShareInfo())
}
