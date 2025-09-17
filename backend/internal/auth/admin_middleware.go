package auth

import (
	"context"
	"database/sql"
	"net/http"
	"time"

	"backend/pkg/logger"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func AdminOnly(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := GetUserIDFromCtx(c.Request.Context())
		if userID == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
		defer cancel()

		var isAdmin bool
		err := db.QueryRowContext(ctx, "SELECT is_admin FROM users WHERE id=$1", userID).Scan(&isAdmin)
		if err != nil {
			if err == sql.ErrNoRows {
				logger.L.Warn("admin check user missing", zap.String("userID", userID))
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "not allowed"})
				return
			}
			logger.L.Error("admin check db error", zap.Error(err))
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "internal"})
			return
		}
		if !isAdmin {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "admin required"})
			return
		}
		c.Next()
	}
}
