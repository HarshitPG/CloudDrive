package rest

import (
	"net/http"
	"time"

	"backend/internal/auth"
	"backend/internal/database"
	"backend/pkg/logger"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type authHandlers struct {
	authSvc auth.Service
}

func RegisterAuthRoutes(rg *gin.RouterGroup, db database.Service) {

	emailSvc, err := auth.NewSMTPEmailService()
	if err != nil {
		logger.L.Fatal("Failed to initialize email service", zap.Error(err))
	}

	h := &authHandlers{
		authSvc: auth.NewService(db.DB(), emailSvc),
	}

	auth := rg.Group("/auth")
	{
		auth.POST("/signup", h.signup)
		auth.GET("/verify", h.verify) // ?token=<token>
		auth.POST("/login", h.login)
		auth.POST("/refresh", h.refresh)
		auth.POST("/logout", h.logout)
		auth.POST("/forgot-password", h.forgotPassword)
		auth.POST("/reset-password", h.resetPassword)
	}
}

type signupRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=6"`
	FullName string `json:"fullName"`
}

func (h *authHandlers) signup(c *gin.Context) {
	var req signupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	token, err := h.authSvc.Signup(c.Request.Context(), req.Email, req.Password, req.FullName)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	//‼️ Return verification token in dev; in prod do not return token, send email instead
	c.JSON(http.StatusCreated, gin.H{"message": "user created", "verification_token": token})
}

func (h *authHandlers) verify(c *gin.Context) {
	token := c.Query("token")
	if token == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "token required"})
		return
	}
	if err := h.authSvc.VerifyEmail(c.Request.Context(), token); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "email verified"})
}

type loginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

func (h *authHandlers) login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	access, refresh, err := h.authSvc.Login(c.Request.Context(), req.Email, req.Password)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}
	c.SetCookie("refresh_token", refresh, int((7 * 24 * time.Hour).Seconds()), "/", "", false, true)
	c.JSON(http.StatusOK, gin.H{"access_token": access})
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

func (h *authHandlers) refresh(c *gin.Context) {
	rt, err := c.Cookie("refresh_token")
	if err != nil {
		var r refreshRequest
		if err := c.ShouldBindJSON(&r); err != nil || r.RefreshToken == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "refresh token required"})
			return
		}
		rt = r.RefreshToken
	}
	newAccess, newRefresh, err := h.authSvc.Refresh(c.Request.Context(), rt)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}
	c.SetCookie("refresh_token", newRefresh, int((7 * 24 * time.Hour).Seconds()), "/", "", false, true)
	c.JSON(http.StatusOK, gin.H{"access_token": newAccess})
}

type logoutRequest struct {
	RefreshToken string `json:"refresh_token"`
}

func (h *authHandlers) logout(c *gin.Context) {
	rt, _ := c.Cookie("refresh_token")
	if rt == "" {
		var r logoutRequest
		_ = c.ShouldBindJSON(&r)
		rt = r.RefreshToken
	}
	if rt != "" {
		_ = h.authSvc.Logout(c.Request.Context(), rt)
		c.SetCookie("refresh_token", "", -1, "/", "", false, true)
	}
	c.JSON(http.StatusOK, gin.H{"message": "logged out"})
}

type forgotRequest struct {
	Email string `json:"email" binding:"required,email"`
}

func (h *authHandlers) forgotPassword(c *gin.Context) {
	var req forgotRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	_ = h.authSvc.ForgotPassword(c.Request.Context(), req.Email)
	c.JSON(http.StatusOK, gin.H{"message": "if the email exists, a reset link was sent"})
}

type resetRequest struct {
	Token       string `json:"token" binding:"required"`
	NewPassword string `json:"newPassword" binding:"required,min=6"`
}

func (h *authHandlers) resetPassword(c *gin.Context) {
	var req resetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.authSvc.ResetPassword(c.Request.Context(), req.Token, req.NewPassword); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "password reset successful"})
}
